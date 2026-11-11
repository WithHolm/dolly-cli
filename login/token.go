package login

import (
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/withholm/dolly-cli/graph/permissions"
	"github.com/withholm/dolly-cli/graph/repo"
)

const (
	ttl           = 3599 // seconds
	assertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
)

func itoa(n int) string { return strconv.Itoa(n) }

// isGraphAudience says if a token is for Microsoft Graph. Graph refuses a token for another audience.
func isGraphAudience(aud string) bool {
	return strings.EqualFold(aud, permissions.GraphAppID) || strings.EqualFold(strings.TrimSuffix(aud, "/"), "https://graph.microsoft.com")
}

// the scopes that are for the sign-in itself, not for a resource
var oidcScopes = []string{"openid", "profile", "email", "offline_access"}

func (a *API) token(w http.ResponseWriter, r *http.Request, tid string, v2 bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		a.fail(w, &oauthErr{http.StatusBadRequest, "invalid_request", 900144, "The request body is not a form."})
		return
	}
	resp, si, e := a.issue(r, tid, v2)
	if e != nil {
		si.Result, si.Message = "AADSTS"+itoa(e.aadsts), e.desc
		a.record(si)
		a.fail(w, e)
		return
	}
	si.Result = "issued"
	a.record(si)
	writeJSON(w, http.StatusOK, resp)
}

