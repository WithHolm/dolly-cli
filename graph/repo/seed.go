package repo

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"strings"
	"time"
)

// Department has the job titles that people in it can have. The sets are fixed,
// so a demo or a test knows the values. Names, phones, and IDs are made up.
type Department struct {
	Name   string
	Titles []string
	Weight int // how common the department is
}

// Departments is the fixed set of departments.
var Departments = []Department{
	{"Sales", []string{"Account Executive", "Sales Manager", "Sales Development Representative"}, 14},
	{"Engineering", []string{"Software Engineer", "Senior Software Engineer", "Engineering Manager", "DevOps Engineer"}, 22},
	{"Marketing", []string{"Marketing Specialist", "Content Manager", "Brand Manager"}, 8},
	{"Finance", []string{"Accountant", "Financial Analyst", "Finance Manager"}, 7},
	{"Human Resources", []string{"HR Business Partner", "Recruiter", "HR Manager"}, 5},
	{"IT", []string{"IT Support Specialist", "Systems Administrator", "Security Analyst"}, 7},
	{"Customer Support", []string{"Support Agent", "Support Team Lead"}, 12},
	{"Legal", []string{"Legal Counsel", "Compliance Officer"}, 3},
	{"Operations", []string{"Operations Coordinator", "Logistics Manager", "Operations Manager"}, 10},
	{"Product", []string{"Product Manager", "Product Designer", "UX Researcher"}, 6},
}

// Office is an office location.
type Office struct {
	Name, City, Country, Usage, Postal string
}

// Offices is the fixed set of office locations.
var Offices = []Office{
	{"Stockholm HQ", "Stockholm", "Sweden", "SE", "111 22"},
	{"Gothenburg", "Gothenburg", "Sweden", "SE", "411 03"},
	{"Oslo", "Oslo", "Norway", "NO", "0150"},
	{"Copenhagen", "Copenhagen", "Denmark", "DK", "1050"},
	{"Helsinki", "Helsinki", "Finland", "FI", "00100"},
	{"London", "London", "United Kingdom", "GB", "EC2A 4NE"},
	{"Berlin", "Berlin", "Germany", "DE", "10117"},
	{"Amsterdam", "Amsterdam", "Netherlands", "NL", "1012 AB"},
}

var firstNames = strings.Fields(`Anna Erik Maria Lars Sofia Johan Elin Oskar Ida Nils Klara Anders Linnea Mats Hanna
	Viktor Emma Henrik Julia Fredrik Alice Magnus Sara Daniel Frida Karl Nora Simon Tilda Jonas Olivia Peter Maja
	Gustav Elsa Patrik Agnes David Lena Axel`)

var lastNames = strings.Fields(`Andersson Johansson Karlsson Nilsson Eriksson Larsson Olsson Persson Svensson Gustafsson
	Pettersson Jonsson Jansson Hansson Bengtsson Lindberg Lindqvist Lindgren Berg Axelsson Bergstrom Lundberg
	Lundgren Mattsson Berglund Fredriksson Sandberg Henriksson Forsberg Sjoberg Wallin Engstrom Eklund Danielsson`)

// apps are the demo apps. Each gets role groups and three target groups.
var apps = []struct {
	Name        string
	Title       string
	Contributor []string // departments whose people are contributors
	Reader      []string
}{
	{"acme", "Acme Portal", []string{"Sales", "Marketing"}, []string{"Customer Support", "Legal"}},
	{"contoso", "Contoso Planner", []string{"Engineering", "Product"}, []string{"Operations", "Human Resources"}},
	{"globex", "Globex Ledger", []string{"Finance", "Operations"}, []string{"Sales", "Legal"}},
}

// Options sets the size of a new world.
type Options struct {
	Users int
	Seed  int64
}

type gen struct {
	rng *rand.Rand
	w   *World
}

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
	g := &gen{rng: rand.New(rand.NewPCG(uint64(o.Seed), 0x9e3779b97f4a7c15)), w: &World{Seed: o.Seed, Managers: map[string]string{}}}
	g.users(o.Users)
	g.managers()
	g.groups()
	g.apps()
	g.places()
	g.w.Organization = newOrganization()
	g.w.index()
	return g.w
}

