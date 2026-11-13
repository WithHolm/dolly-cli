package login

import (
	"github.com/withholm/dolly-cli/graph/repo"
)

// shapeAccessToken gives an access token the version that the API asks for (requestedAccessTokenVersion of its app registration, null is 1), whatever endpoint the
// client used: the endpoint only decides the version of an ID token (Learn: apiApplication).
//
//	version 1: ver 1.0, issuer https://sts.windows.net/<tenant>/, and aud is the Application ID URI of the API (its client ID when it has none)
//	version 2: ver 2.0, issuer issuerV2, and aud is the client ID of the API
//
// Microsoft Graph keeps the tokens that it had. The audience of a v1 token is from the docs of Entra, as the author remembers them. Learn does not say.
func shapeAccessToken(w *repo.World, claims map[string]any, res repo.ResourceInfo, tid, issuerV2 string) {
	if res.Builtin != "" {
		return
	}
	version, uri := w.AccessTokenVersion(res.AppID)
	if version == 2 {
		claims["iss"], claims["ver"], claims["aud"] = issuerV2, "2.0", res.AppID
		return
	}
	claims["iss"], claims["ver"], claims["aud"] = "https://sts.windows.net/"+tid+"/", "1.0", firstNonEmpty(uri, res.AppID)
}
