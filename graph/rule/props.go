package rule

import (
	"fmt"
	"regexp"
	"strings"
)

// kind is the type of a property in the rule language.
type kind int

const (
	kString kind = iota
	kBool
	kDate
	kCollection // a list of strings (otherMails, proxyAddresses): use -any or -all
)

// propSpec says how a property of a rule is read from a user. get gives the value as
// Graph writes it: a string, a bool, a []any of strings, or nil.
type propSpec struct {
	kind kind
	get  func(user map[string]any) any
}

func key(name string) propSpec {
	return propSpec{kString, func(u map[string]any) any { return u[name] }}
}

func boolKey(name string) propSpec {
	return propSpec{kBool, func(u map[string]any) any { return u[name] }}
}

// props are the user properties that the rule language of Entra has (Learn: "Manage rules for
// dynamic membership groups", "Supported properties"). The names are in lowercase: a name in a
// rule is read without regard to case. The rule language and Graph call some properties by
// different names, so each one says the name that it has in Graph.
var props = map[string]propSpec{
	"accountenabled":   boolKey("accountEnabled"),
	"dirsyncenabled":   boolKey("onPremisesSyncEnabled"),
	"employeehiredate": {kDate, func(u map[string]any) any { return u["employeeHireDate"] }},

	"city": key("city"), "country": key("country"), "companyname": key("companyName"),
	"department": key("department"), "displayname": key("displayName"), "employeeid": key("employeeId"),
	"facsimiletelephonenumber": key("faxNumber"), "givenname": key("givenName"), "jobtitle": key("jobTitle"),
	"mail": key("mail"), "mailnickname": key("mailNickname"), "mobile": key("mobilePhone"),
	"objectid": key("id"), "onpremisesdistinguishedname": key("onPremisesDistinguishedName"),
	"onpremisessecurityidentifier": key("onPremisesSecurityIdentifier"), "passwordpolicies": key("passwordPolicies"),
	"physicaldeliveryofficename": key("officeLocation"), "postalcode": key("postalCode"),
	"preferredlanguage": key("preferredLanguage"), "state": key("state"), "streetaddress": key("streetAddress"),
	"surname": key("surname"), "usagelocation": key("usageLocation"), "userprincipalname": key("userPrincipalName"),
	"usertype": key("userType"),
	// Graph has no sip proxy address on a user: it is null for every user here.
	"sipproxyaddress": {kString, func(map[string]any) any { return nil }},
	// the telephone number is the first of the business phones
	"telephonenumber": {kString, func(u map[string]any) any {
		if list, _ := u["businessPhones"].([]any); len(list) > 0 {
			return list[0]
		}
		return nil
	}},

	"othermails":     {kCollection, func(u map[string]any) any { return u["otherMails"] }},
	"proxyaddresses": {kCollection, func(u map[string]any) any { return u["proxyAddresses"] }},
}

func init() {
	for i := 1; i <= 15; i++ {
		name := fmt.Sprintf("extensionAttribute%d", i)
		props[strings.ToLower(name)] = propSpec{kString, func(u map[string]any) any {
			ext, _ := u["onPremisesExtensionAttributes"].(map[string]any)
			return ext[name]
		}}
	}
}

// customExtension is a custom extension property: extension_<app id without dashes>_<name>.
var customExtension = regexp.MustCompile(`^extension_[0-9a-z]{32}_\w+$`)

// notEvaluated are properties that the language has and that dolly does not work out. A rule
// that uses one is refused, so a tool does not get a group that looks right and is empty.
var notEvaluated = map[string]string{
	"memberof":      "dolly does not work out group membership inside a rule",
	"assignedplans": "dolly users have no assigned plans",
}

// lookup finds the spec of a property of a user rule.
func lookup(name string) (propSpec, error) {
	lower := strings.ToLower(name)
	if spec, ok := props[lower]; ok {
		return spec, nil
	}
	if why, ok := notEvaluated[lower]; ok {
		return propSpec{}, fmt.Errorf("the property user.%s is in the rule language, and %s", name, why)
	}
	if customExtension.MatchString(lower) {
		// a custom extension property: dolly users have none, so it is null
		return propSpec{kString, func(map[string]any) any { return nil }}, nil
	}
	return propSpec{}, fmt.Errorf("the property user.%s is not a property that a membership rule can use", name)
}
