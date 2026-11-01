// Checks every case in cases.mjs against a dollycli that this script builds and starts: one on http (port 8661) and one on https (8662),
// each with its own data folder in a temporary folder. It prints a table, and it exits with 1 when a case does not do what it expects.
//
//   npm install && npm run check
//
// Environment: DOLLY_BIN (a built dolly, instead of building one), CHROMIUM (the path of Chromium, for the browser cases).
import { execFileSync, spawn } from "node:child_process";
import { X509Certificate, createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import tls from "node:tls";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));

// ---- child: runs the cases, in a process that trusts the CA of the https server ----
if (process.argv.includes("--child")) {
  const { cases } = await import("./cases.mjs");
  for (const c of cases) {
    const skip = c.skip?.() ?? "";
    let got = "ok";
    let detail = "";
    if (skip) {
      got = "skipped";
      detail = skip;
    } else {
      try {
        detail = await c.run();
      } catch (e) {
        got = "fail";
        detail = e.message;
      }
    }
    const pass = got === "skipped" ? null : got === c.expect && (got === "ok" || detail.includes(c.text));
    console.log("RESULT " + JSON.stringify({ id: c.id, lib: c.lib, transport: c.transport, setting: c.setting, expect: c.expect, got, pass, detail }));
  }
  process.exit(0);
}

// ---- orchestrator ----
const root = path.resolve(here, "../.."); // docs/compat -> the root of the repo
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dollycli-compat-"));
const servers = [];

function bin() {
  if (process.env.DOLLY_BIN) return process.env.DOLLY_BIN;
  const out = path.join(tmp, "dolly");
  execFileSync("go", ["build", "-o", out, "./cmd/dolly"], { cwd: root, stdio: "inherit" });
  return out;
}

async function start(exe, name, port, extra, scheme) {
  const child = spawn(exe, ["serve", "--dir", path.join(tmp, name), "--port", String(port), "--users", "50", ...extra], { stdio: "ignore" });
  servers.push(child);
  const base = `${scheme}://127.0.0.1:${port}`;
  for (let i = 0; i < 60; i++) {
    await new Promise((r) => setTimeout(r, 250));
    try {
      if (scheme === "http" && (await fetch(`${base}/_dolly/apis`)).ok) return base;
      if (scheme === "https" && fs.existsSync(path.join(tmp, name, "tls", "ca.pem"))) {
        await dial(port, path.join(tmp, name, "tls", "ca.pem")); // the check of the certificate is on: it uses the CA of this server
        return base;
      }
    } catch {
      // not up yet
    }
  }
  throw new Error(`dolly on port ${port} did not start`);
}

// a TLS connection that checks the certificate against the CA of the server. It gives the certificate (DER).
function dial(port, caFile) {
  return new Promise((resolve, reject) => {
    const s = tls.connect({ host: "127.0.0.1", port, ca: fs.readFileSync(caFile) }, () => {
      const raw = s.getPeerCertificate().raw;
      s.end();
      resolve(raw);
    });
    s.on("error", reject);
  });
}

// the hash of the public key of the certificate: Chromium trusts this one key and no other (no blanket switch-off of the check)
async function spki(port, caFile) {
  const der = new X509Certificate(await dial(port, caFile)).publicKey.export({ type: "spki", format: "der" });
  return createHash("sha256").update(der).digest("base64");
}

let code = 1;
try {
  const exe = bin();
  const http = await start(exe, "http", 8661, [], "http");
  const https = await start(exe, "https", 8662, ["--tls"], "https");
  const caFile = path.join(tmp, "https", "tls", "ca.pem");
  const pin = await spki(8662, caFile);
  const results = await new Promise((resolve) => {
    // the child trusts the CA (NODE_EXTRA_CA_CERTS is read when Node starts, so it is set here), and Chromium trusts the pinned key
    const child = spawn(process.execPath, [path.join(here, "run.mjs"), "--child"], {
      env: { ...process.env, BASE_HTTP: http, BASE_HTTPS: https, SPKI: pin, NODE_EXTRA_CA_CERTS: caFile },
      stdio: ["ignore", "pipe", "inherit"],
    });
    let out = "";
    child.stdout.on("data", (d) => (out += d));
    child.on("close", () =>
      resolve(
        out
          .split("\n")
          .filter((l) => l.startsWith("RESULT "))
          .map((l) => JSON.parse(l.slice(7))),
      ),
    );
  });

  const cell = (s, n) => String(s).replace(/\s+/g, " ").slice(0, n).padEnd(n);
  console.log(`\n${cell("case", 30)} ${cell("transport", 9)} ${cell("setting", 24)} ${cell("expect", 6)} ${cell("got", 8)} result  detail`);
  for (const r of results) {
    const mark = r.pass === null ? "skip  " : r.pass ? "ok    " : "FAILED";
    console.log(`${cell(r.id, 30)} ${cell(r.transport, 9)} ${cell(r.setting, 24)} ${cell(r.expect, 6)} ${cell(r.got, 8)} ${mark}  ${cell(r.detail, 100).trimEnd()}`);
  }
  const versions = ["@azure/msal-browser", "@azure/msal-node", "@azure/identity", "oidc-client-ts", "openid-client"].map(
    (p) => `${p} ${JSON.parse(fs.readFileSync(path.join(here, "node_modules", p, "package.json"), "utf8")).version}`,
  );
  console.log(`\nLibraries: ${versions.join(", ")}. Node ${process.versions.node}. Date ${new Date().toISOString().slice(0, 10)}.`);
  const failed = results.filter((r) => r.pass === false);
  console.log(failed.length ? `${failed.length} of ${results.length} cases did not do what they expect.` : `All ${results.filter((r) => r.pass).length} cases do what they expect.`);
  code = failed.length || results.length === 0 ? 1 : 0;
} catch (e) {
  console.error(e.message);
} finally {
  for (const s of servers) s.kill();
  fs.rmSync(tmp, { recursive: true, force: true });
}
process.exit(code);
