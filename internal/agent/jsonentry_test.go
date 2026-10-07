package agent

import (
	"encoding/json"
	"errors"
	"testing"
)

const entry = ".agents/index/opencode.md"

// TestJSONEnsureEntry inserts the entry while keeping every byte outside
// the edited range, following the neighbouring layout.
func TestJSONEnsureEntry(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "absent file is created",
			in:   "",
			want: "{\n  \"instructions\": [\".agents/index/opencode.md\"]\n}\n",
		},
		{
			name: "empty object",
			in:   "{}\n",
			want: "{\n  \"instructions\": [\".agents/index/opencode.md\"]\n}\n",
		},
		{
			name: "pretty object without the key keeps order and indentation",
			in:   "{\n    \"$schema\": \"https://opencode.ai/config.json\",\n    \"model\": \"x\"\n}\n",
			want: "{\n    \"$schema\": \"https://opencode.ai/config.json\",\n    \"model\": \"x\",\n    \"instructions\": [\".agents/index/opencode.md\"]\n}\n",
		},
		{
			name: "compact object without the key",
			in:   `{"model":"x","b":{"c":[1,2]}}`,
			want: `{"model":"x","b":{"c":[1,2]}, "instructions": [".agents/index/opencode.md"]}`,
		},
		{
			name: "empty array",
			in:   "{\"instructions\": [], \"z\": 1}",
			want: "{\"instructions\": [\".agents/index/opencode.md\"], \"z\": 1}",
		},
		{
			name: "multi-line array",
			in:   "{\n  \"instructions\": [\n    \"CONTRIBUTING.md\",\n    \"docs/*.md\"\n  ],\n  \"z\": true\n}\n",
			want: "{\n  \"instructions\": [\n    \"CONTRIBUTING.md\",\n    \"docs/*.md\",\n    \".agents/index/opencode.md\"\n  ],\n  \"z\": true\n}\n",
		},
		{
			name: "inline array",
			in:   `{"instructions": ["a.md"]}`,
			want: `{"instructions": ["a.md", ".agents/index/opencode.md"]}`,
		},
		{
			name: "duplicate keys elsewhere and unicode survive",
			in:   "{\"note\": \"café <b>\", \"instructions\": [\"a.md\"]}",
			want: "{\"note\": \"café <b>\", \"instructions\": [\"a.md\", \".agents/index/opencode.md\"]}",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, changed, err := ensureJSONArrayEntry([]byte(tc.in), "instructions", entry)
			if err != nil {
				t.Fatalf("ensure: %v", err)
			}
			if !changed {
				t.Error("changed = false")
			}
			if string(out) != tc.want {
				t.Errorf("got\n%s\nwant\n%s", out, tc.want)
			}
			var doc struct{ Instructions []string }
			if err := json.Unmarshal(out, &doc); err != nil {
				t.Fatalf("output is not JSON: %v", err)
			}
			if doc.Instructions[len(doc.Instructions)-1] != entry {
				t.Errorf("entry not last: %v", doc.Instructions)
			}
		})
	}
}

// TestJSONEnsureEntry_Idempotent writes nothing the second time, and
// leaves an entry the user already listed (anywhere) alone.
func TestJSONEnsureEntry_Idempotent(t *testing.T) {
	once, _, err := ensureJSONArrayEntry([]byte("{\n  \"model\": \"x\"\n}\n"), "instructions", entry)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	twice, changed, err := ensureJSONArrayEntry(once, "instructions", entry)
	if err != nil || changed || string(twice) != string(once) {
		t.Errorf("second ensure: changed=%v err=%v\n%s", changed, err, twice)
	}

	listed := `{"instructions": [".agents/index/opencode.md", "b.md"]}`
	out, changed, err := ensureJSONArrayEntry([]byte(listed), "instructions", entry)
	if err != nil || changed || string(out) != listed {
		t.Errorf("already listed: changed=%v err=%v out=%s", changed, err, out)
	}
}

