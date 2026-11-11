import assert from "node:assert/strict";
import { test } from "node:test";
import { assignableRoles, DEFAULT_ACCESS, memberTypeFor, roleLabel } from "./roles.ts";
import type { AppRoleDef } from "./types.ts";

const role = (id: string, displayName: string, value: string | null, types: AppRoleDef["allowedMemberTypes"], isEnabled = true): AppRoleDef => ({
  id,
  displayName,
  description: "",
  value,
  allowedMemberTypes: types,
  isEnabled,
  origin: "Application",
});

const roles = [
  role("1", "Reader", "Reader", ["User"]),
  role("2", "Sync", "Sync.All", ["Application"]),
  role("3", "Both", "Both", ["User", "Application"]),
  role("4", "Old", "Old", ["User"], false),
];

test("memberTypeFor: a group has the type of a user, and a service principal has the type Application", () => {
  assert.equal(memberTypeFor("User"), "User");
  assert.equal(memberTypeFor("Group"), "User");
  assert.equal(memberTypeFor("ServicePrincipal"), "Application");
});

test("assignableRoles: leaves out a disabled role and a role of the other type", () => {
  assert.deepEqual(assignableRoles(roles, "User").map((r) => r.id), ["1", "3"]);
  assert.deepEqual(assignableRoles(roles, "Group").map((r) => r.id), ["1", "3"]);
  assert.deepEqual(assignableRoles(roles, "ServicePrincipal").map((r) => r.id), ["2", "3"]);
});

test("roleLabel: the name of the role, with its value when the value is not the name", () => {
  assert.equal(roleLabel(roles, "2"), "Sync (Sync.All)");
  assert.equal(roleLabel(roles, "1"), "Reader");
  assert.equal(roleLabel([role("5", "msiam_access", null, ["User"])], "5"), "msiam_access");
});

test("roleLabel: Default access, and a role that is gone", () => {
  assert.equal(roleLabel(roles, DEFAULT_ACCESS), "Default access");
  assert.equal(roleLabel(roles, "99"), "99");
});
