package common

import (
	"fmt"
	"os"
	"path/filepath"
)

// GeneratedFiles are the artifacts `promise bind webidl` produces that this
// repo owns and commits.
//
// Note what is deliberately absent: promise.toml. `bind` writes one into its
// output directory, stamped with the compiler's epoch — but modules here are
// epoch-less and the repo's promise.toml is hand-maintained source, not a
// generated artifact. Generating into a scratch directory and copying out only
// these two files keeps the generator from clobbering it.
var GeneratedFiles = []string{"web.pr", "web.js"}

// RunBindgen regenerates web.pr and web.js from the pinned WebIDL definitions.
//
// It is the whole binding pipeline in one command:
//
//	idl/ (fetched, pinned)  ->  promise bind webidl  ->  web.pr + web.js
//
// Flags:
//
//	-force   re-fetch idl/ even when it is already at the pinned version
//	-clean   remove idl/ once generation succeeds
func RunBindgen(repoRoot string, args []string) error {
	force := false
	clean := false
	for _, a := range args {
		switch a {
		case "-force":
			force = true
		case "-clean":
			clean = true
		default:
			return fmt.Errorf("unknown flag: %s\nusage: bin/bindgen [-force] [-clean]", a)
		}
	}

	promise, err := findPromise()
	if err != nil {
		return err
	}

	fmt.Println("==> WebIDL inputs")
	if err := EnsureIDL(repoRoot, force); err != nil {
		return err
	}

	// Generate into a scratch directory: `bind` also emits a promise.toml, and
	// this repo's is source rather than output. Only GeneratedFiles come back.
	stage, err := os.MkdirTemp("", "web-bindgen-")
	if err != nil {
		return fmt.Errorf("creating scratch directory: %w", err)
	}
	defer os.RemoveAll(stage)

	fmt.Println("==> promise bind webidl")
	// -name web is required: without it the module is named after the first
	// interface, and "web" specifically is treated as unset and falls back to
	// the input filename.
	bindArgs := []string{"bind", "webidl", "-name", "web", "-o", stage}
	bindArgs = append(bindArgs, IDLPaths(repoRoot)...)

	if err := RunIn(repoRoot, promise, bindArgs...); err != nil {
		return fmt.Errorf("promise bind webidl: %w", err)
	}

	// `bind` can exit 0 having written output that does not compile, so confirm
	// the artifacts it promised actually landed before copying anything in.
	for _, name := range GeneratedFiles {
		if !Exists(filepath.Join(stage, name)) {
			return fmt.Errorf("promise bind webidl reported success but did not write %s", name)
		}
	}

	fmt.Println("==> installing generated bindings")
	for _, name := range GeneratedFiles {
		n, err := copyFile(filepath.Join(stage, name), filepath.Join(repoRoot, name))
		if err != nil {
			return err
		}
		fmt.Printf("    %-8s %d bytes\n", name, n)
	}

	if clean {
		if err := os.RemoveAll(filepath.Join(repoRoot, IDLDir)); err != nil {
			return fmt.Errorf("removing %s/: %w", IDLDir, err)
		}
		fmt.Printf("    removed %s/\n", IDLDir)
	}

	fmt.Println("\n✅ bindings regenerated")
	return nil
}

// copyFile copies src over dst, returning the number of bytes written.
func copyFile(src, dst string) (int, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return 0, fmt.Errorf("writing %s: %w", dst, err)
	}
	return len(data), nil
}

// findPromise locates the Promise toolchain: PATH first, then the default root
// that `promise install` writes to.
func findPromise() (string, error) {
	if p := Which("promise"); p != "" {
		return p, nil
	}

	if home, err := os.UserHomeDir(); err == nil {
		fallback := filepath.Join(home, ".promise", "bin", BinaryName("promise"))
		if Exists(fallback) {
			return fallback, nil
		}
	}

	return "", fmt.Errorf("the promise compiler is not installed or not on PATH\n" +
		"install it from https://github.com/promise-language/promise, " +
		"or add ~/.promise/bin to PATH")
}
