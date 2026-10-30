package common

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/promise-language/forge/primitives"
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

// BindgenInput is what bin/bindgen takes: every flag is a field.
type BindgenInput struct {
	Force bool `flag:"force" help:"re-fetch idl/ even when it is already at the pinned version"`
	Clean bool `flag:"clean" help:"remove idl/ once generation succeeds"`
}

// Bindings is what one regeneration installed.
type Bindings struct {
	Webref string          `json:"webref"`
	Files  []InstalledFile `json:"files"`
	// CleanedIDL is whether idl/ was removed afterwards (-clean).
	CleanedIDL bool `json:"cleaned_idl"`
}

// InstalledFile is one generated file copied into the tree.
type InstalledFile struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

// Human is one line per installed file.
func (b Bindings) Human(w io.Writer) error {
	for _, f := range b.Files {
		if _, err := fmt.Fprintf(w, "%-8s %d bytes\n", f.Path, f.Bytes); err != nil {
			return err
		}
	}
	if b.CleanedIDL {
		if _, err := fmt.Fprintf(w, "removed %s/\n", IDLDir); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "bindings regenerated from @webref/idl@%s\n", b.Webref)
	return err
}

// RunBindgen regenerates web.pr and web.js from the pinned WebIDL definitions.
//
// It is the whole binding pipeline in one command:
//
//	idl/ (fetched, pinned)  ->  promise bind webidl  ->  web.pr + web.js
//
// Progress, and the compiler's own output, go to narrate; the result is what
// was installed.
func RunBindgen(ctx context.Context, repoRoot string, in BindgenInput, narrate io.Writer) (Bindings, error) {
	promise, err := findPromise()
	if err != nil {
		return Bindings{}, err
	}

	fmt.Fprintln(narrate, "==> WebIDL inputs")
	if err := EnsureIDL(repoRoot, in.Force, narrate); err != nil {
		return Bindings{}, err
	}

	// Generate into a scratch directory: `bind` also emits a promise.toml, and
	// this repo's is source rather than output. Only GeneratedFiles come back.
	// The scratch lives under the clone's gitignored .home/tmp/, so nothing is
	// written outside the checkout.
	tmp := filepath.Join(repoRoot, ".home", "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return Bindings{}, fmt.Errorf("creating %s: %w", tmp, err)
	}
	stage, err := os.MkdirTemp(tmp, "bindgen-")
	if err != nil {
		return Bindings{}, fmt.Errorf("creating scratch directory: %w", err)
	}
	defer os.RemoveAll(stage)

	fmt.Fprintln(narrate, "==> promise bind webidl")
	// -name web is required: without it the module is named after the first
	// interface, and "web" specifically is treated as unset and falls back to
	// the input filename.
	bindArgs := []string{"bind", "webidl", "-name", "web", "-o", stage}
	bindArgs = append(bindArgs, IDLPaths(repoRoot)...)

	cmd := exec.CommandContext(ctx, promise, bindArgs...)
	cmd.Dir = repoRoot
	cmd.Stdout = narrate
	cmd.Stderr = narrate
	if err := cmd.Run(); err != nil {
		return Bindings{}, fmt.Errorf("promise bind webidl: %w", err)
	}

	// `bind` can exit 0 having written output that does not compile, so confirm
	// the artifacts it promised actually landed before copying anything in.
	for _, name := range GeneratedFiles {
		if !Exists(filepath.Join(stage, name)) {
			return Bindings{}, fmt.Errorf("promise bind webidl reported success but did not write %s", name)
		}
	}

	out := Bindings{Webref: WebrefVersion}
	for _, name := range GeneratedFiles {
		n, err := copyFile(filepath.Join(stage, name), filepath.Join(repoRoot, name))
		if err != nil {
			return Bindings{}, err
		}
		out.Files = append(out.Files, InstalledFile{Path: name, Bytes: n})
	}

	if in.Clean {
		if err := os.RemoveAll(filepath.Join(repoRoot, IDLDir)); err != nil {
			return Bindings{}, fmt.Errorf("removing %s/: %w", IDLDir, err)
		}
		out.CleanedIDL = true
	}
	return out, nil
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
		fallback := filepath.Join(home, ".promise", "bin", primitives.BinaryName("promise"))
		if Exists(fallback) {
			return fallback, nil
		}
	}

	return "", fmt.Errorf("the promise compiler is not installed or not on PATH\n" +
		"install it from https://github.com/promise-language/promise, " +
		"or add ~/.promise/bin to PATH")
}
