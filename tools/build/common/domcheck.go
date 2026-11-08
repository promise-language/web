package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/promise-language/forge/primitives/tooling"
)

// The web module is a library whose every declaration is `target(web)`. A host
// build of it has no main to link (promise-language/promise#97), a host test
// run finds no test to run (promise-language/promise#604), and neither would
// measure the module if it could. So this project's Promise toolchain measures
// `builds` and `tested` through the module's check program, DOMCheckDir:
//
//   - builds: the program built for wasm32-web, which compiles the whole
//     generated module for the one target it exists for;
//   - tested: that build run by the pinned Node through the generated glue
//     against a minimal DOM, whose checks judge what the program left in the
//     host's DOM.
//
// This is the definition's "replace the measurement behind a concept or an
// instance" move (forge docs/project-tools.md, The definition): the instances
// are builds:promise and tested:promise, and they report the concepts'
// required metrics.

// DOMCheckDir is the module's check program, relative to the unit it checks:
// a nested module (so it is not compiled into web) under testdata (so it is a
// fixture rather than a unit of its own).
const DOMCheckDir = "testdata/dom"

// webBuilds is the builds measurement: the check program builds for wasm32-web.
func webBuilds(r *tooling.Run, u tooling.Unit) (tooling.UnitResult, error) {
	if _, incomplete, err := buildDOMCheck(r, u); err != nil || incomplete != "" {
		return tooling.UnitResult{Incomplete: incomplete}, err
	}
	return tooling.UnitResult{Counts: []tooling.Tally{{Name: "unbuildable_modules", N: 0}}}, nil
}

// webTested is the tested measurement: the check program's run against the
// minimal DOM, one test per check the driver records.
func webTested(r *tooling.Run, u tooling.Unit) (tooling.UnitResult, error) {
	wasm, incomplete, err := buildDOMCheck(r, u)
	if err != nil || incomplete != "" {
		return tooling.UnitResult{Incomplete: incomplete}, err
	}

	unitDir := r.Abs(u.Dir)
	out, stderr, over, runErr := r.Output(u, "node",
		filepath.Join(unitDir, DOMCheckDir, "driver.mjs"), wasm, filepath.Join(unitDir, "web.js"))
	if runErr != nil {
		return tooling.UnitResult{}, fmt.Errorf("the DOM check driver in %s did not run, so nothing was measured: %w: %s",
			u.Label(), runErr, strings.TrimSpace(stderr))
	}
	records, err := decodeDOMChecks(out)
	if err != nil {
		return tooling.UnitResult{}, fmt.Errorf("the DOM check driver in %s wrote what it does not write: %w", u.Label(), err)
	}

	result := tallyDOMChecks(records)
	narrateFailures(r.Narrate, u.Label(), result.Members)
	if over {
		result.Incomplete = tooling.OverflowReason("node driver.mjs")
	}
	return result, nil
}

// narrateFailures writes each check that did not pass onto the narration, under
// the unit's prefix: its name and outcome, then what it said. The driver's
// stdout is parsed rather than relayed, so nothing else shows a person watching
// which check failed (forge docs/project-tools.md, What the standard toolchains
// measure: each failure is named in the envelope and in the narration).
func narrateFailures(w io.Writer, label string, members []tooling.Member) {
	for _, m := range members {
		fmt.Fprintf(w, "%s: --- %s (%s)\n", label, m.Name, m.Outcome)
		for _, line := range strings.Split(strings.TrimRight(m.Detail, " \t\r\n"), "\n") {
			if line != "" {
				fmt.Fprintf(w, "%s: %s\n", label, line)
			}
		}
	}
}

