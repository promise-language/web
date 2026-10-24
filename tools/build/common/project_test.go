package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/promise-language/forge/primitives/tooling"
)

// A defect in the definition fails this gate rather than the first invocation
// that reaches one of the five tools.
func TestTheDefinitionHasNoDefects(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "tools", "build", "cmd")); err != nil {
		t.Skipf("not running in a checkout: %v", err)
	}
	// The real build set, so the check that no gate name is also a command's
	// is made against the names this project actually builds.
	commands, err := tooling.BuildSet(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, defect := range tooling.Check(Define(), commands) {
		t.Errorf("this project's definition: %v", defect)
	}
	// Derived instance names come from the tree, so two of them that coincide
	// are found in the checkout rather than in the definition. Before git init
	// git tracks nothing, so there are no units yet to derive names from.
	coinciding, err := tooling.CheckUnits(Define(), root)
	if err != nil {
		t.Skipf("no units to check until git tracks this tree: %v", err)
	}
	for _, defect := range coinciding {
		t.Errorf("this project's units: %v", defect)
	}
}
