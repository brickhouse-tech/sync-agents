package agent

import (
	"testing"
)

func TestBucketForDir_Found(t *testing.T) {
	b, ok := BucketForDir("rules")
	if !ok {
		t.Fatal("expected rules bucket found")
	}
	if b.Artifact != ArtifactRule {
		t.Errorf("expected ArtifactRule, got %s", b.Artifact)
	}
}

func TestBucketForDir_NotFound(t *testing.T) {
	_, ok := BucketForDir("nonexistent")
	if ok {
		t.Error("expected false for unknown dir")
	}
}

func TestArtifactNames_CoversAllBuckets(t *testing.T) {
	names := ArtifactNames()
	if len(names) != len(Buckets) {
		t.Errorf("ArtifactNames has %d entries, Buckets has %d", len(names), len(Buckets))
	}
	for i, b := range Buckets {
		if names[i] != string(b.Artifact) {
			t.Errorf("names[%d] = %q, want %q", i, names[i], string(b.Artifact))
		}
	}
}

func TestSyncsToLocalTarget_NoRestriction(t *testing.T) {
	b := Bucket{} // zero value, no LocalTools
	if !b.SyncsToLocalTarget("anything") {
		t.Error("empty LocalTools should sync to any target")
	}
}

func TestSyncsToLocalTarget_Restricted(t *testing.T) {
	b := Bucket{LocalTools: []string{"claude"}}
	if !b.SyncsToLocalTarget("claude") {
		t.Error("should sync to claude")
	}
	if b.SyncsToLocalTarget("cursor") {
		t.Error("should NOT sync to cursor")
	}
}

func TestBucketDirs(t *testing.T) {
	dirs := BucketDirs()
	if len(dirs) != len(Buckets) {
		t.Errorf("BucketDirs has %d entries, Buckets has %d", len(dirs), len(Buckets))
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		if seen[d] {
			t.Errorf("duplicate dir: %s", d)
		}
		seen[d] = true
	}
}

func TestInitBucketDirs_SubsetOfAll(t *testing.T) {
	initDirs := InitBucketDirs()
	allDirs := BucketDirs()
	allSet := map[string]bool{}
	for _, d := range allDirs {
		allSet[d] = true
	}
	for _, d := range initDirs {
		if !allSet[d] {
			t.Errorf("init dir %q not in BucketDirs", d)
		}
	}
	// Classic three must be in init.
	for _, want := range []string{"rules", "skills", "workflows"} {
		found := false
		for _, d := range initDirs {
			if d == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in init dirs", want)
		}
	}
}

func TestBucketForArtifact(t *testing.T) {
	tests := []struct {
		typ  ArtifactType
		want string
	}{
		{ArtifactRule, "rules"},
		{ArtifactSkill, "skills"},
		{ArtifactWorkflow, "workflows"},
		{ArtifactAgent, "agents"},
		{ArtifactPlan, "plans"},
		{ArtifactSpec, "specs"},
		{ArtifactHook, "hooks"},
	}
	for _, tt := range tests {
		b, ok := BucketForArtifact(tt.typ)
		if !ok {
			t.Errorf("BucketForArtifact(%s) not found", tt.typ)
		} else if b.Dir != tt.want {
			t.Errorf("BucketForArtifact(%s).Dir = %q, want %q", tt.typ, b.Dir, tt.want)
		}
	}
}

func TestBucketForTypeString(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"rule", "rules"},
		{"rules", "rules"},
		{"RULE", "rules"},
		{"skill", "skills"},
		{"Skills", "skills"},
		{"workflow", "workflows"},
		{"WORKFLOWS", "workflows"},
		{"agent", "agents"},
		{"plan", "plans"},
		{"spec", "specs"},
		{"hook", "hooks"},
	}
	for _, tt := range tests {
		b, ok := BucketForTypeString(tt.in)
		if !ok {
			t.Errorf("BucketForTypeString(%q) not found", tt.in)
		} else if b.Dir != tt.want {
			t.Errorf("BucketForTypeString(%q).Dir = %q, want %q", tt.in, b.Dir, tt.want)
		}
	}
}

func TestBucketForTypeString_Unknown(t *testing.T) {
	_, ok := BucketForTypeString("nothing")
	if ok {
		t.Error("expected false for unknown type")
	}
}

func TestAllBucketsHaveRequiredFields(t *testing.T) {
	for _, b := range Buckets {
		if b.Dir == "" {
			t.Error("bucket has empty Dir")
		}
		if b.Artifact == "" {
			t.Errorf("bucket %s has empty Artifact", b.Dir)
		}
	}
}
