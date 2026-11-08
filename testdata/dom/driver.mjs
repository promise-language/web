// The web module's DOM check: it loads the generated glue (web.js) over a
// minimal DOM, runs the check program (main.pr, built for wasm32-web), and
// judges what the program left in the host's DOM.
//
//   node driver.mjs <main.wasm> <web.js>
//
// It writes one JSON record per check to stdout, and nothing else there:
// {"check": <name>, "status": "pass" | "fail" | "leak", "detail": <text>}.
// tools/build/common/domcheck.go decodes them. It exits 0 once every check is
// recorded, and non-zero only when it could not run at all.

import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";

const [wasmPath, gluePath] = process.argv.slice(2);
if (!wasmPath || !gluePath) {
  process.stderr.write("usage: node driver.mjs <main.wasm> <web.js>\n");
  process.exit(2);
}

// --- A minimal DOM ---
//
// Only what the retained surface reaches, shaped as the glue reaches it:
// every attribute is an accessor on its interface's prototype, because the
// glue reads members through the declaring interface's prototype.

class EventTarget {}

class Node extends EventTarget {
  #text = "";
  childNodes = [];
  get textContent() {
    // A Document's textContent is null (WHATWG DOM, Node.textContent).
    return this instanceof Document ? null : this.#text;
  }
  set textContent(value) {
    if (!(this instanceof Document)) this.#text = value === null ? "" : String(value);
  }
  appendChild(node) {
    this.childNodes.push(node);
    return node;
  }
}

class Element extends Node {
  constructor(localName) {
    super();
    this.localName = localName;
  }
}

class HTMLElement extends Element {}

class HTMLInputElement extends HTMLElement {
  #value = "";
  get value() {
    return this.#value;
  }
  set value(v) {
    this.#value = String(v);
  }
}

class HTMLDivElement extends HTMLElement {}

class Document extends Node {
  #body = null;
  created = [];
  get body() {
    return this.#body;
  }
  set body(element) {
    this.#body = element;
  }
  createElement(localName) {
    const element =
      localName === "input" ? new HTMLInputElement(localName)
        : localName === "div" ? new HTMLDivElement(localName)
          : new HTMLElement(localName);
    this.created.push(element);
    return element;
  }
}

Object.assign(globalThis, { EventTarget, Node, Element, HTMLElement, HTMLInputElement, HTMLDivElement, Document });
const document = new Document();
globalThis.document = document;

// The glue fetches the module it is given; serve the built one.
globalThis.fetch = async (url) =>
  new Response(await readFile(url), { headers: { "content-type": "application/wasm" } });

// --- The checks ---

const records = [];
const check = (name, ok, detail) => records.push({ check: name, status: ok ? "pass" : "fail", detail });

// The value main.pr writes into the input and expects back.
const typed = 'typed ✓ "quoted"';

// The glue seeds document from the global at load, so it is imported only now.
const glue = await import(pathToFileURL(gluePath).href);
let ran = true;
try {
  await glue.init(wasmPath);
} catch (e) {
  ran = false;
  check("program", false, `the program did not run to completion: ${e && e.stack ? e.stack : e}`);
}
if (ran) check("program", true, "the program ran to completion");

const created = (localName) => document.created.filter((e) => e.localName === localName);
const page = document.body;
const notes = new Map(
  (page ? page.childNodes : [])
    .filter((n) => n.localName === "p")
    .map((n) => n.textContent.split(/=(.*)/s).slice(0, 2)),
);
const noted = (key, want) => {
  const got = notes.get(key);
  return { ok: got === want, detail: `the program noted ${key}=${got === undefined ? "(nothing)" : got}, want ${want}` };
};

{
  const [body] = created("body");
  check("named-root", body !== undefined && page === body,
    body === undefined ? "the program created no body through web.document"
      : page === body ? "web.document is the page's document: the body the program assigned through it is document.body"
        : "the body the program assigned through web.document is not document.body");
}
{
  const before = noted("body-before", "absent");
  check("nullable-body-absent", before.ok, before.detail);
  const after = noted("body-after", "present");
  check("nullable-body-present", after.ok, after.detail);
  const text = noted("document-text", "absent");
  check("nullable-text-absent", text.ok, text.detail);
}
{
  const [input] = created("input");
  check("conversion-input-host", input !== undefined && input.value === typed,
    `the host input's value is ${input === undefined ? "(no input)" : JSON.stringify(input.value)}, want ${JSON.stringify(typed)}`);
  const read = noted("value", typed);
  check("conversion-input-read", read.ok, read.detail);
  const div = noted("div", "none");
  check("conversion-div-none", div.ok, div.detail);
}
{
  const [input] = created("input");
  const [heading] = created("h1");
  const children = page ? page.childNodes : [];
  check("inheritance-append", input !== undefined && heading !== undefined && children.includes(input) && children.includes(heading),
    "append_child(Node) on the body took the HTMLInputElement and the heading");
  check("inheritance-text-host", heading !== undefined && heading.textContent === "hello",
    `the host heading's textContent is ${heading === undefined ? "(no heading)" : JSON.stringify(heading.textContent)}, want "hello"`);
  const text = noted("heading-text", "hello");
  check("inheritance-text-read", text.ok, text.detail);
}
{
  // Every wrapper the program held has dropped, so every slot above the seeded
  // ones it used is free again, exactly once. Fresh stores then fill the slots
  // above the seeded ones without a gap and without a repeat. A slot never
  // released leaves a gap whatever it holds — undefined included, which is
  // what an unreleased JsValue temporary holds, so reading the slots could not
  // tell it from a free one — and a slot released twice is handed out twice.
  // The probes outnumber the handles the program ever holds.
  const seeded = 3;
  const probes = 4096;
  const seen = new Set();
  const twice = [];
  for (let i = 0; i < probes; i++) {
    const handle = glue._refStore({});
    if (seen.has(handle)) twice.push(handle);
    seen.add(handle);
  }
  const gaps = [];
  for (let handle = seeded + 1; handle <= seeded + probes; handle++) {
    if (!seen.has(handle)) gaps.push(handle);
  }
  records.push(
    twice.length > 0
      ? { check: "handles-released", status: "fail", detail: `handles released more than once: ${[...new Set(twice)].join(", ")}` }
      : gaps.length > 0
        ? { check: "handles-released", status: "leak", detail: `handles never released: ${gaps.join(", ")}` }
        : { check: "handles-released", status: "pass", detail: "every handle the program held was released, once" },
  );
}

for (const r of records) process.stdout.write(JSON.stringify(r) + "\n");
