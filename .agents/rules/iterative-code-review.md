---
trigger: always_on
---

# Iterative Code Review Process

## Philosophy
Code review is not a one-shot activity. Iterate until the results are boring and predictable. Less specificity avoids blind spots. Fresh eyes catch what stale ones miss.

## The Process

### Step 1: Initial Review (Uncommitted Changes)
- Review the uncommitted changes
- Don't over-specify what to look for — let the model find problems organically
- Ask: "What problems do you see?" or "Review these changes"
- Avoid listing specific concerns (leads to tunnel vision)

### Step 2: Fresh Session
- Start a new chat session for fresh eyes
- Context from the first review can create blind spots
- New session = new perspective
- Repeat the review prompt

### Step 3: Iterate
- Review the review. Does the feedback make sense?
- Ask follow-up questions: "Why is this a problem?" "What would be better?"
- Challenge the model: "Are you sure?" "Is there a better way?"
- Repeat until the feedback is boring and predictable

### Step 4: Synthesize
- Compile findings from multiple reviews
- Identify patterns (same issue flagged 3x = probably real)
- Discard noise (one-off style preferences)
- Create actionable fix list

## Why Less Specificity?
- Over-specifying leads to **confirmation bias** — model only looks for what you mentioned
- Broad questions surface **unexpected problems** you didn't think to ask about
- Models are better at finding issues when not constrained by human assumptions
- Trust the model's pattern recognition; it's seen millions of code reviews

## Why Fresh Sessions?
- Context accumulates blind spots
- First review shapes expectations for second review
- New session = clean slate, no preconceptions
- Like getting a second opinion from a new doctor

## When to Stop
- Feedback is **boring and predictable** (no new insights)
- Same issues keep appearing across sessions (stable findings)
- You've addressed all critical/high-priority items
- Diminishing returns (minor style tweaks only)

## Anti-patterns
- **Reviewing in the same session repeatedly** — blind spots compound
- **Over-specifying concerns** — "check for SQL injection" → model ignores everything else
- **Stopping too early** — first review often misses real problems
- **Accepting all suggestions** — model feedback isn't gospel; evaluate each point
- **Skipping iteration** — one review is rarely enough

## Example Workflow

```
Session 1:
You: Review the uncommitted changes
Model: [finds 10 issues, 3 critical]

Session 2 (fresh):
You: Review the uncommitted changes
Model: [finds 8 issues, 2 critical, some overlap with session 1]

Session 3 (fresh):
You: Review the uncommitted changes
Model: [finds 5 issues, 1 new critical item, rest are minor]

Synthesis:
- Critical issues flagged in multiple sessions → fix these
- New issue from session 3 → investigate (might be subtle)
- Style preferences → discard or fix if easy
```

## Integration with Testing
- After code review, run tests
- Review findings may reveal missing tests
- Add tests for edge cases surfaced during review
- Test coverage validates review findings

## Integration with Trade-offs
- Review may surface architectural trade-offs
- Use the trade-offs rule to document them
- Don't make decisions in review; document options and let the human decide
