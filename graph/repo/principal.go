package repo

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// What Dolly knows about a service principal apart from Graph: how it signs in, and what it may
// do. The sign-in API reads it. The service principal itself is in apps.go.
//
// Dolly has no app registrations yet (the /applications objects). When the sign-in API sees a
// client ID that it does not know, the service principal is all that it makes. Where Entra keeps
// the secrets of a client on its app registration, Dolly keeps the credential state here.

// Events that these make.
const (
	evTenantSet    = "tenant.set"                  // the ID of the tenant changed (From and To)
	evSPCreate     = "serviceprincipal.create"     // made by the sign-in API when it saw the client ID first (SP has it)
	evSPMode       = "serviceprincipal.mode"       // the mode changed (AppID and To)
	evSPCredential = "serviceprincipal.credential" // the credential state changed (AppID and Credential)
	evSPFlag       = "serviceprincipal.flag"       // a flag changed (AppID, Field and To)
)

// The flags of a service principal: what the Graph API does with the calls of this client.
const (
	FlagExemptFromThrottle = "exemptFromThrottle" // the limits of Graph do not throttle this client (the cost is counted)
	FlagHideCalls          = "hideCalls"          // the calls of this client are not in the list of calls
)

// The modes of a service principal.
const (
	ModeOpen    = "open"    // everything is allowed, and the calls are recorded. The default.
	ModeEnforce = "enforce" // only what is granted to the service principal is allowed
)

// The states of the credential of a service principal.
const (
	CredAny      = "any"      // any credential is accepted. The default.
	CredArmed    = "armed"    // the next credential that the client shows is kept
	CredCaptured = "captured" // only the kept credential is accepted
)

// The kinds of credential.
const (
	KindSecret      = "secret"
	KindCertificate = "certificate" // Hash is the hash of its thumbprint
)

// CredState says what Dolly asks of a client when it signs in. The secret is never kept, only a
// hash of it, and the first characters of it for the page, so the files of dolly do not have
// a secret that may be real.
type CredState struct {
	State string    `json:"state"`
	Kind  string    `json:"kind,omitempty"`
	Hash  string    `json:"hash,omitempty"`
	Hint  string    `json:"hint,omitempty"`
	At    time.Time `json:"at,omitzero"`
}

// ServicePrincipalInfo is a service principal as the sign-in API and the admin API see it.
type ServicePrincipalInfo struct {
	ID         string    `json:"id"`    // the object ID
	AppID      string    `json:"appId"` // the application (client) ID
	Name       string    `json:"displayName"`
	Auto       bool      `json:"auto"`              // made by the sign-in API, not seeded or made by a person
	Builtin    string    `json:"builtin,omitempty"` // graph: Microsoft Graph, whose permissions are the catalog
	Mode       string    `json:"mode"`
	Credential CredState `json:"credential"`

	System             bool `json:"system"`
	ExemptFromThrottle bool `json:"exemptFromThrottle"`
	HideCalls          bool `json:"hideCalls"`
}

func (sp *ServicePrincipal) info() ServicePrincipalInfo {
	i := ServicePrincipalInfo{
		ID: sp.ID, AppID: sp.AppID, Name: sp.Name, Auto: sp.Auto, Builtin: sp.Builtin, Mode: sp.Mode, Credential: CredState{State: CredAny},
		System: sp.System, ExemptFromThrottle: sp.ExemptFromThrottle, HideCalls: sp.HideCalls,
	}
	if i.Mode == "" {
		i.Mode = ModeOpen
	}
	if sp.Cred != nil {
		i.Credential = *sp.Cred
	}
	return i
}

var guidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsGUID reports whether s is a GUID in the usual form.
func IsGUID(s string) bool { return guidRe.MatchString(s) }

// HashCredential gives the hash and the hint of a credential (a secret, or the thumbprint of a certificate).
func HashCredential(value string) (hash, hint string) {
	sum := sha256.Sum256([]byte(value))
	hint = value
	if len(hint) > 4 {
		hint = hint[:4]
	}
	return hex.EncodeToString(sum[:]), hint
}

// ---- the tenant ----

// TenantID is the ID of the tenant: the ID of its organization. It is the one that the tokens and
// /organization have. It is all zeros until a person changes it.
func (w *World) TenantID() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return str(w.Organization["id"])
}

