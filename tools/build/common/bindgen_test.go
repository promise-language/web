package common

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The invocation is the one place the generator is told the module name, the
// keep list and the inputs, in the order that fixes web.pr's.
func TestBindArgs(t *testing.T) {
	root, stage := "/repo", "/stage"
	got := bindArgs(root, stage)

	want := []string{"bind", "webidl", "-name", "web", "-keep", filepath.Join(root, KeepList), "-o", stage}
	want = append(want, IDLInput().Paths(root)...)
	if !slices.Equal(got, want) {
		t.Errorf("bindArgs =\n  %v\nwant\n  %v", got, want)
	}
}

// pinnedRoot is a repository root whose inputs are already at their pins, so
// nothing is fetched, with the Baseline data baselineFixture and the keep list
// given.
func pinnedRoot(t *testing.T, keep string) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range pinnedInputs() {
		dir := filepath.Join(root, p.Dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, dir, "VERSION", p.Version+"\n")
		for _, m := range p.Members {
			writeFile(t, dir, m, "input\n")
		}
	}
	writeFile(t, filepath.Join(root, BaselineDir), BaselineData, baselineFixture)
	writeFile(t, root, KeepList, keep)
	return root
}

// A keep list naming what the pinned data does not report Baseline is refused
// before the compiler runs at all, and nothing is generated.
func TestRegenerateRefusesANonBaselineKeepListBeforeBinding(t *testing.T) {
	// No compiler is there: one that ran would fail to start, which is a
	// different refusal from the one this test expects.
	absent := filepath.Join(t.TempDir(), "promise")

	t.Run("refused", func(t *testing.T) {
		root := pinnedRoot(t, "Document\nDocument.caretPositionFromPoint\n")
		_, err := regenerate(context.Background(), root, absent, BindgenInput{}, io.Discard)
		if err == nil {
			t.Fatal("regenerated from a keep list naming a non-Baseline member")
		}
		if !strings.Contains(err.Error(), "keep list entry Document.caretPositionFromPoint:") {
			t.Errorf("refusal = %v, want the Baseline check naming Document.caretPositionFromPoint", err)
		}
		if strings.Contains(err.Error(), "promise bind webidl:") {
			t.Errorf("the compiler ran despite the refusal: %v", err)
		}
		for _, name := range GeneratedFiles {
			if Exists(filepath.Join(root, name)) {
				t.Errorf("a refused run wrote %s", name)
			}
		}
	})

	// The control: a Baseline keep list over the same inputs gets as far as
	// running the compiler, so the refusal above is the check's and not the
	// inputs'.
	t.Run("accepted", func(t *testing.T) {
		root := pinnedRoot(t, "Document\nDocument.body\n")
		_, err := regenerate(context.Background(), root, absent, BindgenInput{}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "promise bind webidl:") {
			t.Fatalf("regenerate = %v, want it to reach the compiler", err)
		}
	})
}

// A checkout whose dependency record does not name the pinned compiler cannot
// regenerate: the compiler is never looked for on PATH, and nothing is
// written.
func TestRunBindgenRefusesWithoutThePinnedCompiler(t *testing.T) {
	root := t.TempDir()
	if _, err := RunBindgen(context.Background(), root, BindgenInput{}, io.Discard); err == nil {
		t.Fatal("RunBindgen ran with no dependency record to resolve the compiler from")
	}
	for _, name := range append([]string{IDLDir, BaselineDir}, GeneratedFiles...) {
		if Exists(filepath.Join(root, name)) {
			t.Errorf("a refused run wrote %s", name)
		}
	}
}
