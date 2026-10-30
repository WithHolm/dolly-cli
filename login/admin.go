package login

import (
	"net/http"
	"time"
)

// SignIn is one try to get a token, for the page.
type SignIn struct {
	ID         int       `json:"id"`
	Time       time.Time `json:"time"`
	AppID      string    `json:"appId"`
	App        string    `json:"app,omitempty"` // the name of the service principal
	Registered bool      `json:"registered"`    // the try made the service principal
	Grant      string    `json:"grant"`
	Credential string    `json:"credential"` // secret, certificate, or none
	Resource   string    `json:"resource,omitempty"`
	User       string    `json:"user,omitempty"`
	Result     string    `json:"result"` // issued, or AADSTS and a number
	Message    string    `json:"message,omitempty"`
}

const keep = 200 // the sign-ins that the page can show

// record saves a sign-in for the page, unless its client has the flag hideCalls (the portal signs in as one).
func (a *API) record(s SignIn) {
	if _, hide := a.world.ClientFlags(s.AppID); hide {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.count++
	s.ID, s.Time = a.count, a.now()
	a.log = append(a.log, s)
	if len(a.log) > keep {
		a.log = a.log[len(a.log)-keep:]
	}
}

// AdminHandler is the admin API, at /_dolly/login/.
func (a *API) AdminHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		tid := a.world.TenantID()
		root := base(r) + "/" + tid
		writeJSON(w, http.StatusOK, map[string]any{
			"tenantId":  tid,
			"authority": root,
			"issuer":    issuerOf(r, tid),
			"discovery": root + "/v2.0/.well-known/openid-configuration",
			"jwks":      root + "/discovery/v2.0/keys",
			"token":     root + "/oauth2/v2.0/token",
			"keyId":     a.kid,
		})
	})
	m.HandleFunc("GET /signins", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		out := make([]SignIn, len(a.log))
		for i, s := range a.log { // the newest first
			out[len(a.log)-1-i] = s
		}
		a.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"signins": out})
	})
	m.HandleFunc("DELETE /signins", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		a.log = nil
		a.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	return m
}
