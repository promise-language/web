# Documentation Index

This is the map of `docs/`. Every tracked file under it is listed here, wherever
it lives — the section an entry sits under is where its binding status is written
down. A document nobody can reach from the index is one nobody reads, and one
nobody updates.

**This project's status query.** Each root document's tag is a GitHub label,
spelled as the file's basename minus `.md`, and the remaining work for a document
is:

> `gh issue list --label <tag> --state open --limit 200`

`--state open` and `--limit 200` are both written out deliberately: `gh issue
list` defaults to a limit of 30, so a specification with more remaining work than
that would silently under-report and read as nearly done.

## Specifications

_Nothing yet._ Add a line here in the same commit that adds the document; an
index brought up to date afterwards is an index that was wrong in between.

## Dependencies

The repositories whose documents this project references or whose code it
imports, and the release of each it stands on. A specification in a declared
repository's root binds this project exactly as one in this directory's root does,
at the release named here; this tree holds no copy of it, and an issue about one
is filed where the document originates.

| Repository | Release |
|---|---|
| `org` | `v2.6.0` |
| `forge` | `v0.14.0` |

`org` is declared because `CLAUDE.md` cites it, under "The org norms this
project is held to", at this release. `forge` is declared because
`tools/build/go.mod` requires its module at this release, and `tools/build/go.sum`
verifies that release: both restate this row, so raising the pin moves all three
in one commit. Add a row here in the same commit that adds the first reference to
any other repository, or the first import of its code; a reference at a release
nothing declares is a reference to whatever the repository says today.
