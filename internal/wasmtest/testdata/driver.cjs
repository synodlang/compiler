// Drives synod.wasm under plain Node for internal/wasmtest.
//
//	node driver.cjs <synod.wasm> <wasm_exec.js> <requests.json> <responses.json>
//
// requests.json is [{fn, args}]; responses.json gets {responses: [{result,
// ms}], shape: [...], problems: [...]}. The Go test compares the results; this
// script only asserts the API shape and the glue-level error handling.
"use strict";

const fs = require("fs");
const path = require("path");

async function main() {
  const [wasmPath, execPath, reqPath, outPath] = process.argv.slice(2);
  if (!outPath) throw new Error("usage: driver.cjs wasm wasm_exec.js requests.json responses.json");
  require(path.resolve(execPath));
  const go = new Go();
  const { instance } = await WebAssembly.instantiate(fs.readFileSync(wasmPath), go.importObject);
  go.run(instance); // never settles: main blocks forever
  for (let i = 0; i < 100 && !globalThis.synod; i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  const synod = globalThis.synod;
  const problems = [];
  const bad = (msg) => problems.push(msg);

  // API shape.
  const names = Object.keys(synod).sort();
  if (JSON.stringify(names) !== JSON.stringify(["check", "format", "run", "version"])) {
    bad("synod has " + JSON.stringify(names));
  }
  for (const n of names) if (typeof synod[n] !== "function") bad(n + " is not a function");
  const v = synod.version();
  if (v.api !== 1) bad("version().api = " + v.api);

  // Requests, in order.
  const requests = JSON.parse(fs.readFileSync(reqPath, "utf8"));
  const responses = [];
  for (const r of requests) {
    const t0 = process.hrtime.bigint();
    let result;
    try {
      result = synod[r.fn](...r.args);
    } catch (e) {
      result = { threw: String(e) };
    }
    const ms = Number(process.hrtime.bigint() - t0) / 1e6;
    responses.push({ result, ms });
  }

  // Glue-level type errors: usage errors, never exceptions, and the next call
  // still works.
  const cyc = {};
  cyc.self = cyc;
  const okFiles = { "a.synod": "// nothing\n" };
  const glue = [
    ["check", ["not an object"]],
    ["check", [null]],
    ["check", [undefined]],
    ["check", [[]]],
    ["check", [{ "a.synod": 1 }]],
    ["check", [{ "a.synod": "x" }, "options"]],
    ["check", [cyc]],
    ["run", [okFiles, undefined]],
    ["run", [okFiles]],
    ["run", [okFiles, { program: 7 }]],
    ["format", [42]],
    ["format", [okFiles, 42]],
  ];
  for (const [fn, args] of glue) {
    const label = fn + "(" + args.map((a) => (a === cyc ? "cyclic" : JSON.stringify(a))).join(", ") + ")";
    let res;
    try {
      res = synod[fn](...args);
    } catch (e) {
      bad(label + " threw " + e);
      continue;
    }
    if (!res || res.exitCode !== 2 || !res.error || res.error.code !== "usage" || typeof res.error.message !== "string") {
      bad(label + " = " + JSON.stringify(res));
    }
    const after = synod.version();
    if (!after || after.api !== 1) bad("version() after " + label + " = " + JSON.stringify(after));
  }
  const c = synod.check({ "a.synod": "x\n" });
  if (c.error !== null || (c.exitCode !== 0 && c.exitCode !== 1)) bad("check after glue errors: " + JSON.stringify(c));

  fs.writeFileSync(outPath, JSON.stringify({ responses, shape: names, problems }));
}

main().catch((e) => {
  console.error(e && e.stack ? e.stack : e);
  process.exit(1);
});