func slug(s string) string {
	return strings.ToLower(strings.NewReplacer(" ", "-", "&", "and").Replace(s))
}

func (g *gen) department() Department {
	total := 0
	for _, d := range Departments {
		total += d.Weight
	}
	n := g.rng.IntN(total)
	for _, d := range Departments {
		if n < d.Weight {
			return d
		}
		n -= d.Weight
	}
	return Departments[0]
}

func (g *gen) users(n int) {
	taken := map[string]int{}
	for i := 0; i < n; i++ {
		first, last := g.pick(firstNames), g.pick(lastNames)
		nick := strings.ToLower(first + "." + last)
		if taken[nick]++; taken[nick] > 1 {
			nick = fmt.Sprintf("%s%d", nick, taken[nick])
		}
		dept := g.department()
		office := Offices[g.rng.IntN(len(Offices))]
		guest := g.rng.IntN(100) < 3
		upn := nick + "@acme.example"
		mail := upn
		userType := "Member"
		if guest {
			userType = "Guest"
			upn = fmt.Sprintf("%s_partner.example#EXT#@acme.onmicrosoft.com", nick)
			mail = nick + "@partner.example"
		}
		synced := !guest && g.rng.IntN(100) < 60
		u := Object{
			"id":                            g.guid(),
			"displayName":                   first + " " + last,
			"givenName":                     first,
			"surname":                       last,
			"userPrincipalName":             upn,
			"mail":                          mail,
			"mailNickname":                  nick,
			"department":                    dept.Name,
			"jobTitle":                      dept.Titles[g.rng.IntN(len(dept.Titles))],
			"officeLocation":                office.Name,
			"city":                          office.City,
			"country":                       office.Country,
			"usageLocation":                 office.Usage,
			"postalCode":                    office.Postal,
			"companyName":                   "Acme",
			"employeeId":                    fmt.Sprintf("E%05d", 10000+i),
			"mobilePhone":                   fmt.Sprintf("+%s 7%d %03d %02d %02d", map[string]string{"SE": "46", "NO": "47", "DK": "45", "FI": "358", "GB": "44", "DE": "49", "NL": "31"}[office.Usage], g.rng.IntN(10), g.rng.IntN(1000), g.rng.IntN(100), g.rng.IntN(100)),
			"preferredLanguage":             map[bool]string{true: "en-US", false: "sv-SE"}[g.rng.IntN(100) < 40],
			"accountEnabled":                g.rng.IntN(100) < 97,
			"userType":                      userType,
			"passwordPolicies":              "None",
			"onPremisesSyncEnabled":         nil,
			"onPremisesExtensionAttributes": g.extensionAttributes(),
		}
		g.moreFields(u, i, dept, office, guest, mail)
		if synced {
			u["onPremisesSyncEnabled"] = true
			u["onPremisesDistinguishedName"] = fmt.Sprintf("CN=%s %s,OU=%s,DC=acme,DC=example", first, last, dept.Name)
			u["onPremisesSecurityIdentifier"] = fmt.Sprintf("S-1-5-21-1004336348-1177238915-682003330-%d", 1000+i)
		}
		switch i {
		case n - 1:
			makeOwner(u) // the last user is the owner of this Dolly, so the count of users is as asked
		case n - 2:
			makeAdmin(u) // the one before is the admin account of the owner
		}
		g.w.Users = append(g.w.Users, u)
	}
}

// OwnerName and OwnerUPN are the user that every generated world has: Philip Meholm, a tech lead in Engineering. The
// examples of the portal use this user, and a tool that signs in as a user can use it too. The address is fake.
const (
	OwnerName = "Philip Meholm"
	OwnerUPN  = "philip.meholm@acme.example"
)

