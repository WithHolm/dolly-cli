import assert from "node:assert/strict";
import { test } from "node:test";
import { accessTokenAudience, consentWho, defaultIdentifierUri, scopeFullName } from "./scopes.ts";

const app = "081f3a64-5873-444a-a3c9-e9697fbd33bf";

test("defaultIdentifierUri: the usual Application ID URI is api:// and the client ID", () => {
  assert.equal(defaultIdentifierUri(app), `api://${app}`);
});

test("scopeFullName: what a client asks for is the Application ID URI, a slash, and the value", () => {
  assert.equal(scopeFullName(["api://files-api"], app, "Files.Read"), "api://files-api/Files.Read");
  assert.equal(scopeFullName(["https://files.example/api/"], app, "Files.Read"), "https://files.example/api/Files.Read");
});

test("scopeFullName: with no URI the app has the one that dollycli takes: api:// and the client ID", () => {
  assert.equal(scopeFullName([], app, "Files.Read"), `api://${app}/Files.Read`);
});

test("accessTokenAudience: version 1 (and null) has the Application ID URI, version 2 the client ID", () => {
  assert.equal(accessTokenAudience(null, ["api://files-api"], app), "api://files-api");
  assert.equal(accessTokenAudience(1, ["api://files-api"], app), "api://files-api");
  assert.equal(accessTokenAudience(2, ["api://files-api"], app), app);
});

test("accessTokenAudience: a version 1 token of an app with no URI has the client ID", () => {
  assert.equal(accessTokenAudience(null, [], app), app);
});

test("consentWho: a type User is for admins and users, a type Admin for admins only", () => {
  assert.equal(consentWho("User"), "Admins and users");
  assert.equal(consentWho("Admin"), "Admins only");
});
