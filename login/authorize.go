package login

import (
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/withholm/dolly-cli/graph/repo"
)

// The browser flow (authorization code, with PKCE). A client sends the person to /authorize. Dolly shows a page
// with the users, with no password: the person picks who signs in. Then Dolly sends the browser back
// to the redirect address of the client with a code, and the client trades the code at the token endpoint.

// authParams are the parameters of an authorization request that are kept from the page to the answer.
type authParams struct {
	clientID, redirect, responseType, mode, scope, state, nonce, challenge, method string
}

var keptParams = []string{"client_id", "redirect_uri", "response_type", "response_mode", "scope", "state", "nonce", "code_challenge", "code_challenge_method", "prompt"}

func paramsOf(f url.Values) authParams {
	return authParams{
		clientID: strings.ToLower(f.Get("client_id")), redirect: f.Get("redirect_uri"), responseType: f.Get("response_type"),
		mode: f.Get("response_mode"), scope: f.Get("scope"), state: f.Get("state"), nonce: f.Get("nonce"),
		challenge: f.Get("code_challenge"), method: f.Get("code_challenge_method"),
	}
}

func (a *API) authorize(w http.ResponseWriter, r *http.Request, tid string) {
	if err := r.ParseForm(); err != nil {
		errorPage(w, http.StatusBadRequest, 900144, "The request is not a form.")
		return
	}
	p := paramsOf(r.Form)

	// What is wrong with these cannot be told to the redirect address: it is not safe yet.
	if !repo.IsGUID(p.clientID) {
		errorPage(w, http.StatusBadRequest, 700016, "Application with identifier '"+p.clientID+"' was not found in the directory '"+tid+"'. The client ID must be a GUID.")
		return
	}
	ru, err := url.Parse(p.redirect)
	if p.redirect == "" || err != nil || ru.Scheme == "" || slices.Contains([]string{"javascript", "data", "vbscript", "file"}, strings.ToLower(ru.Scheme)) {
		errorPage(w, http.StatusBadRequest, 50011, "The redirect URI '"+p.redirect+"' is missing or not valid. Send redirect_uri with a scheme, such as http://localhost:3000/callback.")
		return
	}

	// What is wrong with the rest goes back to the client, as an error.
	scope := strings.Fields(p.scope)
	switch {
	case !slices.Contains(strings.Fields(p.responseType), "code") || len(strings.Fields(p.responseType)) != 1:
		a.back(w, r, p, url.Values{"error": {"unsupported_response_type"}, "error_description": {"AADSTS70003: Dolly supports response_type=code only."}})
		return
	case len(scope) == 0:
		a.back(w, r, p, url.Values{"error": {"invalid_request"}, "error_description": {"AADSTS900144: The request must contain the parameter 'scope'."}})
		return
	case p.challenge != "" && p.method != "" && p.method != "S256" && p.method != "plain":
		a.back(w, r, p, url.Values{"error": {"invalid_request"}, "error_description": {"AADSTS501491: The code_challenge_method must be S256 or plain."}})
		return
	}
	if _, _, e := a.delegated(p.clientID, "", scope); e != nil {
		a.back(w, r, p, url.Values{"error": {e.code}, "error_description": {"AADSTS" + itoa(e.aadsts) + ": " + e.desc}})
		return
	}
	_, existed := a.world.FindServicePrincipal(p.clientID)
	sp, spErr := a.world.EnsureServicePrincipal(p.clientID, repo.Actor{Source: "login", Agent: r.UserAgent()})
	if spErr != nil {
		errorPage(w, http.StatusBadRequest, 700016, "The client ID must be a GUID.")
		return
	}

	// A browser with a session signs in at once (single sign-on), unless the client asks for the page with prompt=login,
	// select_account, or consent. With prompt=none and no session, the client gets login_required.
	if prompt := r.Form.Get("prompt"); r.Method != http.MethodPost && (prompt == "" || prompt == "none") {
		if user, ok := a.sessionUser(r); ok {
			a.complete(w, r, p, SignIn{AppID: p.clientID, App: sp.Name, Registered: !existed, Grant: "authorize", Credential: "none"}, user)
			return
		}
		if prompt == "none" {
			a.back(w, r, p, url.Values{"error": {"login_required"}, "error_description": {"AADSTS50058: A silent sign-in request was sent but no user is signed in. The browser has no session. The person must pick a user."}})
			return
		}
	}

	if r.Method == http.MethodPost && r.Form.Get("username") != "" {
		si := SignIn{AppID: p.clientID, App: sp.Name, Registered: !existed, Grant: "authorize", Credential: "none"}
		user, ok := a.world.GetUser(r.Form.Get("username"))
		switch {
		case !ok:
			si.Result, si.Message = "AADSTS50034", "The user is not in the directory."
			a.record(si)
			errorPage(w, http.StatusBadRequest, 50034, "The user account "+r.Form.Get("username")+" does not exist in the "+tid+" directory.")
		case user["accountEnabled"] != true:
			si.User, si.Result, si.Message = str(user["userPrincipalName"]), "AADSTS50057", "The user account is disabled."
			a.record(si)
			errorPage(w, http.StatusBadRequest, 50057, "The user account is disabled.")
		default:
			a.setSession(w, r, str(user["id"]))
			a.complete(w, r, p, si, user)
		}
		return
	}

	q := r.Form.Get("login_q")
	if q == "" {
		q = r.Form.Get("login_hint")
	}
	users, total := a.world.SearchUsers(q, 50)
	hidden := url.Values{}
	for _, k := range keptParams {
		if v := r.Form.Get(k); v != "" {
			hidden.Set(k, v)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	_ = pickPage.Execute(w, map[string]any{
		"Action": r.URL.Path, "Hidden": hidden, "App": sp.Name, "ClientID": p.clientID, "Scopes": scope,
		"Users": users, "Total": total, "Q": q, "Tenant": tid,
	})
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// back sends the browser to the redirect address of the client, with the answer in the query, the
// fragment, or a form that posts, as response_mode says.
// complete signs the user in: it records the sign-in, makes the code, and sends the browser back to the client.
func (a *API) complete(w http.ResponseWriter, r *http.Request, p authParams, si SignIn, user map[string]any) {
	si.User, si.Result = str(user["userPrincipalName"]), "issued"
	a.record(si)
	code := a.newCode(authCode{
		clientID: p.clientID, userID: str(user["id"]), redirect: p.redirect, scope: p.scope, nonce: p.nonce,
		challenge: p.challenge, method: cmpOr(p.method, "plain"),
	})
	a.back(w, r, p, url.Values{"code": {code}, "session_state": {guid()}})
}

func (a *API) back(w http.ResponseWriter, r *http.Request, p authParams, vals url.Values) {
	if p.state != "" {
		vals.Set("state", p.state)
	}
	u, err := url.Parse(p.redirect)
	if err != nil {
		errorPage(w, http.StatusBadRequest, 50011, "The redirect URI is not valid.")
		return
	}
	switch p.mode {
	case "form_post":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = postPage.Execute(w, map[string]any{"Action": p.redirect, "Fields": vals})
		return
	case "fragment":
		// the fragment is written as it is: u.Fragment would escape the % of a value that is encoded already
		u.Fragment, u.RawFragment = "", ""
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, u.String()+"#"+vals.Encode(), http.StatusFound)
		return
	default:
		q := u.Query()
		for k, v := range vals {
			q[k] = v
		}
		u.RawQuery = q.Encode()
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func errorPage(w http.ResponseWriter, status int, aadsts int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = errPage.Execute(w, map[string]any{"Code": "AADSTS" + itoa(aadsts), "Message": msg})
}

const pageStyle = `<style>
:root{color-scheme:light dark;--bg:#fff;--fg:#1c1d21;--muted:#666a73;--line:#e3e4e8;--hover:#f3f4f7;--primary:#4f46e5}
@media(prefers-color-scheme:dark){:root{--bg:#0f0f12;--fg:#ececf0;--muted:#9a9ca5;--line:#2a2b31;--hover:#1a1a1f;--primary:#8b85ff}}
body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.45 system-ui,sans-serif}
main{max-width:520px;margin:48px auto;padding:0 20px}
h1{font-size:22px;letter-spacing:-.02em;margin:0 0 4px}.muted{color:var(--muted)}code{font-size:12.5px}
.scopes{display:flex;flex-wrap:wrap;gap:4px;margin:8px 0 16px}.scopes span{font-size:12px;padding:1px 8px;border:1px solid var(--line);border-radius:99px}
ul{list-style:none;margin:12px 0;padding:0;border:1px solid var(--line);border-radius:8px}
li+li{border-top:1px solid var(--line)}
li button{all:unset;box-sizing:border-box;display:block;width:100%;padding:8px 12px;cursor:pointer}
li button:hover:not(:disabled),li button:focus-visible{background:var(--hover)}li button:disabled{opacity:.5;cursor:not-allowed}
li small{display:block;color:var(--muted)}
input[type=search]{width:calc(100% - 78px);padding:6px 10px;font:inherit;color:inherit;background:var(--bg);border:1px solid var(--line);border-radius:6px}
form.find button{padding:6px 12px;font:inherit;border:1px solid var(--line);border-radius:6px;background:var(--bg);color:var(--fg);cursor:pointer}
.note{font-size:13px}
</style>`

var pickPage = template.Must(template.New("pick").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Sign in · dollycli</title>` + pageStyle + `</head><body><main>
<h1>Pick a user to sign in</h1>
<p class="muted"><strong>{{.App}}</strong> <code>{{.ClientID}}</code> wants to sign in to the tenant <code>{{.Tenant}}</code>.</p>
<div class="scopes" aria-label="Scopes that the client asks for">{{range .Scopes}}<span>{{.}}</span>{{end}}</div>
<form class="find" method="get" action="{{.Action}}">
{{range $k, $vs := .Hidden}}{{range $vs}}<input type="hidden" name="{{$k}}" value="{{.}}">{{end}}{{end}}
<input type="search" name="login_q" value="{{.Q}}" placeholder="Find a user" aria-label="Find a user" autofocus> <button type="submit">Find</button>
</form>
<form method="post" action="{{.Action}}">
{{range $k, $vs := .Hidden}}{{range $vs}}<input type="hidden" name="{{$k}}" value="{{.}}">{{end}}{{end}}
<ul>{{range .Users}}<li><button type="submit" name="username" value="{{.UPN}}"{{if not .Enabled}} disabled{{end}}>{{.Name}}{{if not .Enabled}} (disabled){{end}}
<small>{{.UPN}}{{if .JobTitle}} · {{.JobTitle}}{{end}}{{if .Department}} · {{.Department}}{{end}}</small></button></li>
{{else}}<li><button type="button" disabled>No user matches</button></li>{{end}}</ul>
</form>
{{if gt .Total 50}}<p class="muted note">Showing 50 of {{.Total}}. Type in the search to find the others.</p>{{end}}
<p class="muted note">dollycli asks for no password. The user that you pick signs in, and the client gets what it asked for.</p>
</main></body></html>`))

var postPage = template.Must(template.New("post").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Signing in…</title></head>
<body onload="document.forms[0].submit()"><form method="post" action="{{.Action}}">
{{range $k, $vs := .Fields}}{{range $vs}}<input type="hidden" name="{{$k}}" value="{{.}}">{{end}}{{end}}
<noscript><button type="submit">Continue</button></noscript></form></body></html>`))

var errPage = template.Must(template.New("err").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Sign-in error · dollycli</title>` + pageStyle + `</head><body><main>
<h1>dollycli cannot sign you in</h1><p><code>{{.Code}}</code></p><p>{{.Message}}</p></main></body></html>`))
