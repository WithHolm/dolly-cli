package repo

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// A profile says what a generated tenant looks like: the organization, the offices, the departments and their titles, the names, the rates
// (how many guests, how many synced users), the formats of ids and phone numbers, the extension attributes, and the groups. The generator reads
// it, and the same profile with the same seed always makes the same data.
//
// The default profile is Fjordvik (profiles/fjordvik.json). The tenant that dollycli made before profiles, and before Fjordvik, is Acme (profiles/acme.json):
// exactly the same data, for the same seed (golden_test.go).
// A profile is data that other people may share, so it is not trusted: ParseProfile checks every field and every size.

// ProfileVersion is the version of the format of a profile.
const ProfileVersion = 1

// Profile is the description of a tenant.
type Profile struct {
	Version     int    `json:"version"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	Organization ProfileOrganization `json:"organization"`
	Offices      []ProfileOffice     `json:"offices"`
	Departments  []ProfileDepartment `json:"departments"`
	Names        ProfileNames        `json:"names"`
	People       ProfilePeople       `json:"people"`
	Formats      ProfileFormats      `json:"formats"`
	Extension    []ProfileExtension  `json:"extensionAttributes,omitempty"`
	Groups       ProfileGroups       `json:"groups"`
	Owner        ProfileOwner        `json:"owner"`
	Admin        ProfileAdmin        `json:"admin"`
}

// ProfileOrganization is the tenant: its name, its domains, and its address.
type ProfileOrganization struct {
	DisplayName   string `json:"displayName"`
	CompanyName   string `json:"companyName"`   // the companyName of the users
	Domain        string `json:"domain"`        // the default domain: the users have addresses at it
	InitialDomain string `json:"initialDomain"` // the .onmicrosoft.com domain
	// DomainWeight is how common the default domain is in the addresses of the users, and Domains are the other verified domains, with
	// their weights. With no other domain every member has the default one. The initial domain may be one of them.
	DomainWeight      int             `json:"domainWeight,omitempty"`
	Domains           []ProfileDomain `json:"domains,omitempty"`
	GuestDomain       string          `json:"guestDomain,omitempty"` // where the mail of a guest is: one domain (see People.GuestDomains for more)
	Street            string          `json:"street"`
	PostalCode        string          `json:"postalCode"`
	City              string          `json:"city"`
	State             string          `json:"state"`
	Country           string          `json:"country"`
	CountryLetterCode string          `json:"countryLetterCode"`
	UsageLocation     string          `json:"usageLocation"`     // the default usage location, two letters
	PreferredLanguage string          `json:"preferredLanguage"` // for example en
	Phone             string          `json:"phone"`
}

// ProfileDomain is a domain and how common it is.
type ProfileDomain struct {
	Name   string `json:"name"`
	Weight int    `json:"weight,omitempty"`
}

// memberDomains are the domains of the addresses of the members: the default one, then the others.
func (p *Profile) memberDomains() []ProfileDomain {
	return append([]ProfileDomain{{p.Organization.Domain, max(p.Organization.DomainWeight, 1)}}, p.Organization.Domains...)
}

// guestDomains are the domains of the mail of the guests: People.GuestDomains, or the one domain of Organization.GuestDomain.
func (p *Profile) guestDomains() []ProfileDomain {
	if len(p.People.GuestDomains) > 0 {
		return p.People.GuestDomains
	}
	return []ProfileDomain{{p.Organization.GuestDomain, 1}}
}

// ProfileOffice is an office. Each user works in one. The first two are also the buildings that have floors and rooms.
type ProfileOffice struct {
	Name        string `json:"name"`
	City        string `json:"city"`
	Country     string `json:"country"`
	Usage       string `json:"usage"`       // the two letters of the usage location
	Postal      string `json:"postal"`      // a postal code
	CallingCode string `json:"callingCode"` // the digits after + of the phones of this office
	Weight      int    `json:"weight,omitempty"`
}

// ProfileDepartment is a department, how common it is, and the job titles in it.
type ProfileDepartment struct {
	Name   string         `json:"name"`
	Weight int            `json:"weight,omitempty"`
	Titles []ProfileTitle `json:"titles"`
}

// ProfileTitle is a job title, and how common it is in its department.
type ProfileTitle struct {
	Name   string `json:"name"`
	Weight int    `json:"weight,omitempty"`
}

// ProfileNames are the names that the people have. A first and a last name are picked at random, with the same chance for each.
type ProfileNames struct {
	First []string `json:"first"`
	Last  []string `json:"last"`
	// ByLanguage gives names for a language of People.Languages (nb-NO and so on). With it, the language of a person is picked first (by
	// the weights of the languages), and the names come from the list of that language. A language with no list uses First and Last.
	ByLanguage map[string]ProfileNameSet `json:"byLanguage,omitempty"`
}

// ProfileNameSet is the names of one language.
type ProfileNameSet struct {
	First []string `json:"first"`
	Last  []string `json:"last"`
}

// nameSet gives the names of a language: its own lists, or the general ones.
func (n ProfileNames) nameSet(language string) ProfileNameSet {
	if s, ok := n.ByLanguage[language]; ok && len(s.First) > 0 && len(s.Last) > 0 {
		return s
	}
	return ProfileNameSet{First: n.First, Last: n.Last}
}

// ProfilePeople are the rates (in percent) and the languages.
type ProfilePeople struct {
	GuestPercent      int               `json:"guestPercent"`
	SyncedPercent     int               `json:"syncedPercent"`     // of the members, how many come from an on-premises directory
	EnabledPercent    int               `json:"enabledPercent"`    // how many accounts are enabled
	ContractorPercent int               `json:"contractorPercent"` // how many have employeeType Contractor
	Languages         []ProfileLanguage `json:"languages"`
	ManagerWord       string            `json:"managerWord"` // a title with this word is a manager
	// GuestDomains are where the mail of the guests is, with weights. It is for more than one domain; Organization.GuestDomain is for one.
	GuestDomains []ProfileDomain `json:"guestDomains,omitempty"`
}

// ProfileLanguage is a preferred language of the users, and how common it is.
type ProfileLanguage struct {
	Code   string `json:"code"`
	Weight int    `json:"weight,omitempty"`
}

// ProfileFormats are the patterns of ids and phone numbers. See the pattern language in pattern.go.
type ProfileFormats struct {
	EmployeeID    ProfileEmployeeID `json:"employeeId"`
	MobilePhone   string            `json:"mobilePhone"`   // {cc} is the calling code of the office
	BusinessPhone string            `json:"businessPhone"` //
	CostCenter    string            `json:"costCenter"`
	Streets       []string          `json:"streets"` // the street names of the addresses of the users
}

// ProfileEmployeeID is the format of the employee ID, counted from Start for the first user.
type ProfileEmployeeID struct {
	Pattern string `json:"pattern"` // for example E#####: the # are the digits of the number
	Start   int    `json:"start"`
}

// ProfileExtension describes one of the on-premises extension attributes (1 to 15). Every attribute that is not described is null.
// The value is one of Values (by weight), or the text that Pattern makes. Fill is the percent of the users that have it (100 if left out).
type ProfileExtension struct {
	Index   int            `json:"index"`
	Values  []ProfileValue `json:"values,omitempty"`
	Pattern string         `json:"pattern,omitempty"`
	Fill    *int           `json:"fill,omitempty"`
}

// ProfileValue is a value of an extension attribute, and how common it is.
type ProfileValue struct {
	Value  string `json:"value"`
	Weight int    `json:"weight,omitempty"`
}

// ProfileGroups says which groups the tenant has. The names start with Prefix.
type ProfileGroups struct {
	Prefix            string       `json:"prefix"` // for example grp.
	DepartmentGroups  bool         `json:"departmentGroups"`
	OfficeGroups      bool         `json:"officeGroups"`
	DynamicDepartment string       `json:"dynamicDepartment,omitempty"`       // a dynamic group with the users of this department
	Microsoft365      []string     `json:"microsoft365Departments,omitempty"` // a Microsoft 365 group with the users of these departments
	AdminDepartment   string       `json:"adminDepartment,omitempty"`         // the department that has the administrators of the apps
	LegalDepartment   string       `json:"legalDepartment,omitempty"`         // its group is inside the reader groups of the apps
	StrayDepartments  []string     `json:"strayDepartments,omitempty"`        // users of these are in the app target groups, where they do not belong
	Apps              []ProfileApp `json:"apps,omitempty"`
}

// ProfileApp is a demo application. It gets role groups, target groups, and an enterprise application.
type ProfileApp struct {
	Name        string   `json:"name"`  // a short name for the group names
	Title       string   `json:"title"` // the name of the enterprise application
	Contributor []string `json:"contributorDepartments"`
	Reader      []string `json:"readerDepartments"`
}

// ProfileOwner is the person who runs dollycli: Philip Meholm, who is in every world. The profile says where he works.
type ProfileOwner struct {
	Department string `json:"department"`
	JobTitle   string `json:"jobTitle"`
}

// ProfileAdmin says how the admin accounts of this organization are named. The owner has one. The patterns can use {first}, {last},
// {f} (the first letter of the first name), and {l}, in lowercase, and {First} and {Last} as they are written. Examples of a user
// principal name: a.{first}, adm.{first}, {first}.a.{last}, {first}.{last}.adm.
type ProfileAdmin struct {
	UserPrincipalName string `json:"userPrincipalName"` // the part before the @
	DisplayName       string `json:"displayName"`
	JobTitle          string `json:"jobTitle"`
	Department        string `json:"department"`
}

// Limits for a profile that comes from outside.
const (
	maxProfileBytes = 1 << 20
	maxList         = 2000
	maxText         = 200
	maxWeight       = 1000
)

// The two profiles that are in the program (profiles/): Fjordvik is the default tenant, and Acme is the one that dollycli made before Fjordvik. A seed that
// has no profile in it was made from Acme.
//
//go:embed profiles/fjordvik.json
var fjordvikJSON []byte

//go:embed profiles/acme.json
var acmeJSON []byte

func builtin(name string, data []byte) *Profile {
	p, err := ParseProfile(bytes.NewReader(data))
	if err != nil {
		panic("dolly: the built-in profile " + name + " is not valid: " + err.Error()) // the tests of this package find it
	}
	return p
}

// DefaultProfile is the tenant that dollycli makes when no profile is given: Fjordvik, a made-up Norwegian energy company. Every call gives a new copy.
func DefaultProfile() *Profile { return builtin("fjordvik", fjordvikJSON) }

// AcmeProfile is Acme Corporation, the tenant that dollycli made before Fjordvik, with English titles and groups and Nordic offices. A seed from before
// Fjordvik has no profile in it, and it was made from this one. Every call gives a new copy.
func AcmeProfile() *Profile { return builtin("acme", acmeJSON) }

// ParseProfile reads a profile from JSON. It refuses a field that it does not know, a value that is out of range, and a file that is too big.
// A profile that is not valid is an error that says which field.
func ParseProfile(r io.Reader) (*Profile, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxProfileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxProfileBytes {
		return nil, fmt.Errorf("the profile is bigger than %d bytes", maxProfileBytes)
	}
	var p Profile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("the profile is not valid JSON for a profile: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate checks every field of the profile, and fills in what it leaves out (a weight of 0 is 1, a fill that is not there is 100).
func (p *Profile) Validate() error {
	var errs []error
	bad := func(field, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", field, fmt.Sprintf(format, args...)))
	}
	text := func(field, s string, required bool) {
		switch {
		case required && strings.TrimSpace(s) == "":
			bad(field, "is needed")
		case len(s) > maxText:
			bad(field, "is longer than %d characters", maxText)
		case strings.ContainsAny(s, "\x00\r\n"):
			bad(field, "has a control character")
		}
	}
	weight := func(field string, w *int) {
		if *w == 0 {
			*w = 1
		}
		if *w < 1 || *w > maxWeight {
			bad(field, "must be from 1 to %d", maxWeight)
		}
	}
	percent := func(field string, n int) {
		if n < 0 || n > 100 {
			bad(field, "must be from 0 to 100")
		}
	}
	list := func(field string, n, min int) {
		if n < min || n > maxList {
			bad(field, "needs %d to %d items, it has %d", min, maxList, n)
		}
	}
	domain := func(field, s string) {
		text(field, s, true)
		if !domainRe.MatchString(s) {
			bad(field, "%q is not a domain name (letters, digits, hyphens, and dots)", s)
		}
	}

	if p.Version != ProfileVersion {
		bad("version", "must be %d, it is %d", ProfileVersion, p.Version)
	}
	text("name", p.Name, false)
	text("description", p.Description, false)

	o := &p.Organization
	for field, s := range map[string]string{
		"organization.displayName": o.DisplayName, "organization.companyName": o.CompanyName, "organization.street": o.Street,
		"organization.postalCode": o.PostalCode, "organization.city": o.City, "organization.state": o.State, "organization.country": o.Country,
		"organization.phone": o.Phone,
	} {
		text(field, s, field != "organization.state")
	}
	domain("organization.domain", o.Domain)
	domain("organization.initialDomain", o.InitialDomain)
	weight("organization.domainWeight", &o.DomainWeight)
	list("organization.domains", len(o.Domains), 0)
	seenDomain := map[string]bool{o.Domain: true}
	for i := range o.Domains {
		f := fmt.Sprintf("organization.domains[%d]", i)
		domain(f+".name", o.Domains[i].Name)
		if seenDomain[o.Domains[i].Name] {
			bad(f+".name", "%q is there twice", o.Domains[i].Name)
		}
		seenDomain[o.Domains[i].Name] = true
		weight(f+".weight", &o.Domains[i].Weight)
	}
	if len(p.People.GuestDomains) == 0 {
		domain("organization.guestDomain", o.GuestDomain)
	} else if o.GuestDomain != "" {
		domain("organization.guestDomain", o.GuestDomain)
	}
	list("people.guestDomains", len(p.People.GuestDomains), 0)
	seenGuest := map[string]bool{}
	for i := range p.People.GuestDomains {
		f := fmt.Sprintf("people.guestDomains[%d]", i)
		domain(f+".name", p.People.GuestDomains[i].Name)
		if seenGuest[p.People.GuestDomains[i].Name] {
			bad(f+".name", "%q is there twice", p.People.GuestDomains[i].Name)
		}
		seenGuest[p.People.GuestDomains[i].Name] = true
		weight(f+".weight", &p.People.GuestDomains[i].Weight)
	}
	for field, s := range map[string]string{"organization.countryLetterCode": o.CountryLetterCode, "organization.usageLocation": o.UsageLocation} {
		if len(s) != 2 {
			bad(field, "must be two letters")
		}
	}
	text("organization.preferredLanguage", o.PreferredLanguage, true)

	list("offices", len(p.Offices), 2) // the first two are the buildings
	for i := range p.Offices {
		of := &p.Offices[i]
		f := fmt.Sprintf("offices[%d]", i)
		for k, s := range map[string]string{"name": of.Name, "city": of.City, "country": of.Country, "postal": of.Postal} {
			text(f+"."+k, s, true)
		}
		if len(of.Usage) != 2 {
			bad(f+".usage", "must be two letters")
		}
		if !digitsRe.MatchString(of.CallingCode) {
			bad(f+".callingCode", "must be digits, for example 47")
		}
		weight(f+".weight", &of.Weight)
	}

	list("departments", len(p.Departments), 1)
	names := map[string]bool{}
	for i := range p.Departments {
		d := &p.Departments[i]
		f := fmt.Sprintf("departments[%d]", i)
		text(f+".name", d.Name, true)
		if names[d.Name] {
			bad(f+".name", "%q is there twice", d.Name)
		}
		names[d.Name] = true
		weight(f+".weight", &d.Weight)
		list(f+".titles", len(d.Titles), 1)
		for j := range d.Titles {
			text(fmt.Sprintf("%s.titles[%d].name", f, j), d.Titles[j].Name, true)
			weight(fmt.Sprintf("%s.titles[%d].weight", f, j), &d.Titles[j].Weight)
		}
	}
	dept := func(field, name string, required bool) {
		if name == "" && !required {
			return
		}
		if !names[name] {
			bad(field, "%q is not a department of the profile", name)
		}
	}

	// the general lists are needed only for a language that has no lists of its own
	generalMin := 1
	if len(p.Names.ByLanguage) > 0 {
		generalMin = 0
	}
	list("names.first", len(p.Names.First), generalMin)
	list("names.last", len(p.Names.Last), generalMin)
	for i, n := range p.Names.First {
		text(fmt.Sprintf("names.first[%d]", i), n, true)
	}
	for i, n := range p.Names.Last {
		text(fmt.Sprintf("names.last[%d]", i), n, true)
	}
	spoken := map[string]bool{}
	for _, l := range p.People.Languages {
		spoken[l.Code] = true
	}
	languages := make([]string, 0, len(p.Names.ByLanguage))
	for code := range p.Names.ByLanguage {
		languages = append(languages, code)
	}
	slices.Sort(languages) // the same errors in the same order
	for _, code := range languages {
		f, set := fmt.Sprintf("names.byLanguage[%s]", code), p.Names.ByLanguage[code]
		if !spoken[code] {
			bad(f, "is not a language in people.languages")
		}
		list(f+".first", len(set.First), 1)
		list(f+".last", len(set.Last), 1)
		for i, n := range set.First {
			text(fmt.Sprintf("%s.first[%d]", f, i), n, true)
		}
		for i, n := range set.Last {
			text(fmt.Sprintf("%s.last[%d]", f, i), n, true)
		}
	}
	if len(p.Names.ByLanguage) > 0 && generalMin == 0 && (len(p.Names.First) == 0 || len(p.Names.Last) == 0) {
		for _, l := range p.People.Languages {
			if _, ok := p.Names.ByLanguage[l.Code]; !ok {
				bad("names.byLanguage", "%q has no names: add a list for it", l.Code)
			}
		}
	}

	pe := &p.People
	percent("people.guestPercent", pe.GuestPercent)
	percent("people.syncedPercent", pe.SyncedPercent)
	percent("people.enabledPercent", pe.EnabledPercent)
	percent("people.contractorPercent", pe.ContractorPercent)
	text("people.managerWord", pe.ManagerWord, true)
	list("people.languages", len(pe.Languages), 1)
	for i := range pe.Languages {
		text(fmt.Sprintf("people.languages[%d].code", i), pe.Languages[i].Code, true)
		weight(fmt.Sprintf("people.languages[%d].weight", i), &pe.Languages[i].Weight)
	}

	fm := &p.Formats
	if err := checkPattern(fm.EmployeeID.Pattern, patternSeq); err != nil {
		bad("formats.employeeId.pattern", "%v", err)
	}
	if fm.EmployeeID.Start < 0 || fm.EmployeeID.Start > 1_000_000_000 {
		bad("formats.employeeId.start", "must be from 0 to 1000000000")
	}
	for field, pat := range map[string]string{"formats.mobilePhone": fm.MobilePhone, "formats.businessPhone": fm.BusinessPhone, "formats.costCenter": fm.CostCenter} {
		if err := checkPattern(pat, patternFree); err != nil {
			bad(field, "%v", err)
		}
	}
	list("formats.streets", len(fm.Streets), 1)
	for i, s := range fm.Streets {
		text(fmt.Sprintf("formats.streets[%d]", i), s, true)
	}

	seen := map[int]bool{}
	for i := range p.Extension {
		x := &p.Extension[i]
		f := fmt.Sprintf("extensionAttributes[%d]", i)
		if x.Index < 1 || x.Index > 15 {
			bad(f+".index", "must be from 1 to 15")
		}
		if seen[x.Index] {
			bad(f+".index", "%d is there twice", x.Index)
		}
		seen[x.Index] = true
		switch {
		case len(x.Values) > 0 && x.Pattern != "":
			bad(f, "has values and a pattern: give one of them")
		case len(x.Values) == 0 && x.Pattern == "":
			bad(f, "needs values or a pattern")
		}
		list(f+".values", len(x.Values), 0)
		for j := range x.Values {
			text(fmt.Sprintf("%s.values[%d].value", f, j), x.Values[j].Value, true)
			weight(fmt.Sprintf("%s.values[%d].weight", f, j), &x.Values[j].Weight)
		}
		if x.Pattern != "" {
			if err := checkPattern(x.Pattern, patternFree); err != nil {
				bad(f+".pattern", "%v", err)
			}
		}
		if x.Fill == nil {
			full := 100
			x.Fill = &full
		}
		percent(f+".fill", *x.Fill)
	}
	if len(p.Extension) > 15 {
		bad("extensionAttributes", "has more than 15 items")
	}

	g := &p.Groups
	text("groups.prefix", g.Prefix, true)
	dept("groups.dynamicDepartment", g.DynamicDepartment, false)
	dept("groups.adminDepartment", g.AdminDepartment, false)
	dept("groups.legalDepartment", g.LegalDepartment, false)
	for i, d := range g.Microsoft365 {
		dept(fmt.Sprintf("groups.microsoft365Departments[%d]", i), d, true)
	}
	for i, d := range g.StrayDepartments {
		dept(fmt.Sprintf("groups.strayDepartments[%d]", i), d, true)
	}
	if len(g.Apps) > 20 {
		bad("groups.apps", "has more than 20 items")
	}
	for i, a := range g.Apps {
		f := fmt.Sprintf("groups.apps[%d]", i)
		text(f+".name", a.Name, true)
		if a.Name != slug(a.Name) || strings.ContainsAny(a.Name, ". ") {
			bad(f+".name", "%q must be lowercase letters, digits, and hyphens", a.Name)
		}
		text(f+".title", a.Title, true)
		for j, d := range a.Contributor {
			dept(fmt.Sprintf("%s.contributorDepartments[%d]", f, j), d, true)
		}
		for j, d := range a.Reader {
			dept(fmt.Sprintf("%s.readerDepartments[%d]", f, j), d, true)
		}
	}

	text("owner.jobTitle", p.Owner.JobTitle, true)
	dept("owner.department", p.Owner.Department, true)
	if err := checkAdminPattern(p.Admin.UserPrincipalName, true); err != nil {
		bad("admin.userPrincipalName", "%v", err)
	}
	if err := checkAdminPattern(p.Admin.DisplayName, false); err != nil {
		bad("admin.displayName", "%v", err)
	}
	text("admin.jobTitle", p.Admin.JobTitle, true)
	dept("admin.department", p.Admin.Department, true)

	return errors.Join(errs...)
}

// Clone gives a copy that can be changed.
func (p *Profile) Clone() *Profile {
	b, _ := json.Marshal(p)
	var c Profile
	_ = json.Unmarshal(b, &c)
	return &c
}
