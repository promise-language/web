package common

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// The spec list drives both the fetch and the generator invocation, so a
// malformed entry breaks the pipeline in a confusing place. Catch it here.
func TestWebrefSpecsWellFormed(t *testing.T) {
	if len(WebrefSpecs) == 0 {
		t.Fatal("WebrefSpecs is empty: the bindings would have no input")
	}

	seen := make(map[string]bool, len(WebrefSpecs))
	for _, s := range WebrefSpecs {
		switch {
		case s.Stem == "":
			t.Error("spec with an empty Stem")
		case s.Purpose == "":
			t.Errorf("spec %q has no Purpose: it documents the Coverage table in README.md", s.Stem)
		case filepath.Ext(s.Stem) != "":
			t.Errorf("spec %q carries an extension: Stem is the bare file stem", s.Stem)
		case seen[s.Stem]:
			// A duplicate would hand the same file to `promise bind webidl`
			// twice, which merges it into itself.
			t.Errorf("spec %q listed more than once", s.Stem)
		}
		seen[s.Stem] = true
	}
}

// The pin is what makes a binding diff traceable to a spec revision, so both
// halves of it must be present and plausible.
func TestWebrefPin(t *testing.T) {
	if WebrefVersion == "" {
		t.Error("WebrefVersion is empty")
	}
	if len(WebrefSHA256) != 64 {
		t.Errorf("WebrefSHA256 = %q (%d chars), want a 64-char hex sha256",
			WebrefSHA256, len(WebrefSHA256))
	}
	for _, r := range WebrefSHA256 {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("WebrefSHA256 contains non-lowercase-hex %q", r)
			break
		}
	}
}

func TestIDLPaths(t *testing.T) {
	paths := IDLPaths("/repo")
	if len(paths) != len(WebrefSpecs) {
		t.Fatalf("got %d paths, want %d", len(paths), len(WebrefSpecs))
	}
	want := filepath.Join("/repo", IDLDir, WebrefSpecs[0].Stem+".idl")
	if paths[0] != want {
		t.Errorf("paths[0] = %q, want %q", paths[0], want)
	}
}

// IDLFresh gates the network call, so each way of being stale matters.
func TestIDLFresh(t *testing.T) {
	writeIDL := func(root, version string, stems []string) {
		t.Helper()
		dir := filepath.Join(root, IDLDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if version != "" {
			if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for _, s := range stems {
			if err := os.WriteFile(filepath.Join(dir, s+".idl"), []byte("// idl\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	allStems := make([]string, 0, len(WebrefSpecs))
	for _, s := range WebrefSpecs {
		allStems = append(allStems, s.Stem)
	}

	t.Run("no idl dir", func(t *testing.T) {
		if IDLFresh(t.TempDir()) {
			t.Error("fresh with no idl/ at all")
		}
	})

	t.Run("complete and pinned", func(t *testing.T) {
		root := t.TempDir()
		writeIDL(root, WebrefVersion, allStems)
		if !IDLFresh(root) {
			t.Error("stale despite a complete idl/ at the pinned version")
		}
	})

	t.Run("version mismatch", func(t *testing.T) {
		root := t.TempDir()
		writeIDL(root, "0.0.1-not-the-pin", allStems)
		if IDLFresh(root) {
			t.Error("fresh despite a VERSION that does not match the pin")
		}
	})

	t.Run("missing VERSION", func(t *testing.T) {
		root := t.TempDir()
		writeIDL(root, "", allStems)
		if IDLFresh(root) {
			t.Error("fresh despite no VERSION stamp")
		}
	})

	t.Run("missing a spec", func(t *testing.T) {
		root := t.TempDir()
		writeIDL(root, WebrefVersion, allStems[:len(allStems)-1])
		if IDLFresh(root) {
			t.Error("fresh despite a spec file missing")
		}
	})
}

func TestExtract(t *testing.T) {
	tarball := buildTarball(t, map[string]string{
		"package/dom.idl":     "interface Node {};",
		"package/console.idl": "namespace console {};",
		"package/ignored.idl": "interface Unwanted {};",
		"package/README.md":   "not idl",
	})

	dst := t.TempDir()
	found, err := extract(tarball, map[string]string{
		"package/dom.idl":     "dom",
		"package/console.idl": "console",
	}, dst)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	if len(found) != 2 || !found["dom"] || !found["console"] {
		t.Errorf("found = %v, want dom and console", found)
	}

	got, err := os.ReadFile(filepath.Join(dst, "dom.idl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "interface Node {};" {
		t.Errorf("dom.idl = %q", got)
	}

	// Members outside the wanted set must not be written: the fetch is scoped
	// to the spec list, not "whatever the tarball happens to contain".
	if Exists(filepath.Join(dst, "ignored.idl")) {
		t.Error("extract wrote a file that was not requested")
	}
}

// A spec that has disappeared upstream must surface as a missing member rather
// than a silently short binding surface.
func TestExtractReportsMissing(t *testing.T) {
	tarball := buildTarball(t, map[string]string{"package/dom.idl": "interface Node {};"})

	found, err := extract(tarball, map[string]string{
		"package/dom.idl":  "dom",
		"package/gone.idl": "gone",
	}, t.TempDir())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if found["gone"] {
		t.Error("reported a member the tarball does not contain")
	}
	if len(found) != 1 {
		t.Errorf("found = %v, want just dom", found)
	}
}

// buildTarball returns a gzipped tar holding the given path -> content pairs.
func buildTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)

	for name, body := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
