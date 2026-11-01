import assert from "node:assert/strict";
import { test } from "node:test";
import { ENTERPRISE_TAG, isMicrosoftOrBuiltIn, missingTagAdvice, registrationText } from "./identity.ts";

const plain = { enterprise: false, builtin: undefined, system: false };

test("missingTagAdvice: only a service principal with no tag, that is not built in", () => {
  assert.match(missingTagAdvice(plain), new RegExp(ENTERPRISE_TAG));
  assert.match(missingTagAdvice(plain), /Enterprise applications/);
  assert.equal(missingTagAdvice({ ...plain, enterprise: true }), "");
  assert.equal(missingTagAdvice({ ...plain, system: true }), "");
  assert.equal(missingTagAdvice({ ...plain, builtin: "graph" }), "");
});

test("registrationText: the app registration is in this tenant, at Microsoft, or in another tenant", () => {
  assert.equal(registrationText({ application: "x", builtin: undefined, system: false }), "");
  assert.equal(registrationText({ application: undefined, builtin: "graph", system: false }), "Registered by Microsoft");
  assert.match(registrationText({ application: undefined, builtin: undefined, system: false }), /another tenant/);
  assert.equal(isMicrosoftOrBuiltIn({ builtin: undefined, system: true }), true);
});
