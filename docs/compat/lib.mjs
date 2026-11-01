// Helpers that the cases share. The orchestrator (run.mjs) gives the addresses in the environment.
import fs from "node:fs";
import http from "node:http";
import puppeteer from "puppeteer-core";

export const CLIENT = "11111111-2222-3333-4444-555555555555";
export const ADMIN = "a.philip@acme.example"; // the admin account that every seed of two users or more has
export const APP_PORT = 3017; // the web app of a browser case runs on http://localhost:3017
export const APP_ORIGIN = `http://localhost:${APP_PORT}`;
export const BASE = { http: process.env.BASE_HTTP, https: process.env.BASE_HTTPS };

export async function tenantId(base) {
  return (await (await fetch(`${base}/_dolly/login/info`)).json()).tenantId;
}

/** A person picks a user on the sign-in page. It gives the address that the browser goes back to. */
export async function pick(authorizeUrl, username) {
  const u = new URL(authorizeUrl);
  const body = new URLSearchParams(u.searchParams);
  body.set("username", username);
  const r = await fetch(u.origin + u.pathname, { method: "POST", body, redirect: "manual" });
  const loc = r.headers.get("location");
  if (!loc) throw new Error(`no redirect: ${r.status} ${(await r.text()).slice(0, 200)}`);
  return loc;
}

export const chromium = () =>
  process.env.CHROMIUM || ["/usr/bin/chromium", "/usr/bin/chromium-browser", "/usr/bin/google-chrome"].find((p) => fs.existsSync(p));

/**
 * Runs a web app in Chromium. `page` is the HTML for every path of the app, and `files` has the other files (the library).
 * A person picks the user on the sign-in page. The page writes lines in <pre id=out>. It gives the lines when one of
 * `done` is in them (or after 12 seconds). The browser trusts the key of the https server of dollycli, and no other.
 */
export async function webApp({ page, files = {}, done }) {
  const srv = http.createServer((q, s) => {
    const path = q.url.split("?")[0];
    if (files[path]) {
      s.setHeader("content-type", "text/javascript");
      s.end(files[path]);
    } else {
      s.setHeader("content-type", "text/html");
      s.end(page);
    }
  });
  await new Promise((r) => srv.listen(APP_PORT, r));
  const browser = await puppeteer.launch({
    executablePath: chromium(),
    headless: true,
    args: ["--no-sandbox", `--ignore-certificate-errors-spki-list=${process.env.SPKI}`],
  });
  try {
    const p = await browser.newPage();
    await p.goto(APP_ORIGIN + "/");
    let clicked = false;
    for (let i = 0; i < 48; i++) {
      await new Promise((r) => setTimeout(r, 250));
      if (!clicked && /oauth2\/v2\.0\/authorize/.test(p.url())) {
        const btn = await p.$(`button[value="${ADMIN}"]`);
        if (btn) {
          clicked = true;
          await btn.click();
        }
      }
      const text = await p.evaluate(() => document.getElementById("out")?.textContent ?? "").catch(() => "");
      if (done.some((d) => text.includes(d))) return text.trim();
    }
    return (await p.evaluate(() => document.getElementById("out")?.textContent ?? "").catch(() => "")).trim() || "TIMEOUT at " + p.url();
  } finally {
    await browser.close();
    srv.close();
  }
}
