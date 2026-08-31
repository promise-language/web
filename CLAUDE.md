<!-- forge:dev-tooling -->
## Dev tooling

Dev tools are compiled from a single in-repo Go module (`tools/build/`) into
`bin/`, which is gitignored — the tools are always built locally, never committed.

**Fresh clone — bootstrap once, before anything else:**

```bash
./make            # Windows: .\make.cmd
```

`./make` compiles every tool into `bin/` and wires up the git pre-commit hook.
It is idempotent and finishes in well under a second once built.

This step is **mandatory and comes first**. The Claude Code guard hook is wired
to fail closed (`bin/guard || exit 2` in `.claude/settings.json`), so until
`bin/` exists every `Bash`, `Edit`, and `Write` call is blocked — including the
agent's own attempt to run `./make`. On a fresh clone a human runs `./make`
once; after that the guard is in charge and an agent can work normally.

**Before every commit — run the gate:**

```bash
bin/verify        # format → vet → build → test, then a pass/FAIL summary
```

A green "OK to Commit" line means it is safe to commit; a red FAIL means it is
not. The pre-commit hook (`bin/precommit`) also blocks staged binaries, enforces
GitHub noreply commit identities, and keeps the tree source-only.

**Edit a tool, then rebuild.** If any tool prints
`tools source has changed — run: ./make`, re-run `./make` to rebuild `bin/`.
That is the whole loop: edit → `./make` → use. Add a tool by dropping a new dir
under `tools/build/cmd/` and re-running `./make` — no registration step.
<!-- /forge:dev-tooling -->

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
