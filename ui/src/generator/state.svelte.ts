import { ApiError, graph } from "../lib/api";
import { namesForLocale } from "./countries";
import { fieldName, normalize, parseIssues, type Preview, type Profile } from "./profile";

// The state of the wizard, shared by its steps. The work stays in the tab when the page is loaded again (sessionStorage).

const KEEP = "dollycli-generator";

/** The step that a path of the profile is edited in. A path that no step has gives "". */
export function stepOf(path: string): string {
  if (path === "organization.guestDomain") return "people"; // the guest domains are with the people
  if (path.startsWith("organization")) return "organization";
  if (path.startsWith("offices")) return "offices";
  if (path.startsWith("departments") || path === "people.managerWord") return "departments"; // the manager word is with the job titles
  if (path.startsWith("names") || path.startsWith("people")) return "people";
  if (path.startsWith("formats")) return "formats";
  if (path.startsWith("extensionAttributes")) return "attributes";
  if (path.startsWith("owner") || path.startsWith("admin")) return "admin";
  return "";
}

const organizationLabels: Record<string, string> = {
  displayName: "Display name",
  companyName: "Company name",
  domain: "Default domain",
  initialDomain: "Tenant name",
  domainWeight: "Default domain, weight",
  domains: "Domains",
  guestDomain: "Guest domain",
  street: "Street",
  postalCode: "Postal code",
  city: "City",
  state: "State",
  country: "Country",
  countryLetterCode: "Country code",
  usageLocation: "Usage location",
  preferredLanguage: "Language",
  phone: "Phone",
};

class Wizard {
  profile = $state<Profile | null>(null);
  users = $state("200");
  seed = $state("1");
  preview = $state.raw<Preview | null>(null);
  problem = $state("");
  /** The steps where the person pressed Next. A step shows its errors after that, and not while the person is still filling it in. */
  tried = $state<Record<string, boolean>>({});
  #sent = 0;

  constructor() {
    try {
      const v = JSON.parse(sessionStorage.getItem(KEEP) ?? "null");
      if (v && typeof v.profile === "object" && v.profile !== null) {
        this.profile = normalize(v.profile, namesForLocale);
        this.users = String(v.users ?? "200");
        this.seed = String(v.seed ?? "1");
        this.tried = v.tried ?? {};
      }
    } catch {
      // nothing kept, or not readable: start from the cards
    }
  }

  readonly issues = $derived(
    parseIssues(this.problem).map((i) => ({ ...i, label: i.path ? fieldName(i.path, organizationLabels) : "", step: stepOf(i.path) })),
  );

  issuesOf(step: string) {
    return this.issues.filter((i) => i.step === step || (step === "" && i.step === ""));
  }

  /** A field is marked when it has a problem, and its step was tried. */
  bad(path: string): true | undefined {
    const hit = this.issues.some((i) => (i.path === path || i.path.startsWith(path + "[") || i.path.startsWith(path + ".")) && this.tried[i.step]);
    return hit ? true : undefined;
  }

  start(profile: Profile) {
    this.profile = normalize(profile, namesForLocale);
    this.preview = null;
    this.problem = "";
    this.tried = {};
  }

  startOver() {
    this.profile = null;
    this.preview = null;
    this.problem = "";
    this.tried = {};
  }

  body() {
    return { profile: this.profile, users: Number(this.users) || 0, seed: Number(this.seed) || 0 };
  }

  save() {
    if (this.profile) sessionStorage.setItem(KEEP, JSON.stringify({ profile: this.profile, users: this.users, seed: this.seed, tried: this.tried }));
    else sessionStorage.removeItem(KEEP);
  }

  /** Ask the server what the profile makes. Only the newest answer counts. */
  async validate(payload = JSON.stringify(this.body())): Promise<void> {
    const mine = ++this.#sent;
    try {
      const pv = await graph.post<Preview>("/profile/preview", JSON.parse(payload));
      if (mine === this.#sent) [this.preview, this.problem] = [pv, ""];
    } catch (e) {
      if (mine === this.#sent) this.problem = e instanceof ApiError ? e.message : String(e);
    }
  }

  /** The preview follows the profile: a short wait after the last change. It gives the function that stops the wait. */
  schedule(payload: string): () => void {
    const timer = setTimeout(() => void this.validate(payload), 400);
    return () => clearTimeout(timer);
  }
}

export const wiz = new Wizard();
