import type { AppRoleDef, RoleAssignment } from "./types";

/** The role ID of an assignment with no app role: the user can sign in, and has no role. */
export const DEFAULT_ACCESS = "00000000-0000-0000-0000-000000000000";

/** The member type that a kind of principal needs in the allowedMemberTypes of a role: a group is assigned a role as a user is. */
export const memberTypeFor = (principalType: RoleAssignment["principalType"]): "User" | "Application" =>
  principalType === "ServicePrincipal" ? "Application" : "User";

/** The roles that can be assigned to this kind of principal: enabled, and for its member type. The server checks the same. */
export const assignableRoles = (roles: readonly AppRoleDef[], principalType: RoleAssignment["principalType"]): AppRoleDef[] =>
  roles.filter((r) => r.isEnabled && r.allowedMemberTypes.includes(memberTypeFor(principalType)));

/** How the page names a role: its name, with its value when the value is not the name. */
export function roleLabel(roles: readonly AppRoleDef[], id: string): string {
  if (id === DEFAULT_ACCESS) return "Default access";
  const r = roles.find((x) => x.id.toLowerCase() === id.toLowerCase());
  if (!r) return id;
  return r.value && r.value !== r.displayName ? `${r.displayName} (${r.value})` : r.displayName;
}
