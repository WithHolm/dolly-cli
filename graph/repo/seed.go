package repo

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
)

// Department has the job titles that people in it can have. This is the set of the default profile (profile.go), so a demo or a
// test knows the values. Another profile has other departments. Names, phones, and IDs are made up.
type Department struct {
	Name   string
	Titles []string
	Weight int // how common the department is
}

// Departments is the set of departments of the default profile.
var Departments = func() []Department {
	var out []Department
	for _, d := range DefaultProfile().Departments {
		titles := make([]string, len(d.Titles))
		for i, t := range d.Titles {
			titles[i] = t.Name
		}
		out = append(out, Department{d.Name, titles, d.Weight})
	}
	return out
}()

// Office is an office location.
type Office struct {
	Name, City, Country, Usage, Postal string
}

// Offices is the set of office locations of the default profile.
var Offices = func() []Office {
	var out []Office
	for _, o := range DefaultProfile().Offices {
		out = append(out, Office{o.Name, o.City, o.Country, o.Usage, o.Postal})
	}
	return out
}()

var firstNames = strings.Fields(`Anna Erik Maria Lars Sofia Johan Elin Oskar Ida Nils Klara Anders Linnea Mats Hanna
	Viktor Emma Henrik Julia Fredrik Alice Magnus Sara Daniel Frida Karl Nora Simon Tilda Jonas Olivia Peter Maja
	Gustav Elsa Patrik Agnes David Lena Axel`)

var lastNames = strings.Fields(`Andersson Johansson Karlsson Nilsson Eriksson Larsson Olsson Persson Svensson Gustafsson
	Pettersson Jonsson Jansson Hansson Bengtsson Lindberg Lindqvist Lindgren Berg Axelsson Bergstrom Lundberg
	Lundgren Mattsson Berglund Fredriksson Sandberg Henriksson Forsberg Sjoberg Wallin Engstrom Eklund Danielsson`)

// Options sets the size of a new world, and what it looks like.
type Options struct {
	Users int
	Seed  int64
	// Profile says what the tenant looks like. nil is the default profile. Check a profile from outside with ParseProfile first.
	Profile *Profile
}

type gen struct {
	rng *rand.Rand
	w   *World
	p   *Profile
}

// weighted picks the index of one item, by weight. One draw, so a list of equal weights picks as a plain choice from the list does.
func (g *gen) weighted(n int, weight func(i int) int) int {
	total := 0
	for i := 0; i < n; i++ {
		total += weight(i)
	}
	pick := g.rng.IntN(total)
	for i := 0; i < n; i++ {
		if pick < weight(i) {
			return i
		}
		pick -= weight(i)
	}
	return 0
}

func (g *gen) office() ProfileOffice {
	offices := g.p.Offices
	return offices[g.weighted(len(offices), func(i int) int { return offices[i].Weight })]
}

func (g *gen) title(d ProfileDepartment) string {
	return d.Titles[g.weighted(len(d.Titles), func(i int) int { return d.Titles[i].Weight })].Name
}

func (g *gen) language() string {
	langs := g.p.People.Languages
	return langs[g.weighted(len(langs), func(i int) int { return langs[i].Weight })].Code
}

// userLanguage is the language that was picked before the names, or a new pick (at the place in the draws where it always was).
func userLanguage(g *gen, picked string) string {
	if picked != "" {
		return picked
	}
	return g.language()
}

// dcOf writes a domain as the parts of a distinguished name: acme.example is DC=acme,DC=example.
func dcOf(domain string) string { return "DC=" + strings.ReplaceAll(domain, ".", ",DC=") }

