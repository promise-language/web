package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newGitRepo makes a throwaway repository with one commit, so staged changes
// have something to diff against.
func newGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "Test"},
		{"config", "user.email", "test@users.noreply.github.com"},
		// The hook under test must not fire while the test drives git itself.
		{"config", "core.hooksPath", filepath.Join(root, ".no-hooks")},
	} {
		if err := RunSilentIn(root, "git", args...); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}

	write(t, root, "seed.txt", "seed\n")
	gitOK(t, root, "add", "seed.txt")
	gitOK(t, root, "commit", "-qm", "seed")
	return root
}

func write(t *testing.T, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitOK(t *testing.T, root string, args ...string) {
	t.Helper()
	if err := RunSilentIn(root, "git", args...); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

// A path containing a space is the case the older --name-status parser got
// wrong: git C-quotes such paths for a terminal, and a quoted path is not the
// path. NUL-separated --raw is what makes it exact.
func TestStagedEntriesHandlesAwkwardPaths(t *testing.T) {
	root := newGitRepo(t)

	write(t, root, "a file with spaces.txt", "text\n")
	write(t, root, "quote\"and'apostrophe.txt", "text\n")
	write(t, root, "nested/dir/file.txt", "text\n")
	gitOK(t, root, "add", "-A")

	entries, err := stagedEntries(root)
	if err != nil {
		t.Fatalf("stagedEntries: %v", err)
	}

	got := make(map[string]bool, len(entries))
	for _, e := range entries {
		got[e.path] = true
		if len(e.sha) != 40 {
			t.Errorf("%s: sha %q is not a full object id — the null-sha comparison "+
				"for deletions depends on full length", e.path, e.sha)
		}
	}

	for _, want := range []string{
		"a file with spaces.txt",
		"quote\"and'apostrophe.txt",
		"nested/dir/file.txt",
	} {
		if !got[want] {
			t.Errorf("missing staged entry %q — a file the binary check never sees "+
				"is a file that can carry a binary in", want)
		}
	}
}

// A deletion records no content, so it must pass rather than be reported as
// unreadable. This is the case that regressed when --raw abbreviated its SHAs.
func TestStagedEntriesSkipsDeletions(t *testing.T) {
	root := newGitRepo(t)
	gitOK(t, root, "rm", "-q", "seed.txt")

	entries, err := stagedEntries(root)
	if err != nil {
		t.Fatalf("stagedEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries for a pure deletion, want 0: %+v", len(entries), entries)
	}
	if err := checkNoBinaryBlobs(root); err != nil {
		t.Errorf("deletion rejected: %v", err)
	}
}

// A rename carries two paths in one record; the destination is the one with
// content to scan. Mis-parsing it shifts every following record.
func TestStagedEntriesRename(t *testing.T) {
	root := newGitRepo(t)
	gitOK(t, root, "mv", "seed.txt", "renamed.txt")
	write(t, root, "after.txt", "text\n")
	gitOK(t, root, "add", "-A")

	entries, err := stagedEntries(root)
	if err != nil {
		t.Fatalf("stagedEntries: %v", err)
	}

	got := make(map[string]bool, len(entries))
	for _, e := range entries {
		got[e.path] = true
	}
	if !got["renamed.txt"] {
		t.Error("rename destination missing — its content was never scanned")
	}
	if !got["after.txt"] {
		t.Error("the entry after a rename was lost — two-path records shift the parse")
	}
}

func TestCheckNoBinaryBlobs(t *testing.T) {
	t.Run("text passes", func(t *testing.T) {
		root := newGitRepo(t)
		write(t, root, "source.go", "package main\n")
		gitOK(t, root, "add", "-A")
		if err := checkNoBinaryBlobs(root); err != nil {
			t.Errorf("text file rejected: %v", err)
		}
	})

	t.Run("NUL byte rejected", func(t *testing.T) {
		root := newGitRepo(t)
		write(t, root, "compiled.bin", "ELF\x00\x01payload")
		gitOK(t, root, "add", "-A")
		if err := checkNoBinaryBlobs(root); err == nil {
			t.Error("committed a blob with a NUL byte")
		}
	})

	t.Run("NUL in a spaced path rejected", func(t *testing.T) {
		root := newGitRepo(t)
		write(t, root, "a binary file.bin", "ELF\x00payload")
		gitOK(t, root, "add", "-A")
		if err := checkNoBinaryBlobs(root); err == nil {
			t.Error("a quoted path let a binary through")
		}
	})

	t.Run("oversized rejected even when it is text", func(t *testing.T) {
		root := newGitRepo(t)
		write(t, root, "huge.txt", strings.Repeat("a", maxBlobBytes+1))
		gitOK(t, root, "add", "-A")
		if err := checkNoBinaryBlobs(root); err == nil {
			t.Error("committed a blob over the size limit")
		}
	})

	t.Run("NUL past the scan window passes", func(t *testing.T) {
		// The window is a deliberate limit, not an oversight: git uses the same
		// heuristic. Pinning it means a change to it is a decision, not a drift.
		root := newGitRepo(t)
		write(t, root, "late.txt", strings.Repeat("a", binaryScanPrefix)+"\x00")
		gitOK(t, root, "add", "-A")
		if err := checkNoBinaryBlobs(root); err != nil {
			t.Errorf("rejected a NUL beyond the %d-byte scan window: %v", binaryScanPrefix, err)
		}
	})
}

// The whole point of the rewrite: an entry that cannot be inspected must be
// refused, never skipped. A malformed record is the readable stand-in for
// "git would not tell me what this is".
func TestStagedEntriesFailsClosedOnGarbage(t *testing.T) {
	if _, err := stagedEntries(t.TempDir()); err == nil {
		t.Error("stagedEntries succeeded outside a git repository — " +
			"an unreadable index must refuse the commit, not report zero files")
	}
}

func TestClassifyBlob(t *testing.T) {
	tests := []struct {
		name   string
		size   int64
		head   []byte
		reject bool
	}{
		{"plain text", 12, []byte("hello world\n"), false},
		{"empty", 0, nil, false},
		{"utf-8 text", 9, []byte("héllo ✓\n"), false},
		{"leading NUL", 4, []byte("\x00abc"), true},
		{"NUL mid-buffer", 8, []byte("ab\x00cdefg"), true},
		{"at the size limit", maxBlobBytes, []byte("text"), false},
		{"over the size limit", maxBlobBytes + 1, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			why := classifyBlob(tt.size, tt.head)
			if got := why != ""; got != tt.reject {
				t.Errorf("classifyBlob(%d, %q) = %q, reject=%v want %v",
					tt.size, tt.head, why, got, tt.reject)
			}
		})
	}
}
