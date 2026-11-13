/** The Application ID URI that Entra suggests: api:// and the client ID. */
export const defaultIdentifierUri = (appId: string): string => `api://${appId}`;

/**
 * What a client asks for: the Application ID URI, a slash, and the value of the scope. An app with no URI has the one that dollycli takes for it (the same as the
 * default above), so a client can ask for its scopes before the person has set one.
 */
export function scopeFullName(identifierUris: readonly string[], appId: string, value: string): string {
  const uri = (identifierUris[0] ?? defaultIdentifierUri(appId)).replace(/\/+$/, "");
  return `${uri}/${value}`;
}

/**
 * The audience (aud) that an access token for this API has. Version 1 (null is 1): the Application ID URI, or the client ID when the app has none. Version 2: the
 * client ID. A library that validates the tokens of the API must expect this one.
 */
export const accessTokenAudience = (version: number | null, identifierUris: readonly string[], appId: string): string =>
  version === 2 ? appId : (identifierUris[0] ?? appId);

/** Who can consent to a scope: the type User is for users and administrators, the type Admin for administrators only. */
export const consentWho = (type: "User" | "Admin"): string => (type === "User" ? "Admins and users" : "Admins only");