// buildDOMCheck builds the unit's check program for wasm32-web into scratch
// and returns the module's path. A build that failed is incomplete rather than
// counted: `promise build` states only in prose whether its sources or its
// toolchain failed (promise-language/promise#605).
func buildDOMCheck(r *tooling.Run, u tooling.Unit) (wasm, incomplete string, err error) {
	check := tooling.Unit{Dir: path.Join(u.Dir, DOMCheckDir), Toolchain: u.Toolchain}
	if !Exists(r.Abs(check.Dir)) {
		return "", "", fmt.Errorf("%s has no check program at %s, so neither its build nor its tests can be measured", u.Label(), check.Dir)
	}
	into := r.Scratch("dom-" + u.Label())
	if err := os.MkdirAll(into, 0o755); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", into, err)
	}
	wasm = filepath.Join(into, "main.wasm")

	_, stderr, over, buildErr := r.Output(check, "promise", "build", "-target", "wasm32-web", "-o", wasm)
	var reasons []string
	if over {
		reasons = append(reasons, tooling.OverflowReason("promise build -target wasm32-web"))
	}
	if buildErr != nil {
		reasons = append(reasons, fmt.Sprintf("promise build -target wasm32-web of %s failed (%v), and says only in prose whether its sources or its toolchain did (promise-language/promise#605): %s",
			check.Dir, buildErr, fromRoot(r.Root, firstLine(stderr))))
	}
	if len(reasons) > 0 {
		return "", strings.Join(reasons, "; "), nil
	}
	return wasm, "", nil
}

// fromRoot writes what a line says about a path under root from the repository
// root. The compiler is handed absolute paths and names files by them; a
// reason is written from the repository root, so it does not carry the
// runner's home directory (forge docs/project-tools.md, What the standard
// toolchains measure).
func fromRoot(root, line string) string {
	return strings.ReplaceAll(line, root+string(os.PathSeparator), "")
}

// firstLine is the first line of text that says something, for a reason
// carried verbatim.
func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// domCheckStatus is how one check came out. The set is closed: the driver
// writes "pass", "fail" or "leak", and anything else is refused where it is
// decoded.
type domCheckStatus string

const (
	domCheckPass domCheckStatus = "pass"
	domCheckFail domCheckStatus = "fail"
	domCheckLeak domCheckStatus = "leak"
)

// UnmarshalJSON refuses a status outside the closed set.
func (s *domCheckStatus) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("status %s is not a string", data)
	}
	switch status := domCheckStatus(text); status {
	case domCheckPass, domCheckFail, domCheckLeak:
		*s = status
		return nil
	}
	return fmt.Errorf("status %q is none of pass, fail or leak", text)
}

// domCheckRecord is one line driver.mjs writes on stdout: one check, how it
// came out, and what explains the outcome.
type domCheckRecord struct {
	Check  string         `json:"check"`
	Status domCheckStatus `json:"status"`
	Detail string         `json:"detail"`
}

// decodeDOMChecks reads the driver's stdout: one record per line and nothing
// else. Output with no record at all measured nothing, and is refused.
func decodeDOMChecks(out string) ([]domCheckRecord, error) {
	var records []domCheckRecord
	for i, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if out == "" {
			break
		}
		dec := json.NewDecoder(bytes.NewReader([]byte(line)))
		dec.DisallowUnknownFields()
		var record domCheckRecord
		if err := dec.Decode(&record); err != nil {
			return nil, fmt.Errorf("line %d, %q: %w", i+1, line, err)
		}
		if rest := strings.TrimSpace(line[dec.InputOffset():]); rest != "" {
			return nil, fmt.Errorf("line %d, %q: carries %q after its one record", i+1, line, rest)
		}
		if record.Check == "" || record.Status == "" {
			return nil, fmt.Errorf("line %d, %q: a record names its check and its status", i+1, line)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no check record, so nothing was measured")
	}
	return records, nil
}

// tallyDOMChecks counts the checks as the tested concept's metrics, and names
// each check that did not pass.
func tallyDOMChecks(records []domCheckRecord) tooling.UnitResult {
	var failed, leaked int64
	var members []tooling.Member
	for _, r := range records {
		switch r.Status {
		case domCheckFail:
			failed++
			members = append(members, tooling.Member{Name: r.Check, Outcome: tooling.OutcomeFail, Detail: r.Detail})
		case domCheckLeak:
			leaked++
			members = append(members, tooling.Member{Name: r.Check, Outcome: tooling.OutcomeLeak, Detail: r.Detail})
		}
	}
	return tooling.UnitResult{
		Counts: []tooling.Tally{
			{Name: "failed_tests", N: failed},
			{Name: "leaked_tests", N: leaked},
			{Name: "test_count", N: int64(len(records))},
		},
		Members: members,
	}
}
