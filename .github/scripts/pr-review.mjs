#!/usr/bin/env node
// AI PR reviewer powered by OpenRouter. Zero npm deps (Node 20 global fetch).
//
// Env:
//   BASE_REF            base branch name (e.g. "main")
//   PR_NUMBER           pull request number
//   REPO                "owner/repo"
//   OPENROUTER_API_KEY  OpenRouter API key
//   GITHUB_TOKEN        Actions token (pull-requests: write)
//
// Design constraints:
//   - Never fails the workflow: every error path is caught, reported in the
//     sticky comment when possible, and the process exits 0.
//   - One sticky summary comment (marker <!-- pr-review-bot -->) updated in
//     place on every push instead of stacking new comments.
//   - Inline review comments only where the diff supports the line; anything
//     that can't be anchored lands in the summary instead.

import { execFileSync } from "node:child_process";

const MARKER = "<!-- pr-review-bot -->";
const PRIMARY_MODEL = "deepseek/deepseek-v4-flash";
const FALLBACK_MODEL = "qwen/qwen3.7-plus";
const MAX_DIFF_CHARS = 80_000;
const MAX_FINDINGS = 8;

// Ballpark $/1M tokens for cost estimate in the summary (not billing-grade).
const PRICES = {
  [PRIMARY_MODEL]: { in: 0.05, out: 0.25 },
  [FALLBACK_MODEL]: { in: 0.4, out: 1.2 },
};

const EXCLUDE_RE =
  /(package-lock|yarn\.lock|\.min\.js|^dist\/|\/dist\/|^build\/|\/build\/|CHANGELOG\.md|\.gitignore|^vendor\/|\/vendor\/|\.sum$)/;

const { BASE_REF, PR_NUMBER, REPO, OPENROUTER_API_KEY, GITHUB_TOKEN } =
  process.env;

const gh = async (method, path, body) => {
  const res = await fetch(`https://api.github.com${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${GITHUB_TOKEN}`,
      Accept: "application/vnd.github+json",
      "X-GitHub-Api-Version": "2022-11-28",
      ...(body ? { "Content-Type": "application/json" } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    throw new Error(`GitHub ${method} ${path} -> ${res.status}: ${await res.text()}`);
  }
  return res.status === 204 ? null : res.json();
};

function git(...args) {
  return execFileSync("git", args, {
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
  });
}

function changedFiles(base) {
  return git("diff", "--name-only", `origin/${base}...HEAD`)
    .split("\n")
    .map((f) => f.trim())
    .filter((f) => f && !EXCLUDE_RE.test(f));
}

function getDiff(base, files) {
  const raw = git("diff", `origin/${base}...HEAD`, "--", ...files);
  if (raw.length <= MAX_DIFF_CHARS) return { diff: raw, truncated: false };
  return {
    diff: raw.slice(0, MAX_DIFF_CHARS) + "\n... [diff truncated] ...\n",
    truncated: true,
  };
}

// Parse the unified diff into path -> Set of RIGHT-side line numbers that a
// PR review comment can legally anchor to (added + context lines in hunks).
function rightSideLines(diff) {
  const anchors = new Map();
  let path = null;
  let line = 0;
  for (const l of diff.split("\n")) {
    if (l.startsWith("+++ b/")) {
      path = l.slice(6);
      if (!anchors.has(path)) anchors.set(path, new Set());
      continue;
    }
    if (l.startsWith("+++ /dev/null")) {
      path = null; // deleted file: nothing on the right side
      continue;
    }
    const hunk = /^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(l);
    if (hunk) {
      line = parseInt(hunk[1], 10);
      continue;
    }
    if (!path) continue;
    if (l.startsWith("+") || l.startsWith(" ") || l === "") {
      anchors.get(path).add(line);
      line++;
    }
    // '-' lines only advance the left side
  }
  return anchors;
}

const SYSTEM_PROMPT = `You are a senior Go engineer reviewing a pull request for sync-agents, a Go CLI with bats integration tests and a thin npm wrapper. Repo conventions: table-driven Go tests, deterministic file output, conventional commits.

Review ONLY the diff you are given. Report exclusively:
- correctness bugs (logic errors, nil derefs, races, broken error handling)
- security issues (injection, path traversal, secret leakage, unsafe exec)
- real maintainability problems (broken invariants, misleading APIs, dead code introduced by this change)

Do NOT report style nits, formatting, naming preferences, or hypothetical concerns. "No findings" is a valid and PREFERRED outcome when the diff is fine — return an empty findings array rather than inventing issues.

Respond with strict JSON only (no markdown fences, no prose outside the JSON):
{"summary": "<1-3 sentence overall assessment>", "findings": [{"path": "<file path from the diff>", "line": <new-file line number the issue is on>, "severity": "critical|warning|suggestion", "body": "<specific, actionable comment>"}]}

Maximum ${MAX_FINDINGS} findings. Each finding must be distinct — never report the same issue more than once. Order by severity (critical first).`;

async function callModel(model, diff, files) {
  const res = await fetch("https://openrouter.ai/api/v1/chat/completions", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${OPENROUTER_API_KEY}`,
      "Content-Type": "application/json",
      "HTTP-Referer": `https://github.com/${REPO}`,
      "X-Title": "sync-agents PR review",
    },
    body: JSON.stringify({
      model,
      temperature: 0.1,
      messages: [
        { role: "system", content: SYSTEM_PROMPT },
        {
          role: "user",
          content: `Changed files:\n${files.join("\n")}\n\nUnified diff (base...head):\n\n${diff}`,
        },
      ],
    }),
  });
  if (!res.ok) {
    throw new Error(`OpenRouter ${model} -> ${res.status}: ${await res.text()}`);
  }
  const data = await res.json();
  const content = data.choices?.[0]?.message?.content;
  if (!content) throw new Error(`OpenRouter ${model}: empty completion`);
  return { content, usage: data.usage ?? {} };
}

