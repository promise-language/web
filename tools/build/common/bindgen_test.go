package common

import (
	"context"
	"fmt"
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

// fakeCompilerMode, set in the environment, makes the test binary stand in for
// `promise bind webidl` (TestMain), the way the mode says:
//
//   - binds: writes web.pr, web.js and an epoch-stamped promise.toml into the
//     -o directory, reports a dropped member on stderr, and exits 0;
//   - fails: reports a parse error on stderr and exits 1;
//   - half: writes web.pr only, and exits 0.
//
// The web.pr it writes is the command line it was given, so a test reads back
// what it was told.
const fakeCompilerMode = "WEB_BINDGEN_TEST_FAKE_PROMISE"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeCompilerMode); mode != "" {
		os.Exit(fakeCompiler(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeCompiler(mode string, args []string) int {
	stage := ""
	for i, arg := range args {
		if arg == "-o" && i+1 < len(args) {
			stage = args[i+1]
		}
	}
	if stage == "" {
		fmt.Fprintln(os.Stderr, "fake promise: no -o")
		return 2
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(stage, name), []byte(body), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	switch mode {
	case "binds":
		write("web.pr", strings.Join(args, " ")+"\n")
		write("web.js", "// glue\n")
		write("promise.toml", "[module]\nname = \"web\"\nepoch = \"2026.13\"\n")
		fmt.Fprintln(os.Stderr, "keep: dropped Fake.member: it names Gone, which the keep list drops")
		return 0
	case "fails":
		fmt.Fprintln(os.Stderr, "idl/dom.idl:1:1: unexpected token")
		return 1
	case "half":
		write("web.pr", "// half\n")
		return 0
	}
	fmt.Fprintln(os.Stderr, "fake promise: unknown mode", mode)
	return 2
}

// fakeCompilerIn is the path of the test binary, which TestMain turns into the
// stand-in compiler for the mode given.
func fakeCompilerIn(t *testing.T, mode string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeCompilerMode, mode)
	return self
}

// handMaintained is the repository's own promise.toml, which a regeneration
// never touches: bind writes one of its own, stamped with an epoch.
const handMaintained = "[module]\nname = \"web\"\n"

// A regeneration installs exactly web.pr and web.js from what bind wrote: the
// promise.toml bind writes beside them stays in scratch, the scratch is
// removed, and what bind said on stderr — the members -keep reports dropping —
// reaches the narration.
func TestRegenerateInstallsOnlyTheGeneratedFiles(t *testing.T) {
	root := pinnedRoot(t, "Document\nDocument.body\n")
	writeFile(t, root, "promise.toml", handMaintained)
	compiler := fakeCompilerIn(t, "binds")

	var narration strings.Builder
	got, err := regenerate(context.Background(), root, compiler, BindgenInput{Clean: true}, &narration)
	if err != nil {
		t.Fatalf("regenerate: %v\n%s", err, narration.String())
	}

	pr, err := os.ReadFile(filepath.Join(root, "web.pr"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pr), "-keep "+filepath.Join(root, KeepList)+" ") {
		t.Errorf("bind was not given the keep list: it was run as %q", pr)
	}
	if js, err := os.ReadFile(filepath.Join(root, "web.js")); err != nil || string(js) != "// glue\n" {
		t.Errorf("web.js = %q, %v, want what bind wrote", js, err)
	}
	if toml, err := os.ReadFile(filepath.Join(root, "promise.toml")); err != nil || string(toml) != handMaintained {
		t.Errorf("promise.toml = %q, %v: a regeneration overwrote the hand-maintained manifest", toml, err)
	}
	var names []string
	for _, f := range got.Files {
		names = append(names, f.Path)
	}
	if !slices.Equal(names, GeneratedFiles) {
		t.Errorf("installed %v, want %v", names, GeneratedFiles)
	}

	if !strings.Contains(narration.String(), "keep: dropped Fake.member: it names Gone") {
		t.Errorf("the narration does not relay what bind reported dropping:\n%s", narration.String())
	}
	if scratch, _ := filepath.Glob(filepath.Join(root, ".home", "tmp", "bindgen-*")); len(scratch) != 0 {
		t.Errorf("the scratch directory was left behind: %v", scratch)
	}
	if !got.CleanedIDL || Exists(filepath.Join(root, IDLDir)) {
		t.Errorf("-clean left %s/ in place after a regeneration that succeeded", IDLDir)
	}
}

// A bind that fails, or that exits 0 without writing everything it owes,
// installs nothing — not even the file it did write — and -clean keeps the
// inputs, which the next attempt needs.
func TestRegenerateInstallsNothingWhenBindFails(t *testing.T) {
	cases := []struct {
		mode string
		want string // what the refusal names
	}{
		{"fails", "promise bind webidl:"},
		{"half", "did not write web.js"},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			root := pinnedRoot(t, "Document\nDocument.body\n")
			compiler := fakeCompilerIn(t, c.mode)

			var narration strings.Builder
			_, err := regenerate(context.Background(), root, compiler, BindgenInput{Clean: true}, &narration)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("regenerate = %v, want a refusal naming %q", err, c.want)
			}
			for _, name := range GeneratedFiles {
				if Exists(filepath.Join(root, name)) {
					t.Errorf("a failed regeneration installed %s", name)
				}
			}
			if !Exists(filepath.Join(root, IDLDir)) {
				t.Errorf("-clean removed %s/ although the regeneration failed", IDLDir)
			}
		})
	}

	t.Run("its diagnostics reach the narration", func(t *testing.T) {
		root := pinnedRoot(t, "Document\n")
		var narration strings.Builder
		if _, err := regenerate(context.Background(), root, fakeCompilerIn(t, "fails"), BindgenInput{}, &narration); err == nil {
			t.Fatal("regenerate succeeded over a bind that failed")
		}
		if !strings.Contains(narration.String(), "idl/dom.idl:1:1: unexpected token") {
			t.Errorf("the narration does not carry what bind said:\n%s", narration.String())
		}
	})
}
