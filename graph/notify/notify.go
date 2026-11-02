// Package notify is the change notifications of Graph: the subscriptions that a client makes (/subscriptions) and the POSTs that dollycli sends to the notification URL of
// each when what it subscribed to changes. It does not know the resources. A resource says that it can be subscribed to (api.Subscribable), the server says to the engine which
// resources there are, and a change of an object is told to it with Notify.
//
// The rules are from the pages of Microsoft Learn about change notifications. Nothing but GET /subscriptions could be tried against the real Graph (an empty list in the
// sample tenant): the validation request, the notifications, and the errors of a write are what the pages say, or a guess, and the code says which.
//
//   - A subscription has a notification URL. When it is made, dollycli POSTs to that URL with ?validationToken=<token>, and the endpoint must answer 200, text/plain, with the
//     token in the body, in 10 seconds. If not, the subscription is not made.
//   - A change is a POST of {"value":[{...}]} to the URL of each subscription that matches. This part keeps a log of what was sent and what came back.
//   - The subscriptions are in a file, so they are there after a restart. They belong to a tenant: when the seed of the data changes (a new generation), they are gone.
//   - dollycli calls the URL that the client gives. It does not call a link-local address (169.254.0.0/16, the address of the metadata service of a cloud, and fe80::/10),
//     does not follow a redirect, and does not use a proxy.
package notify

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Resource is something that can be subscribed to: a path such as users or users/{id}, the kind of object that a change names, and how long a subscription can last.
type Resource struct {
	Pattern    string
	Kind       string
	MaxMinutes int
}

// Who made the request: the application (client) and the user, when the token says.
type Who struct{ AppID, UserID string }

// Change says that an object changed. Kind is the kind of the resource (user, group), and Type is created, updated, or deleted.
type Change struct{ Kind, ID, Type string }

// Error is what Graph answers when a call fails.
type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(status int, code, format string, args ...any) *Error {
	return &Error{status, code, fmt.Sprintf(format, args...)}
}

// Config sets up the engine.
type Config struct {
	// Dir is the folder of the data. The subscriptions are in subscriptions.json in it. Empty keeps them in memory.
	Dir string
	// SeedID names the seed of the data: when it changes, the subscriptions are gone. TenantID is the tenant, as the notifications name it.
	SeedID, TenantID func() string
	Now              func() time.Time
	Resources        []Resource
	// ValidationTimeout is how long an endpoint has to answer the validation request. Graph gives 10 seconds, and that is the default.
	ValidationTimeout time.Duration
	// DeliveryTimeout is how long a notification has to be answered. The default is 10 seconds.
	DeliveryTimeout time.Duration
	// Copies says how many times the next notification is sent (1 when nil or less). The copies are the same message, with the same ID: in Graph several services hold
	// the data and are not always in step, so the same change can come from more than one.
	Copies func() int
}

// Delivery is one notification that was sent, and what came back.
type Delivery struct {
	Time           time.Time `json:"time"`
	SubscriptionID string    `json:"subscriptionId"`
	Resource       string    `json:"resource"`
	ChangeType     string    `json:"changeType"`
	URL            string    `json:"url"`
	Status         int       `json:"status"` // 0 when there was no answer
	Millis         float64   `json:"millis"`
	Error          string    `json:"error,omitempty"`
	Body           string    `json:"body"` // what was sent
}

type record struct {
	ID              string    `json:"id"`
	Resource        string    `json:"resource"`
	ChangeType      string    `json:"changeType"`
	NotificationURL string    `json:"notificationUrl"`
	LifecycleURL    string    `json:"lifecycleNotificationUrl,omitempty"`
	ClientState     string    `json:"clientState,omitempty"`
	Expires         time.Time `json:"expirationDateTime"`
	ApplicationID   string    `json:"applicationId,omitempty"`
	CreatorID       string    `json:"creatorId,omitempty"`
	Kind            string    `json:"kind"`
	Target          string    `json:"target,omitempty"` // the lowercase ID of one object, for a path such as users/{id}
}

type file struct {
	Seed          string    `json:"seed"`
	Subscriptions []*record `json:"subscriptions"`
}