// SetTenantID changes the ID of the tenant. It must be a GUID.
func (w *World) SetTenantID(id string, a Actor) *apiError {
	id = strings.ToLower(strings.TrimSpace(id))
	if !IsGUID(id) {
		return badRequest("The tenant ID must be a GUID, for example %s.", FakeTenantID)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	old := str(w.Organization["id"])
	if old == id {
		return nil
	}
	w.Organization["id"] = id
	w.record(Event{Type: evTenantSet, Actor: a, From: old, To: id})
	return nil
}

// ---- the service principals ----

func (w *World) spByAppID(appID string) *ServicePrincipal {
	return w.byAppID[strings.ToLower(appID)]
}

// FindServicePrincipal gives the service principal with this application (client) ID.
func (w *World) FindServicePrincipal(appID string) (ServicePrincipalInfo, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	sp := w.spByAppID(appID)
	if sp == nil {
		return ServicePrincipalInfo{}, false
	}
	return sp.info(), true
}

// ListServicePrincipalInfo gives every service principal, in the order of the data.
func (w *World) ListServicePrincipalInfo() []ServicePrincipalInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]ServicePrincipalInfo, len(w.SPs))
	for i, sp := range w.SPs {
		out[i] = sp.info()
	}
	return out
}

// EnsureServicePrincipal makes the service principal for this application (client) ID, in the
// open mode, when there is none. It gives the one that is there in any case. The sign-in API
// calls it for a client ID that it sees first.
func (w *World) EnsureServicePrincipal(appID string, a Actor) (ServicePrincipalInfo, *apiError) {
	appID = strings.ToLower(strings.TrimSpace(appID))
	if !IsGUID(appID) {
		return ServicePrincipalInfo{}, badRequest("The client ID must be a GUID.")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if sp := w.spByAppID(appID); sp != nil {
		return sp.info(), nil
	}
	sp := &ServicePrincipal{
		ID: newGUID(), AppID: appID, Auto: true, Mode: ModeOpen,
		Name: "Auto-created service principal (" + appID[:8] + ")",
	}
	w.addServicePrincipal(sp)
	cp := *sp
	w.record(Event{Type: evSPCreate, Actor: a, AppID: appID, SP: &cp})
	return sp.info(), nil
}

func (w *World) addServicePrincipal(sp *ServicePrincipal) {
	w.SPs = append(w.SPs, sp)
	w.sps[strings.ToLower(sp.ID)] = sp
	w.byAppID[strings.ToLower(sp.AppID)] = sp
}

// SetServicePrincipalMode changes the mode of a service principal: open or enforce.
func (w *World) SetServicePrincipalMode(appID, mode string, a Actor) *apiError {
	if mode != ModeOpen && mode != ModeEnforce {
		return badRequest("The mode must be '%s' or '%s'.", ModeOpen, ModeEnforce)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	sp := w.spByAppID(appID)
	if sp == nil {
		return notFound("Service principal", appID)
	}
	if sp.info().Mode == mode {
		return nil
	}
	sp.Mode = mode
	w.record(Event{Type: evSPMode, Actor: a, AppID: sp.AppID, PrincipalName: sp.Name, To: mode})
	return nil
}

// SetServicePrincipalFlag turns a flag of a service principal on or off: FlagExemptFromThrottle or FlagHideCalls.
func (w *World) SetServicePrincipalFlag(appID, flag string, on bool, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	sp := w.spByAppID(appID)
	if sp == nil {
		return notFound("Service principal", appID)
	}
	field := spFlag(sp, flag)
	if field == nil {
		return badRequest("The flag must be '%s' or '%s'.", FlagExemptFromThrottle, FlagHideCalls)
	}
	if *field == on {
		return nil
	}
	*field = on
	w.record(Event{Type: evSPFlag, Actor: a, AppID: sp.AppID, PrincipalName: sp.Name, Field: flag, To: strconv.FormatBool(on)})
	return nil
}

func spFlag(sp *ServicePrincipal, flag string) *bool {
	switch flag {
	case FlagExemptFromThrottle:
		return &sp.ExemptFromThrottle
	case FlagHideCalls:
		return &sp.HideCalls
	}
	return nil
}

// ClientFlags gives the flags of the client with this application (client) ID. A client with no service
// principal has none.
func (w *World) ClientFlags(appID string) (exemptFromThrottle, hideCalls bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if sp := w.spByAppID(appID); sp != nil {
		return sp.ExemptFromThrottle, sp.HideCalls
	}
	return false, false
}

func (w *World) setCred(sp *ServicePrincipal, st *CredState, a Actor) {
	if st != nil && st.State == CredAny {
		st = nil
	}
	sp.Cred = st
	var cp *CredState
	if st != nil {
		c := *st
		cp = &c
	}
	w.record(Event{Type: evSPCredential, Actor: a, AppID: sp.AppID, PrincipalName: sp.Name, Credential: cp})
}

// ArmCredential makes the service principal keep the next credential that a client shows, and
// accept only that one from then on.
func (w *World) ArmCredential(appID string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	sp := w.spByAppID(appID)
	if sp == nil {
		return notFound("Service principal", appID)
	}
	w.setCred(sp, &CredState{State: CredArmed}, a)
	return nil
}

// ClearCredential makes the service principal accept any credential again.
func (w *World) ClearCredential(appID string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	sp := w.spByAppID(appID)
	if sp == nil {
		return notFound("Service principal", appID)
	}
	if sp.Cred != nil {
		w.setCred(sp, nil, a)
	}
	return nil
}

// CheckCredential decides if a client may sign in as a service principal with a credential. kind is
// KindSecret or KindCertificate; value is the secret, or the thumbprint, and it is empty for a
// client that shows no credential (a public client).
//
//   - A service principal in the state any accepts everything.
//   - An armed one keeps the first credential that is not empty, and accepts it. With no
//     credential, it stays armed.
//   - One with a kept credential accepts only that one.
func (w *World) CheckCredential(appID, kind, value string, a Actor) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	sp := w.spByAppID(appID)
	if sp == nil {
		return false
	}
	st := sp.Cred
	switch {
	case st == nil || st.State == CredAny:
		return true
	case st.State == CredArmed:
		if value != "" {
			hash, hint := HashCredential(value)
			w.setCred(sp, &CredState{State: CredCaptured, Kind: kind, Hash: hash, Hint: hint, At: w.now()}, a)
		}
		return true
	}
	hash, _ := HashCredential(value)
	return value != "" && kind == st.Kind && subtle.ConstantTimeCompare([]byte(hash), []byte(st.Hash)) == 1
}

// ---- the history ----

func principalEventText(e Event) (string, bool) {
	switch e.Type {
	case evTenantSet:
		return fmt.Sprintf("Changed the tenant ID from %s to %s", e.From, e.To), true
	case evSPCreate:
		name := e.AppID
		if e.SP != nil {
			name = e.SP.Name
		}
		return fmt.Sprintf("Created the service principal %s (client ID %s) when it signed in the first time", name, e.AppID), true
	case evSPMode:
		return fmt.Sprintf("Set the permission mode of the service principal %s to %s", firstNonEmpty(e.PrincipalName, e.AppID), e.To), true
	case evSPFlag:
		who := firstNonEmpty(e.PrincipalName, e.AppID)
		if e.Field == FlagHideCalls {
			if e.To == "true" {
				return fmt.Sprintf("The calls of the service principal %s are not in the list of calls now", who), true
			}
			return fmt.Sprintf("The calls of the service principal %s are in the list of calls again", who), true
		}
		if e.To == "true" {
			return fmt.Sprintf("The limits of Graph do not throttle the service principal %s now", who), true
		}
		return fmt.Sprintf("The limits of Graph throttle the service principal %s again", who), true
	case evSPCredential:
		who := firstNonEmpty(e.PrincipalName, e.AppID)
		switch c := e.Credential; {
		case c == nil || c.State == CredAny:
			return fmt.Sprintf("The service principal %s accepts any credential", who), true
		case c.State == CredArmed:
			return fmt.Sprintf("The service principal %s keeps the next credential that a client shows", who), true
		default:
			return fmt.Sprintf("The service principal %s kept a %s (%s…). It accepts only that one now", who, c.Kind, c.Hint), true
		}
	}
	return "", false
}

func (w *World) applyPrincipalEvent(e Event) bool {
	switch e.Type {
	case evTenantSet:
		w.Organization["id"] = e.To
	case evSPCreate:
		if e.SP != nil && w.spByAppID(e.SP.AppID) == nil {
			sp := *e.SP
			w.addServicePrincipal(&sp)
		}
	case evSPMode:
		if sp := w.spByAppID(e.AppID); sp != nil {
			sp.Mode = e.To
		}
	case evSPFlag:
		if sp := w.spByAppID(e.AppID); sp != nil {
			if f := spFlag(sp, e.Field); f != nil {
				*f = e.To == "true"
			}
		}
	case evSPCredential:
		if sp := w.spByAppID(e.AppID); sp != nil {
			sp.Cred = nil
			if e.Credential != nil && e.Credential.State != CredAny {
				c := *e.Credential
				sp.Cred = &c
			}
		}
	default:
		return false
	}
	return true
}
