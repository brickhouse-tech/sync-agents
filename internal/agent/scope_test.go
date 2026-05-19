package agent

import "testing"

// TestScope_String covers the round-trip from the Scope value to its
// string form. The strings are part of the CLI surface (the --scope=
// flag and log messages), so this test guards against accidentally
// changing them.
func TestScope_String(t *testing.T) {
	cases := []struct {
		in   Scope
		want string
	}{
		{ScopeLocal, "local"},
		{ScopeGlobal, "global"},
		{Scope(99), "unknown"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			if got := c.in.String(); got != c.want {
				t.Errorf("Scope(%d).String() = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestParseScope_Valid checks every documented case-variant.
func TestParseScope_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want Scope
	}{
		{"local", ScopeLocal},
		{"Local", ScopeLocal},
		{"LOCAL", ScopeLocal},
		{"global", ScopeGlobal},
		{"Global", ScopeGlobal},
		{"GLOBAL", ScopeGlobal},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := ParseScope(c.in)
			if !ok {
				t.Fatalf("ParseScope(%q) returned ok=false; want true", c.in)
			}
			if got != c.want {
				t.Errorf("ParseScope(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestParseScope_Invalid documents that anything other than the
// documented inputs returns false. The contract here is "false means
// invalid, don't silently default" — see ParseScope's doc comment.
func TestParseScope_Invalid(t *testing.T) {
	for _, in := range []string{"", "user", "project", " local ", "Both"} {
		t.Run(in, func(t *testing.T) {
			_, ok := ParseScope(in)
			if ok {
				t.Errorf("ParseScope(%q) returned ok=true; want false", in)
			}
		})
	}
}
