// What an app registration and a service principal are, in words. They are two objects with the same client ID (appId) and different object IDs:
// the app registration is the definition, in its home tenant only, and the service principal is the instance of the app in a tenant.

import type { ServicePrincipal } from "./types";

/** The tag that makes a service principal an enterprise application in Entra. */
export const ENTERPRISE_TAG = "WindowsAzureActiveDirectoryIntegratedApp";

type Flags = Pick<ServicePrincipal, "enterprise" | "builtin" | "system">;

/** The service principals of Microsoft (Microsoft Graph, the clients of Microsoft, the portal of dollycli) are left out of the advice about the tag. */
export const isMicrosoftOrBuiltIn = (sp: Pick<ServicePrincipal, "builtin" | "system">): boolean => !!sp.builtin || sp.system;

/** The advice for a service principal that has no enterprise tag. Empty when it has the tag, or when it is a built-in one. */
export function missingTagAdvice(sp: Flags): string {
  if (sp.enterprise || isMicrosoftOrBuiltIn(sp)) return "";
  return `It does not have the tag ${ENTERPRISE_TAG}, so Entra does not show it under Enterprise applications. Add the tag to the service principal to see it there: PATCH /servicePrincipals/{id} with the tags.`;
}

/** Where the app is registered, for the overview. */
export const registrationText = (sp: Pick<ServicePrincipal, "application" | "builtin" | "system">): string =>
  sp.application ? "" : isMicrosoftOrBuiltIn(sp) ? "Registered by Microsoft" : "Registered in another tenant: this tenant has the service principal only";
