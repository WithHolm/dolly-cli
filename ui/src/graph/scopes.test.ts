import assert from "node:assert/strict";
import { test } from "node:test";
import { consentWho, defaultIdentifierUri, scopeFullName } from "./scopes.ts";

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

test("consentWho: a type User is for admins and users, a type Admin for admins only", () => {
  assert.equal(consentWho("User"), "Admins and users");
  assert.equal(consentWho("Admin"), "Admins only");
});
