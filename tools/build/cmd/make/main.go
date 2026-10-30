// Command make is the meta-builder. It compiles every other tool under cmd/
// into <root>/bin, stamping each binary with the root, the source hash and the
// source set. It is the one tool the ./make trampoline compiles fresh on every
// run, into scratch rather than bin/, so it is never stale, which is what breaks
// the bootstrap cycle.
//
// It is also the recovery every refusal names, which is why it is the one main
// without a stamp: a builder that could refuse on the ground every other
// refusal names would close the way out.
package main

import (
	"os"

	"github.com/promise-language/forge/primitives/tooling"

	"github.com/promise-language/web/tools/build/common"
)

func main() {
	os.Exit(tooling.MakeTool(common.Define()).Run(os.Args[1:]))
}