// maxSubscriptions is the most that dollycli keeps (the quota of a tenant in Graph is 1,000 for users and groups).
const maxSubscriptions = 1000

const maxDeliveries = 200

// Engine keeps the subscriptions and sends the notifications.
type Engine struct {
	cfg    Config
	client *http.Client

	mu         sync.Mutex
	subs       []*record
	seed       string
	deliveries []Delivery

	queue   chan Change
	pending sync.WaitGroup
	done    chan struct{}
	closed  sync.Once
	stopped sync.WaitGroup
}

// New makes the engine, reads the file of the subscriptions, and drops what is not good any more: a subscription of another seed, and one that expired.
func New(cfg Config) (*Engine, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.ValidationTimeout <= 0 {
		cfg.ValidationTimeout = 10 * time.Second
	}
	if cfg.DeliveryTimeout <= 0 {
		cfg.DeliveryTimeout = 10 * time.Second
	}
	if cfg.SeedID == nil {
		cfg.SeedID = func() string { return "" }
	}
	if cfg.TenantID == nil {
		cfg.TenantID = func() string { return "" }
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip != nil && blocked(ip) {
			return fmt.Errorf("dollycli does not call the address %s", host)
		}
		return nil
	}}
	e := &Engine{
		cfg: cfg, queue: make(chan Change, 1024), done: make(chan struct{}),
		client: &http.Client{
			Transport:     &http.Transport{DialContext: dialer.DialContext, DialTLSContext: dialTLS(dialer), Proxy: nil, DisableKeepAlives: true},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		seed: cfg.SeedID(),
	}
	if err := e.load(); err != nil {
		return nil, err
	}
	e.stopped.Add(1)
	go e.run()
	return e, nil
}

// dialTLS makes the TLS connections. A certificate is checked as always, except when the receiver is on this machine (a loopback address): a dev certificate, or the CA of
// dollycli itself, is not trusted by the system, and the traffic does not leave the machine.
func dialTLS(d *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := d.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		host, _, _ := net.SplitHostPort(addr)
		cfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		if tcp, ok := conn.RemoteAddr().(*net.TCPAddr); ok && tcp.IP.IsLoopback() {
			cfg.InsecureSkipVerify = true
		}
		tc := tls.Client(conn, cfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return tc, nil
	}
}

// blocked says whether dollycli does not call an address: link-local (this has the metadata service of a cloud), multicast, and the unspecified address.
func blocked(ip net.IP) bool {
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}

// Close stops the engine.
func (e *Engine) Close() {
	e.closed.Do(func() { close(e.done) })
	e.stopped.Wait()
}

func (e *Engine) path() string {
	if e.cfg.Dir == "" {
		return ""
	}
	return filepath.Join(e.cfg.Dir, "subscriptions.json")
}

func (e *Engine) load() error {
	p := e.path()
	if p == "" {
		return nil
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("notify: %s is not valid: %w", p, err)
	}
	if f.Seed != e.seed {
		if len(f.Subscriptions) > 0 {
			slog.Info("Dolly dropped the subscriptions: the data has a new seed.", "subscriptions", len(f.Subscriptions))
		}
		e.subs = nil
	} else {
		e.subs = f.Subscriptions
	}
	e.purgeLocked()
	return e.saveLocked()
}

func (e *Engine) saveLocked() error {
	p := e.path()
	if p == "" {
		return nil
	}
	if err := os.MkdirAll(e.cfg.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file{Seed: e.seed, Subscriptions: e.subs}, "", " ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // the client states are secrets
		return err
	}
	return os.Rename(tmp, p)
}

// purgeLocked drops what expired, and everything when the seed has changed. Call it first in every call, with the lock held.
func (e *Engine) purgeLocked() {
	changed := false
	if cur := e.cfg.SeedID(); cur != e.seed {
		changed = len(e.subs) > 0
		e.subs, e.seed = nil, cur
		if changed {
			slog.Info("Dolly dropped the subscriptions: the data has a new seed.")
		}
		_ = e.saveLocked()
		return
	}
	now := e.cfg.Now()
	n := len(e.subs)
	e.subs = slices.DeleteFunc(e.subs, func(s *record) bool { return !s.Expires.After(now) })
	if len(e.subs) != n {
		_ = e.saveLocked()
	}
}

