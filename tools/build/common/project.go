// Package common holds this project's tooling definition and nothing the
// library carries.
//
// The machinery — the invocation surface, the envelope and the listings, the
// judge's comparison, the ratchet and the verified-tree record, the staleness
// check, the confinement of what a tool writes — is
// github.com/promise-language/forge/primitives/tooling, at the version
// tools/build/go.mod pins. What lives here is the part with more than one right
// answer: which gates this project answers, what verify runs, and what each is
// judged by.
package common

import "github.com/promise-language/forge/primitives/tooling"

// Define is this project's tooling.
//
// The standard definition measures and repairs a Go project without a line of
// customization. It carries both toolchains, integration composed of
// formatted, builds, checked and tested, fit divided into fit:disk and
// fit:toolchain, the standard verify, and setup's checks. A toolchain with no
// units in this repository contributes nothing, so a project in one language
// pays nothing for the other.
//
// This is a Promise project, so it also pins the promise it runs, because forge
// cannot choose one version for every project: see promise below.
//
// Shape it here and nowhere else. The definition admits four moves — add a gate
// or a verify step, replace a standard step or the measurement behind a
// concept, compose what integration is made of, remove a concept this project
// does not have:
//
//	func Define() tooling.Project {
//		p := tooling.Standard()
//		p.Gates.Add(tooling.Gate{Name: "size", Summary: "bytes in the release binary"})
//		p.Integration(tooling.Formatted, tooling.Builds, tooling.Checked, tooling.Tested, "size")
//		return p
//	}
func Define() tooling.Project {
	p := tooling.Standard()
	p.Require("promise", promise)
	return p
}

// promise is the compiler the promise toolchain runs over this repository's
// one unit, the root promise.toml. The thin build is the one install.sh fetches
// by default; its digests are the release's SHA256SUMS. Moving to a newer epoch
// is an edit here, Version, Tag and every digest together.
var promise = tooling.Dependency{
	Name:    "promise",
	Version: "2026.10",
	Source:  tooling.GitHubRelease{Repository: "promise-language/promise", Tag: "epoch-2026.10"},
	Platforms: map[string]tooling.Asset{
		"darwin/arm64":  {Name: "promise-darwin-arm64.gz", Format: tooling.Gzip, SHA256: "de286ae2d32e86b74812dd9fd3b8d8fd8be94a62e90e790c0ab58d1f28d516c3"},
		"linux/amd64":   {Name: "promise-linux-amd64.gz", Format: tooling.Gzip, SHA256: "7dec9ed47fb6ac92186a360a01d5201bc04062563f3b65193bbdfbc8b02898ba"},
		"linux/arm64":   {Name: "promise-linux-arm64.gz", Format: tooling.Gzip, SHA256: "6465a132704739921f5a530414316f571f1a0df34341a9331e4dafee75c5ce24"},
		"windows/amd64": {Name: "promise-windows-amd64.exe.gz", Format: tooling.Gzip, SHA256: "73fad9157387b6f7f18c2028bcc084015f048ee513e692a58ad739e65f02498e"},
	},
}
