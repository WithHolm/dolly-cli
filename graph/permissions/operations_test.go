package permissions

import (
	"slices"
	"strings"
	"testing"
)

// The two forms of the table of permissions of Microsoft Learn, as they are in the files.
const includeForm = `---
description: "Automatically generated file. DO NOT MODIFY"
---

|Permission type|Least privileged permissions|Higher privileged permissions|
|:---|:---|:---|
|Delegated (work or school account)|User.ReadBasic.All|User.Read.All, User.ReadWrite.All, Directory.Read.All|
|Delegated (personal Microsoft account)|Not supported.|Not supported.|
|Application|User.Read.All|User.ReadWrite.All, Directory.Read.All, Directory.ReadWrite.All|
`

const pageForm = `## Permissions

One of the following permissions is required to call this API.
|Permission type      | Permissions (from least to most privileged)              |
|:--------------------|:---------------------------------------------------------|
|Delegated (work or school account) | User.ReadBasic.All, User.Read.All, Directory.Read.All    |
|Delegated (personal Microsoft account) | Not supported.    |
|Application | Not supported. |

Guests can't call this API.
`

const andForm = `|Permission type|Least privileged permissions|Higher privileged permissions|
|:---|:---|:---|
|Delegated (work or school account)|User.Read and User.ReadBasic.All|Directory.Read.All, User.Read.All|
|Application|AppRoleAssignment.ReadWrite.All and Application.Read.All|AppRoleAssignment.ReadWrite.All and Directory.Read.All, Application.ReadWrite.All|
`

func TestOperationPermissions_ParsesTheTableOfAnIncludeFile(t *testing.T) {
	got, dropped, ok := ParseTable([]byte(includeForm))
	if !ok || !got.Table || len(dropped) != 0 {
		t.Fatalf("%v %v %+v", ok, dropped, got)
	}
	if !slices.Equal(got.Delegated.Least, []string{"User.ReadBasic.All"}) || !slices.Equal(got.Delegated.Higher, []string{"User.Read.All", "User.ReadWrite.All", "Directory.Read.All"}) {
		t.Errorf("delegated: %+v", got.Delegated)
	}
	if !slices.Equal(got.Application.Least, []string{"User.Read.All"}) || len(got.Application.Higher) != 3 || got.Application.NotSupported {
		t.Errorf("application: %+v", got.Application)
	}
}

func TestOperationPermissions_ParsesTheTableInsideAPage(t *testing.T) {
	got, _, ok := ParseTable([]byte(pageForm))
	if !ok {
		t.Fatal("no table")
	}
	// from least to most privileged: the first is the least
	if !slices.Equal(got.Delegated.Least, []string{"User.ReadBasic.All"}) || !slices.Equal(got.Delegated.Higher, []string{"User.Read.All", "Directory.Read.All"}) {
		t.Errorf("delegated: %+v", got.Delegated)
	}
	if !got.Application.NotSupported || len(got.Application.Least) != 0 {
		t.Errorf("application: %+v", got.Application)
	}
}

// "A and B" is one alternative that needs both: it is kept as it is, and not dropped.
func TestOperationPermissions_AnAlternativeThatNeedsTwo(t *testing.T) {
	got, dropped, _ := ParseTable([]byte(andForm))
	if len(dropped) != 0 {
		t.Fatalf("dropped %v", dropped)
	}
	if !slices.Equal(got.Delegated.Least, []string{"User.Read and User.ReadBasic.All"}) {
		t.Errorf("%+v", got.Delegated)
	}
	if !slices.Equal(got.Application.Higher, []string{"AppRoleAssignment.ReadWrite.All and Directory.Read.All", "Application.ReadWrite.All"}) {
		t.Errorf("%+v", got.Application)
	}
	if !slices.Equal(Alternatives(got.Delegated.Least[0]), []string{"User.Read", "User.ReadBasic.All"}) {
		t.Error("the parts of an alternative")
	}
}

// Text that is no permission is not kept, and it is given back, so a tool can stop when the form of a table has changed.
func TestOperationPermissions_AnUnreadablePartIsReported(t *testing.T) {
	_, dropped, _ := ParseTable([]byte("|Permission type|Least privileged permissions|Higher privileged permissions|\n|:-|:-|:-|\n|Application|See the note below|Directory.Read.All|\n"))
	if len(dropped) != 1 || !strings.Contains(dropped[0], "See the note") {
		t.Errorf("%v", dropped)
	}
	if _, _, ok := ParseTable([]byte("# A page with no table\n\nText.")); ok {
		t.Error("no table")
	}
}