// makeOwner turns a generated user into the owner. It draws nothing from the random numbers, so every other user of a
// seed is the same as it was before the owner was added. Only the fields that say who the person is change.
func makeOwner(u Object) {
	u["displayName"], u["givenName"], u["surname"] = OwnerName, "Philip", "Meholm"
	u["userPrincipalName"], u["mail"], u["mailNickname"] = OwnerUPN, OwnerUPN, "philip.meholm"
	u["department"], u["jobTitle"] = "Engineering", "Tech Lead"
	u["accountEnabled"], u["userType"] = true, "Member"
	u["proxyAddresses"] = []any{"SMTP:" + OwnerUPN}
	u["showInAddressList"], u["creationType"], u["externalUserState"] = true, nil, nil
	if org, ok := u["employeeOrgData"].(Object); ok {
		org["division"] = "Engineering"
	}
	if _, synced := u["onPremisesDistinguishedName"]; synced {
		u["onPremisesDistinguishedName"] = "CN=Philip Meholm,OU=Engineering,DC=acme,DC=example"
	}
}

// AdminName and AdminUPN are the admin account of the owner, as many companies make one (a. and the name). The quick connect
// for PowerShell signs in as this user. A world of one user has no admin account.
const (
	AdminName = "Philip (admin)"
	AdminUPN  = "a.philip@acme.example"
)