const timeFormat = "2006-01-02T15:04:05.0000000Z"

func (r *record) object() map[string]any {
	str := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	return map[string]any{
		"id": r.ID, "resource": r.Resource, "applicationId": str(r.ApplicationID), "changeType": r.ChangeType, "clientState": str(r.ClientState),
		"notificationUrl": r.NotificationURL, "notificationQueryOptions": nil, "lifecycleNotificationUrl": str(r.LifecycleURL),
		"expirationDateTime": r.Expires.UTC().Format(timeFormat), "creatorId": str(r.CreatorID), "includeResourceData": false,
		"latestSupportedTlsVersion": "v1_2", "encryptionCertificate": nil, "encryptionCertificateId": nil, "notificationContentType": "application/json",
	}
}

// List gives the subscriptions.
func (e *Engine) List() []map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.purgeLocked()
	out := make([]map[string]any, len(e.subs))
	for i, s := range e.subs {
		out[i] = s.object()
	}
	return out
}

func (e *Engine) find(id string) *record {
	for _, s := range e.subs {
		if strings.EqualFold(s.ID, id) {
			return s
		}
	}
	return nil
}

func notFound(id string) *Error {
	// a guess: Graph's text for a subscription that is not there is not on the pages
	return fail(http.StatusNotFound, "ResourceNotFound", "Subscription Id '%s' was not found.", id)
}

// Get gives one subscription.
func (e *Engine) Get(id string) (map[string]any, *Error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.purgeLocked()
	s := e.find(id)
	if s == nil {
		return nil, notFound(id)
	}
	return s.object(), nil
}

// Delete takes a subscription away.
func (e *Engine) Delete(id string) *Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.purgeLocked()
	s := e.find(id)
	if s == nil {
		return notFound(id)
	}
	e.subs = slices.DeleteFunc(e.subs, func(x *record) bool { return x == s })
	_ = e.saveLocked()
	return nil
}

var changeTypes = []string{"created", "updated", "deleted"}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return strings.TrimSpace(s)
}

func required(m map[string]any, k string) *Error {
	if str(m, k) == "" {
		return fail(http.StatusBadRequest, "ValidationError", "Property '%s' is required.", k)
	}
	return nil
}

// match finds the resource that a path is: users is the pattern users, and users/<id> is users/{id}. It gives the resource, and the ID of the object when the path has one.
func (e *Engine) match(path string) (Resource, string, bool) {
	segs := strings.Split(strings.Trim(strings.ToLower(strings.TrimSpace(path)), "/"), "/")
	for _, r := range e.cfg.Resources {
		pat := strings.Split(r.Pattern, "/")
		if len(pat) != len(segs) {
			continue
		}
		id, ok := "", true
		for i, p := range pat {
			switch {
			case p == "{id}" && segs[i] != "":
				id = segs[i]
			case p != strings.ToLower(segs[i]):
				ok = false
			}
		}
		if ok {
			return r, id, true
		}
	}
	return Resource{}, "", false
}

// checkURL says why dollycli does not call a notification URL, or nil.
func checkURL(prop, raw string) (*url.URL, *Error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fail(http.StatusBadRequest, "ValidationError", "Property '%s' must be an https or http URL.", prop)
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && blocked(ip) {
		return nil, fail(http.StatusBadRequest, "ValidationError", "Property '%s' is an address that dollycli does not call (a link-local address).", prop)
	}
	return u, nil
}