// A table by type of object: the permission depends on the object. The first cell can have a link, a path, and words.
const variantForm = `## Permissions

The following table shows the least privileged permission that's required by each resource type when calling this API.

| Supported resource                        | Delegated (work or school account)                      | Delegated (personal Microsoft account) | Application                                             |
|:------------------------------------------|:--------------------------------------------------------|:---------------------------------------|:--------------------------------------------------------|
| [device](../resources/device.md)          | GroupMember.ReadWrite.All and Device.Read.All           | Not supported.                         | GroupMember.ReadWrite.All and Device.ReadWrite.All      |
| [group](../resources/group.md)            | GroupMember.ReadWrite.All                               | Not supported.                         | GroupMember.ReadWrite.All       |
|[callRecording](../resources/callrecording.md) <br /> ` + "`communications/onlineMeetings/getAllRecordings`" + ` <br /> All recordings. | Not supported. | Not supported. | OnlineMeetingRecording.Read.All |

| Other table | Not a table of permissions |
|:--|:--|
| Application | Nothing.Here |
`

func TestOperationPermissions_ATableByTypeOfObject(t *testing.T) {
	got, dropped, ok := ParseTable([]byte(variantForm))
	if !ok || !got.Table || len(dropped) != 0 || len(got.Variants) != 3 {
		t.Fatalf("%v %v %+v", ok, dropped, got)
	}
	if got.Delegated.Least != nil || got.Application.Least != nil {
		t.Errorf("the top is empty when the call depends on the object: %+v %+v", got.Delegated, got.Application)
	}
	dev, rec := got.Variants[0], got.Variants[2]
	if dev.Resource != "device" || !slices.Equal(dev.Delegated.Least, []string{"GroupMember.ReadWrite.All and Device.Read.All"}) || !slices.Equal(dev.Application.Least, []string{"GroupMember.ReadWrite.All and Device.ReadWrite.All"}) {
		t.Errorf("device: %+v", dev)
	}
	if rec.Resource != "callRecording" || rec.Detail != "communications/onlineMeetings/getAllRecordings · All recordings." || !rec.Delegated.NotSupported || !slices.Equal(rec.Application.Least, []string{"OnlineMeetingRecording.Read.All"}) {
		t.Errorf("callRecording: %+v", rec)
	}
	// the table after it is not part of it
	for _, v := range got.Variants {
		if v.Resource == "Application" {
			t.Error("a row of the next table")
		}
	}
}

// Words after a permission say when it holds, and a star is resource-specific consent. They are kept, not dropped.
func TestOperationPermissions_ConditionsAfterAName(t *testing.T) {
	got, dropped, _ := ParseTable([]byte(`|Permission type|Least privileged permissions|Higher privileged permissions|
|:-|:-|:-|
|Application|OnlineMeetingRecording.Read.All (for online meetings), CallRecording.Read.All (for ad hoc calls)|Chat.Read.Group*, Directory.Read.All|
`))
	if len(dropped) != 0 {
		t.Fatalf("dropped %v", dropped)
	}
	n := got.Application
	if !slices.Equal(n.Least, []string{"OnlineMeetingRecording.Read.All", "CallRecording.Read.All"}) || !slices.Equal(n.Higher, []string{"Chat.Read.Group", "Directory.Read.All"}) {
		t.Errorf("%+v", n)
	}
	if n.Conditions["OnlineMeetingRecording.Read.All"] != "for online meetings" || n.Conditions["CallRecording.Read.All"] != "for ad hoc calls" || n.Conditions["Chat.Read.Group"] != "resource-specific consent" {
		t.Errorf("conditions: %v", n.Conditions)
	}
}

const requestsPage = `# Get user

## HTTP request

For a specific user:
<!-- { "blockType": "ignored" } -->
` + "```http" + `
GET /me
GET /users/{id | userPrincipalName}
GET /users/{id}/memberOf/microsoft.graph.group?$count=true
` + "```" + `

### More

` + "```http" + `
DELETE /users/{id}/manager/$ref
` + "```" + `

## Request headers

` + "```http" + `
GET /not/in/the/section
` + "```" + `

## Response

` + "```http" + `
GET https://graph.microsoft.com/v1.0/me
` + "```" + `
`