async function review(diff, files) {
  try {
    const r = await callModel(PRIMARY_MODEL, diff, files);
    return { ...r, model: PRIMARY_MODEL };
  } catch (err) {
    console.error(`primary model failed, falling back: ${err.message}`);
    const r = await callModel(FALLBACK_MODEL, diff, files);
    return { ...r, model: FALLBACK_MODEL };
  }
}

function parseFindings(content) {
  let text = content.trim();
  const fenced = /^```(?:json)?\s*([\s\S]*?)\s*```$/.exec(text);
  if (fenced) text = fenced[1];
  // Tolerate models that wrap the JSON in prose despite instructions.
  const start = text.indexOf("{");
  const end = text.lastIndexOf("}");
  if (start === -1 || end === -1) throw new Error("no JSON object in model output");
  const parsed = JSON.parse(text.slice(start, end + 1));
  const findings = (Array.isArray(parsed.findings) ? parsed.findings : [])
    .slice(0, MAX_FINDINGS)
    .map((f) => ({
      path: String(f.path ?? ""),
      line: Number.isInteger(f.line) ? f.line : parseInt(f.line, 10) || null,
      severity: ["critical", "warning", "suggestion"].includes(f.severity)
        ? f.severity
        : "suggestion",
      body: String(f.body ?? "").trim(),
    }))
    .filter((f) => f.path && f.body);
  // Models sometimes emit near-duplicate findings; keep one per (path, line).
  const seen = new Set();
  const deduped = findings.filter((f) => {
    const key = `${f.path}:${f.line}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
  return { summary: String(parsed.summary ?? "").trim(), findings: deduped };
}

function estimateCost(model, usage) {
  // OpenRouter reports actual cost in usage.cost; fall back to a ballpark
  // from the price table if it's ever absent.
  if (typeof usage.cost === "number") return usage.cost;
  const p = PRICES[model];
  if (!p || (!usage.prompt_tokens && !usage.completion_tokens)) return null;
  return (
    ((usage.prompt_tokens ?? 0) * p.in + (usage.completion_tokens ?? 0) * p.out) /
    1_000_000
  );
}

const SEV_ICON = { critical: "🔴", warning: "🟡", suggestion: "🔵" };

async function upsertSticky(body) {
  const comments = await gh(
    "GET",
    `/repos/${REPO}/issues/${PR_NUMBER}/comments?per_page=100`,
  );
  const existing = comments.find((c) => c.body?.includes(MARKER));
  if (existing) {
    await gh("PATCH", `/repos/${REPO}/issues/comments/${existing.id}`, { body });
  } else {
    await gh("POST", `/repos/${REPO}/issues/${PR_NUMBER}/comments`, { body });
  }
}

async function postInline(findings) {
  if (findings.length === 0) return true;
  try {
    // Don't re-post an identical inline comment on a later push.
    const existing = await gh(
      "GET",
      `/repos/${REPO}/pulls/${PR_NUMBER}/comments?per_page=100`,
    );
    const posted = new Set(
      existing
        .filter((c) => c.body?.includes(`${MARKER}-inline`))
        .map((c) => `${c.path}:${c.line}:${c.body}`),
    );
    findings = findings.filter(
      (f) =>
        !posted.has(
          `${f.path}:${f.line}:${SEV_ICON[f.severity]} **${f.severity}**: ${f.body}\n\n${MARKER}-inline`,
        ),
    );
    if (findings.length === 0) return true;
    await gh("POST", `/repos/${REPO}/pulls/${PR_NUMBER}/reviews`, {
      event: "COMMENT",
      body: "",
      comments: findings.map((f) => ({
        path: f.path,
        line: f.line,
        side: "RIGHT",
        body: `${SEV_ICON[f.severity]} **${f.severity}**: ${f.body}\n\n${MARKER}-inline`,
      })),
    });
    return true;
  } catch (err) {
    console.error(`inline review post failed: ${err.message}`);
    return false;
  }
}

function summaryBody({ model, summary, inline, unanchored, truncated, cost, error }) {
  const lines = [MARKER, "## 🤖 AI Code Review", ""];
  if (error) {
    lines.push(
      `⚠️ The reviewer failed to run on this push, so no review was produced (merges are NOT blocked by this).`,
      "",
      `\`\`\`\n${error}\n\`\`\``,
    );
    return lines.join("\n");
  }
  const all = [...inline, ...unanchored];
  if (all.length === 0) {
    lines.push(`✅ Clean review — no findings. (model: \`${model}\`)`);
  } else {
    const counts = { critical: 0, warning: 0, suggestion: 0 };
    for (const f of all) counts[f.severity]++;
    lines.push(
      summary || "",
      "",
      `**Findings:** ${counts.critical} critical · ${counts.warning} warning · ${counts.suggestion} suggestion` +
        (inline.length ? ` (${inline.length} posted inline)` : ""),
    );
    if (unanchored.length) {
      lines.push("", "**Findings not anchorable to a diff line:**", "");
      for (const f of unanchored) {
        lines.push(
          `- ${SEV_ICON[f.severity]} **${f.severity}** \`${f.path}${f.line ? `:${f.line}` : ""}\` — ${f.body}`,
        );
      }
    }
  }
  lines.push("");
  const meta = [`model: \`${model}\``];
  if (truncated) meta.push("diff truncated to ~80k chars — review may be partial");
  if (cost != null) meta.push(`est. cost: $${cost.toFixed(4)}`);
  lines.push(`<sub>${meta.join(" · ")}</sub>`);
  return lines.join("\n");
}

