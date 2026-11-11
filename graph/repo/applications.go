package repo

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/withholm/dolly-cli/graph/permissions"
)

// The app registrations of the tenant: /applications. An application is the definition of an app, and it lives in its home tenant only. A service
// principal (apps.go) is the instance of it in a tenant. They are linked by the application (client) ID, appId, and by nothing else: the object IDs
// differ. A service principal with no application here is an app of another tenant (a third-party app, Microsoft Graph, a client of Microsoft).
//
// The first part keeps the two apart: an application does not copy its appRoles and scopes to its service principal, and the service principal keeps
// its own. Checked against real Graph (the sample tenant, GET only): the object and its properties, GET by ID, by appId (applications(appId='...')),
// and $filter=appId eq, the 404 for an ID that is not there, and an empty owners list. A guess (the pages of Learn name the calls, a write cannot be
// tried): what PATCH and DELETE answer, and that a PATCH of an object property changes only the properties that it names.

// Events that the applications make.
const (
	evAppCreate = "application.create"
	evAppUpdate = "application.update"
	evAppDelete = "application.delete"
)

// ApplicationProps are the properties that a call can set, with their kinds. The rest is read-only.
var ApplicationProps = map[string]propKind{
	"displayName": kString, "description": kString, "notes": kString, "signInAudience": kString, "groupMembershipClaims": kString,
	"serviceManagementReference": kString, "defaultRedirectUri": kString, "samlMetadataUrl": kString,
	"identifierUris": kList, "tags": kList, "requiredResourceAccess": kList, "appRoles": kList, "addIns": kList,
	"web": kObject, "spa": kObject, "publicClient": kObject, "api": kObject, "info": kObject, "optionalClaims": kObject,
	"parentalControlSettings": kObject, "isFallbackPublicClient": kBool, "isDeviceOnlyAuthSupported": kBool,
}

// applicationFixed cannot be set by a call.
var applicationFixed = []string{"id", "appId", "createdDateTime", "deletedDateTime", "publisherDomain", "createdByAppId", "passwordCredentials", "keyCredentials"}

// signInAudiences are the values that Graph takes.
var signInAudiences = []string{"AzureADMyOrg", "AzureADMultipleOrgs", "AzureADandPersonalMicrosoftAccount", "PersonalMicrosoftAccount"}

// applicationObject is a new application with the values that Graph gives it, and the properties as real Graph returns them (null where it is null).
func (w *World) newApplication(id, appID, name string, created time.Time) Object {
	return Object{
		"id": id, "appId": appID, "displayName": name,
		"addIns": []any{}, "api": Object{"acceptMappedClaims": nil, "knownClientApplications": []any{}, "requestedAccessTokenVersion": nil, "oauth2PermissionScopes": []any{}, "preAuthorizedApplications": []any{}},
		"appRoles": []any{}, "applicationTemplateId": nil, "certification": nil, "createdByAppId": nil,
		"createdDateTime":    created.UTC().Format("2006-01-02T15:04:05Z"),
		"defaultRedirectUri": nil, "deletedDateTime": nil, "description": nil, "disabledByMicrosoftStatus": nil,
		"groupMembershipClaims": nil, "identifierUris": []any{},
		"info":                      Object{"logoUrl": nil, "marketingUrl": nil, "privacyStatementUrl": nil, "supportUrl": nil, "termsOfServiceUrl": nil},
		"isDeviceOnlyAuthSupported": nil, "isDisabled": nil, "isFallbackPublicClient": false, "keyCredentials": []any{},
		"nativeAuthenticationApisEnabled": nil, "notes": nil, "optionalClaims": nil,
		"parentalControlSettings": Object{"countriesBlockedForMinors": []any{}, "legalAgeGroupRule": "Allow"},
		"passwordCredentials":     []any{}, "publicClient": Object{"redirectUris": []any{}}, "publisherDomain": w.defaultDomain(),
		"requestSignatureVerification": nil, "requiredResourceAccess": []any{}, "samlMetadataUrl": nil, "serviceManagementReference": nil,
		"servicePrincipalLockConfiguration": nil, "signInAudience": "AzureADMyOrg", "spa": Object{"redirectUris": []any{}}, "tags": []any{},
		"tokenEncryptionKeyId": nil, "uniqueName": nil,
		"verifiedPublisher": Object{"displayName": nil, "verifiedPublisherId": nil, "addedDateTime": nil},
		"web":               Object{"homePageUrl": nil, "logoutUrl": nil, "redirectUris": []any{}, "implicitGrantSettings": Object{"enableAccessTokenIssuance": false, "enableIdTokenIssuance": false}},
	}
}

func (w *World) addApplication(a Object) {
	w.Applications = append(w.Applications, a)
	w.apps[strings.ToLower(str(a["id"]))] = a
	w.appsByAppID[strings.ToLower(str(a["appId"]))] = a
}

