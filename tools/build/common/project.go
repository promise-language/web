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
	p.Require("promise", node)
	// The tools' own tests run the DOM check driver under the same node
	// (domdriver_test.go), so the Go toolchain declares it too.
	p.Require("go", node)
	for i := range p.Toolchains {
		switch p.Toolchains[i].Name {
		case tooling.Promise().Name:
			// The web module builds and tests for wasm32-web through its check
			// program (domcheck.go), which the pinned node runs.
			p.Toolchains[i].Needs = append(p.Toolchains[i].Needs, node.Name)
			p.Toolchains[i].Measure[tooling.Builds] = webBuilds
			p.Toolchains[i].Measure[tooling.Tested] = webTested
		case tooling.Go().Name:
			p.Toolchains[i].Needs = append(p.Toolchains[i].Needs, node.Name)
		}
	}
	return p
}

// node runs the web module's DOM check (testdata/dom/driver.mjs): 22.22.2 is
// the Node the Promise compiler's own wasm32-web harness pins. Its digests are
// the release's SHASUMS256.txt.
var node = tooling.Dependency{
	Name:    "node",
	Version: "22.22.2",
	Source:  tooling.NodejsDist{},
	Platforms: map[string]tooling.Asset{
		"darwin/arm64":  {Name: "node-v22.22.2-darwin-arm64.tar.gz", Format: tooling.TarGz, Program: "node-v22.22.2-darwin-arm64/bin/node", SHA256: "db4b275b83736df67533529a18cc55de2549a8329ace6c7bcc68f8d22d3c9000"},
		"linux/amd64":   {Name: "node-v22.22.2-linux-x64.tar.gz", Format: tooling.TarGz, Program: "node-v22.22.2-linux-x64/bin/node", SHA256: "978978a635eef872fa68beae09f0aad0bbbae6757e444da80b570964a97e62a3"},
		"linux/arm64":   {Name: "node-v22.22.2-linux-arm64.tar.gz", Format: tooling.TarGz, Program: "node-v22.22.2-linux-arm64/bin/node", SHA256: "b2f3a96f31486bfc365192ad65ced14833ad2a3c2e1bcefec4846902f264fa28"},
		"windows/amd64": {Name: "node-v22.22.2-win-x64.zip", Format: tooling.Zip, Program: "node-v22.22.2-win-x64/node.exe", SHA256: "7c93e9d92bf68c07182b471aa187e35ee6cd08ef0f24ab060dfff605fcc1c57c"},
	},
}

// promise is the compiler the promise toolchain runs over this repository's
// one unit, the root promise.toml. The thin build is the one install.sh fetches
// by default; its digests are the release's SHA256SUMS. Moving to a newer epoch
// is an edit here, Version, Tag and every digest together.
var promise = tooling.Dependency{
	Name:    "promise",
	Version: "2026.13",
	Source:  tooling.GitHubRelease{Repository: "promise-language/promise", Tag: "epoch-2026.13"},
	Platforms: map[string]tooling.Asset{
		"darwin/arm64":  {Name: "promise-darwin-arm64.gz", Format: tooling.Gzip, SHA256: "de6ce940381d7669cc63361e470a29164ebb34b53fb2147e497ee66ef6ff74ca"},
		"linux/amd64":   {Name: "promise-linux-amd64.gz", Format: tooling.Gzip, SHA256: "79a8b654bb54d2a4e3dd637b9631ced0a833ddefefce8f48ca77ae54f97f04a2"},
		"linux/arm64":   {Name: "promise-linux-arm64.gz", Format: tooling.Gzip, SHA256: "5ce8270e2115111c7e0539802166107b778ed46c5047c821d4aed23c867e23dd"},
		"windows/amd64": {Name: "promise-windows-amd64.exe.gz", Format: tooling.Gzip, SHA256: "8638258fd468c5eb644fa325ab7cbe7faf00ab3a0e7b868096128e19f720226e"},
	},
}
