# web

Web platform (browser) bindings for the
[Promise programming language](https://github.com/promise-language/promise).

This module provides typed bindings to the Web platform APIs — DOM, events,
console, timers, fetch, and the `JsValue` dynamic-value bridge — for
`` `target(web) `` builds targeting `wasm32-web`.

The bindings are generated from the upstream WebIDL definitions via
`promise bind webidl` and then polished with inline `` `doc `` annotations.

> **Status:** repository scaffolding only. The generated binding sources are not
> in place yet; see [docs/wasm-bindings.md](https://github.com/promise-language/promise/blob/main/docs/wasm-bindings.md)
> in the compiler repository for the binding architecture and the planned module
> surface.

## Usage

Registered in the Promise module catalog under the name **`web`**:

```promise
use web;

main() `target(web) {
    doc := document();
    element := doc.create_element("div");
    element.set_inner_text("Hello from Promise!");
    doc.body().append_child(move element);
}
```

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