func (g *gen) guid() string {
	var b [16]byte
	for i := range b {
		b[i] = byte(g.rng.UintN(256))
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (g *gen) pick(list []string) string { return list[g.rng.IntN(len(list))] }

// Generate makes a new world. The same options always make the same world.
func Generate(o Options) *World {
	if o.Users <= 0 {
		o.Users = 500
	}
	if o.Seed == 0 {
		o.Seed = 1
	}
	if o.Profile == nil {
		o.Profile = DefaultProfile()
	}
	// The seed keeps its profile, the default one too: a seed with no profile in it is read as Acme, the tenant of before Fjordvik (loadBytes).
	g := &gen{rng: rand.New(rand.NewPCG(uint64(o.Seed), 0x9e3779b97f4a7c15)), w: &World{Seed: o.Seed, Managers: map[string]string{}}, p: o.Profile}
	g.w.Profile = o.Profile
	g.users(o.Users)
	g.cast()       // the people of the cast that need the others to be there (their department, their office)...
	g.managers()   // ...before the manager tree and the groups are made from the users...
	g.castBosses() // ...and the links that the tree could not know about, after it
	g.groups()
	g.apps()
	g.places()
	g.w.Organization = newOrganization(o.Profile)
	g.sites() // after the organization: the host of the sites comes from it
	g.driveItems()
	if admin := adminUPN(o.Profile); admin != AdminUPN {
		g.w.Admin = admin // a world with the admin account of the default profile has no field
	}
	g.w.index()
	return g.w
}

// GenerateChecked is Generate for a profile that may not be valid. It checks the profile first, and gives the error that says which field.
func GenerateChecked(o Options) (*World, error) {
	if o.Profile != nil {
		if err := o.Profile.Clone().Validate(); err != nil {
			return nil, err
		}
	}
	return Generate(o), nil
}

func slug(s string) string {
	return strings.ToLower(strings.NewReplacer(" ", "-", "&", "and").Replace(s))
}

func (g *gen) department() ProfileDepartment {
	deps := g.p.Departments
	return deps[g.weighted(len(deps), func(i int) int { return deps[i].Weight })]
}

func (g *gen) users(n int) {
	taken := map[string]int{}
	for i := 0; i < n; i++ {
		// With names by language the language is picked first, and the names follow it. Without them the draws are in the order they always were.
		var lang, first, last string
		if byLanguage := len(g.p.Names.ByLanguage) > 0; byLanguage {
			lang = g.language()
			set := g.p.Names.nameSet(lang)
			first, last = g.pick(set.First), g.pick(set.Last)
		} else {
			first, last = g.pick(g.p.Names.First), g.pick(g.p.Names.Last)
		}
		nick := ascii(strings.ToLower(first)) + "." + ascii(strings.ToLower(last)) // no accents in an address: Håkon is hakon
		if taken[nick]++; taken[nick] > 1 {
			nick = fmt.Sprintf("%s%d", nick, taken[nick])
		}
		dept := g.department()
		office := g.office()
		guest := g.rng.IntN(100) < g.p.People.GuestPercent
		org := g.p.Organization
		// a domain is picked by weight only when there is a choice, so a profile with one domain draws the same numbers as before
		domain := org.Domain
		if ds := g.p.memberDomains(); !guest && len(ds) > 1 {
			domain = ds[g.weighted(len(ds), func(i int) int { return ds[i].Weight })].Name
		}
		upn := nick + "@" + domain
		mail := upn
		userType := "Member"
		if guest {
			userType = "Guest"
			guestDomain := g.p.guestDomains()[0].Name
			if ds := g.p.guestDomains(); len(ds) > 1 {
				guestDomain = ds[g.weighted(len(ds), func(i int) int { return ds[i].Weight })].Name
			}
			upn = fmt.Sprintf("%s_%s#EXT#@%s", nick, guestDomain, org.InitialDomain)
			mail = nick + "@" + guestDomain
		}
		synced := !guest && g.rng.IntN(100) < g.p.People.SyncedPercent
		u := Object{
			"id":                            g.guid(),
			"displayName":                   first + " " + last,
			"givenName":                     first,
			"surname":                       last,
			"userPrincipalName":             upn,
			"mail":                          mail,
			"mailNickname":                  nick,
			"department":                    dept.Name,
			"jobTitle":                      g.title(dept),
			"officeLocation":                office.Name,
			"city":                          office.City,
			"country":                       office.Country,
			"usageLocation":                 office.Usage,
			"postalCode":                    office.Postal,
			"companyName":                   org.CompanyName,
			"employeeId":                    numbered(g.p.Formats.EmployeeID.Pattern, g.p.Formats.EmployeeID.Start+i),
			"mobilePhone":                   g.pattern(g.p.Formats.MobilePhone, map[string]string{"cc": office.CallingCode}),
			"preferredLanguage":             userLanguage(g, lang),
			"accountEnabled":                g.rng.IntN(100) < g.p.People.EnabledPercent,
			"userType":                      userType,
			"passwordPolicies":              "None",
			"onPremisesSyncEnabled":         nil,
			"onPremisesExtensionAttributes": g.extensionAttributes(),
		}
		g.moreFields(u, i, dept, office, guest, mail)
		if synced {
			u["onPremisesSyncEnabled"] = true
			u["onPremisesDistinguishedName"] = fmt.Sprintf("CN=%s %s,OU=%s,%s", first, last, dept.Name, dcOf(org.Domain))
			u["onPremisesSecurityIdentifier"] = fmt.Sprintf("S-1-5-21-1004336348-1177238915-682003330-%d", 1000+i)
		}
		switch i {
		case n - 1:
			makeOwner(u, g.p) // the last user is the owner of this Dolly, so the count of users is as asked
		case n - 2:
			makeAdmin(u, g.p) // the one before is the admin account of the owner
		case n - 3:
			makeNora(u, g.p) // and the one before that is Nora Helmer, when there is room for her
		}
		g.w.Users = append(g.w.Users, u)
	}
}

// OwnerName and OwnerUPN are the user that every generated world has: Philip Meholm. His department and job title are the ones of the profile
// (Identitetsarkitekt in IT og digitalisering at Fjordvik, Tech Lead in Engineering at Acme). The examples of the portal use this user, and a tool
// that signs in as a user can use it too. The address is fake.
const (
	OwnerName = "Philip Meholm"
	OwnerUPN  = "philip.meholm@fjordvik.example" // in the default profile (Fjordvik); a profile says its own domain
)

// makeOwner turns a generated user into the owner. It draws nothing from the random numbers, so every other user of a
// seed is the same as it was before the owner was added. Only the fields that say who the person is change.
// The owner is always Philip Meholm. The profile says the domain of the address, and where he works.
func makeOwner(u Object, p *Profile) {
	upn := "philip.meholm@" + p.Organization.Domain
	u["displayName"], u["givenName"], u["surname"] = OwnerName, "Philip", "Meholm"
	u["userPrincipalName"], u["mail"], u["mailNickname"] = upn, upn, "philip.meholm"
	u["department"], u["jobTitle"] = p.Owner.Department, p.Owner.JobTitle
	u["accountEnabled"], u["userType"] = true, "Member"
	u["proxyAddresses"] = []any{"SMTP:" + upn}
	u["showInAddressList"], u["creationType"], u["externalUserState"] = true, nil, nil
	if org, ok := u["employeeOrgData"].(Object); ok {
		org["division"] = p.Owner.Department
	}
	if _, synced := u["onPremisesDistinguishedName"]; synced {
		u["onPremisesDistinguishedName"] = fmt.Sprintf("CN=%s,OU=%s,%s", OwnerName, p.Owner.Department, dcOf(p.Organization.Domain))
	}
}

// NoraName is the second person that every generated world has, when it has three users or more: Nora Helmer, from Ibsen's A Doll's House (a "doll's house"
// is where the name of this tool is from). Her address is nora.helmer@ and the domain of the profile. She is an ordinary member of the tenant.
const NoraName = "Nora Helmer"

// makeNora turns a generated user into Nora Helmer. Like makeOwner, it draws nothing from the random numbers, so every other user of a seed is the
// same as it was. Only the fields that say who she is change: the department, the job title, the office, and the rest stay what the seed gave the user.
func makeNora(u Object, p *Profile) {
	upn := "nora.helmer@" + p.Organization.Domain
	u["displayName"], u["givenName"], u["surname"] = NoraName, "Nora", "Helmer"
	u["userPrincipalName"], u["mail"], u["mailNickname"] = upn, upn, "nora.helmer"
	u["accountEnabled"], u["userType"] = true, "Member"
	u["proxyAddresses"] = []any{"SMTP:" + upn}
	u["showInAddressList"], u["creationType"], u["externalUserState"] = true, nil, nil
	if dept, ok := u["department"].(string); ok {
		if _, synced := u["onPremisesDistinguishedName"]; synced {
			u["onPremisesDistinguishedName"] = fmt.Sprintf("CN=%s,OU=%s,%s", NoraName, dept, dcOf(p.Organization.Domain))
		}
	}
}

// The rest of the cast of A Doll's House, besides Nora Helmer (who is made while the users are): Torvald Helmer is her boss, Nils Krogstad is a colleague with a
// disabled account, and Dr. Rank is a guest. They are the generated users four, five, and six from the end, when there is room. They draw nothing from the
// random numbers. The people of the story are made after the users and before the manager tree and the groups (cast), so the departments, the manager tree, and
// the groups agree with them. The links to Torvald are set after the tree is made (castBosses), where a loop is checked for.

// castAt gives the generated user that many places from the end (1 is the owner), or nil when there is no such user.
func (g *gen) castAt(fromEnd int) Object {
	if i := len(g.w.Users) - fromEnd; i >= 0 && i < len(g.w.Users) {
		return g.w.Users[i]
	}
	return nil
}

// colleagueOf makes u a colleague of nora: the same department and office. The groups of the department and the office are made later, from these.
func colleagueOf(u, nora Object) {
	dept := str(nora["department"])
	u["department"] = dept
	if org, ok := u["employeeOrgData"].(Object); ok {
		org["division"] = dept
	}
	for _, k := range []string{"officeLocation", "city", "country", "usageLocation", "postalCode", "state", "streetAddress"} {
		u[k] = nora[k]
	}
}

// makeMember gives a generated user the name of someone in the story, as an enabled member of the tenant.
func makeMember(u Object, first, last string, p *Profile) {
	nick := strings.ToLower(first + "." + last)
	upn := nick + "@" + p.Organization.Domain
	u["displayName"], u["givenName"], u["surname"] = first+" "+last, first, last
	u["userPrincipalName"], u["mail"], u["mailNickname"] = upn, upn, nick
	u["accountEnabled"], u["userType"] = true, "Member"
	u["proxyAddresses"] = []any{"SMTP:" + upn}
	u["showInAddressList"], u["creationType"], u["externalUserState"] = true, nil, nil
}

// syncedName writes the on-premises name of a synced user again, for the name and the department that the user has now.
func syncedName(u Object, p *Profile) {
	if _, synced := u["onPremisesDistinguishedName"]; synced {
		u["onPremisesDistinguishedName"] = fmt.Sprintf("CN=%s,OU=%s,%s", str(u["displayName"]), str(u["department"]), dcOf(p.Organization.Domain))
	}
}

// bossTitle gives the title of a department that says that its person leads (it has the manager word of the profile), or "" when none does.
func bossTitle(p *Profile, dept string) string {
	word := strings.ToLower(p.People.ManagerWord)
	for _, d := range p.Departments {
		if d.Name != dept {
			continue
		}
		for _, t := range d.Titles {
			if word != "" && strings.Contains(strings.ToLower(t.Name), word) {
				return t.Name
			}
		}
	}
	return ""
}

// cast makes Torvald Helmer, Nils Krogstad, and Dr. Rank from generated users, when there is room: Nora must be there (3 from the end), then Torvald (4), Nils (5), and Dr. Rank (6).
func (g *gen) cast() {
	nora := g.castAt(3)
	if nora == nil {
		return
	}
	if torvald := g.castAt(4); torvald != nil {
		makeMember(torvald, "Torvald", "Helmer", g.p)
		colleagueOf(torvald, nora)
		if title := bossTitle(g.p, str(nora["department"])); title != "" {
			torvald["jobTitle"] = title // a boss has the title of one: the manager tree takes the people with such a title as the managers of the department
		}
		syncedName(torvald, g.p)
	}
	if nils := g.castAt(5); nils != nil {
		makeMember(nils, "Nils", "Krogstad", g.p)
		colleagueOf(nils, nora)
		nils["accountEnabled"] = false // Torvald dismisses him
		syncedName(nils, g.p)
	}
	if rank := g.castAt(6); rank != nil {
		guestDomain := g.p.guestDomains()[0].Name
		upn, mail := fmt.Sprintf("dr.rank_%s#EXT#@%s", guestDomain, g.p.Organization.InitialDomain), "dr.rank@"+guestDomain
		rank["displayName"], rank["givenName"], rank["surname"] = "Dr. Rank", "Dr.", "Rank"
		rank["userPrincipalName"], rank["mail"], rank["mailNickname"] = upn, mail, "dr.rank"
		rank["accountEnabled"], rank["userType"] = true, "Guest"
		rank["proxyAddresses"] = []any{"SMTP:" + mail}
		rank["showInAddressList"], rank["creationType"], rank["externalUserState"] = false, "Invitation", "Accepted"
		rank["onPremisesSyncEnabled"] = nil // an invited guest is not synced from an on-premises directory
		delete(rank, "onPremisesDistinguishedName")
		delete(rank, "onPremisesSecurityIdentifier")
	}
}

// castBosses makes Torvald the manager of Nora and of Nils, after the manager tree is made. The tree is checked for a loop: if Torvald's own chain of managers
// reaches Nora or Nils (they led the company or the department), Torvald has no manager, and leads. A guest has no manager.
func (g *gen) castBosses() {
	nora, torvald, nils := g.castAt(3), g.castAt(4), g.castAt(5)
	if nora == nil || torvald == nil {
		return
	}
	key := func(u Object) string { return strings.ToLower(str(u["id"])) }
	g.w.Managers[key(nora)] = key(torvald)
	if nils != nil {
		g.w.Managers[key(nils)] = key(torvald)
	}
	for cur, steps := g.w.Managers[key(torvald)], 0; cur != "" && steps <= len(g.w.Users); cur, steps = g.w.Managers[cur], steps+1 {
		if cur == key(nora) || (nils != nil && cur == key(nils)) {
			delete(g.w.Managers, key(torvald))
			return
		}
	}
}

// AdminName and AdminUPN are the admin account of the owner in the default profile, as many companies make one (adm. and the name). The
// quick connect for PowerShell signs in as this user. A world of one user has no admin account. Another profile names its admin accounts
// in its own way (admin.userPrincipalName), and World.AdminAccount gives the address.
const (
	AdminName = "Philip (adm)"
	AdminUPN  = "adm.philip@fjordvik.example"
)

// adminUPN is the user principal name of the admin account that a profile makes for the owner.
func adminUPN(p *Profile) string {
	return adminName(p.Admin.UserPrincipalName, "Philip", "Meholm") + "@" + p.Organization.Domain
}

// makeAdmin turns a generated user into the admin account of the owner. Like makeOwner, it draws nothing from the random numbers. The
// profile says how the account is named, and the name is the name of the owner.
func makeAdmin(u Object, p *Profile) {
	upn := adminUPN(p)
	name := adminName(p.Admin.DisplayName, "Philip", "Meholm")
	u["displayName"], u["givenName"], u["surname"] = name, "Philip", "Meholm"
	u["userPrincipalName"], u["mail"], u["mailNickname"] = upn, upn, strings.TrimSuffix(upn, "@"+p.Organization.Domain)
	u["department"], u["jobTitle"] = p.Admin.Department, p.Admin.JobTitle
	u["accountEnabled"], u["userType"] = true, "Member"
	u["proxyAddresses"] = []any{"SMTP:" + upn}
	u["showInAddressList"], u["creationType"], u["externalUserState"] = false, nil, nil
	if org, ok := u["employeeOrgData"].(Object); ok {
		org["division"] = p.Admin.Department
	}
	if _, synced := u["onPremisesDistinguishedName"]; synced {
		u["onPremisesDistinguishedName"] = fmt.Sprintf("CN=%s,OU=%s,%s", name, p.Admin.Department, dcOf(p.Organization.Domain))
	}
}

// extensionAttributes gives the 15 on-premises extension attributes of a user. The
// first five have values that a blueprint can use (a cost center, a kind of worker,
// a division, a site code, a badge level). The next five are set for some users. The
// rest are null, as in most tenants.
func (g *gen) extensionAttributes() Object {
	ext := Object{}
	for i := 1; i <= 15; i++ {
		ext[fmt.Sprintf("extensionAttribute%d", i)] = nil
	}
	// the attributes of the profile, from the first to the last. One draw for the fill (when it is not 100), then one for the value.
	attrs := slices.Clone(g.p.Extension)
	slices.SortStableFunc(attrs, func(a, b ProfileExtension) int { return a.Index - b.Index })
	for _, a := range attrs {
		if a.Fill != nil && *a.Fill < 100 && g.rng.IntN(100) >= *a.Fill {
			continue
		}
		name := fmt.Sprintf("extensionAttribute%d", a.Index)
		switch {
		case a.Pattern != "":
			ext[name] = g.pattern(a.Pattern, nil)
		case len(a.Values) == 1:
			ext[name] = a.Values[0].Value // one value: nothing to choose, so no draw (the default profile has two such attributes)
		default:
			ext[name] = a.Values[g.weighted(len(a.Values), func(i int) int { return a.Values[i].Weight })].Value
		}
	}
	return ext
}

// seedEpoch is when the generated tenant "began". The users were made after it.
var seedEpoch = time.Date(2022, 3, 1, 8, 0, 0, 0, time.UTC)

// moreFields adds the other properties that people use in a tenant: employee
// type and hire date, address, phones, other mail, the org data, and when the
// account was made. Guests have the properties of an invited user.
func (g *gen) moreFields(u Object, i int, dept ProfileDepartment, office ProfileOffice, guest bool, mail string) {
	created := seedEpoch.Add(time.Duration(i) * 9 * time.Hour)
	u["createdDateTime"] = created.Format(time.RFC3339)
	u["lastPasswordChangeDateTime"] = created.Add(time.Duration(g.rng.IntN(400)) * 24 * time.Hour).Format(time.RFC3339)
	u["employeeType"] = map[bool]string{true: "Contractor", false: "Employee"}[g.rng.IntN(100) < g.p.People.ContractorPercent]
	u["employeeHireDate"] = created.AddDate(0, 0, -g.rng.IntN(30)).Format(time.RFC3339)
	u["employeeOrgData"] = Object{"division": dept.Name, "costCenter": g.pattern(g.p.Formats.CostCenter, nil)}
	u["streetAddress"] = fmt.Sprintf("%d %s", 1+g.rng.IntN(200), g.pick(g.p.Formats.Streets))
	u["state"] = office.City
	u["businessPhones"] = []any{g.pattern(g.p.Formats.BusinessPhone, nil)}
	u["otherMails"] = []any{}
	u["proxyAddresses"] = []any{"SMTP:" + mail}
	u["faxNumber"] = nil
	u["ageGroup"] = nil
	u["showInAddressList"] = !guest
	u["creationType"] = nil
	u["externalUserState"] = nil
	if guest {
		u["creationType"] = "Invitation"
		u["externalUserState"] = "Accepted"
	}
}

// managers gives each user a manager. Each department has a head (its first
// manager by title, or its first user). The head of the first department leads
// the company and has no manager. The other heads report to them. The other
// managers report to their head, and everyone else to a manager of the
// department. A guest has no manager. The result is a tree, so there is no loop.
func (g *gen) managers() {
	byDept := map[string][]Object{}
	var order []string
	for _, u := range g.w.Users {
		if u["userType"] == "Guest" {
			continue
		}
		d := str(u["department"])
		if _, ok := byDept[d]; !ok {
			order = append(order, d)
		}
		byDept[d] = append(byDept[d], u)
	}
	if len(order) == 0 {
		return
	}
	link := func(u, boss Object) { g.w.Managers[strings.ToLower(str(u["id"]))] = strings.ToLower(str(boss["id"])) }

	var top Object
	for n, d := range order {
		people := byDept[d]
		var mgrs []Object
		for _, u := range people {
			if strings.Contains(str(u["jobTitle"]), g.p.People.ManagerWord) {
				mgrs = append(mgrs, u)
			}
		}
		head := people[0]
		if len(mgrs) > 0 {
			head = mgrs[0]
		}
		if n == 0 {
			top = head
		} else {
			link(head, top)
		}
		for _, u := range people {
			switch {
			case u["id"] == head["id"]:
			case strings.Contains(str(u["jobTitle"]), g.p.People.ManagerWord) || len(mgrs) == 0:
				link(u, head)
			default:
				link(u, mgrs[g.rng.IntN(len(mgrs))])
			}
		}
	}
}

// places makes two buildings. Each has three floors, each floor has two sections
// with desks, and rooms and a workspace on the floor. The rooms of a building are
// also in one room list.
func (g *gen) places() {
	g.w.RoomListMembersOf = map[string][]string{}
	for b := 0; b < 2; b++ {
		office := g.p.Offices[b]
		building := g.place("building", office.Name, "", Object{
			"address": Object{"street": fmt.Sprintf("%d Main Street", 10+b), "city": office.City, "postalCode": office.Postal, "countryOrRegion": office.Country},
		})
		list := g.place("roomList", office.Name+" rooms", "", Object{"emailAddress": fmt.Sprintf("rooms-%s@%s", slug(office.Name), g.p.Organization.Domain)})
		listKey := strings.ToLower(str(list["id"]))
		for f := 1; f <= 3; f++ {
			floor := g.place("floor", fmt.Sprintf("%d", f), str(building["id"]), Object{"sortOrder": float64(f)})
			for s := 1; s <= 2; s++ {
				section := g.place("section", fmt.Sprintf("Section %d", s), str(floor["id"]), nil)
				for d := 1; d <= 4; d++ {
					g.place("desk", fmt.Sprintf("Desk %d.%d.%d", f, s, d), str(section["id"]), nil)
				}
			}
			for r := 1; r <= 4; r++ {
				name := fmt.Sprintf("%s %d.%02d", slug(office.Name), f, r)
				room := g.place("room", name, str(floor["id"]), Object{
					"capacity": float64(2 + g.rng.IntN(14)), "floorNumber": float64(f), "building": office.Name,
					"isWheelChairAccessible": g.rng.IntN(100) < 70,
				})
				g.w.RoomListMembersOf[listKey] = append(g.w.RoomListMembersOf[listKey], strings.ToLower(str(room["id"])))
			}
			ws := g.place("workspace", fmt.Sprintf("Deskpool %s %d", slug(office.Name), f), str(floor["id"]), Object{"capacity": float64(10)})
			g.w.RoomListMembersOf[listKey] = append(g.w.RoomListMembersOf[listKey], strings.ToLower(str(ws["id"])))
		}
	}
}

// place makes one place with the defaults that Graph gives, and adds it to the world.
func (g *gen) place(kind, name, parent string, extra Object) Object {
	id := g.guid()
	p := Object{"@odata.type": placeOdata + kind, "id": id, "placeId": id, "displayName": name, "tags": []any{}, "isWheelChairAccessible": false}
	if parent != "" {
		p["parentId"] = parent
	}
	maps.Copy(p, extra)
	g.w.placeDefaults(p, kind)
	if mail := str(p["emailAddress"]); mail != "" && kind != "roomList" {
		p["emailAddress"] = strings.ToLower(mailSlug(name)) + "@" + g.p.Organization.Domain
	}
	g.w.Places = append(g.w.Places, p)
	return p
}

// usersWhere gives the IDs of the users that the test accepts, in world order.
func (g *gen) usersWhere(test func(Object) bool) []string {
	var ids []string
	for _, u := range g.w.Users {
		if test(u) {
			ids = append(ids, u["id"].(string))
		}
	}
	return ids
}

func (g *gen) group(name string, members []string) *Group {
	grp := &Group{ID: g.guid(), Name: name, Security: true, Members: append([]string{}, members...)}
	g.w.Groups = append(g.w.Groups, grp)
	return grp
}

// firstN gives the first n items, or all of them when there are fewer.
func firstN(ids []string, n int) []string { return ids[:min(len(ids), n)] }

func inDepts(depts ...string) func(Object) bool {
	return func(u Object) bool {
		for _, d := range depts {
			if u["department"] == d {
				return true
			}
		}
		return false
	}
}

// groups makes the groups from the departments and the offices.
func (g *gen) groups() {
	gp := g.p.Groups
	if gp.DepartmentGroups {
		for _, d := range g.p.Departments {
			g.group(gp.Prefix+"dept."+slug(d.Name), g.usersWhere(inDepts(d.Name)))
		}
	}
	if gp.OfficeGroups {
		for _, o := range g.p.Offices {
			g.group(gp.Prefix+"loc."+slug(o.City), g.usersWhere(func(u Object) bool { return u["officeLocation"] == o.Name }))
		}
	}
	// A dynamic group: Entra decides its members, so nobody can change them by hand.
	if gp.DynamicDepartment != "" {
		dyn := g.group(gp.Prefix+"dyn."+slug(gp.DynamicDepartment), g.usersWhere(inDepts(gp.DynamicDepartment)))
		dyn.Dynamic = true
		dyn.Rule = fmt.Sprintf(`(user.department -eq %q)`, gp.DynamicDepartment)
	}
	// A Microsoft 365 group holds users only.
	if len(gp.Microsoft365) > 0 {
		m365 := g.group(gp.Prefix+"m365.announcements", g.usersWhere(inDepts(gp.Microsoft365...)))
		m365.Security, m365.Mail, m365.Unified = false, true, true
	}
}

// byName finds a group made earlier.
func (g *gen) byName(name string) *Group {
	for _, grp := range g.w.Groups {
		if grp.Name == name {
			return grp
		}
	}
	return nil
}

// apps makes, for each demo app, role groups with nested groups, three target
// groups that hold the role groups (and some users who should not be there),
// an enterprise application, and the role assignments of the role groups.
func (g *gen) apps() {
	gp := g.p.Groups
	admins := g.usersWhere(inDepts(gp.AdminDepartment))
	var firstEnterprise *ServicePrincipal // the one that is also an app registration of this tenant
	for _, a := range gp.Apps {
		contrib := g.usersWhere(inDepts(a.Contributor...))
		readers := g.usersWhere(inDepts(a.Reader...))
		cGroup := g.group(gp.Prefix+"app."+a.Name+".contributor", contrib)
		rGroup := g.group(gp.Prefix+"app."+a.Name+".reader", firstN(readers, 60))
		// A group inside a group: the readers also hold a whole department group.
		if legal := g.byName(gp.Prefix + "dept." + slug(gp.LegalDepartment)); legal != nil {
			rGroup.Members = append(rGroup.Members, legal.ID)
		}
		aGroup := g.group(gp.Prefix+"app."+a.Name+".admin", firstN(admins, 6))

		for _, env := range []string{"main", "uat", "prod"} {
			// The target holds the role groups, a part of their users (as an old run left
			// it), and a few users from nowhere. A run then has users to add and remove.
			t := g.group(gp.Prefix+"app."+a.Name+"."+env, []string{cGroup.ID, rGroup.ID, aGroup.ID})
			for i, id := range contrib {
				if i%3 != 0 {
					t.Members = append(t.Members, id)
				}
			}
			for _, id := range firstN(g.usersWhere(inDepts(gp.StrayDepartments...)), 4) {
				if !t.has(id) {
					t.Members = append(t.Members, id)
				}
			}
		}

		sp := &ServicePrincipal{ID: g.guid(), Name: a.Title, AppID: g.guid(), Enterprise: true, Roles: []AppRole{
			{g.guid(), "Admin"}, {g.guid(), "Contributor"}, {g.guid(), "Reader"},
		}}
		g.w.SPs = append(g.w.SPs, sp)
		if firstEnterprise == nil {
			firstEnterprise = sp
		}
		for i, grp := range []*Group{aGroup, cGroup, rGroup} {
			g.w.Assignments = append(g.w.Assignments, Assignment{sp.ID, grp.ID, "Group", sp.Roles[i].ID})
		}
		for _, id := range firstN(admins, 1) {
			g.w.Assignments = append(g.w.Assignments, Assignment{sp.ID, id, "User", DefaultAccess})
		}
	}
	// An app that is not an enterprise application: the finder must leave it out.
	backend := &ServicePrincipal{ID: g.guid(), Name: g.p.Organization.CompanyName + " Backend Service", AppID: g.guid()}
	g.w.SPs = append(g.w.SPs, backend)
	// The app registrations of this tenant (no draw of the random generator: the IDs come from the client IDs). The first enterprise application is
	// registered here as well, and the backend service is registered with a service principal that has no enterprise tag. The other enterprise
	// applications are apps of other tenants: a service principal and no registration.
	g.w.Applications = nil
	for i, sp := range []*ServicePrincipal{firstEnterprise, backend} {
		if sp == nil {
			continue
		}
		app := g.w.newApplication(stableGUID("dolly/application/"+sp.AppID), sp.AppID, sp.Name, seedEpoch.Add(time.Duration(i+1)*24*time.Hour))
		var roles []any
		for _, r := range sp.Roles {
			roles = append(roles, Object{"allowedMemberTypes": []any{"User"}, "description": r.Name, "displayName": r.Name, "id": r.ID, "isEnabled": true, "origin": "Application", "value": nil})
		}
		if roles != nil {
			app["appRoles"] = roles
		}
		g.w.Applications = append(g.w.Applications, app)
	}

	// The portal of Dolly, as a client of the Graph API. It has the same IDs in every world, and no draw of the random
	// generator, so the rest of the data does not change. Its calls are not throttled and not in the list of calls.
	g.w.SPs = append(g.w.SPs, &ServicePrincipal{
		ID: stableGUID("dolly/servicePrincipal/" + PortalAppID), AppID: PortalAppID, Name: PortalName,
		System: true, ExemptFromThrottle: true, HideCalls: true,
	})
	// The clients of Microsoft that tools sign in as. A real tenant has them too, with these client IDs.
	for _, c := range MicrosoftClients {
		g.w.SPs = append(g.w.SPs, &ServicePrincipal{ID: stableGUID("dolly/servicePrincipal/" + c.AppID), AppID: c.AppID, Name: c.Name, System: true})
	}
}

// GraphPowerShellAppID is the client ID of Microsoft Graph Command Line Tools: the client that Microsoft Graph PowerShell signs in as.
const GraphPowerShellAppID = "14d82eec-204b-4c2f-b7e8-296a70dab67e"

// MicrosoftClients are the well-known client IDs of the tools of Microsoft. Source: the Microsoft Learn page "Verify first-party
// Microsoft applications in sign-in reports" confirms the first two. Azure CLI and Azure PowerShell are from the community list of
// well-known client IDs at rakhesh.com/azure/well-known-client-ids, which Learn does not confirm.
var MicrosoftClients = []struct{ Name, AppID string }{
	{"Microsoft Graph Command Line Tools", GraphPowerShellAppID},
	{"Graph Explorer", "de8bc8b5-d9f9-48b1-a8ad-b748da725064"},
	{"Microsoft Azure CLI", "04b07795-8ddb-461a-bbee-02f9e1bf7b46"},
	{"Microsoft Azure PowerShell", "1950a258-227b-4e31-a9cf-717495945fc2"},
}

// PortalName and PortalAppID name the service principal of the portal of Dolly.
const PortalName = "Dolly portal"

var PortalAppID = stableGUID("dolly/portal/appId")
