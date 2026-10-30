// The shapes that the admin API of Graph (/_dolly/graph) sends. They follow graph/*.go and graph/repo/*.go.

export interface Call {
  id: number;
  batch?: number; // the ID of the $batch call that this call is a part of
  index?: number;
  seq: number;
  time: string;
  method: string;
  path: string;
  query?: string;
  status: number;
  ms: number;
  bytes: number;
  ru?: number; // the cost, in resource units
  consistency?: string;
  agent?: string;
  fault?: string; // why dolly changed the answer
  request?: string;
  response?: string;
}

export interface ValueCount {
  value: string;
  count: number;
}

export interface State {
  seed: number;
  users: number;
  groups: number;
  servicePrincipals: number;
  departments: ValueCount[] | null;
  jobTitles: ValueCount[] | null;
  offices: ValueCount[] | null;
}

/** A pointer to another object. */
export interface Ref {
  id: string;
  name: string;
}

export interface UserInfo {
  id: string;
  displayName: string;
  userPrincipalName: string;
  department: string;
  jobTitle: string;
  officeLocation: string;
  accountEnabled: boolean;
  userType: string;
  /** The other text properties of the user. A property that the user does not have is an empty string. */
  fields: Record<string, string>;
}

export type GroupKind = "assigned" | "dynamic" | "microsoft365";

export interface GroupRef extends Ref {
  kind: GroupKind;
}

/** A user in the tree of the organisation. `reports` is how many people report to the user, so a node knows if it can open. */
export interface Person {
  id: string;
  name: string;
  jobTitle: string;
  reports: number;
}

/** A user with what it is related to. */
export interface UserDetail extends UserInfo {
  manager: Ref | null;
  /** The managers above the user, the top first. Empty for the head of the company. */
  chain: Person[];
  directReports: Person[];
  groups: GroupRef[];
}

/** An application that a user is assigned to: the roles, directly or through a group. */
export interface UserApp {
  appId: string;
  name: string;
  roles: string[];
  via?: string; // the name of the group that has the assignment
}

/** A consent to delegated permissions that applies to a user. */
export interface UserConsent {
  appId: string;
  name: string;
  resource: string;
  scopes: string[];
  forAll: boolean;
}

export interface UserApps {
  assigned: UserApp[];
  consents: UserConsent[];
}

export interface GroupInfo {
  id: string;
  name: string;
  kind: GroupKind;
  users: number;
  groups: number;
  others: number; // service principals
  dynamic: boolean;
  rule?: string;
  state?: "On" | "Paused";
}

export interface GroupDetail extends GroupInfo {
  description?: string;
  mailNickname?: string;
  securityEnabled: boolean;
  mailEnabled: boolean;
  roleAssignable: boolean;
  created?: string;
  owners: Ref[];
  memberOf: Ref[];
}

export interface Member {
  id: string;
  type: "user" | "group" | "servicePrincipal";
  name: string;
  detail?: string;
}

export interface Organization {
  id: string;
  displayName: string;
  tenantType: string;
  city?: string;
  country?: string;
  countryLetterCode?: string;
  preferredLanguage?: string;
  createdDateTime?: string;
  verifiedDomains: { name: string; type: string; isDefault: boolean; isInitial: boolean; capabilities: string }[];
}

export interface Actor {
  source: string; // admin, graph (a tool through the Graph API), or login
  agent?: string;
  call?: number;
}

export interface HistoryEvent {
  seq: number;
  time: string;
  type: string;
  actor: Actor;
  text: string;
}

export interface Endpoint {
  method: string;
  path: string;
  area: string;
  summary: string;
  writes: boolean;
  query?: string[]; // the query options that Dolly answers
  doc?: string; // the page of Microsoft Learn about the call
  schema?: string; // the schema of the objects, for the properties of $filter
  graphQuery?: string[]; // the query options that the description of Graph lists for the call
  graphHeaders?: string[];
  /** The key of the page of the reference that the call is on. The server sets it from the markdown files. */
  resource?: string;
  /** The section that the call is in, when its page only covers it (/me on the page of the users). */
  section?: string;
  /** The text of the call, from the markdown files. */
  notes?: CallNotes;
  calls: number;
  errors: number;
  lastStatus?: number;
}

