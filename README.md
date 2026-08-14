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
type Document `public `target(web) {
    i32 _handle;

    get title string `public { ... }
    create_element(this, string local_name) Element `public { ... }
}
```

> **Status:** repository scaffolding only — the generated bindings are not in
> place yet. A hello-world does run in Chrome today, but only from a
> hand-trimmed IDL subset and with a post-processing pass over the generated
> glue: the WebIDL path emits canonical-ABI JS while the compiler lowers WebIDL
> externs the Promise-native way, and the generator does not yet handle
> interface inheritance or overloaded operations. Those are compiler-side fixes;
> this repository will carry the bindings once they generate cleanly.

## Usage

Registered in the Promise module catalog under the name **`web`**:

```promise
use web;

main() {
    // The glue seeds handles before calling _initialize: 1 = globalThis,
    // 2 = document, 3 = console. Generated bindings have no way to name a JS
    // global, so the roots are constructed from their handles.
    doc := web.Document(_handle: 2);
    console := web.Console(_handle: 3);

    console.log("hello from Promise, running as WebAssembly");

    page := doc.body ? {
        console.log("no document.body — nothing to do");
        return;
    };

    heading := doc.create_element("h1");
    heading.text_content = "hello world";
    heading.id = "greeting";

    page.append_child(heading);

    console.log("document.title is: {doc.title}");
}
```

Build for the browser and load the glue from the page:

```bash
promise build --target wasm32-web main.pr    # -> main.wasm
```

```html
<script type="module">
  import { init } from "./web.js";
  await init("main.wasm");
</script>
```

Handles are owned: each wrapper releases its handle when it drops, so never
construct a second wrapper over a handle another wrapper already holds.

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
