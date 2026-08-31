package common

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// The WebIDL inputs to the generated bindings.
//
// This file is the single source of truth for *which* Web platform definitions
// produce web.pr and web.js, and at which revision. The .idl files themselves
// are not committed (see .gitignore) — they are fetched on demand into idl/,
// which keeps third-party spec text out of the tree while still pinning the
// exact input a binding diff can be traced to.
//
// To move to a newer webref release: bump WebrefVersion, update WebrefSHA256
// to the sha256 of the new tarball, and re-run `bin/bindgen`. The tool refuses
// to proceed on a hash mismatch, so a stale pin fails loudly rather than
// silently regenerating against different definitions.
const (
	// WebrefVersion pins the @webref/idl release. webref is the W3C's curated
	// extraction of the WebIDL blocks from the published living standards.
	// https://github.com/w3c/webref
	WebrefVersion = "3.83.0"

	// WebrefSHA256 is the sha256 of the pinned npm tarball.
	WebrefSHA256 = "0ef2bc72b2a50f9a61dd4e7f45a427d4b384ad89dfa29a07e75d717d55599883"

	// webrefTarball is the npm registry URL for the pinned release.
	webrefTarball = "https://registry.npmjs.org/@webref/idl/-/idl-%s.tgz"
)

// WebrefSpec is one Web platform specification contributing to the bindings.
type WebrefSpec struct {
	Stem    string // @webref/idl file stem, without the .idl extension
	Purpose string // what it contributes to the binding surface
}

// WebrefSpecs is the ordered list of specs fed to `promise bind webidl`. The
// order fixes the order interfaces appear in the generated web.pr, so keep it
// stable — reordering produces a large, meaningless diff.
//
// Keep in sync with the "Coverage" section of README.md.
var WebrefSpecs = []WebrefSpec{
	// Core object model.
	{"dom", "WHATWG DOM: EventTarget, Node, Element, Document, Range"},
	{"html", "WHATWG HTML: Window, HTMLElement and the element hierarchy"},
	{"console", "WHATWG Console: the console namespace"},

	// Events.
	{"uievents", "UI Events: UIEvent, MouseEvent, KeyboardEvent"},

	// Styling and layout.
	{"cssom", "CSSOM: CSSStyleDeclaration, CSSRule, StyleSheet"},
	{"cssom-view", "CSSOM View: getBoundingClientRect, scrolling, DOMRect on elements"},
	{"geometry", "Geometry Interfaces: DOMPoint, DOMRect, DOMMatrix"},

	// Networking and URLs.
	{"url", "WHATWG URL: URL, URLSearchParams"},
	{"xhr", "XMLHttpRequest, FormData, ProgressEvent"},
}

// IDLDir is the directory the fetched .idl files are written to, relative to
// the repo root. It is gitignored: generated input, not source.
const IDLDir = "idl"

// IDLPaths returns the absolute path of every vendored .idl file, in
// WebrefSpecs order.
func IDLPaths(repoRoot string) []string {
	paths := make([]string, 0, len(WebrefSpecs))
	for _, s := range WebrefSpecs {
		paths = append(paths, filepath.Join(repoRoot, IDLDir, s.Stem+".idl"))
	}
	return paths
}

// IDLFresh reports whether idl/ already holds every pinned spec at the pinned
// webref version. A version bump or a missing file makes it stale.
func IDLFresh(repoRoot string) bool {
	stamp, err := os.ReadFile(filepath.Join(repoRoot, IDLDir, "VERSION"))
	if err != nil || strings.TrimSpace(string(stamp)) != WebrefVersion {
		return false
	}
	for _, p := range IDLPaths(repoRoot) {
		if !Exists(p) {
			return false
		}
	}
	return true
}

// EnsureIDL makes idl/ hold every pinned spec at the pinned webref version,
// fetching only when it has to. Pass force to re-fetch unconditionally.
//
// This is the one path to a populated idl/: bin/fetch-idl calls it directly,
// and bin/bindgen calls it before handing the files to `promise bind webidl`.
func EnsureIDL(repoRoot string, force bool) error {
	if !force && IDLFresh(repoRoot) {
		fmt.Printf("    %s/ is already at @webref/idl@%s (-force to re-fetch)\n", IDLDir, WebrefVersion)
		return nil
	}
	return FetchIDL(repoRoot)
}

// FetchIDL downloads the pinned @webref/idl tarball, verifies its sha256, and
// extracts the specs named in WebrefSpecs into idl/.
//
// It talks to the npm registry over HTTPS directly rather than shelling out to
// npm: the tool then has no dependency beyond the Go toolchain, and behaves
// identically on Windows, macOS, and Linux.
func FetchIDL(repoRoot string) error {
	url := fmt.Sprintf(webrefTarball, WebrefVersion)
	fmt.Printf("    fetching @webref/idl@%s\n", WebrefVersion)

	body, err := download(url)
	if err != nil {
		return err
	}

	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != WebrefSHA256 {
		return fmt.Errorf("@webref/idl@%s sha256 mismatch:\n  want %s\n  got  %s\n"+
			"the pinned tarball changed under us — update WebrefSHA256 in\n"+
			"tools/build/common/webref.go only after verifying the new contents",
			WebrefVersion, WebrefSHA256, got)
	}

	// Map the specs we want by their path inside the tarball. npm tarballs put
	// everything under a single "package/" prefix.
	wanted := make(map[string]string, len(WebrefSpecs))
	for _, s := range WebrefSpecs {
		wanted["package/"+s.Stem+".idl"] = s.Stem
	}

	found, err := extract(body, wanted, filepath.Join(repoRoot, IDLDir))
	if err != nil {
		return err
	}

	if len(found) != len(wanted) {
		var missing []string
		for _, s := range WebrefSpecs {
			if !found[s.Stem] {
				missing = append(missing, s.Stem+".idl")
			}
		}
		return fmt.Errorf("@webref/idl@%s does not contain: %s",
			WebrefVersion, strings.Join(missing, ", "))
	}

	stamp := filepath.Join(repoRoot, IDLDir, "VERSION")
	if err := os.WriteFile(stamp, []byte(WebrefVersion+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", stamp, err)
	}

	fmt.Printf("    extracted %d spec(s) into %s/\n", len(found), IDLDir)
	return nil
}

// download GETs url and returns the whole body.
func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: HTTP %s", url, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	return body, nil
}

// extract writes the wanted members of a gzipped tarball into dstDir, naming
// each by its mapped stem. It returns the set of stems actually written.
func extract(tarball []byte, wanted map[string]string, dstDir string) (map[string]bool, error) {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dstDir, err)
	}

	zr, err := gzip.NewReader(strings.NewReader(string(tarball)))
	if err != nil {
		return nil, fmt.Errorf("gunzip: %w", err)
	}
	defer zr.Close()

	found := make(map[string]bool, len(wanted))
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		stem, ok := wanted[path.Clean(hdr.Name)]
		if !ok {
			continue
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("reading %s from tar: %w", hdr.Name, err)
		}

		dst := filepath.Join(dstDir, stem+".idl")
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", dst, err)
		}
		found[stem] = true
	}

	return found, nil
}