// issue does the grant. It gives the answer, or the error. It gives the line for the list of sign-ins in
// any case.
func (a *API) issue(r *http.Request, tid string, v2 bool) (map[string]any, SignIn, *oauthErr) {
	f := r.PostForm
	si := SignIn{Grant: f.Get("grant_type"), Credential: "none"}
	bad := func(status int, code string, n int, desc string) (map[string]any, SignIn, *oauthErr) {
		return nil, si, &oauthErr{status, code, n, desc}
	}

	// who is the client, and what does it show
	clientID, secret, hasBasic := r.BasicAuth()
	if !hasBasic {
		clientID, secret = "", f.Get("client_secret")
	}
	if clientID == "" {
		clientID = f.Get("client_id")
	}
	kind, value := repo.KindSecret, secret
	if f.Get("client_assertion_type") == assertionType && f.Get("client_assertion") != "" {
		header, payload, ok := jwtParts(f.Get("client_assertion"))
		if !ok {
			return bad(http.StatusUnauthorized, "invalid_client", 700025, "Client assertion is not a JWT.")
		}
		kind, value = repo.KindCertificate, firstNonEmpty(str(header["x5t#S256"]), str(header["x5t"]), str(header["kid"]))
		if clientID == "" {
			clientID = firstNonEmpty(str(payload["iss"]), str(payload["sub"]))
		}
	}
	if value != "" {
		si.Credential = kind
	}
	if clientID == "" {
		return bad(http.StatusBadRequest, "invalid_request", 900144, "The request body must contain the following parameter: 'client_id'.")
	}
	clientID = strings.ToLower(clientID)
	si.AppID = clientID

	if !slices.Contains([]string{"client_credentials", "password", "authorization_code", "refresh_token"}, f.Get("grant_type")) {
		return bad(http.StatusBadRequest, "unsupported_grant_type", 70003, "The app requested a grant type that Dolly does not support: '"+f.Get("grant_type")+"'. Use authorization_code, refresh_token, client_credentials, or password.")
	}

	// the service principal: made when this client ID is new
	_, existed := a.world.FindServicePrincipal(clientID)
	actor := repo.Actor{Source: "login", Agent: r.UserAgent()}
	sp, spErr := a.world.EnsureServicePrincipal(clientID, actor)
	if spErr != nil {
		return bad(http.StatusBadRequest, "invalid_request", 700016, "Application with identifier '"+clientID+"' was not found in the directory '"+tid+"'. The client ID must be a GUID.")
	}
	si.App, si.Registered = sp.Name, !existed
	if !a.world.CheckCredential(clientID, kind, value, actor) {
		if value == "" {
			return bad(http.StatusUnauthorized, "invalid_client", 7000218, "The request body must contain the following parameter: 'client_assertion' or 'client_secret'.")
		}
		if kind == repo.KindCertificate {
			return bad(http.StatusUnauthorized, "invalid_client", 700027, "Client assertion contains an invalid signature. The certificate is not the one that Dolly kept for this service principal.")
		}
		return bad(http.StatusUnauthorized, "invalid_client", 7000215, "Invalid client secret provided. Ensure the secret being sent in the request is the client secret value, not the client secret ID, for a secret added to app '"+clientID+"'.")
	}

	now := a.now()
	claims := map[string]any{
		"iss": issuerOf(r, tid), "tid": tid, "ver": "2.0",
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Unix() + ttl,
		"appid": clientID, "azp": clientID, "azpv": "2", "uti": guid(),
	}
	if !v2 {
		claims["iss"], claims["ver"] = "https://sts.windows.net/"+tid+"/", "1.0"
	}
	resp := map[string]any{"token_type": "Bearer", "expires_in": ttl, "ext_expires_in": ttl}

	scope := strings.Fields(f.Get("scope"))
	if len(scope) == 0 && f.Get("resource") != "" { // the v1 endpoint asks for a resource
		scope = []string{strings.TrimSuffix(f.Get("resource"), "/") + "/.default"}
	}

	switch f.Get("grant_type") {
	case "client_credentials":
		if len(scope) != 1 || !strings.HasSuffix(scope[0], "/.default") {
			return bad(http.StatusBadRequest, "invalid_scope", 1002012, "The provided value for scope '"+f.Get("scope")+"' is not valid. Client credential flows must have a scope value with /.default suffixed to the resource identifier (application ID URI).")
		}
		res, ok := a.resource(strings.TrimSuffix(scope[0], "/.default"))
		if !ok {
			return bad(http.StatusBadRequest, "invalid_resource", 500011, "The resource principal named "+strings.TrimSuffix(scope[0], "/.default")+" was not found in the tenant named "+tid+". This can happen if the application has not been installed by the administrator of the tenant or consented to by any user in the tenant.")
		}
		si.Resource = res.Name
		var roles []string
		if g, ok := a.world.GrantsOf(clientID); ok {
			for _, ag := range g.Application {
				if strings.EqualFold(ag.ResourceAppID, res.AppID) {
					roles = append(roles, ag.Value)
				}
			}
		}
		sort.Strings(roles)
		claims["aud"], claims["oid"], claims["sub"] = res.AppID, sp.ID, sp.ID
		claims["idtyp"], claims["appidacr"] = "app", "1"
		if kind == repo.KindCertificate {
			claims["appidacr"] = "2"
		}
		if len(slices.Compact(roles)) > 0 {
			claims["roles"] = slices.Compact(roles)
		}
	case "password":
		if f.Get("username") == "" || f.Get("password") == "" {
			return bad(http.StatusBadRequest, "invalid_request", 900144, "The request body must contain the following parameters: 'username' and 'password'.")
		}
		user, ok := a.world.GetUser(f.Get("username"))
		if !ok {
			return bad(http.StatusBadRequest, "invalid_grant", 50034, "The user account "+f.Get("username")+" does not exist in the "+tid+" directory. To sign into this application, the account must be added to the directory.")
		}
		if e := a.userTokens(&si, claims, resp, user, clientID, tid, scope, ""); e != nil {
			return nil, si, e
		}
	case "authorization_code":
		if f.Get("code") == "" {
			return bad(http.StatusBadRequest, "invalid_request", 900144, "The request body must contain the following parameter: 'code'.")
		}
		code, e := a.redeem(f.Get("code"), clientID, f.Get("redirect_uri"), f.Get("code_verifier"))
		if e != nil {
			return nil, si, e
		}
		user, ok := a.world.GetUser(code.userID)
		if !ok {
			return bad(http.StatusBadRequest, "invalid_grant", 50034, "The user of this code is not in the directory any more.")
		}
		if e := a.userTokens(&si, claims, resp, user, clientID, tid, strings.Fields(code.scope), code.nonce); e != nil {
			return nil, si, e
		}
	case "refresh_token":
		rt, ok := a.parseRefresh(f.Get("refresh_token"))
		if !ok {
			return bad(http.StatusBadRequest, "invalid_grant", 70000, "The refresh token is not valid, or it has expired. The user must sign in again.")
		}
		if !strings.EqualFold(str(rt["appid"]), clientID) {
			return bad(http.StatusBadRequest, "invalid_grant", 70000, "The refresh token was issued to another client.")
		}
		user, ok := a.world.GetUser(str(rt["oid"]))
		if !ok {
			return bad(http.StatusBadRequest, "invalid_grant", 50034, "The user of this refresh token is not in the directory any more.")
		}
		if len(scope) == 0 {
			scope = strings.Fields(str(rt["scope"]))
		}
		if e := a.userTokens(&si, claims, resp, user, clientID, tid, scope, ""); e != nil {
			return nil, si, e
		}
	}
	tok, err := a.sign(claims)
	if err != nil {
		return bad(http.StatusInternalServerError, "server_error", 50000, err.Error())
	}
	resp["access_token"] = tok
	return resp, si, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// resource finds the service principal that a scope or an audience names: its namespace
// (https://graph.microsoft.com), its application ID, or api://<application ID>.
func (a *API) resource(name string) (repo.ResourceInfo, bool) {
	name = strings.ToLower(strings.TrimSuffix(name, "/"))
	for _, r := range a.world.Resources() {
		if strings.ToLower(r.Namespace) == name || strings.ToLower(r.AppID) == name || "api://"+strings.ToLower(r.AppID) == name {
			return r, true
		}
	}
	return repo.ResourceInfo{}, false
}

// delegated works out the resource and the scopes of a delegated token from the scope parameter. A
// scope with no resource ("User.Read") is one of Microsoft Graph. .default means every permission
// that is consented to this client for this user.
func (a *API) delegated(clientID, userID string, scope []string) (repo.ResourceInfo, []string, *oauthErr) {
	var res repo.ResourceInfo
	var found bool
	var values []string
	for _, s := range scope {
		if slices.Contains(oidcScopes, strings.ToLower(s)) {
			continue
		}
		ns, v := "https://graph.microsoft.com", s
		if i := strings.LastIndex(s, "/"); i >= 0 {
			ns, v = s[:i], s[i+1:]
		}
		r, ok := a.resource(ns)
		if !ok {
			return res, nil, &oauthErr{http.StatusBadRequest, "invalid_resource", 500011, "The resource principal named " + ns + " was not found in the tenant."}
		}
		if !found {
			res, found = r, true
		}
		if r.AppID == res.AppID {
			values = append(values, v)
		}
	}
	if !found {
		r, _ := a.resource("https://graph.microsoft.com")
		res = r
	}

	// the consented values of this client for this user
	var consented []string
	if g, ok := a.world.GrantsOf(clientID); ok {
		for _, d := range g.Delegated {
			if strings.EqualFold(d.ResourceAppID, res.AppID) && (d.ConsentType == "AllPrincipals" || strings.EqualFold(d.PrincipalID, userID)) {
				for _, p := range d.Permissions {
					consented = append(consented, p.Value)
				}
			}
		}
	}
	// the app registration of the API pre-authorizes some clients: they need no consent for those scopes
	for _, v := range a.world.PreAuthorizedScopes(clientID, res.AppID) {
		if !slices.Contains(consented, v) {
			consented = append(consented, v)
		}
	}
	var out []string
	for _, v := range values {
		if v == ".default" {
			out = append(out, consented...)
			continue
		}
		list, _, _ := a.world.ResourcePermissions(res.AppID, permissions.Delegated, v, 0)
		if !slices.ContainsFunc(list, func(p repo.PermissionInfo) bool { return strings.EqualFold(p.Value, v) }) {
			return res, nil, &oauthErr{http.StatusBadRequest, "invalid_scope", 70011, "The provided request must include a 'scope' input parameter. The provided value for the input parameter 'scope' is not valid: '" + v + "' is not a permission of " + res.Name + "."}
		}
		out = append(out, v)
	}
	sort.Strings(out)
	return res, slices.Compact(out), nil
}
