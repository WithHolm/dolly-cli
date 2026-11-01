// One case for each way to use a library with dollycli. A case that works has `expect: "ok"`. A case that must stop (a library that
// refuses http, for example) has `expect: "fail"` and `text`, a piece of the error. run.mjs checks every case against its expectation,
// so a change in dollycli (or in a new version of a library) that changes a result shows up as a failed row.
import { readFileSync } from "node:fs";
import http from "node:http";
import { ClientSecretCredential, UsernamePasswordCredential } from "@azure/identity";
import { ConfidentialClientApplication } from "@azure/msal-node";
import * as client from "openid-client";
import { ADMIN, APP_ORIGIN, BASE, CLIENT, chromium, pick, tenantId, webApp } from "./lib.mjs";

const lib = (path) => readFileSync(new URL(`./node_modules/${path}`, import.meta.url));
const REDIRECT = APP_ORIGIN + "/callback";
const SCOPES = ["https://graph.microsoft.com/User.Read", "openid", "offline_access"];

const clean = (e) => `${e.errorCode ?? e.name}: ${String(e.message).replace(/\s+/g, " ").slice(0, 160)}`;
async function expectMe(base, token) {
  const me = await (await fetch(`${base}/v1.0/me`, { headers: { Authorization: `Bearer ${token}` } })).json();
  if (me.userPrincipalName !== ADMIN) throw new Error(`/me is ${me.userPrincipalName}, not ${ADMIN}`);
}

// ---- MSAL Node ----
async function msalNode(transport, extra) {
  const base = BASE[transport];
  try {
    const cca = new ConfidentialClientApplication({
      auth: { clientId: CLIENT, clientSecret: "anything", authority: `${base}/${await tenantId(base)}`, ...extra(base) },
    });
    const url = await cca.getAuthCodeUrl({ scopes: SCOPES, redirectUri: REDIRECT, state: "s1" });
    const code = new URL(await pick(url, ADMIN)).searchParams.get("code");
    const res = await cca.acquireTokenByCode({ code, scopes: SCOPES, redirectUri: REDIRECT });
    await expectMe(base, res.accessToken);
    return `${res.account.username}, /me is the same user`;
  } catch (e) {
    throw new Error(clean(e));
  }
}

// ---- @azure/identity ----
async function identity(transport, options, make) {
  const base = BASE[transport];
  try {
    const tid = await tenantId(base);
    const credential = make(tid, { authorityHost: base, ...options });
    const token = await credential.getToken(make.scope);
    const users = await fetch(`${base}/v1.0/users?$top=1`, { headers: { Authorization: `Bearer ${token.token}` } });
    if (users.status !== 200) throw new Error(`Graph answered ${users.status}`);
    return "token for Graph, /users is 200";
  } catch (e) {
    throw new Error(clean(e));
  }
}
const secret = Object.assign((tid, o) => new ClientSecretCredential(tid, CLIENT, "anything", o), { scope: "https://graph.microsoft.com/.default" });
const userPassword = Object.assign((tid, o) => new UsernamePasswordCredential(tid, CLIENT, ADMIN, "any password", o), {
  scope: "https://graph.microsoft.com/User.Read",
});

// ---- openid-client ----
async function openidClient(options) {
  const base = BASE.http;
  try {
    const config = await client.discovery(new URL(`${base}/${await tenantId(base)}/v2.0`), CLIENT, { client_secret: "anything" }, undefined, options);
    const verifier = client.randomPKCECodeVerifier();
    const state = client.randomState();
    const nonce = client.randomNonce();
    const url = client.buildAuthorizationUrl(config, {
      redirect_uri: REDIRECT,
      scope: "openid offline_access https://graph.microsoft.com/User.Read",
      code_challenge: await client.calculatePKCECodeChallenge(verifier),
      code_challenge_method: "S256",
      state,
      nonce,
    });
    const tokens = await client.authorizationCodeGrant(config, new URL(await pick(url.href, ADMIN)), {
      pkceCodeVerifier: verifier,
      expectedState: state,
      expectedNonce: nonce,
    });
    await expectMe(base, tokens.access_token);
    const again = await client.refreshTokenGrant(config, tokens.refresh_token);
    if (!again.access_token) throw new Error("the refresh gave no token");
    return `${tokens.claims().preferred_username}, ID token checked, refresh works`;
  } catch (e) {
    throw new Error(clean(e));
  }
}