// validate sends the validation request: a POST with the token in the query. The endpoint must answer 200, text/plain, and the token, in the time that Graph gives.
func (e *Engine) validate(ctx context.Context, prop string, u *url.URL) *Error {
	raw := make([]byte, 31) // 44 characters of base64, with a padding: a token that has to be decoded
	_, _ = rand.Read(raw)
	token := base64.StdEncoding.EncodeToString(raw)
	v := *u
	q := v.Query()
	q.Set("validationToken", token)
	v.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, e.cfg.ValidationTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.String(), nil)
	if err == nil {
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
		req.Header.Set("User-Agent", "dollycli")
		var res *http.Response
		if res, err = e.client.Do(req); err == nil {
			defer res.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
			if res.StatusCode == http.StatusOK && strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "text/plain") && string(body) == token {
				return nil
			}
			err = fmt.Errorf("it answered %d, %q, and %d bytes", res.StatusCode, res.Header.Get("Content-Type"), len(body))
		}
	}
	// a guess: the code of the real Graph for this is ValidationError, and the text is as it is remembered
	slog.Debug("dolly: the validation of a notification URL failed", "property", prop, "url", u.Redacted(), "why", err)
	return fail(http.StatusBadRequest, "ValidationError", "Subscription validation request failed. Notification endpoint must respond with 200 OK to validation request.")
}

func (e *Engine) checkExpiry(r Resource, raw string) (time.Time, *Error) {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fail(http.StatusBadRequest, "ValidationError", "Property 'expirationDateTime' is not a date and time: %q.", raw)
	}
	now := e.cfg.Now()
	if !t.After(now) {
		return time.Time{}, fail(http.StatusBadRequest, "ValidationError", "Property 'expirationDateTime' must be in the future.")
	}
	if max := time.Duration(r.MaxMinutes) * time.Minute; r.MaxMinutes > 0 && t.After(now.Add(max)) {
		return time.Time{}, fail(http.StatusBadRequest, "ValidationError", "Subscription expiration can only be %d minutes in the future.", r.MaxMinutes)
	}
	return t.UTC(), nil
}

// sameTypes compares two lists of change types, in any order.
func sameTypes(stored string, types []string) bool {
	a, b := strings.Split(stored, ","), slices.Clone(types)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(slices.Compact(a), slices.Compact(b))
}

