package common

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
)

// RunFetchIDL materializes idl/ from the pinned @webref/idl release.
//
// This is the fetch half of the binding pipeline on its own, for when you want
// to read or grep the WebIDL without regenerating anything — the .idl files are
// not committed, so there is otherwise no copy in the tree. `bin/bindgen` runs
// the same fetch itself; neither is a prerequisite for the other.
//
// Flags:
//
//	-force   re-fetch even when idl/ is already at the pinned version
//	-list    print the pinned release and spec list, fetch nothing
//	-clean   remove idl/ and exit
func RunFetchIDL(repoRoot string, args []string) error {
	force := false
	list := false
	clean := false
	for _, a := range args {
		switch a {
		case "-force":
			force = true
		case "-list":
			list = true
		case "-clean":
			clean = true
		default:
			return fmt.Errorf("unknown flag: %s\nusage: bin/fetch-idl [-force] [-list] [-clean]", a)
		}
	}

	if list {
		printSpecs()
		return nil
	}

	idlPath := filepath.Join(repoRoot, IDLDir)

	if clean {
		if !Exists(idlPath) {
			fmt.Printf("%s/ is already absent\n", IDLDir)
			return nil
		}
		if err := os.RemoveAll(idlPath); err != nil {
			return fmt.Errorf("removing %s/: %w", IDLDir, err)
		}
		fmt.Printf("removed %s/\n", IDLDir)
		return nil
	}

	fmt.Printf("==> @webref/idl@%s\n", WebrefVersion)
	if err := EnsureIDL(repoRoot, force); err != nil {
		return err
	}

	fmt.Printf("\n%d spec(s) in %s/ — not committed, safe to delete (bin/fetch-idl -clean)\n",
		len(WebrefSpecs), IDLDir)
	return nil
}

// printSpecs reports the pin and the binding surface without touching the disk
// or the network.
func printSpecs() {
	fmt.Printf("@webref/idl@%s\n", WebrefVersion)
	fmt.Printf("sha256 %s\n\n", WebrefSHA256)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, s := range WebrefSpecs {
		fmt.Fprintf(w, "  %s.idl\t%s\n", s.Stem, s.Purpose)
	}
	w.Flush()

	fmt.Printf("\n%d spec(s). Edit the list in tools/build/common/webref.go.\n", len(WebrefSpecs))
}
