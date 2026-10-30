package login

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	codeTTL    = 10 * time.Minute
	refreshTTL = 90 * 24 * time.Hour
)

// userTokens fills the claims and the answer of a delegated token for a user. The password grant, a
// code, and a refresh token all end here. The scope is the one that the client asked for, as it wrote it.
func (a *API) userTokens(si *SignIn, claims, resp, user map[string]any, clientID, tid string, scope []string, nonce string) *oauthErr {
	si.User = str(user["userPrincipalName"])
	if enabled, _ := user["accountEnabled"].(bool); !enabled {
		return &oauthErr{http.StatusBadRequest, "invalid_grant", 50057, "The user account is disabled."}
	}
	res, scp, e := a.delegated(clientID, str(user["id"]), scope)
	if e != nil {
		return e
	}
	si.Resource = res.Name
	claims["aud"], claims["oid"], claims["sub"] = res.AppID, user["id"], user["id"]
	claims["name"], claims["preferred_username"], claims["upn"] = user["displayName"], user["userPrincipalName"], user["userPrincipalName"]
	claims["scp"], claims["appidacr"] = strings.Join(scp, " "), "1"
	if si.Credential == "none" {
		claims["appidacr"] = "0" // a public client: it showed no secret
	}
	resp["scope"] = strings.Join(scp, " ")
	if slices.Contains(scope, "openid") {
		id := map[string]any{
			"iss": claims["iss"], "aud": clientID, "iat": claims["iat"], "nbf": claims["nbf"], "exp": claims["exp"],
			"sub": user["id"], "oid": user["id"], "tid": tid, "ver": claims["ver"],
			"name": user["displayName"], "preferred_username": user["userPrincipalName"],
		}
		if nonce != "" {
			id["nonce"] = nonce
		}
		tok, err := a.sign(id)
		if err != nil {
			return &oauthErr{http.StatusInternalServerError, "server_error", 50000, err.Error()}
		}
		resp["id_token"] = tok
	}
	if slices.Contains(scope, "offline_access") {
		rt, err := a.refreshToken(clientID, str(user["id"]), scope)
		if err != nil {
			return &oauthErr{http.StatusInternalServerError, "server_error", 50000, err.Error()}
		}
		resp["refresh_token"] = rt
		resp["refresh_token_expires_in"] = int(refreshTTL.Seconds())
	}
	return nil
}

// ---- refresh tokens ----

// A refresh token is a JWT that Dolly signs (rt: true). It needs no storage, so it is good after a restart.
func (a *API) refreshToken(clientID, userID string, scope []string) (string, error) {
	now := a.now()
	return a.sign(map[string]any{
		"rt": true, "appid": clientID, "oid": userID, "scope": strings.Join(scope, " "),
		"iat": now.Unix(), "exp": now.Add(refreshTTL).Unix(), "uti": guid(),
	})
}

func (a *API) parseRefresh(tok string) (map[string]any, bool) {
	header, payload, ok := jwtParts(tok)
	if !ok || header["kid"] != a.kid || payload["rt"] != true || !a.checkSignature(tok) {
		return nil, false
	}
	if exp, _ := payload["exp"].(float64); float64(a.now().Unix()) >= exp {
		return nil, false
	}
	return payload, true
}

// ---- authorization codes ----

// authCode is what Dolly keeps about a code until the client trades it for tokens.
type authCode struct {
	clientID, userID, redirect, scope, nonce string
	challenge, method                        string // PKCE
	exp                                      time.Time
}

func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b64.EncodeToString(b)
}

// newCode keeps a code for a user who signed in, and gives the text of the code.
func (a *API) newCode(c authCode) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for k, v := range a.codes { // the codes that nobody traded
		if now.After(v.exp) {
			delete(a.codes, k)
		}
	}
	if a.codes == nil {
		a.codes = map[string]*authCode{}
	}
	c.exp = now.Add(codeTTL)
	code := "0." + random()
	a.codes[code] = &c
	return code
}

// redeem trades a code. A code works once, for the client and the redirect address that asked for it,
// and (with PKCE) for the client that has the verifier.
func (a *API) redeem(code, clientID, redirect, verifier string) (*authCode, *oauthErr) {
	a.mu.Lock()
	c := a.codes[code]
	delete(a.codes, code)
	a.mu.Unlock()
	switch {
	case c == nil || a.now().After(c.exp):
		return nil, &oauthErr{http.StatusBadRequest, "invalid_grant", 70000, "The authorization code is not valid: it is wrong, it has expired, or it was used before."}
	case !strings.EqualFold(c.clientID, clientID):
		return nil, &oauthErr{http.StatusBadRequest, "invalid_grant", 70000, "The authorization code was issued to another client."}
	case c.redirect != redirect:
		return nil, &oauthErr{http.StatusBadRequest, "invalid_grant", 50011, "The redirect URI specified in the request does not match the one of the authorization request."}
	}
	if c.challenge != "" {
		want := verifier
		if c.method == "S256" {
			sum := sha256.Sum256([]byte(verifier))
			want = b64.EncodeToString(sum[:])
		}
		if verifier == "" || subtle.ConstantTimeCompare([]byte(want), []byte(c.challenge)) != 1 {
			return nil, &oauthErr{http.StatusBadRequest, "invalid_grant", 501481, "The Code_Verifier does not match the code_challenge supplied in the authorization request."}
		}
	}
	return c, nil
}