func (w *World) removeApplication(id string) {
	if a := w.apps[strings.ToLower(id)]; a != nil {
		delete(w.appsByAppID, strings.ToLower(str(a["appId"])))
	}
	delete(w.apps, strings.ToLower(id))
	w.Applications = slices.DeleteFunc(w.Applications, func(a Object) bool { return isID(str(a["id"]), id) })
}

// findApplication finds an application by its object ID, or by its appId (the alternate key, which a call writes as applications(appId='...')).
// Call it with the lock held.
func (w *World) findApplication(ref string) Object {
	ref = strings.ToLower(ref)
	if a := w.apps[ref]; a != nil {
		return a
	}
	return w.appsByAppID[ref]
}

// ListApplications gives the applications as copies, in the order of the data.
func (w *World) ListApplications() []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	items := make([]Object, len(w.Applications))
	for i, a := range w.Applications {
		items[i] = deepCopy(a)
	}
	return items
}

// GetApplication gives a copy of an application. ref is the object ID, or the appId.
func (w *World) GetApplication(ref string) (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	a := w.findApplication(ref)
	if a == nil {
		return nil, false
	}
	return deepCopy(a), true
}

// unknownPermissionID is the answer for an ID of a permission of Microsoft Graph that dollycli does not know. Real Graph answers 400 "Permission being
// assigned was not found" for an app role assignment. dollycli answers 404 and says what to do: its list of permissions is not all of Microsoft's, and
// its IDs for some of them are not Microsoft's.
func unknownPermissionID(resource, id string) *apiError {
	return &apiError{Status: http.StatusNotFound, Code: "Request_ResourceNotFound", Message: fmt.Sprintf(
		"dollycli does not know a permission of %s with the ID '%s'. Add the permission in the portal, or use the permission names. A permission that is not in the list of dollycli can be added with the permissions of your own tenant (docs/permissions.md).", resource, id)}
}

// checkRequiredResourceAccess checks the shape of requiredResourceAccess: a list of {resourceAppId, resourceAccess: [{id, type}]}. For Microsoft Graph it
// also checks that dollycli knows each permission ID (see unknownPermissionID). The permissions of another resource are not checked.
func checkRequiredResourceAccess(v any) *apiError {
	if v == nil {
		return nil
	}
	list, _ := v.([]any)
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok || str(entry["resourceAppId"]) == "" {
			return badRequest("Invalid value specified for property 'requiredResourceAccess' of resource 'Application': each item needs a resourceAppId.")
		}
		access, _ := entry["resourceAccess"].([]any)
		for _, x := range access {
			r, ok := x.(map[string]any)
			kind := permissions.Delegated
			switch str(r["type"]) {
			case "Role":
				kind = permissions.Application
			case "Scope":
			default:
				ok = false
			}
			if !ok || str(r["id"]) == "" {
				return badRequest("Invalid value specified for property 'requiredResourceAccess' of resource 'Application': each resourceAccess needs an id and a type (Role or Scope).")
			}
			if isID(str(entry["resourceAppId"]), permissions.GraphAppID) {
				if _, known := catalogOnce().ByID(kind, str(r["id"])); !known {
					return unknownPermissionID(permissions.GraphName, str(r["id"]))
				}
			}
		}
	}
	return nil
}

func checkApplicationBody(body map[string]any) *apiError {
	if e := checkProps(body, ApplicationProps, "Application"); e != nil {
		return e
	}
	if v, has := body["signInAudience"]; has {
		if s, _ := v.(string); !slices.Contains(signInAudiences, s) {
			return badRequest("Invalid value specified for property 'signInAudience' of resource 'Application'. Use %s.", strings.Join(signInAudiences, ", "))
		}
	}
	for _, k := range []string{"identifierUris", "tags", "requiredResourceAccess", "appRoles", "addIns", "displayName", "isFallbackPublicClient"} {
		if v, has := body[k]; has && v == nil {
			return badRequest("Property '%s' cannot be null.", k)
		}
	}
	return checkRequiredResourceAccess(body["requiredResourceAccess"])
}

