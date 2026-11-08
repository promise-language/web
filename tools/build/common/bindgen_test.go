package common

import (
	"context"
	"io"
	"path/filepath"
	"slices"
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
