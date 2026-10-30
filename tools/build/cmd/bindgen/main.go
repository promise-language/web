// Command bindgen regenerates web.pr and web.js from the pinned WebIDL
// definitions.
//
// Usage: bin/bindgen [-force] [-clean]
//
// The WebIDL inputs are fetched into idl/ (gitignored) at the revision pinned
// in tools/build/common/webref.go, then handed to `promise bind webidl`. Both
// the spec list and the pin live in that one file.
//
// It is a project command on forge's command library: parsing, help, the
// output modes, the staleness refusal and the exit status are the library's,
// and what it does is common.RunBindgen.
package main

import (
	"os"

	"github.com/promise-language/forge/primitives/command"
	"github.com/promise-language/forge/primitives/tooling"

	"github.com/promise-language/web/tools/build/common"
)

// stamp is written at link time by ./make.
var stamp string

func main() {
	os.Exit(command.Run(tool(stamp), os.Args[1:], command.Stdio()))
}

func tool(stamp string) command.Tool {
	root := tooling.StampedRoot(stamp)
	version := ""
	if s, ok := tooling.DecodeStamp(stamp); ok {
		version = s.Hash
	}
	return command.Tool{
		Project: "web",
		Version: version,
		Fit:     tooling.MayAct(stamp),
		Root: command.Command{
			Name:    "bindgen",
			Summary: "regenerate web.pr and web.js from the pinned WebIDL",
			Operation: command.Operation[common.BindgenInput, common.Bindings]{
				Run: func(c *command.Call, in common.BindgenInput) (common.Bindings, error) {
					return common.RunBindgen(c.Context(), root, in, c.Narrate)
				},
			},
		},
	}
}
