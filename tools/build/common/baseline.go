package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// KeepList is the app keep list, relative to the repo root: the interfaces and
// members `promise bind webidl -keep` generates, one entry per line. What it
// covers is recorded in the "Coverage" section of README.md.
const KeepList = "web.keep"

// The keep list's format is the compiler's (`promise bind webidl -keep`,
// promise-language/promise docs/wasm-bindings.md §"Generating a subset"): one
// `Interface` or `Interface.member` per line, surrounding whitespace trimmed and
// blank lines skipped. ReadKeepList reads it for the Baseline check only; the
// compiler reads it again for generation, which is where the format is owned.

// ReadKeepList returns the keep list's entries in file order.
func ReadKeepList(path string) ([]string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the keep list: %w", err)
	}
	var entries []string
	for _, line := range strings.Split(string(src), "\n") {
		if entry := strings.TrimSpace(line); entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// CompatKeys are the browser-compat-data keys a keep-list entry may stand for,
// by #2's mapping read backwards: an interface I is api.I, its constructor
// api.I.I, and a member m either api.I.m or, for a static member,
// api.I.m_static.
func CompatKeys(entry string) []string {
	iface, member, isMember := strings.Cut(entry, ".")
	switch {
	case !isMember:
		return []string{"api." + iface}
	case member == "constructor":
		return []string{"api." + iface + "." + iface}
	default:
		return []string{"api." + iface + "." + member, "api." + iface + "." + member + "_static"}
	}
}

// Baseline is a compat key's Baseline status in web-features. The set is
// closed: web-features writes "high", "low" or false, and anything else is
// refused where it is decoded.
type Baseline int

const (
	// NotBaseline is web-features' false: some core browser lacks it.
	NotBaseline Baseline = iota
	// BaselineLow is newly available across the core browsers.
	BaselineLow
	// BaselineHigh is widely available across the core browsers.
	BaselineHigh
)

// UnmarshalJSON reads web-features' "high", "low" or false.
func (b *Baseline) UnmarshalJSON(data []byte) error {
	switch string(bytes.TrimSpace(data)) {
	case `"high"`:
		*b = BaselineHigh
	case `"low"`:
		*b = BaselineLow
	case `false`:
		*b = NotBaseline
	default:
		return fmt.Errorf("baseline status %s is none of \"high\", \"low\" or false", data)
	}
	return nil
}

// baselineData is the part of web-features' data.json the check reads: each
// feature's status per compat key. A moved or split feature carries no status
// and so contributes no key.
type baselineData struct {
	Features map[string]struct {
		Status *struct {
			ByCompatKey map[string]struct {
				Baseline Baseline `json:"baseline"`
			} `json:"by_compat_key"`
		} `json:"status"`
	} `json:"features"`
}

// ReadBaseline decodes the pinned web-features data into each compat key's
// Baseline status. A key two features both report must agree, or the data
// answers nothing about it and is refused.
func ReadBaseline(path string) (map[string]Baseline, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the Baseline data: %w", err)
	}
	var data baselineData
	if err := json.Unmarshal(src, &data); err != nil {
		return nil, fmt.Errorf("decoding the Baseline data %s: %w", path, err)
	}
	if len(data.Features) == 0 {
		return nil, fmt.Errorf("the Baseline data %s names no features", path)
	}

	statuses := map[string]Baseline{}
	var conflicts []error
	for id, feature := range data.Features {
		if feature.Status == nil {
			continue
		}
		for key, status := range feature.Status.ByCompatKey {
			if prior, seen := statuses[key]; seen && prior != status.Baseline {
				conflicts = append(conflicts, fmt.Errorf("compat key %s has two Baseline statuses, one from feature %s", key, id))
				continue
			}
			statuses[key] = status.Baseline
		}
	}
	if len(conflicts) > 0 {
		return nil, errors.Join(conflicts...)
	}
	return statuses, nil
}

// CheckKeepList refuses a keep list that names anything the pinned
// web-features release does not report Baseline, low or high. Every entry that
// fails is reported, not only the first.
func CheckKeepList(keepPath, dataPath string) error {
	entries, err := ReadKeepList(keepPath)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("the keep list %s names nothing", keepPath)
	}
	statuses, err := ReadBaseline(dataPath)
	if err != nil {
		return err
	}

	var problems []error
	for _, entry := range entries {
		if err := entryIsBaseline(entry, statuses); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

// entryIsBaseline is nil when the first compat key the entry stands for that
// web-features reports is Baseline.
func entryIsBaseline(entry string, statuses map[string]Baseline) error {
	keys := CompatKeys(entry)
	for _, key := range keys {
		status, reported := statuses[key]
		if !reported {
			continue
		}
		if status == NotBaseline {
			return fmt.Errorf("keep list entry %s: web-features@%s reports %s as not Baseline", entry, WebFeaturesVersion, key)
		}
		return nil
	}
	return fmt.Errorf("keep list entry %s: web-features@%s reports no Baseline status for %s", entry, WebFeaturesVersion, strings.Join(keys, " or "))
}
