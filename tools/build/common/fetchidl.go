package common

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"
)

// FetchIDLInput is what bin/fetch-idl takes: every flag is a field.
type FetchIDLInput struct {
	Force bool `flag:"force" help:"re-fetch even when idl/ is already at the pinned version"`
	List  bool `flag:"list" help:"print the pinned release and spec list, fetch nothing"`
	Clean bool `flag:"clean" help:"remove idl/ and exit"`
}

// Validate refuses the combinations that would mean two things at once.
func (in FetchIDLInput) Validate() []error {
	var errs []error
	if in.List && in.Clean {
		errs = append(errs, fmt.Errorf("-list and -clean contradict each other: -list touches nothing, -clean removes idl/"))
	}
	if in.Force && (in.List || in.Clean) {
		errs = append(errs, fmt.Errorf("-force applies only to a fetch, not to -list or -clean"))
	}
	return errs
}

// Pin is the pinned release and the binding surface: what -list answers.
type Pin struct {
	Version string       `json:"version"`
	SHA256  string       `json:"sha256"`
	Specs   []WebrefSpec `json:"specs"`
}

// Human is the pin, one spec per line.
func (p Pin) Human(w io.Writer) error {
	fmt.Fprintf(w, "@webref/idl@%s\nsha256 %s\n\n", p.Version, p.SHA256)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, s := range p.Specs {
		fmt.Fprintf(tw, "  %s.idl\t%s\n", s.Stem, s.Purpose)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\n%d spec(s). Edit the list in tools/build/common/webref.go.\n", len(p.Specs))
	return err
}

// IDLState is what a fetch or a clean left in idl/.
type IDLState struct {
	Dir     string `json:"dir"`
	Version string `json:"version,omitempty"`
	Specs   int    `json:"specs"`
	// Removed is true when -clean removed idl/, false when it was already absent.
	Removed *bool `json:"removed,omitempty"`
}

// Human is one line saying what idl/ now holds.
func (s IDLState) Human(w io.Writer) error {
	var err error
	switch {
	case s.Removed == nil:
		_, err = fmt.Fprintf(w, "%d spec(s) in %s/ at @webref/idl@%s — not committed, safe to delete (bin/fetch-idl -clean)\n",
			s.Specs, s.Dir, s.Version)
	case *s.Removed:
		_, err = fmt.Fprintf(w, "removed %s/\n", s.Dir)
	default:
		_, err = fmt.Fprintf(w, "%s/ is already absent\n", s.Dir)
	}
	return err
}

// PinnedSpecs reports the pin and the binding surface without touching the
// disk or the network.
func PinnedSpecs() Pin {
	return Pin{Version: WebrefVersion, SHA256: WebrefSHA256, Specs: WebrefSpecs}
}

// RunFetchIDL materializes idl/ from the pinned @webref/idl release.
//
// This is the fetch half of the binding pipeline on its own, for when you want
// to read or grep the WebIDL without regenerating anything — the .idl files are
// not committed, so there is otherwise no copy in the tree. `bin/bindgen` runs
// the same fetch itself; neither is a prerequisite for the other.
func RunFetchIDL(repoRoot string, force bool, narrate io.Writer) (IDLState, error) {
	fmt.Fprintf(narrate, "==> @webref/idl@%s\n", WebrefVersion)
	if err := EnsureIDL(repoRoot, force, narrate); err != nil {
		return IDLState{}, err
	}
	return IDLState{Dir: IDLDir, Version: WebrefVersion, Specs: len(WebrefSpecs)}, nil
}

// CleanIDL removes idl/.
func CleanIDL(repoRoot string) (IDLState, error) {
	idlPath := filepath.Join(repoRoot, IDLDir)
	removed := Exists(idlPath)
	if removed {
		if err := os.RemoveAll(idlPath); err != nil {
			return IDLState{}, fmt.Errorf("removing %s/: %w", IDLDir, err)
		}
	}
	return IDLState{Dir: IDLDir, Removed: &removed}, nil
}