/** The text that a markdown file has for a call: the part before the first title, and the parts with a title. */
export interface CallNotes {
  key: string;
  body: string; // markdown
  sections?: { title: string; body: string }[];
}

/** A page of the reference, from a markdown file. */
export interface PageDoc {
  resource: string;
  name: string;
  order: number;
  description: string; // markdown
}

/** One form of $filter that Dolly answers. The text comes from the table that answers it. */
export interface FilterForm {
  name: string;
  syntax: string;
  example: string;
  note: string;
  needsConsistency: boolean;
}

export interface SearchForm {
  syntax: string;
  example: string;
  note: string;
  fields: string[];
  needsConsistency: boolean;
}

/** A property that Graph says $filter works on, with the words of Graph about it. */
export interface FilterProperty {
  name: string;
  type: string;
  filter: string;
}

/** When an operator works on a property of a directory object. Microsoft's table says it. */
export type Mode = "default" | "default+advanced" | "defaultOnly" | "advanced" | "none";

/** A property of a directory object, with what $filter and $orderby need on it. */
export interface FilterRule {
  property: string;
  operators: Record<string, Mode>; // eq, startsWith, ge/le, eqNull, count0, count1
  orderby?: Mode;
}

export interface QueryOption {
  name: string;
  text: string;
  /** The forms of the option, written like the help of a command. */
  usage?: { syntax: string; note: string; example: string }[];
}

/** The answer of GET /api: the operations, and what to say about them. */
export interface ApiDocs {
  endpoints: Endpoint[];
  pages: PageDoc[];
  unsupported: UnsupportedCall[] | null;
  options: QueryOption[];
  filterForms: FilterForm[];
  searchForm: SearchForm;
  filterProperties: Record<string, FilterProperty[]>;
  /** For a directory object (user, group, servicePrincipal): the rules of Graph for each property. */
  filterRules: Record<string, FilterRule[]>;
}

export interface UnsupportedCall {
  method: string;
  path: string;
  calls: number;
  status: number;
}

export interface CredState {
  state: "any" | "armed" | "captured";
  kind?: string;
  hint?: string;
  at?: string;
}

export interface ServicePrincipal {
  id: string;
  appId: string;
  displayName: string;
  auto: boolean;
  builtin?: string;
  mode: "open" | "enforce";
  credential: CredState;
  system: boolean;
  exemptFromThrottle: boolean;
  hideCalls: boolean;
}

export interface Usage {
  appId: string;
  calls: number;
  ru: number;
  throttled: number;
}

export interface Behavior {
  latencyMs: number;
  failPercent: number;
  failStatus: number;
  retryAfter: number;
  unauthorized: boolean;
  pageSize: number;
  throttle: boolean;
}

export type Kind = "delegated" | "application";

export interface Resource {
  id: string;
  appId: string;
  displayName: string;
  namespace: string;
  builtin?: string;
  application: number;
  delegated: number;
}

export interface Permission {
  id: string;
  value: string;
  displayName: string;
  description: string;
  admin: boolean;
  scope: string; // what the client asks for: https://graph.microsoft.com/User.Read, or .../.default
}

export interface AppGrant extends Permission {
  assignmentId: string;
  resourceId: string;
  resourceAppId: string;
  resourceName: string;
}

export interface DelegatedGrant {
  grantId: string;
  resourceId: string;
  resourceAppId: string;
  resourceName: string;
  consentType: "AllPrincipals" | "Principal";
  principalId?: string;
  principalName?: string;
  permissions: Permission[];
  scope: string; // the scope parameter that asks for all of them
}

export interface Grants {
  application: AppGrant[];
  delegated: DelegatedGrant[];
}
