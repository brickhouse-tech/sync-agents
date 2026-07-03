package source

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// hash.go implements SPEC-003 §Hash function: the deterministic
// content hash recorded in _origin.json and sources.lock, used for
// local-modification detection and tarball-integrity verification.
//
// The hash is sha256 over a sorted per-file manifest:
//
//	"{relative-path}\t{size}\t{sha256(content)}\n"
//
// Determinism notes:
//   - Paths use forward slashes and sort lexicographically, so the
//     same tree hashes identically on macOS/Linux/Windows.
//   - File CONTENT is hashed byte-for-byte — no line-ending
//     normalization. The spec's "LF-only" note refers to the manifest
//     lines above being LF-joined, not to rewriting file bytes; a
//     CRLF file that arrives from upstream must hash the same way it
//     will be verified later, and mutating bytes before hashing would
//     make the recorded hash unverifiable against the real tree.
//   - Origin files (_origin.json, *.origin.json) are excluded:
//     they're written after the hash is computed, so including them
//     would be a chicken-and-egg.

// HashPrefix is prepended to the hex digest so recorded hashes are
// self-describing ("sha256:1f3a…") and a future algorithm change is
// detectable rather than silently miscompared.
const HashPrefix = "sha256:"

// HashTree computes the content hash of an artifact rooted at root.
// root may be a directory (skill dirs, staged trees) or a single file
// (flat rules/workflows); a single file hashes as a one-line manifest
// keyed by its basename so a file's hash is stable across the temp
// staging dir and its final bucket location.
func HashTree(root string) (string, error) {
	fi, err := os.Stat(root)
	if err != nil {
		return "", err
	}

	type line struct {
		rel  string
		size int64
		sum  string
	}
	var lines []line

	addFile := func(rel, full string, size int64) error {
		f, err := os.Open(full)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		lines = append(lines, line{rel: rel, size: size, sum: fmt.Sprintf("%x", h.Sum(nil))})
		return nil
	}

	if !fi.IsDir() {
		if err := addFile(filepath.Base(root), root, fi.Size()); err != nil {
			return "", err
		}
	} else {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			// Symlinks are skipped: the extractor never writes them,
			// and hashing link targets would make the hash depend on
			// filesystem state outside the tree.
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if IsOriginFile(d.Name()) {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			return addFile(filepath.ToSlash(rel), p, info.Size())
		})
		if err != nil {
			return "", err
		}
	}

	sort.Slice(lines, func(i, j int) bool { return lines[i].rel < lines[j].rel })
	h := sha256.New()
	for _, l := range lines {
		fmt.Fprintf(h, "%s\t%d\t%s\n", l.rel, l.size, l.sum)
	}
	return HashPrefix + fmt.Sprintf("%x", h.Sum(nil)), nil
}

// IsOriginFile reports whether name is provenance metadata written by
// this package (and therefore excluded from content hashing and tree
// walks that enumerate artifacts).
func IsOriginFile(name string) bool {
	return name == originDirFileName || strings.HasSuffix(name, originFileSuffix)
}
