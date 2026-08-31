// Command fetch-idl materializes idl/ from the pinned @webref/idl release.
//
// Usage: bin/fetch-idl [-force] [-list] [-clean]
//
// The WebIDL inputs are not committed, so this is how you get a readable copy
// in the tree. `bin/bindgen` performs the same fetch on its own — reach for
// this one to inspect the definitions, or to check what the pin covers
// (-list) without touching the network.
package main

import (
	"fmt"
	"os"

	"github.com/promise-language/web/tools/build/common"
)

// Injected by the meta-builder via -ldflags at build time; empty otherwise.
var (
	repoRoot   = ""
	sourceHash = ""
)

func main() {
	common.CheckStale(repoRoot, sourceHash)
	if err := common.RunFetchIDL(repoRoot, common.NormalizeArgs(os.Args[1:])); err != nil {
		fmt.Fprintln(os.Stderr, "fetch-idl failed:", err)
		os.Exit(1)
	}
}
