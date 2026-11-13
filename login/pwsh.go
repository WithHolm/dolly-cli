package login

import (
	"net/http"
	"regexp"
	"time"

	"github.com/withholm/dolly-cli/graph/repo"
)

// pwshPath is the quick connect for Microsoft Graph PowerShell: iwr 'http://<server>/pwsh' | iex
// The connection is the admin account of the owner. With ?user=<user principal name> it is that user.
const pwshPath = "/pwsh"

// hostRe is what a host (and port) may have in it. The host goes into a line of PowerShell between single quotes, so nothing else is
// let through: a quote or a line break in a Host header must not become code that someone runs.
var hostRe = regexp.MustCompile(`^[A-Za-z0-9.\-]+(:[0-9]{1,5})?$|^\[[0-9A-Fa-f:.]+\](:[0-9]{1,5})?$`)

// upnRe is the same for a user principal name. The name that goes into the script is the one of the user in the data, and it is checked
// again against this.
var upnRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+$`)

// jwtTokenPath gives a ready-made access token as plain text: GET /jwtToken?hint=<user principal name or object ID>.
// With no hint the user is the admin account of the owner (repo.AdminUPN).
const jwtTokenPath = "/jwtToken"

// tokenLifetime is how long a ready-made token is good. Dolly checks the lifetime of its tokens, so it is long: a person who
// pastes the token in a script should not have to get a new one.
const tokenLifetime = 10 * 365 * 24 * time.Hour

// userFor finds the user of a hint (or the admin account with no hint). It writes the 404 itself.
func (a *API) userFor(w http.ResponseWriter, hint string) (map[string]any, string, bool) {
	if hint == "" {
		hint = a.world.AdminAccount()
	}
	u, ok := a.world.GetUser(hint)
	upn, _ := u["userPrincipalName"].(string)
	if !ok || !upnRe.MatchString(upn) {
		http.Error(w, "No user has the user principal name or object ID '"+safeText(hint)+"'.", http.StatusNotFound)
		return nil, "", false
	}
	return u, upn, true
}

// pwsh answers the quick connect: the lines that point the module at this server and connect it with a token from /jwtToken. It is
// plain text for Invoke-Expression, and it changes nothing on the server. ?user= takes the same values as hint of /jwtToken.
func (a *API) pwsh(w http.ResponseWriter, r *http.Request) {
	if !hostRe.MatchString(r.Host) {
		http.Error(w, "The host of this request cannot be used in a script.", http.StatusBadRequest)
		return
	}
	_, upn, ok := a.userFor(w, r.URL.Query().Get("user"))
	if !ok {
		return
	}
	origin := base(r)
	script := "# Dolly: connect Microsoft Graph PowerShell to " + origin + " as " + upn + "\n" +
		"Add-MgEnvironment -Name Dolly -AzureADEndpoint '" + origin + "' -GraphEndpoint '" + origin + "' | Out-Null\n" +
		"$token = (iwr '" + origin + jwtTokenPath + "?hint=" + upn + "').Content\n" +
		"Connect-MgGraph -Environment Dolly -AccessToken (ConvertTo-SecureString $token -AsPlainText -Force)\n"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(script))
}

// jwtToken gives a delegated access token for a user, with a long life. It is what the password grant gives, for the client of
// Microsoft Graph PowerShell, and it is a sign-in of the user in the list (grant jwtToken). The text has no line break at the end:
// a script puts it in a SecureString as it is.
func (a *API) jwtToken(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.userFor(w, r.URL.Query().Get("hint"))
	if !ok {
		return
	}
	tid, now := a.world.TenantID(), a.now()
	client := repo.GraphPowerShellAppID
	claims := map[string]any{
		"iss": issuerOf(r, tid), "tid": tid, "ver": "2.0",
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(tokenLifetime).Unix(),
		"appid": client, "azp": client, "azpv": "2", "uti": guid(),
		// for the person who reads the token
		"message": "Thank you for using this application!",
		"hello":   "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
	}
	// the user signs in when the token is asked for, so the page of sign-ins shows it
	_, existed := a.world.FindServicePrincipal(client)
	sp, spErr := a.world.EnsureServicePrincipal(client, repo.Actor{Source: "login", Agent: r.UserAgent()})
	if spErr != nil {
		http.Error(w, "The service principal of the client could not be made.", http.StatusInternalServerError)
		return
	}
	si := SignIn{AppID: client, App: sp.Name, Registered: !existed, Grant: "jwtToken", Credential: "none"}
	scope := []string{"User.Read", "User.Read.All", "Group.Read.All", "Directory.Read.All"}
	if e := a.userTokens(&si, claims, map[string]any{}, user, client, tid, issuerOf(r, tid), scope, ""); e != nil {
		si.Result, si.Message = "AADSTS"+itoa(e.aadsts), e.desc
		a.record(si)
		a.fail(w, e)
		return
	}
	tok, err := a.sign(claims)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	si.Result = "issued"
	a.record(si)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(tok))
}

// safeText keeps the letters, digits, and a few signs of a text that comes from the request, for an error message.
func safeText(s string) string {
	out := []rune{}
	for _, r := range s {
		if len(out) >= 80 {
			break
		}
		if r == '@' || r == '.' || r == '-' || r == '_' || r == '+' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			out = append(out, r)
		}
	}
	return string(out)
}
