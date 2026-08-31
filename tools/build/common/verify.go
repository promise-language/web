package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type step struct {
	name string
	run  func(repoRoot string) error
}

// RunVerify is the commit gate. It always prints a summary block (even on
// failure) so an agent tailing the output sees the result without re-running,
// and the process exit code is the only contract.
func RunVerify(repoRoot string, args []string) error {
	steps := verifySteps(repoRoot)
	start := time.Now()

	type result struct {
		name string
		ok   bool
	}
	var results []result
	failed := false

	for _, s := range steps {
		fmt.Printf("==> %s\n", s.name)
		err := s.run(repoRoot)
		results = append(results, result{s.name, err == nil})
		if err != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "    %s failed: %v\n", s.name, err)
			break // stop at the first failure
		}
	}

	fmt.Println("\n──────── verify summary ────────")
	for _, r := range results {
		status := "ok"
		if !r.ok {
			status = "FAIL"
		}
		fmt.Printf("  %-4s  %s\n", status, r.name)
	}
	fmt.Printf("  elapsed %s\n", time.Since(start).Round(time.Millisecond))
	fmt.Println("────────────────────────────────")

	if failed {
		fmt.Println("❌ Verify FAILED: not safe to commit")
		return fmt.Errorf("verify failed")
	}
	fmt.Println("✅ OK to Commit")
	return nil
}

// verifySteps is this repo's gate: the Go dev tooling, then the generated
// Promise bindings.
//
// The bindings are *generated*, so the gate checks that what is committed is
// well-formed and passes its tests — not that it was hand-edited correctly.
// Regeneration itself is `bin/bindgen`, kept out of the gate because it needs
// network access.
func verifySteps(repoRoot string) []step {
	tools := filepath.Join(repoRoot, "tools", "build")

	steps := []step{
		{"tools:format", func(string) error { return gofmtCheck(tools) }},
		{"tools:vet", func(string) error { return RunIn(tools, "go", "vet", "./...") }},
		{"tools:build", func(string) error { return RunIn(tools, "go", "build", "./...") }},
		{"tools:test", func(string) error { return RunIn(tools, "go", "test", "./...") }},
	}

	// The Promise half of the gate needs both the toolchain and generated
	// output. Report a skip rather than a pass when either is missing — a gate
	// that silently checks nothing is worse than one that says so.
	promise, err := findPromise()
	switch {
	case err != nil:
		steps = append(steps, skip("bindings", "the promise compiler is not on PATH"))
	case !Exists(filepath.Join(repoRoot, "web.pr")):
		steps = append(steps, skip("bindings", "web.pr has not been generated yet — run bin/bindgen"))
	default:
		steps = append(steps,
			step{"bindings:format", func(r string) error {
				return RunIn(r, promise, "format", "web.pr")
			}},
			step{"bindings:check", func(r string) error {
				return RunIn(r, promise, "check", r)
			}},
			step{"bindings:test", func(r string) error {
				if !hasTests(r) {
					fmt.Println("    no *_test.pr files yet")
					return nil
				}
				return RunIn(r, promise, "test", r)
			}},
		)
	}

	return steps
}

func skip(name, why string) step {
	return step{name, func(string) error {
		fmt.Printf("    skipped: %s\n", why)
		return nil
	}}
}

// gofmtCheck fails when any file under dir is not gofmt-clean, listing them.
// It reports rather than rewrites, so the gate never mutates the tree it is
// judging.
func gofmtCheck(dir string) error {
	out, err := RunOutputIn(dir, "gofmt", "-l", ".")
	if err != nil {
		return err
	}
	if out != "" {
		return fmt.Errorf("not gofmt-clean:\n%s\nrun: gofmt -w %s", out, dir)
	}
	return nil
}

// hasTests reports whether the module has any Promise test files.
func hasTests(repoRoot string) bool {
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.pr") {
			return true
		}
	}
	return false
}