func sameResource(a, b string) bool {
	norm := func(s string) string { return strings.Trim(strings.ToLower(strings.TrimSpace(s)), "/") }
	return norm(a) == norm(b)
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Create makes a subscription. It needs changeType, notificationUrl, resource, and expirationDateTime. The notification URL is validated before.
func (e *Engine) Create(ctx context.Context, body map[string]any, who Who) (map[string]any, *Error) {
	for _, k := range []string{"changeType", "notificationUrl", "resource", "expirationDateTime"} {
		if err := required(body, k); err != nil {
			return nil, err
		}
	}
	types := strings.Split(strings.ToLower(strings.ReplaceAll(str(body, "changeType"), " ", "")), ",")
	for _, t := range types {
		if !slices.Contains(changeTypes, t) {
			return nil, fail(http.StatusBadRequest, "ValidationError", "Property 'changeType' must be a list of created, updated, and deleted: %q is not.", t)
		}
	}
	res, target, ok := e.match(str(body, "resource"))
	if !ok {
		return nil, fail(http.StatusBadRequest, "ValidationError", "Property 'resource' is not a resource that dollycli can send change notifications for: %q.", str(body, "resource"))
	}
	exp, err := e.checkExpiry(res, str(body, "expirationDateTime"))
	if err != nil {
		return nil, err
	}
	u, err := checkURL("notificationUrl", str(body, "notificationUrl"))
	if err != nil {
		return nil, err
	}
	if v, has := body["includeResourceData"]; has && v == true {
		return nil, fail(http.StatusBadRequest, "ValidationError", "Property 'includeResourceData' is not supported for this resource: its notifications have no resource data.")
	}
	var lifecycle *url.URL
	if raw := str(body, "lifecycleNotificationUrl"); raw != "" {
		if lifecycle, err = checkURL("lifecycleNotificationUrl", raw); err != nil {
			return nil, err
		}
	}
	// the validation is a call to another server: no lock is held for it
	if err := e.validate(ctx, "notificationUrl", u); err != nil {
		return nil, err
	}
	if lifecycle != nil {
		if err := e.validate(ctx, "lifecycleNotificationUrl", lifecycle); err != nil {
			return nil, err
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.purgeLocked()
	if len(e.subs) >= maxSubscriptions {
		// a guess: the text of Graph for a quota that is passed is not on the pages
		return nil, fail(http.StatusBadRequest, "ValidationError", "The quota of subscriptions is passed: dollycli keeps %d.", maxSubscriptions)
	}
	// Learn: the same changeType and resource as a subscription that is there is 409. Whether it is the same application that counts is a guess.
	for _, other := range e.subs {
		if other.ApplicationID == who.AppID && other.Target == target && other.Kind == res.Kind && sameTypes(other.ChangeType, types) && sameResource(other.Resource, str(body, "resource")) {
			return nil, fail(http.StatusConflict, "ExtensionError", "Subscription Id %s already exists for the requested combination", other.ID)
		}
	}
	s := &record{
		ID: newID(), Resource: str(body, "resource"), ChangeType: strings.Join(types, ","), NotificationURL: str(body, "notificationUrl"), LifecycleURL: str(body, "lifecycleNotificationUrl"),
		ClientState: str(body, "clientState"), Expires: exp, ApplicationID: who.AppID, CreatorID: who.UserID, Kind: res.Kind, Target: target,
	}
	e.subs = append(e.subs, s)
	_ = e.saveLocked()
	return s.object(), nil
}

// Update renews a subscription (expirationDateTime), or gives it another notification URL, which is validated. Nothing else can change.
func (e *Engine) Update(ctx context.Context, id string, body map[string]any) (map[string]any, *Error) {
	for k := range body {
		if k != "expirationDateTime" && k != "notificationUrl" {
			return nil, fail(http.StatusBadRequest, "ValidationError", "Property '%s' cannot be changed. Only expirationDateTime and notificationUrl can.", k)
		}
	}
	if str(body, "expirationDateTime") == "" && str(body, "notificationUrl") == "" {
		return nil, fail(http.StatusBadRequest, "ValidationError", "The request must have expirationDateTime or notificationUrl.")
	}
	e.mu.Lock()
	e.purgeLocked()
	s := e.find(id)
	if s == nil {
		e.mu.Unlock()
		return nil, notFound(id)
	}
	res, _, _ := e.match(s.Resource)
	e.mu.Unlock()

	var exp time.Time
	var u *url.URL
	var err *Error
	if raw := str(body, "expirationDateTime"); raw != "" {
		if exp, err = e.checkExpiry(res, raw); err != nil {
			return nil, err
		}
	}
	if raw := str(body, "notificationUrl"); raw != "" {
		if u, err = checkURL("notificationUrl", raw); err != nil {
			return nil, err
		}
		if err = e.validate(ctx, "notificationUrl", u); err != nil {
			return nil, err
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.purgeLocked()
	if s = e.find(id); s == nil {
		return nil, notFound(id)
	}
	if !exp.IsZero() {
		s.Expires = exp
	}
	if u != nil {
		s.NotificationURL = u.String()
	}
	_ = e.saveLocked()
	return s.object(), nil
}

// Resources gives what a subscription can be made for.
func (e *Engine) Resources() []Resource { return slices.Clone(e.cfg.Resources) }

// Emulate sends a notification to one subscription, as if an object had changed. No data changes, and no other subscription gets it. For a subscription to all objects of a
// kind, pick gives the ID of the object to name. The change type must be one of the subscription.
func (e *Engine) Emulate(id, changeType string, pick func(kind string) string) *Error {
	e.mu.Lock()
	e.purgeLocked()
	s := e.find(id)
	if s == nil {
		e.mu.Unlock()
		return notFound(id)
	}
	rec := *s
	e.mu.Unlock()
	changeType = strings.ToLower(strings.TrimSpace(changeType))
	if !slices.Contains(changeTypes, changeType) {
		return fail(http.StatusBadRequest, "ValidationError", "changeType must be created, updated, or deleted.")
	}
	c := Change{Kind: rec.Kind, ID: rec.Target, Type: changeType}
	if c.ID == "" && pick != nil {
		c.ID = pick(rec.Kind)
	}
	if c.ID == "" {
		return fail(http.StatusBadRequest, "ValidationError", "There is no %s to name in the notification.", rec.Kind)
	}
	if !rec.matches(c) {
		return fail(http.StatusBadRequest, "ValidationError", "This subscription is not for %s changes: it has %s.", changeType, rec.ChangeType)
	}
	e.pending.Add(1)
	go func() {
		defer e.pending.Done()
		e.send(rec, c, e.cfg.TenantID())
	}()
	return nil
}

// Deliveries gives what was sent, the newest first.
func (e *Engine) Deliveries() []Delivery {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := slices.Clone(e.deliveries)
	slices.Reverse(out)
	return out
}

// Notify tells the engine that an object changed. It does not wait for the delivery, and it is safe to call with a lock held: when the queue is full, the change is dropped.
func (e *Engine) Notify(c Change) {
	e.pending.Add(1)
	select {
	case e.queue <- c:
	default:
		e.pending.Done()
		slog.Warn("Dolly dropped a change notification: too many are waiting.", "kind", c.Kind, "id", c.ID)
	}
}

// Flush waits until the notifications that were told are delivered. Tests use it.
func (e *Engine) Flush() { e.pending.Wait() }

func (e *Engine) run() {
	defer e.stopped.Done()
	for {
		select {
		case <-e.done:
			return
		case c := <-e.queue:
			e.deliver(c)
			e.pending.Done()
		}
	}
}

// matches says whether a subscription gets a change. Learn: the creation and the deletion of a user or a group also trigger updated.
func (s *record) matches(c Change) bool {
	if s.Kind != c.Kind || (s.Target != "" && s.Target != strings.ToLower(c.ID)) {
		return false
	}
	types := strings.Split(s.ChangeType, ",")
	return slices.Contains(types, c.Type) || (c.Type != "updated" && slices.Contains(types, "updated"))
}

var odataTypes = map[string]string{"user": "User", "group": "Group"}
var odataSets = map[string]string{"user": "Users", "group": "Groups"}

func (e *Engine) deliver(c Change) {
	e.mu.Lock()
	e.purgeLocked()
	var subs []record
	for _, s := range e.subs {
		if s.matches(c) {
			subs = append(subs, *s)
		}
	}
	e.mu.Unlock()
	tenant := e.cfg.TenantID()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for _, s := range subs {
		wg.Add(1)
		sem <- struct{}{}
		go func(s record) {
			defer wg.Done()
			defer func() { <-sem }()
			e.send(s, c, tenant)
		}(s)
	}
	wg.Wait()
}

func (e *Engine) send(s record, c Change, tenant string) {
	set, typ := odataSets[c.Kind], odataTypes[c.Kind]
	resource := set + "/" + c.ID
	n := map[string]any{
		"id": newID(), "subscriptionId": s.ID, "subscriptionExpirationDateTime": s.Expires.UTC().Format(timeFormat), "changeType": c.Type,
		"resource": resource, "tenantId": tenant,
		"resourceData": map[string]any{"@odata.type": "#Microsoft.Graph." + typ, "@odata.id": resource, "id": c.ID},
	}
	if s.ClientState != "" {
		n["clientState"] = s.ClientState
	}
	body, _ := json.Marshal(map[string]any{"value": []any{n}})
	copies := 1
	if e.cfg.Copies != nil {
		copies = max(1, e.cfg.Copies())
	}
	for range copies {
		e.post(s, c, resource, string(body))
	}
}

func (e *Engine) post(s record, c Change, resource, body string) {
	d := Delivery{Time: e.cfg.Now(), SubscriptionID: s.ID, Resource: resource, ChangeType: c.Type, URL: s.NotificationURL, Body: body}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), e.cfg.DeliveryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.NotificationURL, strings.NewReader(body))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "dollycli")
		var res *http.Response
		if res, err = e.client.Do(req); err == nil {
			d.Status = res.StatusCode
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
			res.Body.Close()
		}
	}
	d.Millis = float64(time.Since(start).Microseconds()) / 1000
	if err != nil {
		d.Error = err.Error()
	} else if d.Status < 200 || d.Status > 299 {
		d.Error = fmt.Sprintf("the endpoint answered %d: not delivered", d.Status)
	}
	e.mu.Lock()
	e.deliveries = append(e.deliveries, d)
	if len(e.deliveries) > maxDeliveries {
		e.deliveries = e.deliveries[len(e.deliveries)-maxDeliveries:]
	}
	e.mu.Unlock()
}
