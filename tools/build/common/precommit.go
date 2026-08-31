package common

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// RunPrecommit enforces fast, test-free invariants before a commit. It is the
// body of bin/precommit, which .githooks/pre-commit execs. Keep it sub-second:
// anything that needs to run tests belongs in bin/verify, which the developer
// runs explicitly — putting tests here makes commits slow and gets the hook
// disabled, which kills the whole gate.
//
// The starter checks are language-agnostic and apply to any repo:
//  1. no staged built binaries under bin/ (gitignored, built by ./make),
//  2. author and committer identities are GitHub noreply addresses,
//  3. no binary blobs or oversized files anywhere in the tree — source only.
//
// Grow it from here with project-specific checks: forbidden-pattern scans and
// ratcheted-baseline (.baselines.json) validation.
func RunPrecommit(repoRoot string) error {
	if err := checkNoStagedBinaries(repoRoot); err != nil {
		return err
	}
	if err := checkNoreplyIdentity(repoRoot); err != nil {
		return err
	}
	if err := checkNoBinaryBlobs(repoRoot); err != nil {
		return err
	}
	return nil
}

func checkNoStagedBinaries(repoRoot string) error {
	staged, err := RunOutputIn(repoRoot, "git", "diff", "--cached", "--name-only")
	if err != nil {
		return fmt.Errorf("listing staged files: %w", err)
	}
	var offenders []string
	for _, line := range strings.Split(staged, "\n") {
		f := strings.TrimSpace(line)
		if f == "bin" || strings.HasPrefix(f, "bin/") {
			offenders = append(offenders, f)
		}
	}
	if len(offenders) > 0 {
		fmt.Fprintln(os.Stderr, "❌ pre-commit: refusing to commit built binaries:")
		for _, f := range offenders {
			fmt.Fprintf(os.Stderr, "  %s\n", f)
		}
		fmt.Fprintf(os.Stderr, "bin/ is gitignored and built by ./make. Unstage with:\n  git reset HEAD %s\n",
			strings.Join(offenders, " "))
		return fmt.Errorf("%d staged binary path(s)", len(offenders))
	}
	return nil
}

// noreplyDomain is the only email domain permitted for commit identities. Using
// a GitHub noreply address keeps a personal email out of the public history.
// This is the one project-policy knob in the starter checks — change it if your
// project uses a different identity convention.
const noreplyDomain = "@users.noreply.github.com"

// checkNoreplyIdentity refuses the commit unless both the author and committer
// emails are GitHub noreply addresses. It reads the identities via 'git var',
// which resolves them exactly as the impending commit will — honoring
// GIT_AUTHOR_EMAIL / GIT_COMMITTER_EMAIL env vars and user.email config alike —
// so the check matches what would actually be recorded.
func checkNoreplyIdentity(repoRoot string) error {
	roles := []struct{ label, gitVar string }{
		{"author", "GIT_AUTHOR_IDENT"},
		{"committer", "GIT_COMMITTER_IDENT"},
	}
	for _, r := range roles {
		ident, err := RunOutputIn(repoRoot, "git", "var", r.gitVar)
		if err != nil {
			return fmt.Errorf("reading %s identity: %w", r.label, err)
		}
		email := identEmail(ident)
		if !strings.HasSuffix(strings.ToLower(email), noreplyDomain) {
			fmt.Fprintf(os.Stderr, "❌ pre-commit: %s identity %q is not a %s address.\n", r.label, email, noreplyDomain)
			fmt.Fprintf(os.Stderr, "Set a GitHub noreply email, e.g.:\n  git config user.email \"<id>+<user>%s\"\n", noreplyDomain)
			return fmt.Errorf("%s email %q lacks %s", r.label, email, noreplyDomain)
		}
	}
	return nil
}

// identEmail extracts the address from a git ident string of the form
// "Name <email> <timestamp> <tz>". It returns "" if no <…> field is present.
func identEmail(ident string) string {
	open := strings.IndexByte(ident, '<')
	close := strings.IndexByte(ident, '>')
	if open < 0 || close < open {
		return ""
	}
	return ident[open+1 : close]
}

const (
	// binaryScanPrefix is how many leading bytes of a staged blob the binary
	// check inspects. A NUL byte anywhere in this window means the blob is
	// binary, not source text — git's own heuristic, widened to 16 KiB.
	binaryScanPrefix = 16 * 1024
	// maxBlobBytes is the largest staged file permitted. Anything bigger is
	// almost certainly a generated artifact or vendored blob, not source — and
	// is rejected whether or not it scans as text.
	maxBlobBytes = 256 * 1024
)

// checkNoBinaryBlobs refuses the commit if any staged file is binary (contains
// a NUL byte in its first binaryScanPrefix bytes) or larger than maxBlobBytes.
// The repo holds source only — no compiled artifacts, images, or vendored
// blobs. It inspects the *staged* content via 'git cat-file' (':<path>' reads
// from the index), so it judges exactly what the commit would record.
func checkNoBinaryBlobs(repoRoot string) error {
	staged, err := RunOutputIn(repoRoot, "git", "diff", "--cached", "--name-status")
	if err != nil {
		return fmt.Errorf("listing staged files: %w", err)
	}
	var rejects []string
	for _, line := range strings.Split(staged, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		// Skip blank lines and deletions: a removed path commits nothing.
		if len(fields) < 2 || strings.HasPrefix(fields[0], "D") {
			continue
		}
		path := fields[len(fields)-1] // dst path (handles rename/copy "R old new")
		spec := ":" + path

		// Size first — cheap, no full read. Skip entries that aren't regular
		// blobs (e.g. submodule gitlinks), where cat-file -s errors out.
		sizeOut, err := RunOutputIn(repoRoot, "git", "cat-file", "-s", spec)
		if err != nil {
			continue
		}
		size, err := strconv.ParseInt(sizeOut, 10, 64)
		if err != nil {
			continue
		}

		var head []byte
		if size <= maxBlobBytes { // only read the bytes we need to scan
			data, err := OutputBytesIn(repoRoot, "git", "cat-file", "blob", spec)
			if err != nil {
				continue
			}
			head = data
			if len(head) > binaryScanPrefix {
				head = head[:binaryScanPrefix]
			}
		}
		if why := classifyBlob(size, head); why != "" {
			rejects = append(rejects, fmt.Sprintf("  %s — %s", path, why))
		}
	}
	if len(rejects) > 0 {
		fmt.Fprintln(os.Stderr, "❌ pre-commit: refusing to commit binary or oversized files (this repo is source-only):")
		fmt.Fprintln(os.Stderr, strings.Join(rejects, "\n"))
		fmt.Fprintln(os.Stderr, "Remove the file(s) or unstage them, e.g.:\n  git reset HEAD <path>")
		return fmt.Errorf("%d binary/oversized staged file(s)", len(rejects))
	}
	return nil
}

// classifyBlob returns a human-readable rejection reason for a staged blob, or
// "" if it is acceptable source. size is the blob's full byte count; head is
// its leading bytes (already capped to binaryScanPrefix by the caller, and left
// nil for blobs already over the size limit, which are rejected on size alone).
func classifyBlob(size int64, head []byte) string {
	if size > maxBlobBytes {
		return fmt.Sprintf("%d bytes exceeds the %d KiB limit", size, maxBlobBytes/1024)
	}
	if i := bytes.IndexByte(head, 0); i >= 0 {
		return fmt.Sprintf("NUL byte at offset %d — binary, not source text", i)
	}
	return ""
}