// CreateApplication makes an app registration. displayName is required. id, appId, and the dates are made. The application has no service principal
// until a client signs in (the sign-in API makes one) or someone makes it.
func (w *World) CreateApplication(props map[string]any, a Actor) (Object, *apiError) {
	body := maps.Clone(props)
	delete(body, "@odata.type")
	for _, k := range applicationFixed {
		if _, has := body[k]; has {
			return nil, badRequest("Property '%s' is read-only.", k)
		}
	}
	if name, _ := body["displayName"].(string); strings.TrimSpace(name) == "" {
		return nil, missing("displayName")
	}
	if e := checkApplicationBody(body); e != nil {
		return nil, e
	}
	if e := normalizeApplicationRoles(body, nil); e != nil {
		return nil, e
	}
	if e := normalizeApplicationAPI(body, nil); e != nil {
		return nil, e
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if e := w.checkIdentifierURIs(body["identifierUris"], ""); e != nil {
		return nil, e
	}
	app := w.newApplication(newGUID(), newGUID(), str(body["displayName"]), w.now())
	maps.Copy(app, deepCopy(body))
	app = deepCopy(app)
	w.addApplication(app)
	w.record(Event{Type: evAppCreate, Actor: a, App: str(app["id"]), AppName: str(app["displayName"]), Object: deepCopy(app)})
	return deepCopy(app), nil
}

// UpdateApplication changes properties of an application, found by object ID or appId. An object property (web, spa) changes only the properties that
// the call names.
func (w *World) UpdateApplication(ref string, props map[string]any, a Actor) *apiError {
	body := maps.Clone(props)
	delete(body, "@odata.type")
	for _, k := range applicationFixed {
		if _, has := body[k]; has {
			return badRequest("Property '%s' cannot be changed.", k)
		}
	}
	if e := checkApplicationBody(body); e != nil {
		return e
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	app := w.findApplication(ref)
	if app == nil {
		return notFound("application", ref)
	}
	if e := normalizeApplicationRoles(body, rolesFromList(app["appRoles"])); e != nil {
		return e
	}
	if e := normalizeApplicationAPI(body, app); e != nil {
		return e
	}
	if uris, has := body["identifierUris"]; has {
		if e := w.checkIdentifierURIs(uris, str(app["id"])); e != nil {
			return e
		}
	}
	if len(body) > 0 {
		mergeApplication(app, deepCopy(body))
		w.syncRolesFromApp(app)
		w.syncAPIFromApp(app)
		w.record(Event{Type: evAppUpdate, Actor: a, App: str(app["id"]), AppName: str(app["displayName"]), Set: deepCopy(body)})
	}
	return nil
}

// mergeApplication sets the properties of a change. An object property is merged one level: the other properties of it stay.
func mergeApplication(app Object, set map[string]any) {
	for k, v := range set {
		if nv, ok := v.(map[string]any); ok {
			if old, ok := app[k].(map[string]any); ok {
				maps.Copy(old, nv)
				continue
			}
		}
		app[k] = v
	}
}

// DeleteApplication deletes an application. Its service principal is not deleted (a guess: Learn does not say what Graph does with it).
func (w *World) DeleteApplication(ref string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	app := w.findApplication(ref)
	if app == nil {
		return notFound("application", ref)
	}
	id, name := str(app["id"]), str(app["displayName"])
	w.removeApplication(id)
	w.record(Event{Type: evAppDelete, Actor: a, App: id, AppName: name})
	return nil
}

// ---- the history of applications ----

func applicationEventText(e Event) (string, bool) {
	switch e.Type {
	case evAppCreate:
		return fmt.Sprintf("Registered the app %s", firstNonEmpty(e.AppName, e.App)), true
	case evAppUpdate:
		return fmt.Sprintf("Changed the app registration %s: %s", firstNonEmpty(e.AppName, e.App), setText(e.Set)), true
	case evAppDelete:
		return fmt.Sprintf("Deleted the app registration %s", firstNonEmpty(e.AppName, e.App)), true
	}
	return "", false
}

func (w *World) applyApplicationEvent(e Event) bool {
	switch e.Type {
	case evAppCreate:
		if e.Object != nil && w.apps[strings.ToLower(e.App)] == nil {
			w.addApplication(deepCopy(e.Object))
		}
	case evAppUpdate:
		if app := w.apps[strings.ToLower(e.App)]; app != nil {
			mergeApplication(app, deepCopy(e.Set))
			w.syncRolesFromApp(app)
			w.syncAPIFromApp(app)
		}
	case evAppDelete:
		w.removeApplication(e.App)
	default:
		return false
	}
	return true
}

// ---- what the page of the service principals shows ----

// ApplicationInfo is an app registration for the portal.
type ApplicationInfo struct {
	ID               string `json:"id"`
	AppID            string `json:"appId"`
	Name             string `json:"displayName"`
	SignInAudience   string `json:"signInAudience"`
	Requested        int    `json:"requested"`                  // permissions that it asks for (requiredResourceAccess)
	Roles            int    `json:"roles"`                      // app roles that it defines (appRoles)
	ServicePrincipal string `json:"servicePrincipal,omitempty"` // the object ID of the service principal of the same appId, in this tenant
}

// ListApplicationInfo lists the app registrations for the portal.
func (w *World) ListApplicationInfo() []ApplicationInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]ApplicationInfo, 0, len(w.Applications))
	for _, a := range w.Applications {
		i := ApplicationInfo{ID: str(a["id"]), AppID: str(a["appId"]), Name: str(a["displayName"]), SignInAudience: str(a["signInAudience"])}
		i.Roles = len(anyObjects(a["appRoles"]))
		list, _ := a["requiredResourceAccess"].([]any)
		for _, r := range list {
			entry, _ := r.(map[string]any)
			access, _ := entry["resourceAccess"].([]any)
			i.Requested += len(access)
		}
		if sp := w.byAppID[strings.ToLower(i.AppID)]; sp != nil {
			i.ServicePrincipal = sp.ID
		}
		out = append(out, i)
	}
	return out
}
