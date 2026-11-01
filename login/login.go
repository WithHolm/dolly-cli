// Package login is the fake sign-in API of Microsoft Entra: the OpenID Connect discovery document,
// the signing keys, and the token endpoint. A client gets a real, signed JWT. The Graph API checks
// that token, and reads the client ID in it.
//
// The paths start with the tenant, as in Entra: /{tenant}/oauth2/v2.0/token. The tenant is the
// tenant ID, one of its domains, or common. A client that signs in with a client ID that Dolly has not
// seen gets a service principal (auto-created, open mode). What the service principal accepts as a
// credential is the state that a person sets on the page (any, armed, captured). See repo.CheckCredential.
//
// The grants are client_credentials (an application token with roles), password (a delegated
// token for a user, with scp), authorization_code (the browser flow, see authorize.go), and
// refresh_token. Dolly accepts any password.
package login

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/repo"
	"github.com/withholm/dolly-cli/shared"
)

// Options changes how the API works.
type Options struct {
	// Dir keeps the signing key. Empty keeps it in memory.
	Dir string
	// Now is the clock. The default is time.Now.
	Now func() time.Time
	// CORS wraps the paths that a web app in a browser calls from its own origin: discovery, keys, and the token endpoint. The
	// authorize page, /pwsh, and /jwtToken are not wrapped. Nil adds no CORS headers.
	CORS func(http.Handler) http.Handler
}

// API is the sign-in API. It is a shared.API and a shared.Matcher.
type API struct {
	world *repo.World
	key   *rsa.PrivateKey
	kid   string
	now   func() time.Time
	cors  func(http.Handler) http.Handler

	mu    sync.Mutex
	log   []SignIn
	count int
	codes map[string]*authCode // the codes that were given and not traded yet
}

var (
	_ shared.API     = (*API)(nil)
	_ shared.Matcher = (*API)(nil)
)

// New makes the sign-in API for a world.
func New(w *repo.World, opt Options) (*API, error) {
	key, err := loadKey(opt.Dir)
	if err != nil {
		return nil, err
	}
	a := &API{world: w, key: key, kid: keyID(key), now: opt.Now, cors: opt.CORS}
	if a.now == nil {
		a.now = time.Now
	}
	return a, nil
}

// Name is login.
func (a *API) Name() string { return "login" }

// Prefixes is empty: the paths start with the tenant, so the API is a Matcher.
func (a *API) Prefixes() []string { return nil }

var pathRe = regexp.MustCompile(`^/([^/]+)/(v2\.0/\.well-known/openid-configuration|\.well-known/openid-configuration|discovery/(?:v2\.0/)?keys|oauth2/(?:v2\.0/)?(?:token|authorize|logout))$`)

// Owns says if the request is for the sign-in API.
func (a *API) Owns(r *http.Request) bool {
	return pathRe.MatchString(r.URL.Path) || ((r.URL.Path == pwshPath || r.URL.Path == jwtTokenPath) && r.Method == http.MethodGet)
}

// Handler answers the sign-in paths.
func (a *API) Handler() http.Handler {
	plain := http.HandlerFunc(a.serve)
	if a.cors == nil {
		return plain
	}
	withCORS := a.cors(plain)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m := pathRe.FindStringSubmatch(r.URL.Path); m != nil && !strings.HasSuffix(m[2], "authorize") {
			withCORS.ServeHTTP(w, r)
			return
		}
		plain.ServeHTTP(w, r)
	})
}

func (a *API) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == pwshPath && r.Method == http.MethodGet {
		a.pwsh(w, r)
		return
	}
	if r.URL.Path == jwtTokenPath && r.Method == http.MethodGet {
		a.jwtToken(w, r)
		return
	}
	m := pathRe.FindStringSubmatch(r.URL.Path)
	if m == nil {
		http.NotFound(w, r)
		return
	}
	tid, e := a.tenant(m[1])
	if e != nil {
		a.fail(w, e)
		return
	}
	switch {
	case strings.HasSuffix(m[2], "openid-configuration"):
		a.discovery(w, r, tid, strings.HasPrefix(m[2], "v2.0/"))
	case strings.Contains(m[2], "keys"):
		writeJSON(w, http.StatusOK, a.jwks())
	case strings.HasSuffix(m[2], "authorize"):
		a.authorize(w, r, tid)
	case strings.HasSuffix(m[2], "logout"):
		a.logout(w, r)
	default:
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			a.fail(w, &oauthErr{http.StatusMethodNotAllowed, "invalid_request", 900561, "The endpoint only accepts POST requests. Received a " + r.Method + " request."})
			return
		}
		a.token(w, r, tid, strings.Contains(m[2], "v2.0"))
	}
}

