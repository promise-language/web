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

// The inputs to the generated bindings.
//
// This file is the single source of truth for *which* Web platform definitions
// produce web.pr and web.js, which of them are Baseline, and at which revision.
// Neither input is committed (see .gitignore) — each is fetched on demand,
// which keeps third-party text out of the tree while still pinning the exact
// input a binding diff can be traced to.
//
// To move to a newer release of either: bump its version, update its sha256 to
// the sha256 of the new tarball, and re-run `bin/bindgen`. The tools refuse to
// proceed on a hash mismatch, so a stale pin fails loudly rather than silently
// regenerating against different definitions.
const (
	// WebrefVersion pins the @webref/idl release. webref is the W3C's curated
	// extraction of the WebIDL blocks from the published living standards.
	// https://github.com/w3c/webref
	WebrefVersion = "3.83.0"

	// WebrefSHA256 is the sha256 of the pinned npm tarball.
	WebrefSHA256 = "0ef2bc72b2a50f9a61dd4e7f45a427d4b384ad89dfa29a07e75d717d55599883"

	// webrefTarball is the npm registry URL for the pinned release.
	webrefTarball = "https://registry.npmjs.org/@webref/idl/-/idl-%s.tgz"

	// WebFeaturesVersion pins the web-features release: the WebDX Community
	// Group's Baseline status, computed from @mdn/browser-compat-data, for
	// each compat key. The keep list selects only what it reports Baseline.
	// https://github.com/web-platform-dx/web-features
	WebFeaturesVersion = "3.45.0"

	// WebFeaturesSHA256 is the sha256 of the pinned npm tarball.
	WebFeaturesSHA256 = "f1b743f337b877178e5ddc264151e9e6627af26096ba96cddda8ddd7e6907e25"

	// webFeaturesTarball is the npm registry URL for the pinned release.
	webFeaturesTarball = "https://registry.npmjs.org/web-features/-/web-features-%s.tgz"
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
	{"uievents", "UI Events: UIEvent, KeyboardEvent"},
	{"pointerevents", "Pointer Events: MouseEvent, PointerEvent"},

	// Styling and layout.
	{"cssom", "CSSOM: CSSStyleDeclaration, CSSRule, StyleSheet"},
	{"cssom-view", "CSSOM View: getBoundingClientRect, scrolling, DOMRect on elements"},
	{"geometry", "Geometry Interfaces: DOMPoint, DOMRect, DOMMatrix"},
	{"css-pseudo", "CSS Pseudo-Elements: CSSPseudoElement"},

	// Networking and URLs.
	{"url", "WHATWG URL: URL, URLSearchParams"},
	{"xhr", "XMLHttpRequest, FormData, ProgressEvent"},

	// What the specs above include or extend, so every partial and includes
	// statement in them finds its target.
	{"wai-aria", "WAI-ARIA: the ARIAMixin every element includes"},
	{"SVG", "SVG: SVGElement, which CSSOM and HTML extend"},
	{"mathml-core", "MathML Core: MathMLElement, which CSSOM and HTML extend"},
}

// IDLDir is the directory the fetched .idl files are written to, relative to
// the repo root. It is gitignored: generated input, not source.
const IDLDir = "idl"

// BaselineDir is the directory the fetched Baseline data is written to,
// relative to the repo root. It is gitignored for the same reason as IDLDir.
const BaselineDir = "baseline"

// BaselineData is the one file of the web-features package the Baseline
// check reads.
const BaselineData = "data.json"

// PinnedInput is one third-party input to the bindings: an npm release pinned
// by version and tarball sha256, of which the named members are extracted into
// a gitignored directory of the repo, beside a VERSION stamp.
type PinnedInput struct {
	Package string   // the npm package, as a person names it
	Version string   // the pinned release
	SHA256  string   // the sha256 of the release's tarball
	Tarball string   // the registry URL of the release's tarball
	Dir     string   // where the members are extracted, relative to the repo root
	Members []string // the files extracted, by their path inside the package
}

// IDLInput is the pinned @webref/idl release and the specs it contributes.
func IDLInput() PinnedInput {
	members := make([]string, 0, len(WebrefSpecs))
	for _, s := range WebrefSpecs {
		members = append(members, s.Stem+".idl")
	}
	return PinnedInput{
		Package: "@webref/idl",
		Version: WebrefVersion,
		SHA256:  WebrefSHA256,
		Tarball: fmt.Sprintf(webrefTarball, WebrefVersion),
		Dir:     IDLDir,
		Members: members,
	}
}

