package common

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCompatKeys(t *testing.T) {
	cases := []struct {
		entry string
		want  []string
	}{
		{"Document", []string{"api.Document"}},
		{"Document.body", []string{"api.Document.body", "api.Document.body_static"}},
		{"URL.constructor", []string{"api.URL.URL"}},
		{"URL.canParse", []string{"api.URL.canParse", "api.URL.canParse_static"}},
	}
	for _, c := range cases {
		t.Run(c.entry, func(t *testing.T) {
			if got := CompatKeys(c.entry); !slices.Equal(got, c.want) {
				t.Errorf("CompatKeys(%q) = %v, want %v", c.entry, got, c.want)
			}
		})
	}
}

// writeFile writes body to name under dir and returns its path.
func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// baselineFixture is web-features data in the shape data.json has: one
// feature per status, a moved feature with no status, and a static member.
const baselineFixture = `{
  "browsers": {},
  "features": {
    "dom": {"kind": "feature", "status": {"baseline": "high", "support": {}, "by_compat_key": {
      "api.Document": {"baseline": "high", "support": {}},
      "api.Document.body": {"baseline": "high", "support": {}}
    }}},
    "url-canparse": {"kind": "feature", "status": {"baseline": "low", "support": {}, "by_compat_key": {
      "api.URL.canParse_static": {"baseline": "low", "support": {}},
      "api.URL.URL": {"baseline": "low", "support": {}}
    }}},
    "experimental": {"kind": "feature", "status": {"baseline": false, "support": {}, "by_compat_key": {
      "api.Document.caretPositionFromPoint": {"baseline": false, "support": {}}
    }}},
    "old-name": {"kind": "moved", "redirect_target": "dom"}
  }
}`

func TestCheckKeepList(t *testing.T) {
	dir := t.TempDir()
	data := writeFile(t, dir, "data.json", baselineFixture)

	t.Run("high, low and a static member are accepted", func(t *testing.T) {
		keep := writeFile(t, dir, "ok.keep", "Document\n  Document.body  \n\nURL.constructor\nURL.canParse\n")
		if err := CheckKeepList(keep, data); err != nil {
			t.Errorf("CheckKeepList = %v, want nil", err)
		}
	})

	t.Run("every entry that is not Baseline is reported", func(t *testing.T) {
		keep := writeFile(t, dir, "bad.keep", "Document\nDocument.caretPositionFromPoint\nNoSuchInterface\nDocument.noSuchMember\n")
		err := CheckKeepList(keep, data)
		if err == nil {
			t.Fatal("accepted a keep list naming non-Baseline entries")
		}
		for _, entry := range []string{"Document.caretPositionFromPoint", "NoSuchInterface", "Document.noSuchMember"} {
			if !strings.Contains(err.Error(), "keep list entry "+entry+":") {
				t.Errorf("the refusal does not report %s: %v", entry, err)
			}
		}
		if strings.Contains(err.Error(), "keep list entry Document:") {
			t.Errorf("the refusal reports Document, which is Baseline: %v", err)
		}
	})

	t.Run("an empty keep list is refused", func(t *testing.T) {
		keep := writeFile(t, dir, "empty.keep", "\n  \n")
		if err := CheckKeepList(keep, data); err == nil {
			t.Error("accepted a keep list that names nothing")
		}
	})

	t.Run("a missing keep list is refused", func(t *testing.T) {
		if err := CheckKeepList(filepath.Join(dir, "absent.keep"), data); err == nil {
			t.Error("accepted a keep list that does not exist")
		}
	})
}

// A compat key two features report alike is read, not refused: the pinned
// web-features lists some keys under more than one feature (api.HTMLMediaElement
// under both audio and video), so only a disagreement answers nothing.
func TestReadBaselineAcceptsAKeyTwoFeaturesReportAlike(t *testing.T) {
	path := writeFile(t, t.TempDir(), "data.json", `{"features": {
		"audio": {"status": {"by_compat_key": {"api.HTMLMediaElement": {"baseline": "low"}}}},
		"video": {"status": {"by_compat_key": {"api.HTMLMediaElement": {"baseline": "low"}}}}}}`)
	got, err := ReadBaseline(path)
	if err != nil {
		t.Fatalf("ReadBaseline refused a key two features report alike: %v", err)
	}
	if got["api.HTMLMediaElement"] != BaselineLow {
		t.Errorf("api.HTMLMediaElement = %v, want BaselineLow", got["api.HTMLMediaElement"])
	}
}

// The Baseline data is third-party: anything it states outside the closed set
// of statuses, or that is not its shape at all, is refused rather than read
// as some status.
func TestReadBaselineRefusesWhatItCannotRead(t *testing.T) {
	cases := map[string]string{
		"unknown status":   `{"features": {"f": {"status": {"by_compat_key": {"api.A": {"baseline": "medium"}}}}}}`,
		"status not a str": `{"features": {"f": {"status": {"by_compat_key": {"api.A": {"baseline": true}}}}}}`,
		"not JSON":         `features: none`,
		"no features":      `{"features": {}}`,
		"two statuses for one key": `{"features": {
			"f": {"status": {"by_compat_key": {"api.A": {"baseline": "high"}}}},
			"g": {"status": {"by_compat_key": {"api.A": {"baseline": false}}}}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeFile(t, t.TempDir(), "data.json", body)
			if _, err := ReadBaseline(path); err == nil {
				t.Errorf("ReadBaseline accepted %s", body)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		if _, err := ReadBaseline(filepath.Join(t.TempDir(), "data.json")); err == nil {
			t.Error("ReadBaseline accepted a file that does not exist")
		}
	})
}
