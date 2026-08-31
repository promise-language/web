// Command bindgen regenerates web.pr and web.js from the pinned WebIDL
// definitions.
//
// Usage: bin/bindgen [-refresh] [-clean]
//
// The WebIDL inputs are fetched into idl/ (gitignored) at the revision pinned
// in tools/build/common/webref.go, then handed to `promise bind webidl`. Both
// the spec list and the pin live in that one file.
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
	if err := common.RunBindgen(repoRoot, common.NormalizeArgs(os.Args[1:])); err != nil {
		fmt.Fprintln(os.Stderr, "bindgen failed:", err)
		os.Exit(1)
	}
}
