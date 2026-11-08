# web

Web platform (browser) bindings for the
[Promise programming language](https://github.com/promise-language/promise).

This module provides typed bindings to the DOM for `wasm32-web` builds. The
bindings are generated from WebIDL by `promise bind webidl`, which emits two
files: the Promise module (`web.pr`) and the JavaScript glue (`web.js`) that the
page loads to instantiate the module and implement its `promise_env` imports.

Each generated interface becomes a `` `target(web) `` type wrapping an opaque
`i32` handle into the glue's JS object table, with WebIDL attributes exposed as
`get`/`set` properties and operations as methods:

```promise
type Document is Node `public `target(web) {
    get body HTMLElement? `public { ... }
    set body(HTMLElement? value) `public { ... }
    create_element(this, string local_name, JsValue? options) Element `public { ... }
}
```

> **Status:** an explicitly incomplete, app-sized revision. The generated
> surface is only what the app keep list (`web.keep`) selects, each entry
> Baseline in the pinned web-features data — enough to build a page under
> `web.document.body`, read an `<input>`'s value, and set text. The full
> Baseline surface is a later revision of this same module.

## Coverage

The binding surface is generated from these W3C/WHATWG specifications, taken
from the [webref](https://github.com/w3c/webref) curated extraction:

| Spec | Contributes |
|---|---|
| WHATWG DOM | `EventTarget`, `Node`, `Element`, `Document`, `Range` |
| WHATWG HTML | `Window`, `HTMLElement` and the element hierarchy |
| WHATWG Console | the `console` namespace |
| UI Events | `UIEvent`, `KeyboardEvent` |
| Pointer Events | `MouseEvent`, `PointerEvent` |
| CSSOM | `CSSStyleDeclaration`, `CSSRule`, `StyleSheet` |
| CSSOM View | `getBoundingClientRect`, scrolling, `DOMRect` on elements |
| Geometry Interfaces | `DOMPoint`, `DOMRect`, `DOMMatrix` |
| CSS Pseudo-Elements | `CSSPseudoElement` |
| WHATWG URL | `URL`, `URLSearchParams` |
| XMLHttpRequest | `XMLHttpRequest`, `FormData`, `ProgressEvent` |
| WAI-ARIA | the `ARIAMixin` every element includes |
| SVG | `SVGElement`, which CSSOM and HTML extend |
| MathML Core | `MathMLElement`, which CSSOM and HTML extend |

The last three are bound because the others include or extend what they
define: `promise bind webidl` refuses a `partial` or `includes` whose target no
bound file defines.

Of that IDL, the generated module carries only what the keep list
[`web.keep`](web.keep) names, and `bin/bindgen` refuses a keep list naming
anything the pinned [web-features](https://github.com/web-platform-dx/web-features)
release does not report Baseline (newly or widely available across the core
browsers):

| Keep-list entry | What it gives a program |
|---|---|
| `Document`, `Document.body`, `Document.createElement` | the `web.document` root; the page's nullable `body`; `create_element` |
| `Node`, `Node.appendChild`, `Node.textContent` | `append_child`; nullable `text_content` on every node |
| `HTMLInputElement`, `HTMLInputElement.value` | an input's `value` |

The ancestors of a kept interface (`EventTarget`, `Element`, `HTMLElement`)
come with it, so an `HTMLInputElement` is a `Node` wherever one is expected.
Every interface with a parent has a checked conversion, `from`, which turns the
`Element` that `create_element` returns into the interface the element really
is, or `none`.

The spec list, both pins and the keep list's file name live in
[`tools/build/common/webref.go`](tools/build/common/webref.go) and
[`tools/build/common/baseline.go`](tools/build/common/baseline.go).

## Regenerating the bindings

`web.pr` and `web.js` are generated and committed; the inputs they are generated
*from* are not — the WebIDL is fetched on demand into a gitignored `idl/` and
the Baseline data into a gitignored `baseline/`, so no third-party text lives in
this tree.

```bash
./make            # Windows: .\make.cmd  — compiles the dev tools into bin/
bin/bindgen       # fetch the pinned inputs, check web.keep, regenerate web.pr and web.js
bin/verify        # the commit gate
```

`bin/bindgen` runs the Promise compiler this project pins
([`tools/build/common/project.go`](tools/build/common/project.go)), resolved
from the checkout's dependency record and never from `PATH`, so regenerating
from the committed pins reproduces the committed files. It relays what
`promise bind webidl -keep` reports dropping — a member naming something the
keep list drops — on its narration (`bin/bindgen -narrate`).

To read the WebIDL itself — there is no copy in the tree — materialize it
separately, or print the pin without fetching anything:

```bash
bin/fetch-idl        # write idl/ at the pinned version
bin/fetch-idl -list  # print the pinned release and the spec list
```

`bin/bindgen` pins `@webref/idl` and `web-features` by version *and* tarball
sha256 and refuses to run on a mismatch, so a binding diff can always be traced
to a spec or compat-data revision. Never hand-edit `web.pr` or `web.js` — fix
the generator, the spec list or the keep list.

`bin/verify` builds the module for `wasm32-web` and tests it through
[`testdata/dom`](testdata/dom): a check program, built against the generated
module and run by the pinned Node through `web.js` against a minimal DOM, that
verifies the named root, nullable access, inheritance, the `from` conversion,
and that every handle the program held is released.

## Usage

Registered in the Promise module catalog under the name **`web`**:

```promise
use web;

main() {
    doc := web.document;    // the page's document, a named root: never a handle
    page := doc.body ? {
        return;             // no body yet: nothing to build under
    };

    heading := doc.create_element("h1", none);
    heading.text_content = "hello world";
    page.append_child(heading);

    // create_element returns an Element; `from` converts it to the interface
    // the element really is, or none.
    input := web.HTMLInputElement.from(doc.create_element("input", none)) ? {
        return;
    };
    input.value = "type here";
    page.append_child(input);
}
```

Build for the browser and load the glue from the page:

```bash
promise build -target wasm32-web main.pr    # -> main.wasm
```

```html
<script type="module">
  import { init } from "./web.js";
  await init("main.wasm");
</script>
```

Two things to keep in mind:

- **Wrappers own their handles.** Each wrapper releases its handle when it
  drops. `from` returns a wrapper with a handle of its own, so the wrapper it
  converted is unchanged and both drop independently; `web.document` is
  borrowed, and dropping it releases nothing.
- **Don't name the consuming project `web`.** Every `wasm32-web` build emits a
  bootstrap loader named after its output, so a project named `web` writes a
  `web.js` that overwrites this module's glue of the same name.

## Provenance

The bindings in this module are typed to the Web platform interface definitions
(WebIDL) published as living standards by the WHATWG (<https://whatwg.org>) and
as Recommendations by the W3C (<https://www.w3.org>). See [NOTICE](NOTICE) for
attribution details.

## Contributing

Contributions require signing the Promise Lang Contributor License Agreement
before they can be merged. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Dual-licensed under either of [Apache-2.0](LICENSE-APACHE) or [MIT](LICENSE-MIT)
at your option — the same terms as the Promise compiler. See [NOTICE](NOTICE) for
third-party attributions.