// makeAdmin turns a generated user into the admin account of the owner. Like makeOwner, it draws nothing from the random numbers.
func makeAdmin(u Object) {
	u["displayName"], u["givenName"], u["surname"] = AdminName, "Philip", "Meholm"
	u["userPrincipalName"], u["mail"], u["mailNickname"] = AdminUPN, AdminUPN, "a.philip"
	u["department"], u["jobTitle"] = "IT", "Administrator"
	u["accountEnabled"], u["userType"] = true, "Member"
	u["proxyAddresses"] = []any{"SMTP:" + AdminUPN}
	u["showInAddressList"], u["creationType"], u["externalUserState"] = false, nil, nil
	if org, ok := u["employeeOrgData"].(Object); ok {
		org["division"] = "IT"
	}
	if _, synced := u["onPremisesDistinguishedName"]; synced {
		u["onPremisesDistinguishedName"] = "CN=Philip (admin),OU=IT,DC=acme,DC=example"
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
	ext["extensionAttribute1"] = fmt.Sprintf("CC-%d", 100+g.rng.IntN(20))
	ext["extensionAttribute2"] = map[bool]string{true: "Contractor", false: "Employee"}[g.rng.IntN(100) < 12]
	ext["extensionAttribute3"] = g.pick([]string{"North", "South", "East", "West", "HQ"})
	ext["extensionAttribute4"] = fmt.Sprintf("SITE-%02d", 1+g.rng.IntN(12))
	ext["extensionAttribute5"] = g.pick([]string{"L1", "L2", "L3", "L4"})
	if g.rng.IntN(100) < 40 {
		ext["extensionAttribute6"] = g.pick([]string{"remote", "hybrid", "office"})
	}
	if g.rng.IntN(100) < 25 {
		ext["extensionAttribute7"] = fmt.Sprintf("PRJ-%03d", g.rng.IntN(60))
	}
	if g.rng.IntN(100) < 15 {
		ext["extensionAttribute8"] = "pilot"
	}
	if g.rng.IntN(100) < 10 {
		ext["extensionAttribute9"] = "VIP"
	}
	if g.rng.IntN(100) < 30 {
		ext["extensionAttribute10"] = g.pick([]string{"A", "B", "C"})
	}
	return ext
}

// seedEpoch is when the generated tenant "began". The users were made after it.
var seedEpoch = time.Date(2022, 3, 1, 8, 0, 0, 0, time.UTC)

// moreFields adds the other properties that people use in a tenant: employee
// type and hire date, address, phones, other mail, the org data, and when the
// account was made. Guests have the properties of an invited user.
func (g *gen) moreFields(u Object, i int, dept Department, office Office, guest bool, mail string) {
	created := seedEpoch.Add(time.Duration(i) * 9 * time.Hour)
	u["createdDateTime"] = created.Format(time.RFC3339)
	u["lastPasswordChangeDateTime"] = created.Add(time.Duration(g.rng.IntN(400)) * 24 * time.Hour).Format(time.RFC3339)
	u["employeeType"] = map[bool]string{true: "Contractor", false: "Employee"}[g.rng.IntN(100) < 12]
	u["employeeHireDate"] = created.AddDate(0, 0, -g.rng.IntN(30)).Format(time.RFC3339)
	u["employeeOrgData"] = Object{"division": dept.Name, "costCenter": fmt.Sprintf("CC-%d", 100+g.rng.IntN(20))}
	u["streetAddress"] = fmt.Sprintf("%d %s", 1+g.rng.IntN(200), g.pick([]string{"Main Street", "Harbour Road", "Station Way", "Park Avenue"}))
	u["state"] = office.City
	u["businessPhones"] = []any{fmt.Sprintf("+%d %d %03d %03d", 40+g.rng.IntN(10), 8, g.rng.IntN(1000), g.rng.IntN(1000))}
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
			if strings.Contains(str(u["jobTitle"]), "Manager") {
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
			case strings.Contains(str(u["jobTitle"]), "Manager") || len(mgrs) == 0:
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
		office := Offices[b]
		building := g.place("building", office.Name, "", Object{
			"address": Object{"street": fmt.Sprintf("%d Main Street", 10+b), "city": office.City, "postalCode": office.Postal, "countryOrRegion": office.Country},
		})
		list := g.place("roomList", office.Name+" rooms", "", Object{"emailAddress": fmt.Sprintf("rooms-%s@acme.example", slug(office.Name))})
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
		p["emailAddress"] = strings.ToLower(mailSlug(name)) + "@acme.example"
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
	for _, d := range Departments {
		g.group("grp.dept."+slug(d.Name), g.usersWhere(inDepts(d.Name)))
	}
	for _, o := range Offices {
		g.group("grp.loc."+slug(o.City), g.usersWhere(func(u Object) bool { return u["officeLocation"] == o.Name }))
	}
	// A dynamic group: Entra decides its members, so nobody can change them by hand.
	dyn := g.group("grp.dyn.engineering", g.usersWhere(inDepts("Engineering")))
	dyn.Dynamic = true
	dyn.Rule = `(user.department -eq "Engineering")`
	// A Microsoft 365 group holds users only.
	m365 := g.group("grp.m365.announcements", g.usersWhere(inDepts("Marketing", "Human Resources")))
	m365.Security, m365.Mail, m365.Unified = false, true, true
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
	admins := g.usersWhere(inDepts("IT"))
	for _, a := range apps {
		contrib := g.usersWhere(inDepts(a.Contributor...))
		readers := g.usersWhere(inDepts(a.Reader...))
		cGroup := g.group("grp.app."+a.Name+".contributor", contrib)
		rGroup := g.group("grp.app."+a.Name+".reader", firstN(readers, 60))
		// A group inside a group: the readers also hold a whole department group.
		rGroup.Members = append(rGroup.Members, g.byName("grp.dept.legal").ID)
		aGroup := g.group("grp.app."+a.Name+".admin", firstN(admins, 6))

		for _, env := range []string{"main", "uat", "prod"} {
			// The target holds the role groups, a part of their users (as an old run left
			// it), and a few users from nowhere. A run then has users to add and remove.
			t := g.group("grp.app."+a.Name+"."+env, []string{cGroup.ID, rGroup.ID, aGroup.ID})
			for i, id := range contrib {
				if i%3 != 0 {
					t.Members = append(t.Members, id)
				}
			}
			for _, id := range firstN(g.usersWhere(inDepts("Legal", "Human Resources")), 4) {
				if !t.has(id) {
					t.Members = append(t.Members, id)
				}
			}
		}

		sp := &ServicePrincipal{ID: g.guid(), Name: a.Title, AppID: g.guid(), Enterprise: true, Roles: []AppRole{
			{g.guid(), "Admin"}, {g.guid(), "Contributor"}, {g.guid(), "Reader"},
		}}
		g.w.SPs = append(g.w.SPs, sp)
		for i, grp := range []*Group{aGroup, cGroup, rGroup} {
			g.w.Assignments = append(g.w.Assignments, Assignment{sp.ID, grp.ID, "Group", sp.Roles[i].ID})
		}
		for _, id := range firstN(admins, 1) {
			g.w.Assignments = append(g.w.Assignments, Assignment{sp.ID, id, "User", DefaultAccess})
		}
	}
	// An app that is not an enterprise application: the finder must leave it out.
	g.w.SPs = append(g.w.SPs, &ServicePrincipal{ID: g.guid(), Name: "Acme Backend Service", AppID: g.guid()})

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
