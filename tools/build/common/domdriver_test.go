package common

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/promise-language/forge/primitives/tooling"
)

// The DOM check driver is the oracle the tested measurement reads, so a check
// that cannot fail would make the measurement pass whatever the bindings do.
// These tests run the driver, under the pinned node, against stand-in glue
// modules whose program is the check program's own moves written in JS: one
// faithful, and one per defect a check exists to catch. The real glue and the
// built program are what the tested measurement runs; here only the driver's
// judgement is under test.

// standInGlue is a glue module in the shape driver.mjs reaches: init runs the
// program against the driver's DOM, and _refStore and _refLoad reach a handle
// table seeded like the real one, with the three ambient references in slots
// 1 to 3. The
// program makes the check program's moves, each handle stored and released as
// the generated wrappers do; defect, a JSON object, names what it does wrong.
const standInGlue = `
const refs = [undefined, globalThis, globalThis.document, globalThis.console];
const free = [];
export function _refStore(obj) {
  if (free.length > 0) {
    const handle = free.pop();
    refs[handle] = obj;
    return handle;
  }
  refs.push(obj);
  return refs.length - 1;
}
export function _refLoad(handle) {
  return refs[handle];
}
function release(handle) {
  if (handle <= 3) return;
  refs[handle] = undefined;
  free.push(handle);
}

const defect = DEFECT;

export async function init(wasmPath) {
  const doc = document;
  const hold = (obj) => _refStore(obj);

  const bodyBefore = defect.bodyReadPresent || doc.body !== null ? "present" : "absent";
  const documentText = doc.textContent !== null ? "present" : "absent";

  const page = doc.createElement("body");
  const pageHandle = hold(page);
  if (!defect.bodyNotAssigned) doc.body = page;
  const bodyAfter = doc.body !== null ? "present" : "absent";

  const input = doc.createElement("input");
  const inputHandle = hold(input);
  if (!defect.valueNotSet) input.value = TYPED;
  const div = doc.createElement("div");
  const divHandle = hold(div);
  const divNote = defect.divConverts || div instanceof HTMLInputElement ? "converted" : "none";
  release(divHandle);

  if (!defect.inputNotAppended) page.appendChild(input);
  const heading = doc.createElement("h1");
  const headingHandle = hold(heading);
  heading.textContent = "hello";
  page.appendChild(heading);

  // An unreleased JsValue temporary: a slot that holds undefined.
  if (defect.undefinedSlotLeaks) hold(undefined);

  const note = (text) => {
    const p = doc.createElement("p");
    const handle = hold(p);
    p.textContent = text;
    page.appendChild(p);
    release(handle);
  };
  note("body-before=" + bodyBefore);
  note("document-text=" + documentText);
  note("body-after=" + bodyAfter);
  note("value=" + input.value);
  note("div=" + divNote);
  note("heading-text=" + heading.textContent);

  release(headingHandle);
  release(inputHandle);
  if (defect.releasedTwice) release(inputHandle);
  release(pageHandle);

  if (defect.throws) throw new Error("the program panicked");
}
`

// typedValue is the value driver.mjs expects the program to write into the
// input; the stand-in program writes it too.
const typedValue = `typed ✓ "quoted"`

// runDriver runs testdata/dom/driver.mjs under the pinned node against a
// stand-in glue with the given defect, and returns each check's status. The
// pinned node is resolved from the checkout's record as the gates resolve it,
// and its absence is a failure naming the repair, never a skip.
func runDriver(t *testing.T, defect map[string]bool) map[string]domCheckStatus {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	program, err := tooling.Resolve(root, node, tooling.HostTarget())
	if err != nil {
		t.Fatalf("the DOM check driver runs under the pinned node: %v", err)
	}

	if defect == nil {
		defect = map[string]bool{}
	}
	encoded, err := json.Marshal(defect)
	if err != nil {
		t.Fatal(err)
	}
	quoted, err := json.Marshal(typedValue)
	if err != nil {
		t.Fatal(err)
	}
	glue := strings.NewReplacer("DEFECT", string(encoded), "TYPED", string(quoted)).Replace(standInGlue)
	dir := t.TempDir()
	gluePath := writeFile(t, dir, "glue.mjs", glue)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(program, filepath.Join(root, DOMCheckDir, "driver.mjs"), filepath.Join(dir, "main.wasm"), gluePath)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("driver.mjs did not run: %v\n%s", err, stderr.String())
	}
	records, err := decodeDOMChecks(stdout.String())
	if err != nil {
		t.Fatalf("driver.mjs wrote what the measurement cannot read: %v", err)
	}
	statuses := map[string]domCheckStatus{}
	for _, r := range records {
		if _, twice := statuses[r.Check]; twice {
			t.Fatalf("driver.mjs recorded %s twice", r.Check)
		}
		statuses[r.Check] = r.Status
	}
	return statuses
}

// domChecks are the checks driver.mjs records, one per behaviour the item asks
// the tests to verify and one each for the program's end and its handles.
var domChecks = []string{
	"program",
	"named-root",
	"nullable-body-absent", "nullable-body-present", "nullable-text-absent",
	"conversion-input-host", "conversion-input-read", "conversion-div-none",
	"inheritance-append", "inheritance-text-host", "inheritance-text-read",
	"handles-released",
}

// A program that does everything the check program does passes every check.
func TestTheDriverPassesAFaithfulProgram(t *testing.T) {
	got := runDriver(t, nil)
	if len(got) != len(domChecks) {
		t.Errorf("driver.mjs recorded %d checks, want %d: %v", len(got), len(domChecks), got)
	}
	for _, check := range domChecks {
		if got[check] != domCheckPass {
			t.Errorf("%s = %q for a faithful program, want pass", check, got[check])
		}
	}
}

// Each defect is caught by the check that exists for it.
func TestTheDriverCatchesEachDefect(t *testing.T) {
	cases := []struct {
		defect string
		check  string
		want   domCheckStatus
	}{
		{"throws", "program", domCheckFail},
		{"bodyNotAssigned", "named-root", domCheckFail},
		{"bodyReadPresent", "nullable-body-absent", domCheckFail},
		{"valueNotSet", "conversion-input-host", domCheckFail},
		{"divConverts", "conversion-div-none", domCheckFail},
		{"inputNotAppended", "inheritance-append", domCheckFail},
		// A slot holding undefined reads the same as a free one, so only a
		// check that reuses the slots can see this leak.
		{"undefinedSlotLeaks", "handles-released", domCheckLeak},
		{"releasedTwice", "handles-released", domCheckFail},
	}
	for _, c := range cases {
		t.Run(c.defect, func(t *testing.T) {
			t.Parallel()
			got := runDriver(t, map[string]bool{c.defect: true})
			if got[c.check] != c.want {
				t.Errorf("%s = %q for a program that %s, want %s (all: %v)", c.check, got[c.check], c.defect, c.want, got)
			}
		})
	}
}
