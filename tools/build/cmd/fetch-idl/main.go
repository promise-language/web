// Command fetch-idl materializes idl/ from the pinned @webref/idl release.
//
// Usage: bin/fetch-idl [-force] [-list] [-clean]
//
// The WebIDL inputs are not committed, so this is how you get a readable copy
// in the tree. `bin/bindgen` performs the same fetch on its own — reach for
// this one to inspect the definitions, or to check what the pin covers
// (-list) without touching the network.
//
// It is a project command on forge's command library: parsing, help, the
// output modes, the staleness refusal and the exit status are the library's.
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
			Name:    "fetch-idl",
			Summary: "materialize idl/ at the pinned @webref/idl release",
			Operation: command.Operation[common.FetchIDLInput, command.Result]{
				Validate: func(_ *command.Call, in common.FetchIDLInput) []error {
					return in.Validate()
				},
				Run: func(c *command.Call, in common.FetchIDLInput) (command.Result, error) {
					switch {
					case in.List:
						return common.PinnedSpecs(), nil
					case in.Clean:
						return common.CleanIDL(root)
					default:
						return common.RunFetchIDL(root, in.Force, c.Narrate)
					}
				},
			},
		},
	}
}
