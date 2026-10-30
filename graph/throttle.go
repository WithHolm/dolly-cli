package graph

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/withholm/dolly-cli/graph/cost"
)

// anonymousApp is the client ID of a token that is not a JWT with an application ID in it. All
// such calls are counted as one client.
const anonymousApp = "anonymous"

// bearer gives the token of a request.
func bearer(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		h = h[7:]
	}
	return strings.TrimSpace(h)
}

// appID gives the application (client) ID behind a request. For a token that the sign-in API made,
// it is the one in the checked token. For any other token, it reads appid (or azp) from the payload,
// and does not check it.
func (s *Server) appID(r *http.Request) string {
	if info, ours := s.token(r); ours && info.AppID != "" {
		return info.AppID
	}
	parts := strings.Split(bearer(r), ".")
	if len(parts) != 3 {
		return anonymousApp
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return anonymousApp
	}
	var c struct{ AppID, Azp string }
	if json.Unmarshal(payload, &c) != nil {
		return anonymousApp
	}
	return cmpOr(c.AppID, c.Azp, anonymousApp)
}

func cmpOr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// charge counts what a request costs, for its client. It sets the cost headers of Graph on h
// (x-ms-resource-unit, and x-ms-throttle-limit-percentage when the client has used more than
// 0.8 of its limit). With Behavior.Throttle on, a request over the limits is throttled: charge
// sets the throttle headers and Retry-After, and the caller writes the 429.
func (s *Server) charge(h http.Header, r *http.Request) (ru int, throttled bool) {
	if s.costs == nil {
		return 0, false
	}
	price := s.costs.Price(r.Method, strings.TrimPrefix(r.URL.Path, BasePath), r.URL.Query())
	app := s.appID(r)
	// a client that is exempt is counted, and never throttled
	exempt, _ := s.world.ClientFlags(app)
	dec := s.limiter.Charge(cost.Request{
		App: app, Tenant: s.world.TenantID(), TenantUsers: s.world.UserCount(), Price: price,
	}, s.behavior().Throttle && !exempt)

	h.Set("x-ms-resource-unit", strconv.Itoa(price.RU))
	if dec.Percentage >= 0.8 {
		h.Set("x-ms-throttle-limit-percentage", strconv.FormatFloat(dec.Percentage, 'f', 2, 64))
	}
	if dec.Throttled {
		h.Set("x-ms-throttle-scope", dec.Scope)
		h.Set("x-ms-throttle-information", dec.Info)
		h.Set("Retry-After", strconv.Itoa(int(dec.RetryAfter.Seconds()+0.999)))
	}
	return price.RU, dec.Throttled
}