// ---- browser libraries (they need Chromium) ----
async function msalBrowser(transport) {
  const base = BASE[transport];
  const tid = await tenantId(base);
  const page = `<!doctype html><meta charset=utf-8><script src="/lib.js"></script><pre id=out></pre><script>
const out = (t) => (document.getElementById("out").textContent += t + "\\n");
(async () => {
  try {
    const app = new msal.PublicClientApplication({ auth: { clientId: "${CLIENT}", authority: "${base}/${tid}", knownAuthorities: ["${new URL(base).host}"], redirectUri: location.origin + "/" } });
    await app.initialize();
    const r = await app.handleRedirectPromise();
    if (r) return out("TOKEN " + r.account.username);
    if (sessionStorage.getItem("started")) return out("ERROR the redirect gave no result");
    sessionStorage.setItem("started", "1");
    await app.loginRedirect({ scopes: ["User.Read"], loginHint: "${ADMIN}" });
  } catch (e) { out("ERROR " + (e.errorCode || e.name) + ": " + String(e.message).slice(0, 120)); }
})();
</script>`;
  const text = await webApp({ page, files: { "/lib.js": lib("@azure/msal-browser/lib/msal-browser.min.js") }, done: ["TOKEN", "ERROR"] });
  if (!text.startsWith(`TOKEN ${ADMIN}`)) throw new Error(text);
  return text;
}

async function oidcClientTs(transport, silent) {
  const base = BASE[transport];
  const tid = await tenantId(base);
  const origin = APP_ORIGIN;
  const page = `<!doctype html><meta charset=utf-8><script src="/lib.js"></script><pre id=out></pre><script>
// the app goes through several pages (sign-in, callback, sign-out): keep the lines in sessionStorage, and show all of them
const lines = JSON.parse(sessionStorage.getItem("lines") || "[]");
const show = () => (document.getElementById("out").textContent = lines.join("\\n"));
const out = (t) => { lines.push(t); sessionStorage.setItem("lines", JSON.stringify(lines)); show(); };
show();
const settings = { authority: "${base}/${tid}/v2.0", client_id: "${CLIENT}", redirect_uri: "${origin}/cb", silent_redirect_uri: "${origin}/silent",
  post_logout_redirect_uri: "${origin}/bye", scope: ${silent ? '"openid profile User.Read"' : '"openid profile offline_access User.Read"'}, response_type: "code",
  loadUserInfo: false, automaticSilentRenew: false, userStore: new oidc.WebStorageStateStore({ store: sessionStorage }) };
(async () => {
  const um = new oidc.UserManager(settings);
  try {
    if (location.pathname === "/silent") return await um.signinSilentCallback();
    if (location.pathname === "/bye") return out("SIGNED OUT");
    if (location.pathname === "/cb") {
      const u = await um.signinCallback();
      out("USER " + u.profile.preferred_username + " refresh=" + !!u.refresh_token);
      const me = await fetch("${base}/v1.0/me", { headers: { Authorization: "Bearer " + u.access_token } }).then((r) => r.json());
      out("ME " + me.userPrincipalName);
      ${silent ? 'try { const s = await um.signinSilent(); out("SILENT OK " + s.profile.preferred_username); } catch (e) { out("SILENT FAIL " + e.message); }' : ""}
      return await um.signoutRedirect();
    }
    await um.signinRedirect();
  } catch (e) { out("ERROR " + e.message); }
})();
</script>`;
  const text = await webApp({ page, files: { "/lib.js": lib("oidc-client-ts/dist/browser/oidc-client-ts.min.js") }, done: ["SIGNED OUT", "ERROR"] });
  const oneLine = text.replace(/\n/g, " | ");
  if (!text.includes(`USER ${ADMIN}`) || !text.includes(`ME ${ADMIN}`) || !text.includes("SIGNED OUT") || text.includes("SILENT FAIL")) throw new Error(oneLine);
  return oneLine;
}