func TestOperationPermissions_RequestsOfAPage(t *testing.T) {
	got := RequestsOf([]byte(requestsPage))
	want := []string{"GET /me", "GET /users/{}", "GET /users/{}/memberof/graph.group", "DELETE /users/{}/manager/$ref"}
	if !slices.Equal(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
	// the description and a page write a call alike
	if CallKey("get", "/users/{user-id}/memberOf/microsoft.graph.group") != "GET /users/{}/memberof/graph.group" || CallKey("GET", "/users/{id | userPrincipalName}/") != "GET /users/{}" {
		t.Errorf("%q %q", CallKey("get", "/users/{user-id}/memberOf/microsoft.graph.group"), CallKey("GET", "/users/{id | userPrincipalName}/"))
	}
}

func TestOperationPermissions_ChoosingThePageOfACall(t *testing.T) {
	for name, c := range map[string]struct {
		call    string
		spec    string
		listing map[string]int
		want    CallRef
	}{
		"the link is right":       {"GET /users/{}", "user-get", map[string]int{"user-get": 2, "other": 1}, CallRef{"user-get", "verified", []string{"other"}}},
		"the link is wrong":       {"GET /me/messages", "opentypeextension-get", map[string]int{"user-list-messages": 3}, CallRef{"user-list-messages", "matched", nil}},
		"no link, one page":       {"GET /me/messages", "", map[string]int{"user-list-messages": 3}, CallRef{"user-list-messages", "matched", nil}},
		"no link, many pages":     {"GET /x", "", map[string]int{"b": 5, "a": 5, "c": 1}, CallRef{"c", "matched", []string{"a", "b"}}},
		"only the link":           {"GET /users/{}", "user-get", nil, CallRef{"user-get", "spec-only", nil}},
		"nothing":                 {"GET /x", "", nil, CallRef{}},
		"the link and a page":     {"GET /x", "x", map[string]int{"y": 1}, CallRef{"y", "matched", nil}},
		"a page about fewer wins": {"GET /x", "", map[string]int{"big": 20, "small": 2}, CallRef{"small", "matched", []string{"big"}}},
		// the name of the page decides between pages that list the same call
		"a page for another kind of user": {"GET /users/{}/memberof", "agentuser-list-memberof", map[string]int{"agentuser-list-memberof": 1, "user-list-memberof": 2},
			CallRef{"user-list-memberof", "matched", []string{"agentuser-list-memberof"}}},
		"the link to a wrong message page": {"DELETE /me/messages/{}", "eventmessage-delete", map[string]int{"eventmessage-delete": 1, "message-delete": 2},
			CallRef{"message-delete", "matched", []string{"eventmessage-delete"}}},
		"the group page is more specific": {"DELETE /groups/{}/events/{}", "group-delete-event", map[string]int{"group-delete-event": 1, "event-delete": 3},
			CallRef{"group-delete-event", "verified", []string{"event-delete"}}},
		"the generic page and the pages of a product": {"DELETE /users/{}", "user-delete", map[string]int{"user-delete": 1, "intune-mam-user-delete": 1},
			CallRef{"user-delete", "verified", []string{"intune-mam-user-delete"}}},
		"the list and a page about a property": {"GET /users", "user-list", map[string]int{"user-list": 1, "user-list-manager": 1},
			CallRef{"user-list", "verified", []string{"user-list-manager"}}},
	} {
		got := ResolveCall(c.call, c.spec, c.listing)
		if got.Page != c.want.Page || got.Via != c.want.Via || !slices.Equal(got.Others, c.want.Others) {
			t.Errorf("%s: %+v, want %+v", name, got, c.want)
		}
	}
}

// implies.json: what a permission is assumed to include, and chains are followed.
func TestOperationPermissions_Assumptions(t *testing.T) {
	for name, a := range Assumptions() {
		if a.Note == "" || len(a.Includes) == 0 {
			t.Errorf("%s: it needs includes and a note", name)
		}
	}
	got := Includes("user.readupdate.all")
	if !slices.Contains(got, "user.read.all") || !slices.Contains(got, "user.readbasic.all") {
		t.Errorf("includes: %v", got)
	}
	if len(Includes("User.Read.All")) != 0 {
		t.Error("a permission with no entry includes nothing")
	}
}

// A page with two tables, one under each heading: the signed-in user and another user. The include files are read through a function.
const twoTablesPage = `# List a user's direct memberships

## Permissions

Choose the permission or permissions marked as least privileged for this API.

### Permissions for the signed-in user's direct memberships

[!INCLUDE [permissions-table](../includes/permissions/me-permissions.md)]

### Permissions for another user's direct memberships

[!INCLUDE [permissions-table](../includes/permissions/other-permissions.md)]

## HTTP request

` + "```http" + `
GET /me/memberOf
` + "```" + `
`

func TestOperationPermissions_ParsePage_SeveralTables(t *testing.T) {
	includes := map[string]string{
		"../includes/permissions/me-permissions.md":    "|Permission type|Least privileged permissions|Higher privileged permissions|\n|:-|:-|:-|\n|Delegated (work or school account)|User.Read|Directory.Read.All|\n|Application|Not supported.|Not supported.|\n",
		"../includes/permissions/other-permissions.md": "|Permission type|Least privileged permissions|Higher privileged permissions|\n|:-|:-|:-|\n|Delegated (work or school account)|User.Read.All|Directory.Read.All|\n|Application|Directory.Read.All|Directory.ReadWrite.All|\n",
	}
	got, dropped, notFound, err := ParsePage([]byte(twoTablesPage), func(rel string) ([]byte, bool, error) {
		b, ok := includes[rel]
		return []byte(b), !ok, nil
	})
	if err != nil || len(dropped) != 0 || len(notFound) != 0 || !got.Table || len(got.Variants) != 2 {
		t.Fatalf("%v %v %v %+v", err, dropped, notFound, got)
	}
	if got.Delegated.Least != nil {
		t.Error("with more than one table the top is empty")
	}
	me, other := got.Variants[0], got.Variants[1]
	if me.Scenario != "Permissions for the signed-in user's direct memberships" || !me.Application.NotSupported || !slices.Equal(me.Delegated.Least, []string{"User.Read"}) {
		t.Errorf("me: %+v", me)
	}
	if !slices.Equal(other.Application.Least, []string{"Directory.Read.All"}) || !slices.Equal(other.Delegated.Least, []string{"User.Read.All"}) {
		t.Errorf("other: %+v", other)
	}
	// the path picks the scenario
	if d, a, ok := got.Resolve("/v1.0/me/memberOf"); !ok || !slices.Equal(d.Least, []string{"User.Read"}) || !a.NotSupported {
		t.Errorf("/me: %+v %+v %v", d, a, ok)
	}
	if d, a, ok := got.Resolve("/v1.0/users/{id}/memberOf"); !ok || !slices.Equal(d.Least, []string{"User.Read.All"}) || !slices.Equal(a.Least, []string{"Directory.Read.All"}) {
		t.Errorf("/users: %+v %+v %v", d, a, ok)
	}
	// a missing include file is reported
	_, _, notFound, _ = ParsePage([]byte(twoTablesPage), func(rel string) ([]byte, bool, error) { return nil, true, nil })
	if len(notFound) != 2 {
		t.Errorf("not found: %v", notFound)
	}
}

// Scenarios that are not about the signed-in user and another user cannot be picked by the path.
func TestOperationPermissions_ResolveCannotAlwaysPick(t *testing.T) {
	c := CallPermissions{Table: true, Variants: []Variant{{Scenario: "Permissions for channel"}, {Scenario: "Permissions for chat"}}}
	if _, _, ok := c.Resolve("/v1.0/me/chats/{id}/messages"); ok {
		t.Error("channel or chat: the path does not say")
	}
	byObject := CallPermissions{Table: true, Variants: []Variant{{Resource: "user"}, {Resource: "group"}}}
	if _, _, ok := byObject.Resolve("/v1.0/users/{id}"); ok {
		t.Error("by type of object: the object decides")
	}
	one := CallPermissions{Table: true, Delegated: Need{Least: []string{"A"}}}
	if d, _, ok := one.Resolve("/v1.0/anything"); !ok || d.Least[0] != "A" {
		t.Error("one table: those are the permissions")
	}
}

// A route with no page of its own takes the page of the route that it is a form of.
func TestOperationPermissions_ARouteThatIsAFormOfAnother(t *testing.T) {
	routes := map[string]string{
		"GET /v1.0/users":                      "user-list",
		"GET /v1.0/users/{id}/memberOf":        "user-list-memberof",
		"GET /v1.0/groups/{id}/members/{type}": "group-list-members",
		"PUT /v1.0/users/{id}/manager/$ref":    "user-post-manager",
	}
	for route, want := range map[string]string{
		"GET /v1.0/users/$count":                              "GET /v1.0/users",
		"GET /v1.0/users/{id}/memberOf/microsoft.graph.group": "GET /v1.0/users/{id}/memberOf",
		"GET /v1.0/me/memberOf/$count":                        "GET /v1.0/users/{id}/memberOf",
		"GET /v1.0/me/memberOf/microsoft.graph.group":         "GET /v1.0/users/{id}/memberOf",
		"GET /v1.0/groups/{id}/members/{type}/$count":         "GET /v1.0/groups/{id}/members/{type}",
		"PUT /v1.0/me/manager/$ref":                           "PUT /v1.0/users/{id}/manager/$ref",
	} {
		if got, ok := ParentRoute(route, routes); !ok || got != want {
			t.Errorf("%s: %q %v, want %q", route, got, ok, want)
		}
	}
	for _, route := range []string{"POST /v1.0/$batch", "GET /v1.0/places/$count", "GET /v1.0/memberOf/$count"} {
		if got, ok := ParentRoute(route, routes); ok {
			t.Errorf("%s has no parent, and got %q", route, got)
		}
	}
}

func TestOperationPermissions_PageOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://learn.microsoft.com/graph/api/user-list?view=graph-rest-1.0":               "user-list",
		"https://learn.microsoft.com/en-us/graph/api/group-post-owners?view=graph-rest-1.0": "group-post-owners",
	} {
		if got, ok := PageOf(in); !ok || got != want {
			t.Errorf("%s: %q %v", in, got, ok)
		}
	}
	if _, ok := PageOf("https://learn.microsoft.com/graph/overview"); ok {
		t.Error("not a page about a call")
	}
}

