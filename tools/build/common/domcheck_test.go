package common

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/promise-language/forge/primitives/tooling"
)

func TestTallyDOMChecks(t *testing.T) {
	out := `{"check":"named-root","status":"pass","detail":"ok"}
{"check":"conversion-div-none","status":"fail","detail":"the program noted div=converted, want none"}
{"check":"handles-released","status":"leak","detail":"handles still live after the program: 6"}
{"check":"inheritance-append","status":"pass","detail":"ok"}
`
	records, err := decodeDOMChecks(out)
	if err != nil {
		t.Fatal(err)
	}
	got := tallyDOMChecks(records)

	want := map[string]int64{"failed_tests": 1, "leaked_tests": 1, "test_count": 4}
	if len(got.Counts) != len(want) {
		t.Fatalf("counts = %v, want %v", got.Counts, want)
	}
	for _, c := range got.Counts {
		if c.N != want[c.Name] {
			t.Errorf("%s = %d, want %d", c.Name, c.N, want[c.Name])
		}
	}

	if len(got.Members) != 2 {
		t.Fatalf("members = %v, want the failing and the leaking check only", got.Members)
	}
	if m := got.Members[0]; m.Name != "conversion-div-none" || m.Outcome != tooling.OutcomeFail || !strings.Contains(m.Detail, "want none") {
		t.Errorf("members[0] = %+v", m)
	}
	if m := got.Members[1]; m.Name != "handles-released" || m.Outcome != tooling.OutcomeLeak {
		t.Errorf("members[1] = %+v", m)
	}
}

func TestTallyDOMChecksAllPassing(t *testing.T) {
	records, err := decodeDOMChecks(`{"check":"a","status":"pass","detail":""}` + "\n")
	if err != nil {
		t.Fatal(err)
	}
	got := tallyDOMChecks(records)
	for _, c := range got.Counts {
		if want := map[string]int64{"failed_tests": 0, "leaked_tests": 0, "test_count": 1}[c.Name]; c.N != want {
			t.Errorf("%s = %d, want %d", c.Name, c.N, want)
		}
	}
	if len(got.Members) != 0 {
		t.Errorf("members = %v, want none for an all-passing run", got.Members)
	}
}

// What the driver does not write is refused, never read as a measurement: a
// run that measured nothing must not pass as one that failed nothing.
func TestDecodeDOMChecksRefusesWhatTheDriverDoesNotWrite(t *testing.T) {
	cases := map[string]string{
		"no output":                "",
		"only a newline":           "\n",
		"prose":                    "the program ran\n",
		"unknown status":           `{"check":"a","status":"skipped","detail":""}` + "\n",
		"status not text":          `{"check":"a","status":true,"detail":""}` + "\n",
		"unknown field":            `{"check":"a","status":"pass","detail":"","elapsed":1}` + "\n",
		"no check name":            `{"status":"pass","detail":""}` + "\n",
		"no status":                `{"check":"a","detail":""}` + "\n",
		"a blank line":             `{"check":"a","status":"pass","detail":""}` + "\n\n" + `{"check":"b","status":"pass","detail":""}` + "\n",
		"trailing garbage":         `{"check":"a","status":"pass","detail":""}` + "\n" + "done\n",
		"two records on a line":    `{"check":"a","status":"pass","detail":""}{"check":"b","status":"fail","detail":""}` + "\n",
		"a brace after the record": `{"check":"a","status":"pass","detail":""}}` + "\n",
		"text after the record":    `{"check":"a","status":"pass","detail":""} ok` + "\n",
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			if records, err := decodeDOMChecks(out); err == nil {
				t.Errorf("decodeDOMChecks(%q) = %v, want a refusal", out, records)
			}
		})
	}
}

// A record may be followed on its line by nothing but whitespace.
func TestDecodeDOMChecksAcceptsTrailingWhitespace(t *testing.T) {
	records, err := decodeDOMChecks(`{"check":"a","status":"pass","detail":""}  ` + "\r\n")
	if err != nil || len(records) != 1 {
		t.Fatalf("decodeDOMChecks = %v, %v, want the one record", records, err)
	}
}

// Every check that did not pass is named on the narration, under the unit's
// prefix, with what it said; a check that passed is not.
func TestNarrateFailures(t *testing.T) {
	records, err := decodeDOMChecks(`{"check":"named-root","status":"pass","detail":"ok"}
{"check":"conversion-div-none","status":"fail","detail":"the program noted div=converted\nwant none"}
{"check":"handles-released","status":"leak","detail":"handles never released: 6"}
`)
	if err != nil {
		t.Fatal(err)
	}
	var narration strings.Builder
	narrateFailures(&narration, "root", tallyDOMChecks(records).Members)

	want := "root: --- conversion-div-none (fail)\n" +
		"root: the program noted div=converted\n" +
		"root: want none\n" +
		"root: --- handles-released (leak)\n" +
		"root: handles never released: 6\n"
	if got := narration.String(); got != want {
		t.Errorf("narration =\n%s\nwant\n%s", got, want)
	}
}

// A reason names files from the repository root, never by the runner's
// absolute path.
func TestFromRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"home", "someone", "web")
	line := filepath.Join(root, "testdata", "dom", "main.pr") + ":39:4: type HTMLInputElement has no field or method value"
	want := filepath.Join("testdata", "dom", "main.pr") + ":39:4: type HTMLInputElement has no field or method value"
	if got := fromRoot(root, line); got != want {
		t.Errorf("fromRoot = %q, want %q", got, want)
	}
	if got := fromRoot(root, "error: no main"); got != "error: no main" {
		t.Errorf("fromRoot changed a line naming no path: %q", got)
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("\n  \nerror: no main\nmore\n"); got != "error: no main" {
		t.Errorf("firstLine = %q", got)
	}
	if got := firstLine(""); got != "" {
		t.Errorf("firstLine of nothing = %q", got)
	}
}
