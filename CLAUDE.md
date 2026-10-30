## Regenerating the bindings

`web.pr` and `web.js` are **generated**. Never hand-edit them — the next
regeneration silently discards the edit. Fix the generator or the spec list
instead.

```bash
bin/bindgen             # fetch pinned WebIDL, regenerate web.pr + web.js
bin/bindgen -force      # re-fetch idl/ even if already at the pinned version
bin/bindgen -clean      # drop idl/ afterwards
```

`idl/` is gitignored, so there is no copy of the WebIDL in the tree to read.
`bin/fetch-idl` puts one there without regenerating anything:

```bash
bin/fetch-idl           # materialize idl/ at the pinned version
bin/fetch-idl -list     # print the pin and spec list, touch nothing
bin/fetch-idl -clean    # remove idl/
```

The pinned `@webref/idl` release, its tarball sha256, and the list of specs that
make up the binding surface all live in one file:
[`tools/build/common/webref.go`](tools/build/common/webref.go). Adding a spec or
moving to a newer webref release is an edit there, then `./make && bin/bindgen`.

Two things the generator deliberately does *not* do:

- **It never writes `promise.toml`.** `promise bind webidl` emits one stamped
  with the compiler epoch; modules here are epoch-less and the repo's
  `promise.toml` is hand-maintained source. Generation happens in a scratch
  directory and only `web.pr` and `web.js` are copied back.
- **It never commits `idl/`.** The WebIDL inputs are fetched on demand and
  gitignored, so third-party spec text stays out of the tree. The pin in
  `webref.go` is what makes a binding diff traceable to a spec revision.

<!-- forge:dev-tooling -->
## Dev tooling

Dev tools are compiled from a single in-repo Go module (`tools/build/`) into
`bin/`, which is gitignored — the tools are always built locally, never committed.
The module pins `github.com/promise-language/forge` at one exact version, which
is where the harness under these tools comes from; nothing here is a copy of it.

**An agent reaches these tools as operations.** Every operation `make`,
`setup`, `verify`, `gate` and `run` offer is one an agent reaches through the MCP
server the workspace provisions in this checkout
([entry points](https://github.com/promise-language/org/blob/v2.6.0/docs/cli-guide.md#entry-points)).
The commands below are a person's route to the same operations.

**Fresh clone — bootstrap once.** This one is a person's command: a fresh clone
has no server until the workspace provisions it, and no tool until `./make` has run.

```bash
./make            # Windows: .\make.cmd
```

`./make` compiles every tool into `bin/` and points git at `.githooks`.
It is idempotent and finishes in well under a second once built.

**Before every commit — the `verify` operation:**

```bash
bin/verify        # repair → builds → measure → ratchet → record, then a pass/FAIL summary
```

A green "OK to Commit" line means it is safe to commit; a red FAIL means it is
not. Its last step records the tree it blessed at `.workspace/verified-tree`,
which is what the commit gate (`bin/precommit-guard`, installed by the workspace
rather than built here) reads to refuse a commit of any other tree. The
committed `.githooks/pre-commit`, live once `./make` has run, refuses every
commit until provisioning has installed that guard.

**Measure one thing, or ask what can be measured — the `gate` and `run` operations:**

```bash
bin/gate --list   # every measurement this project answers
bin/run <gate>    # measure one gate and print it beside the term it is judged on
```

The `gate <gate>` operation, given `envelope`, writes one JSON envelope and
nothing else; it holds no threshold and reaches no verdict. The `run <gate>`
operation, given `verdict`, reads that envelope on stdin and writes one verdict,
judged against `tools/gates/thresholds.json`. **The verdict is the JSON, not the
exit status** — given `verdict`, it exits 0 either way.

**Edit a tool, then rebuild.** If any tool refuses with
`tools source has changed — run: ./make`, the `make` operation rebuilds `bin/`;
a person runs it as `./make`. That is the whole loop: edit → `make` → use. Add a
tool by dropping a new dir under `tools/build/cmd/` and running `make` again —
no registration step.

## The org norms this project is held to

These are organisation-wide and pinned at **org v2.6.0**. They are not
derivable from this tree, and they are what a reviewer will hold a change to, so
read the one that covers what you are about to write **before** writing it:

- [cli-guide](https://github.com/promise-language/org/blob/v2.6.0/docs/cli-guide.md)
  — the shape of every command line here: subcommands, flag form,
  [help and version](https://github.com/promise-language/org/blob/v2.6.0/docs/cli-guide.md#help-and-version),
  [output modes](https://github.com/promise-language/org/blob/v2.6.0/docs/cli-guide.md#output-modes)
  (JSON through a pipe, a person's rendering at a terminal) and
  [exit codes](https://github.com/promise-language/org/blob/v2.6.0/docs/cli-guide.md#exit-codes)
  (`2` a malformed invocation, `3` a refusal — a status is not an error message).
- [normative](https://github.com/promise-language/org/blob/v2.6.0/docs/normative.md)
  — what a normative statement is, and the
  [vocabulary](https://github.com/promise-language/org/blob/v2.6.0/docs/normative.md#location)
  `docs/` is checked against.
- [references](https://github.com/promise-language/org/blob/v2.6.0/docs/references.md#the-declaration)
  — how a citation is written, and why it names a release rather than a branch.
- [logging](https://github.com/promise-language/org/blob/v2.6.0/docs/logging.md#where-a-log-is-written)
  — where a log goes and what a scope is.

**Cite a release, never a branch.** A link to `main` documents whatever org says
today, not what this code was written against.
<!-- /forge:dev-tooling -->