// The data that is built in: it loads, it says where it is from, and the permissions of known calls are as Microsoft's pages say them.
func TestOperationPermissions_TheBuiltInData(t *testing.T) {
	o, err := LoadOperations()
	if err != nil {
		t.Fatal(err)
	}
	if o.Source.Commit == "" || o.Source.CommitDate == "" || !strings.Contains(o.Source.License, "CC BY 4.0") || len(o.Pages) < 30 {
		t.Fatalf("source %+v, %d pages", o.Source, len(o.Pages))
	}
	list, ok := o.ForDoc("https://learn.microsoft.com/graph/api/user-list?view=graph-rest-1.0")
	if !ok || !slices.Equal(list.Delegated.Least, []string{"User.ReadBasic.All"}) || !slices.Equal(list.Application.Least, []string{"User.Read.All"}) {
		t.Errorf("user-list: %+v", list)
	}
	post, _ := o.ForDoc("https://learn.microsoft.com/graph/api/group-post-groups?view=graph-rest-1.0")
	if !slices.Equal(post.Application.Least, []string{"Group.Create"}) {
		t.Errorf("group-post-groups: %+v", post.Application)
	}
	if _, ok := o.ForDoc("https://learn.microsoft.com/graph/api/nothing-here"); ok {
		t.Error("a page that is not in the data")
	}
	// every permission in the data is a name of a permission, an alternative is one or more names, and a table says something for both kinds
	for page, p := range o.Pages {
		if !p.Table {
			continue
		}
		needs := map[string]Need{"delegated": p.Delegated, "application": p.Application}
		for _, v := range p.Variants {
			needs["delegated "+v.Scenario+" "+v.Resource], needs["application "+v.Scenario+" "+v.Resource] = v.Delegated, v.Application
		}
		for kind, n := range needs {
			if len(p.Variants) > 0 && (kind == "delegated" || kind == "application") {
				continue // the call depends on the object: the rows have the permissions
			}
			if len(n.Least) == 0 && !n.NotSupported {
				t.Errorf("%s %s has neither a least privileged permission nor 'not supported'", page, kind)
			}
			for _, alt := range append(slices.Clone(n.Least), n.Higher...) {
				for _, name := range Alternatives(alt) {
					if !nameRe.MatchString(name) {
						t.Errorf("%s %s: %q is not a permission", page, kind, name)
					}
				}
			}
		}
	}
}
