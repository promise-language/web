package common

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

// pinnedInputs are the inputs every pinned-input test runs over: each one
// fetches and goes stale the same way, so each must pass the same tests.
func pinnedInputs() []PinnedInput { return []PinnedInput{IDLInput(), BaselineInput()} }

// A pin is what makes a binding diff traceable to an input revision, so both
// halves of every pin must be present and plausible.
func TestPinsArePlausible(t *testing.T) {
	for _, p := range pinnedInputs() {
		t.Run(p.Package, func(t *testing.T) {
			if p.Version == "" {
				t.Error("no version")
			}
			if len(p.SHA256) != 64 {
				t.Errorf("sha256 = %q (%d chars), want a 64-char hex sha256", p.SHA256, len(p.SHA256))
			}
			if strings.Trim(p.SHA256, "0") == "" {
				t.Error("sha256 is all zeros: a placeholder, not a pin")
			}
			for _, r := range p.SHA256 {
				if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
					t.Errorf("sha256 contains non-lowercase-hex %q", r)
					break
				}
			}
			if !strings.Contains(p.Tarball, p.Version) {
				t.Errorf("tarball %s does not name the pinned version %s", p.Tarball, p.Version)
			}
			if len(p.Members) == 0 {
				t.Error("no members: the input would extract nothing")
			}
		})
	}
}

func TestPinnedInputPaths(t *testing.T) {
	idl := IDLInput()
	paths := idl.Paths("/repo")
	if len(paths) != len(WebrefSpecs) {
		t.Fatalf("got %d paths, want %d", len(paths), len(WebrefSpecs))
	}
	if want := filepath.Join("/repo", IDLDir, WebrefSpecs[0].Stem+".idl"); paths[0] != want {
		t.Errorf("paths[0] = %q, want %q", paths[0], want)
	}

	if got, want := BaselineInput().Paths("/repo"), []string{filepath.Join("/repo", BaselineDir, BaselineData)}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("Baseline paths = %v, want %v", got, want)
	}
}

