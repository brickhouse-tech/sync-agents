package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brickhouse-tech/sync-agents/internal/agent/source"
)

// cliFakeFetcher is a minimal in-memory source.Fetcher for exercising
// the CLI wrappers end-to-end without a network.
type cliFakeFetcher struct {
	sha string
	tar []byte
}

func (f *cliFakeFetcher) ResolveRef(_ context.Context, _, _, _ string) (string, error) {
	return f.sha, nil
}

func (f *cliFakeFetcher) Fetch(_ context.Context, _, _, _ string) (io.ReadCloser, bool, error) {
	return io.NopCloser(bytes.NewReader(f.tar)), false, nil
}

func cliTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: "top/" + name, Mode: 0o644, Size: int64(len(content)),
		}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(content))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestReadConfigQuarantine(t *testing.T) {
	dir := t.TempDir()
	if !ReadConfigQuarantine(dir) {
		t.Error("missing config must default to quarantine ON")
	}
	os.WriteFile(filepath.Join(dir, "config"), []byte("targets = claude\nquarantine = off\n"), 0o644)
	if ReadConfigQuarantine(dir) {
		t.Error("quarantine = off not honored")
	}
	os.WriteFile(filepath.Join(dir, "config"), []byte("quarantine = on\n"), 0o644)
	if !ReadConfigQuarantine(dir) {
		t.Error("quarantine = on not honored")
	}
}

// TestCmdQuarantineFlow drives pull → quarantine → approve → reject
// through the App-level wrappers exactly as the CLI does.
func TestCmdQuarantineFlow(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".agents")
	os.MkdirAll(filepath.Join(agentsDir, "rules"), 0o755)

	sha := strings.Repeat("a", 40)
	fetcher := &cliFakeFetcher{sha: sha, tar: cliTarball(t, map[string]string{
		"rules/remote-rule.md": "# remote rule\n",
	})}
	if err := source.SaveManifest(agentsDir, source.Manifest{
		Version: 1,
		Sources: []string{"rule:foo/bar@v1/rules/remote-rule.md"},
	}); err != nil {
		t.Fatal(err)
	}

	out := &bytes.Buffer{}
	a := &App{
		ProjectRoot:   root,
		ActiveTargets: []string{"claude"},
		Stdout:        out,
		Stderr:        &bytes.Buffer{},
	}
	opts := SourceCmdOpts{Fetcher: fetcher}

	// Pull quarantines by default (no config file → ON).
	if err := a.CmdPull(opts); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if !strings.Contains(out.String(), "quarantined") {
		t.Fatalf("pull output missing quarantine notice:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "rules", "remote-rule.md")); !os.IsNotExist(err) {
		t.Fatal("artifact installed despite quarantine")
	}

	// quarantine lists it.
	out.Reset()
	if err := a.CmdQuarantine(opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "remote-rule") {
		t.Fatalf("quarantine listing missing artifact:\n%s", out.String())
	}

	// approve installs it and the follow-up pull is current.
	if err := a.CmdApprove("remote-rule", false, opts); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "rules", "remote-rule.md")); err != nil {
		t.Fatalf("approved artifact missing: %v", err)
	}
	out.Reset()
	if err := a.CmdPull(opts); err != nil {
		t.Fatalf("post-approve pull: %v", err)
	}
	if !strings.Contains(out.String(), "already current") {
		t.Fatalf("post-approve pull not current:\n%s", out.String())
	}

	// Empty-quarantine paths: listing + reject error.
	if err := a.CmdQuarantine(opts); err != nil {
		t.Fatal(err)
	}
	if err := a.CmdReject("remote-rule", false, opts); err == nil {
		t.Fatal("reject on empty quarantine must error")
	}
	if err := a.CmdApprove("", false, opts); err == nil {
		t.Fatal("approve without name/--all must error")
	}
}