// BaselineInput is the pinned web-features release and the one file of it
// the Baseline check reads.
func BaselineInput() PinnedInput {
	return PinnedInput{
		Package: "web-features",
		Version: WebFeaturesVersion,
		SHA256:  WebFeaturesSHA256,
		Tarball: fmt.Sprintf(webFeaturesTarball, WebFeaturesVersion),
		Dir:     BaselineDir,
		Members: []string{BaselineData},
	}
}

// Paths returns the absolute path of every member, in Members order.
func (p PinnedInput) Paths(repoRoot string) []string {
	paths := make([]string, 0, len(p.Members))
	for _, m := range p.Members {
		paths = append(paths, filepath.Join(repoRoot, p.Dir, m))
	}
	return paths
}

// Fresh reports whether the input's directory already holds every member at
// the pinned version. A version bump or a missing file makes it stale.
func (p PinnedInput) Fresh(repoRoot string) bool {
	stamp, err := os.ReadFile(filepath.Join(repoRoot, p.Dir, "VERSION"))
	if err != nil || strings.TrimSpace(string(stamp)) != p.Version {
		return false
	}
	for _, path := range p.Paths(repoRoot) {
		if !Exists(path) {
			return false
		}
	}
	return true
}

// Ensure makes the input's directory hold every member at the pinned version,
// fetching only when it has to. Pass force to re-fetch unconditionally.
//
// This is the one path to a populated input directory: bin/fetch-idl calls it
// for the WebIDL, and bin/bindgen calls it for every input before handing them
// to `promise bind webidl`.
//
// Progress goes to narrate, never to stdout: stdout is the calling command's
// result.
func (p PinnedInput) Ensure(repoRoot string, force bool, narrate io.Writer) error {
	if !force && p.Fresh(repoRoot) {
		fmt.Fprintf(narrate, "    %s/ is already at %s@%s (-force to re-fetch)\n", p.Dir, p.Package, p.Version)
		return nil
	}
	return p.Fetch(repoRoot, narrate)
}

// Fetch downloads the pinned tarball, verifies its sha256, and extracts the
// members into the input's directory.
//
// It talks to the npm registry over HTTPS directly rather than shelling out to
// npm: the tool then has no dependency beyond the Go toolchain, and behaves
// identically on Windows, macOS, and Linux.
func (p PinnedInput) Fetch(repoRoot string, narrate io.Writer) error {
	fmt.Fprintf(narrate, "    fetching %s@%s\n", p.Package, p.Version)

	body, err := fetchPinned(p)
	if err != nil {
		return err
	}

	// Map the members by their path inside the tarball. npm tarballs put
	// everything under a single "package/" prefix.
	wanted := make(map[string]string, len(p.Members))
	for _, m := range p.Members {
		wanted["package/"+m] = m
	}

	found, err := extract(body, wanted, filepath.Join(repoRoot, p.Dir))
	if err != nil {
		return err
	}

	if len(found) != len(wanted) {
		var missing []string
		for _, m := range p.Members {
			if !found[m] {
				missing = append(missing, m)
			}
		}
		return fmt.Errorf("%s@%s does not contain: %s", p.Package, p.Version, strings.Join(missing, ", "))
	}

	stamp := filepath.Join(repoRoot, p.Dir, "VERSION")
	if err := os.WriteFile(stamp, []byte(p.Version+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", stamp, err)
	}

	fmt.Fprintf(narrate, "    extracted %d file(s) into %s/\n", len(found), p.Dir)
	return nil
}

// fetchPinned downloads the input's tarball and returns it only if it hashes
// to the pinned sha256.
func fetchPinned(p PinnedInput) ([]byte, error) {
	body, err := download(p.Tarball)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != p.SHA256 {
		return nil, fmt.Errorf("%s@%s sha256 mismatch:\n  want %s\n  got  %s\n"+
			"the pinned tarball changed under us — update its sha256 in\n"+
			"tools/build/common/webref.go only after verifying the new contents",
			p.Package, p.Version, p.SHA256, got)
	}
	return body, nil
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

// extract writes the wanted members of a gzipped tarball into dstDir, each
// under the file name it is mapped to. It returns the set of names actually
// written.
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

		name, ok := wanted[path.Clean(hdr.Name)]
		if !ok {
			continue
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("reading %s from tar: %w", hdr.Name, err)
		}

		dst := filepath.Join(dstDir, name)
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", dst, err)
		}
		found[name] = true
	}

	return found, nil
}