// ---- CORS: what a web page may do from the browser ----
function preflight(origin) {
  return new Promise((resolve, reject) => {
    const u = new URL(`${BASE.http}/v1.0/me`);
    const req = http.request(
      { host: u.hostname, port: u.port, path: u.pathname, method: "OPTIONS", headers: { Origin: origin, "Access-Control-Request-Method": "GET", "Access-Control-Request-Headers": "authorization" } },
      (res) => {
        res.resume();
        resolve(res.headers["access-control-allow-origin"] ?? "");
      },
    );
    req.on("error", reject);
    req.end();
  });
}

const browser = { skip: () => (chromium() ? "" : "no Chromium (set CHROMIUM)") };

export const cases = [
  { id: "msal-node-http", lib: "MSAL Node", transport: "http", setting: "none", expect: "fail", text: "authority_uri_insecure", run: () => msalNode("http", () => ({})) },
  { id: "msal-node-https-plain", lib: "MSAL Node", transport: "https", setting: "none", expect: "fail", text: "endpoints_resolution_error", run: () => msalNode("https", () => ({})) },
  { id: "msal-node-https", lib: "MSAL Node", transport: "https", setting: "knownAuthorities", expect: "ok", run: () => msalNode("https", (b) => ({ knownAuthorities: [new URL(b).host] })) },
  { id: "identity-http", lib: "@azure/identity", transport: "http", setting: "none", expect: "fail", text: "must use the 'https' protocol", run: () => identity("http", {}, secret) },
  { id: "identity-https-plain", lib: "@azure/identity", transport: "https", setting: "none", expect: "fail", text: "endpoints_resolution_error", run: () => identity("https", {}, secret) },
  { id: "identity-https-secret", lib: "@azure/identity", transport: "https", setting: "disableInstanceDiscovery", expect: "ok", run: () => identity("https", { disableInstanceDiscovery: true }, secret) },
  { id: "identity-https-password", lib: "@azure/identity", transport: "https", setting: "disableInstanceDiscovery", expect: "ok", run: () => identity("https", { disableInstanceDiscovery: true }, userPassword) },
  { id: "openid-client-http-plain", lib: "openid-client", transport: "http", setting: "none", expect: "fail", text: "", run: () => openidClient(undefined) },
  { id: "openid-client-http", lib: "openid-client", transport: "http", setting: "allowInsecureRequests", expect: "ok", run: () => openidClient({ execute: [client.allowInsecureRequests] }) },
  { id: "msal-browser-http", lib: "MSAL.js", transport: "http", setting: "none", expect: "fail", text: "authority_uri_insecure", ...browser, run: () => msalBrowser("http") },
  { id: "msal-browser-https", lib: "MSAL.js", transport: "https", setting: "knownAuthorities", expect: "ok", ...browser, run: () => msalBrowser("https") },
  { id: "oidc-client-ts-http", lib: "oidc-client-ts", transport: "http", setting: "none", expect: "ok", ...browser, run: () => oidcClientTs("http", false) },
  { id: "oidc-client-ts-silent-http", lib: "oidc-client-ts (silent, hidden frame)", transport: "http", setting: "none", expect: "fail", text: "login_required", ...browser, run: () => oidcClientTs("http", true).catch((e) => { throw new Error(e.message.includes("AADSTS50058") ? "login_required: " + e.message : e.message); }) },
  { id: "oidc-client-ts-silent-https", lib: "oidc-client-ts (silent, hidden frame)", transport: "https", setting: "none", expect: "ok", ...browser, run: () => oidcClientTs("https", true) },
  {
    id: "cors-localhost", lib: "CORS, page on localhost", transport: "http", setting: "none", expect: "ok",
    run: async () => {
      const allowed = await preflight(APP_ORIGIN);
      if (allowed !== APP_ORIGIN) throw new Error(`Access-Control-Allow-Origin is "${allowed}"`);
      return "preflight allowed";
    },
  },
  {
    id: "cors-foreign", lib: "CORS, page on another origin", transport: "http", setting: "none", expect: "fail", text: "no CORS header",
    run: async () => {
      const allowed = await preflight("https://evil.example");
      if (allowed) throw new Error(`Access-Control-Allow-Origin is "${allowed}"`);
      throw new Error("no CORS header");
    },
  },
];