async function main() {
  if (!PR_NUMBER || !REPO || !BASE_REF) {
    console.log("Missing PR context (PR_NUMBER/REPO/BASE_REF) — nothing to do.");
    return;
  }
  if (!OPENROUTER_API_KEY) throw new Error("OPENROUTER_API_KEY is not set");

  const files = changedFiles(BASE_REF);
  if (files.length === 0) {
    console.log("No reviewable files after excludes — skipping review.");
    await upsertSticky(
      `${MARKER}\n## 🤖 AI Code Review\n\n✅ No reviewable files in this diff (only excluded/generated paths changed).`,
    );
    return;
  }

  const { diff, truncated } = getDiff(BASE_REF, files);
  const { content, usage, model } = await review(diff, files);
  const { summary, findings } = parseFindings(content);

  const anchors = rightSideLines(diff);
  const anchorable = [];
  const unanchored = [];
  for (const f of findings) {
    if (f.line && anchors.get(f.path)?.has(f.line)) anchorable.push(f);
    else unanchored.push(f);
  }

  let inline = anchorable;
  if (!(await postInline(anchorable))) {
    // Line anchoring rejected by the API — demote everything to the summary.
    unanchored.push(...anchorable);
    inline = [];
  }

  const cost = estimateCost(model, usage);
  await upsertSticky(
    summaryBody({ model, summary, inline, unanchored, truncated, cost }),
  );
  console.log(
    `Review done: model=${model} findings=${findings.length} (inline=${inline.length}) truncated=${truncated}`,
  );
}

main().catch(async (err) => {
  console.error(`pr-review failed: ${err.message}`);
  try {
    if (PR_NUMBER && REPO) {
      await upsertSticky(summaryBody({ error: err.message }));
    }
  } catch (e2) {
    console.error(`could not report failure on PR: ${e2.message}`);
  }
  process.exit(0); // a broken reviewer must never block merges
});