// base is the address that the client used to reach Dolly.
func base(r *http.Request) string { return shared.Scheme(r) + "://" + r.Host }

func (a *API) discovery(w http.ResponseWriter, r *http.Request, tid string, v2 bool) {
	root := base(r) + "/" + tid
	ver, issuer := "", "https://sts.windows.net/"+tid+"/"
	if v2 {
		ver, issuer = "/v2.0", issuerOf(r, tid)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                root + "/oauth2" + ver + "/authorize",
		"token_endpoint":                        root + "/oauth2" + ver + "/token",
		"jwks_uri":                              root + "/discovery" + ver + "/keys",
		"end_session_endpoint":                  root + "/oauth2" + ver + "/logout",
		"frontchannel_logout_supported":         true,
		"http_logout_supported":                 true,
		"response_types_supported":              []string{"code", "id_token", "code id_token", "id_token token"},
		"response_modes_supported":              []string{"query", "fragment", "form_post"},
		"subject_types_supported":               []string{"pairwise"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "private_key_jwt", "client_secret_basic"},
		"scopes_supported":                      []string{"openid", "profile", "email", "offline_access"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "client_credentials", "password"},
		"code_challenge_methods_supported":      []string{"S256", "plain"},
		"claims_supported":                      []string{"sub", "iss", "aud", "exp", "iat", "nbf", "tid", "oid", "name", "preferred_username", "ver"},
		"tenant_region_scope":                   "EU",
		"cloud_instance_name":                   "dolly",
	})
}

// issuerOf is the issuer of the tokens that Dolly makes. It is also in the discovery document.
func issuerOf(r *http.Request, tid string) string { return base(r) + "/" + tid + "/v2.0" }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ---- the tenant ----

// tenant gives the tenant ID for the first part of a path: the tenant ID, a domain of the
// tenant, or common. This is also how a client finds the tenant ID of a domain.
func (a *API) tenant(seg string) (string, *oauthErr) {
	tid := a.world.TenantID()
	s := strings.ToLower(seg)
	if s == tid || s == "common" || s == "organizations" {
		return tid, nil
	}
	if orgs := a.world.ListOrganizations(); len(orgs) > 0 {
		raw, _ := json.Marshal(orgs[0])
		var org struct {
			VerifiedDomains []struct{ Name string }
		}
		_ = json.Unmarshal(raw, &org)
		for _, d := range org.VerifiedDomains {
			if strings.ToLower(d.Name) == s {
				return tid, nil
			}
		}
	}
	return "", &oauthErr{http.StatusBadRequest, "invalid_request", 90002, "Tenant '" + seg + "' not found. Check to make sure you have the correct tenant ID and are signing into the correct cloud."}
}

// ---- errors ----

// oauthErr is an error of the token endpoint, in the form of Entra: an OAuth error code, and
// an AADSTS number in the text.
type oauthErr struct {
	status int
	code   string
	aadsts int
	desc   string
}

func guid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func (a *API) fail(w http.ResponseWriter, e *oauthErr) {
	trace, corr, ts := guid(), guid(), a.now().UTC().Format("2006-01-02 15:04:05Z")
	writeJSON(w, e.status, map[string]any{
		"error":             e.code,
		"error_description": "AADSTS" + itoa(e.aadsts) + ": " + e.desc + "\r\nTrace ID: " + trace + "\r\nCorrelation ID: " + corr + "\r\nTimestamp: " + ts,
		"error_codes":       []int{e.aadsts},
		"timestamp":         ts,
		"trace_id":          trace,
		"correlation_id":    corr,
	})
}

// ---- checking a token, for the Graph API ----

// Verify tells the Graph API about a bearer token. It is a graph.TokenVerifier. A token that this
// Dolly did not sign is not its own, and Graph lets it through.
func (a *API) Verify(tok string) (graph.TokenInfo, bool) {
	header, payload, ok := jwtParts(tok)
	if !ok || header["kid"] != a.kid {
		return graph.TokenInfo{}, false
	}
	info := graph.TokenInfo{AppID: str(payload["appid"])}
	if payload["idtyp"] != "app" {
		info.UserID = str(payload["oid"])
	}
	switch exp, _ := payload["exp"].(float64); {
	case !a.checkSignature(tok):
		info.Problem = "Access token validation failure. Invalid signature."
	case float64(a.now().Unix()) >= exp:
		info.Problem = "Lifetime validation failed, the token is expired."
	case !isGraphAudience(str(payload["aud"])):
		info.Problem = "Access token validation failure. Invalid audience."
	}
	return info, true
}

func str(v any) string { s, _ := v.(string); return s }