// TestJSONEntry_Refusals never edits a file it cannot round-trip: JSONC,
// trailing commas, or an unexpected shape.
func TestJSONEntry_Refusals(t *testing.T) {
	cases := map[string]string{
		"line comment":       "{\n  // mine\n  \"model\": \"x\"\n}\n",
		"block comment":      "{ /* mine */ \"model\": \"x\" }",
		"trailing comma":     "{\"instructions\": [\"a.md\",],}",
		"truncated":          "{\"instructions\": [",
		"top-level array":    "[\"a.md\"]",
		"key is not array":   "{\"instructions\": \"a.md\"}",
		"key is an object":   "{\"instructions\": {\"a\": 1}}",
		"top-level a string": "\"x\"",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ensureJSONArrayEntry([]byte(in), "instructions", entry); !errors.Is(err, errJSONNotEditable) {
				t.Errorf("ensure err = %v, want errJSONNotEditable", err)
			}
			if _, _, err := removeJSONArrayEntry([]byte(in), "instructions", entry); !errors.Is(err, errJSONNotEditable) {
				t.Errorf("remove err = %v, want errJSONNotEditable", err)
			}
		})
	}
}

// TestJSONRemoveEntry removes the entry and one separator, dropping the
// member when the array would be left empty.
func TestJSONRemoveEntry(t *testing.T) {
	cases := []struct {
		name, in, want string
		changed        bool
	}{
		{name: "absent file", in: "", want: "", changed: false},
		{name: "no key", in: `{"model":"x"}`, want: `{"model":"x"}`, changed: false},
		{name: "not listed", in: `{"instructions": ["a.md"]}`, want: `{"instructions": ["a.md"]}`, changed: false},
		{
			name: "first of several", in: `{"instructions": [".agents/index/opencode.md", "a.md"]}`,
			want: `{"instructions": ["a.md"]}`, changed: true,
		},
		{
			name: "middle, multi-line",
			in:   "{\n  \"instructions\": [\n    \"a.md\",\n    \".agents/index/opencode.md\",\n    \"b.md\"\n  ]\n}\n",
			want: "{\n  \"instructions\": [\n    \"a.md\",\n    \"b.md\"\n  ]\n}\n", changed: true,
		},
		{
			name: "duplicates all go", in: `{"instructions": [".agents/index/opencode.md", "a.md", ".agents/index/opencode.md"]}`,
			want: `{"instructions": ["a.md"]}`, changed: true,
		},
		{
			name: "only entry, first member", in: `{"instructions": [".agents/index/opencode.md"], "model": "x"}`,
			want: `{"model": "x"}`, changed: true,
		},
		{
			name: "only entry, only member", in: "{\n  \"instructions\": [\".agents/index/opencode.md\"]\n}\n",
			want: "{}\n", changed: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, changed, err := removeJSONArrayEntry([]byte(tc.in), "instructions", entry)
			if err != nil {
				t.Fatalf("remove: %v", err)
			}
			if changed != tc.changed || string(out) != tc.want {
				t.Errorf("changed=%v out=%q, want changed=%v out=%q", changed, out, tc.changed, tc.want)
			}
		})
	}
}

// TestJSONEntry_RoundTrip restores the user's exact bytes after
// ensure then remove, whenever the user's file did not already hold an
// empty array at the key.
func TestJSONEntry_RoundTrip(t *testing.T) {
	inputs := []string{
		"{}",
		"{\n    \"$schema\": \"https://opencode.ai/config.json\",\n    \"model\": \"x\"\n}\n",
		`{"model":"x","b":{"c":[1,2]}}`,
		"{\n  \"instructions\": [\n    \"CONTRIBUTING.md\"\n  ],\n  \"z\": true\n}\n",
		`{"instructions": ["a.md", "b.md"], "z": null}`,
		"{\n\t\"model\": \"x\"\n}",
	}
	for _, in := range inputs {
		added, _, err := ensureJSONArrayEntry([]byte(in), "instructions", entry)
		if err != nil {
			t.Fatalf("ensure %q: %v", in, err)
		}
		back, _, err := removeJSONArrayEntry(added, "instructions", entry)
		if err != nil {
			t.Fatalf("remove %q: %v", added, err)
		}
		if string(back) != in {
			t.Errorf("round trip changed bytes:\n in %q\nmid %q\nout %q", in, added, back)
		}
	}
}