// Fresh gates the network call, so each way of being stale matters, for
// every input.
func TestPinnedInputFresh(t *testing.T) {
	write := func(t *testing.T, p PinnedInput, root, version string, members []string) {
		t.Helper()
		dir := filepath.Join(root, p.Dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if version != "" {
			if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for _, m := range members {
			if err := os.WriteFile(filepath.Join(dir, m), []byte("input\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, p := range pinnedInputs() {
		t.Run(p.Package, func(t *testing.T) {
			t.Run("no directory", func(t *testing.T) {
				if p.Fresh(t.TempDir()) {
					t.Errorf("fresh with no %s/ at all", p.Dir)
				}
			})

			t.Run("complete and pinned", func(t *testing.T) {
				root := t.TempDir()
				write(t, p, root, p.Version, p.Members)
				if !p.Fresh(root) {
					t.Errorf("stale despite a complete %s/ at the pinned version", p.Dir)
				}
			})

			t.Run("version mismatch", func(t *testing.T) {
				root := t.TempDir()
				write(t, p, root, "0.0.1-not-the-pin", p.Members)
				if p.Fresh(root) {
					t.Error("fresh despite a VERSION that does not match the pin")
				}
			})

			t.Run("missing VERSION", func(t *testing.T) {
				root := t.TempDir()
				write(t, p, root, "", p.Members)
				if p.Fresh(root) {
					t.Error("fresh despite no VERSION stamp")
				}
			})

			t.Run("missing a member", func(t *testing.T) {
				root := t.TempDir()
				write(t, p, root, p.Version, p.Members[:len(p.Members)-1])
				if p.Fresh(root) {
					t.Error("fresh despite a member missing")
				}
			})
		})
	}
}

// servedInput is an input whose tarball a local server serves, pinned to the
// sha256 given, and the count of the requests the server has answered.
func servedInput(t *testing.T, tarball []byte, pin string) (PinnedInput, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if _, err := w.Write(tarball); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	return PinnedInput{
		Package: "probe",
		Version: "1.0.0",
		SHA256:  pin,
		Tarball: srv.URL + "/probe-1.0.0.tgz",
		Dir:     "probe",
		Members: []string{"data.json"},
	}, &requests
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// A tarball that does not hash to its pin is refused before anything is
// extracted, so a changed release never regenerates the bindings silently.
func TestFetchRefusesATarballThatDoesNotMatchItsPin(t *testing.T) {
	tarball := buildTarball(t, map[string]string{"package/data.json": "{}"})
	p, _ := servedInput(t, tarball, strings.Repeat("ab", 32))
	root := t.TempDir()

	err := p.Fetch(root, io.Discard)
	if err == nil {
		t.Fatal("fetched a tarball whose sha256 is not the pin")
	}
	if !strings.Contains(err.Error(), sha256Hex(tarball)) {
		t.Errorf("the refusal does not name the sha256 it got: %v", err)
	}
	if Exists(filepath.Join(root, p.Dir)) {
		t.Error("a refused fetch wrote the input directory")
	}
}

func TestFetchExtractsAndStampsAMatchingTarball(t *testing.T) {
	tarball := buildTarball(t, map[string]string{
		"package/data.json": `{"features":{}}`,
		"package/other.js":  "not wanted",
	})
	p, _ := servedInput(t, tarball, sha256Hex(tarball))
	root := t.TempDir()

	if err := p.Fetch(root, io.Discard); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !p.Fresh(root) {
		t.Error("stale right after a fetch")
	}
	if Exists(filepath.Join(root, p.Dir, "other.js")) {
		t.Error("extracted a file that is not a member")
	}
}

// Ensure gates the network: it fetches an input that is absent or stale, and
// one -force names, and leaves a fresh one alone.
func TestEnsureFetchesOnlyWhenItHasTo(t *testing.T) {
	tarball := buildTarball(t, map[string]string{"package/data.json": "{}"})
	p, requests := servedInput(t, tarball, sha256Hex(tarball))
	root := t.TempDir()

	steps := []struct {
		what  string
		force bool
		stale func()
		want  int32 // requests answered after the step
	}{
		{what: "absent", want: 1},
		{what: "fresh", want: 1},
		{what: "fresh and forced", force: true, want: 2},
		{what: "stamped with another version", want: 3, stale: func() {
			writeFile(t, filepath.Join(root, p.Dir), "VERSION", "0.9.0\n")
		}},
		{what: "missing a member", want: 4, stale: func() {
			if err := os.Remove(filepath.Join(root, p.Dir, "data.json")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, s := range steps {
		if s.stale != nil {
			s.stale()
		}
		if err := p.Ensure(root, s.force, io.Discard); err != nil {
			t.Fatalf("Ensure over an input %s: %v", s.what, err)
		}
		if got := requests.Load(); got != s.want {
			t.Errorf("an input %s: %d request(s) in all, want %d", s.what, got, s.want)
		}
		if !p.Fresh(root) {
			t.Errorf("an input %s is stale after Ensure", s.what)
		}
	}
}

// A member the release does not carry is a missing input, never a short one.
func TestFetchReportsAMissingMember(t *testing.T) {
	tarball := buildTarball(t, map[string]string{"package/README.md": "no data"})
	p, _ := servedInput(t, tarball, sha256Hex(tarball))

	err := p.Fetch(t.TempDir(), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "data.json") {
		t.Fatalf("Fetch = %v, want a refusal naming the missing data.json", err)
	}
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
		"package/dom.idl":     "dom.idl",
		"package/console.idl": "console.idl",
	}, dst)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	if len(found) != 2 || !found["dom.idl"] || !found["console.idl"] {
		t.Errorf("found = %v, want dom.idl and console.idl", found)
	}

	got, err := os.ReadFile(filepath.Join(dst, "dom.idl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "interface Node {};" {
		t.Errorf("dom.idl = %q", got)
	}

	// Members outside the wanted set must not be written: the fetch is scoped
	// to the pinned members, not "whatever the tarball happens to contain".
	if Exists(filepath.Join(dst, "ignored.idl")) {
		t.Error("extract wrote a file that was not requested")
	}
}

// A spec that has disappeared upstream must surface as a missing member rather
// than a silently short binding surface.
func TestExtractReportsMissing(t *testing.T) {
	tarball := buildTarball(t, map[string]string{"package/dom.idl": "interface Node {};"})

	found, err := extract(tarball, map[string]string{
		"package/dom.idl":  "dom.idl",
		"package/gone.idl": "gone.idl",
	}, t.TempDir())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if found["gone.idl"] {
		t.Error("reported a member the tarball does not contain")
	}
	if len(found) != 1 {
		t.Errorf("found = %v, want just dom.idl", found)
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
