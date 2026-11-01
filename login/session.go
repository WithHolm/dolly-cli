package login

import (
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// The browser session. When a person picks a user on the sign-in page, dollycli sets a cookie, as Entra does. With the cookie,
// a request with prompt=none gets a code at once, and so does a request with no prompt (single sign-on). prompt=login and
// prompt=select_account show the page again. The logout path ends the session.
//
// The cookie is a JWT that dollycli signs, so it needs no storage and it is good after a restart.

const (
	sessionCookie = "dolly_session"
	sessionTTL    = 24 * time.Hour
)

func (a *API) setSession(w http.ResponseWriter, r *http.Request, userID string) {
	now := a.now()
	tok, err := a.sign(map[string]any{"ses": true, "oid": userID, "iat": now.Unix(), "exp": now.Add(sessionTTL).Unix()})
	if err != nil {
		return // no session: the person picks again at the next request
	}
	c := sessionCookieFor(r)
	c.Value, c.Expires = tok, now.Add(sessionTTL)
	http.SetCookie(w, c)
}

func clearSession(w http.ResponseWriter, r *http.Request) {
	c := sessionCookieFor(r)
	c.MaxAge = -1
	http.SetCookie(w, c)
}

// sessionCookieFor has the attributes of the cookie. Over https it is SameSite=None, so that a hidden frame of a web app (the silent
// sign-in of MSAL.js, prompt=none) sends it, as the cookie of Entra does. Over http a browser refuses None, so it is Lax.
func sessionCookieFor(r *http.Request) *http.Cookie {
	c := &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
	if r.TLS != nil {
		c.Secure, c.SameSite = true, http.SameSiteNoneMode
	}
	return c
}

// sessionUser gives the user of the session of this browser, if there is one, and the user is still there and enabled.
func (a *API) sessionUser(r *http.Request) (map[string]any, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, false
	}
	header, payload, ok := jwtParts(c.Value)
	if !ok || header["kid"] != a.kid || payload["ses"] != true || !a.checkSignature(c.Value) {
		return nil, false
	}
	if exp, _ := payload["exp"].(float64); float64(a.now().Unix()) >= exp {
		return nil, false
	}
	user, ok := a.world.GetUser(str(payload["oid"]))
	if !ok || user["accountEnabled"] != true {
		return nil, false
	}
	return user, true
}

// logout ends the session. With post_logout_redirect_uri the browser goes there (with state, if it was sent). Entra wants that address
// to be one of the app registration, and dollycli has none: it takes an address with the scheme http or https.
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	clearSession(w, r)
	w.Header().Set("Cache-Control", "no-store")
	if back := r.Form.Get("post_logout_redirect_uri"); back != "" {
		u, err := url.Parse(back)
		if err == nil && slices.Contains([]string{"http", "https"}, strings.ToLower(u.Scheme)) && u.Host != "" {
			if state := r.Form.Get("state"); state != "" {
				q := u.Query()
				q.Set("state", state)
				u.RawQuery = q.Encode()
			}
			http.Redirect(w, r, u.String(), http.StatusFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = signedOutPage.Execute(w, nil)
}

var signedOutPage = template.Must(template.New("out").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Signed out · dollycli</title>` + pageStyle + `</head><body><main>
<h1>You signed out</h1><p class="muted">dollycli ended the session of this browser. The next sign-in shows the page with the users.</p>
</main></body></html>`))
