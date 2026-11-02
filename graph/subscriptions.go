package graph

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/withholm/dolly-cli/graph/api"
	"github.com/withholm/dolly-cli/graph/notify"
	"github.com/withholm/dolly-cli/graph/repo"
)

// The change notifications of Graph: /subscriptions, and the notifications that are sent when a user or a group changes. The engine is graph/notify. The resources say what can
// be subscribed to (api.Subscribable, in their Register), and the world says what changed (World.OnChange).

// subscriptions is the backend of /subscriptions: the engine, and the token of the request.
type subscriptions struct{ s *Server }

func (b subscriptions) engine() *notify.Engine { return b.s.notifier }

func asAPIError(e *notify.Error) *api.Error {
	return &api.Error{Status: e.Status, Code: e.Code, Message: e.Message}
}

func (b subscriptions) ListSubscriptions() []api.Object {
	list := b.engine().List()
	out := make([]api.Object, len(list))
	for i, o := range list {
		out[i] = o
	}
	return out
}

func (b subscriptions) GetSubscription(id string) (api.Object, *api.Error) {
	o, e := b.engine().Get(id)
	if e != nil {
		return nil, asAPIError(e)
	}
	return o, nil
}

func (b subscriptions) CreateSubscription(ctx context.Context, props api.Object, r *http.Request) (api.Object, *api.Error) {
	who := notify.Who{}
	if info, ours := b.s.token(r); ours {
		who.AppID, who.UserID = info.AppID, info.UserID
	}
	o, e := b.engine().Create(ctx, props, who)
	if e != nil {
		return nil, asAPIError(e)
	}
	return o, nil
}

func (b subscriptions) UpdateSubscription(ctx context.Context, id string, props api.Object) (api.Object, *api.Error) {
	o, e := b.engine().Update(ctx, id, props)
	if e != nil {
		return nil, asAPIError(e)
	}
	return o, nil
}

func (b subscriptions) DeleteSubscription(id string) *api.Error {
	if e := b.engine().Delete(id); e != nil {
		return asAPIError(e)
	}
	return nil
}

// ReceiverPath is where dollycli receives change notifications itself: a subscription can name it as its notificationUrl, to see what is sent without running a receiver.
// It answers the validation request, and takes a notification with 202. It needs no token, as the receiver of a notification has none.
const ReceiverPath = "/dolly-receiver/notifications"

func (s *Server) receive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "This address takes the change notifications that dollycli sends: POST.", http.StatusMethodNotAllowed)
		return
	}
	if tok := r.URL.Query().Get("validationToken"); tok != "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, tok)
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))
	w.WriteHeader(http.StatusAccepted)
}

// startNotifier makes the engine of change notifications, with what the resources said can be subscribed to, and tells it of the changes of the world. Call it once the resources
// are mounted.
func (s *Server) startNotifier(subscribable []api.Subscribable) {
	resources := make([]notify.Resource, len(subscribable))
	for i, x := range subscribable {
		resources[i] = notify.Resource{Pattern: x.Pattern, Kind: x.Kind, MaxMinutes: x.MaxMinutes}
	}
	eng, err := notify.New(notify.Config{
		Dir: s.world.Dir(), SeedID: s.world.SeedID, TenantID: s.world.TenantID, Resources: resources,
		Copies: func() int { return s.behavior().copies() },
	})
	if err != nil {
		slog.Error("Dolly cannot read its subscriptions. It starts with none.", "error", err)
		eng, _ = notify.New(notify.Config{SeedID: s.world.SeedID, TenantID: s.world.TenantID, Resources: resources})
	}
	s.notifier = eng
	s.world.OnChange(func(c ChangeEvent) {
		// Only the objects that a client can subscribe to. A change of the members of a group, and of a service principal, are not sent yet.
		if (c.Kind != repo.KindUser && c.Kind != repo.KindGroup) || c.Member != "" {
			return
		}
		typ := "updated"
		switch {
		case c.Created:
			typ = "created"
		case c.Removed:
			typ = "deleted"
		}
		s.notifier.Notify(notify.Change{Kind: c.Kind, ID: c.ID, Type: typ})
	})
}

// FlushNotifications waits until the change notifications that are waiting are delivered. Tests use it.
func (s *Server) FlushNotifications() { s.notifier.Flush() }

// Close stops what the server runs in the background: the delivery of change notifications. It does not close the world.
func (s *Server) Close() { s.notifier.Close() }

// the admin API: the portal shows the subscriptions, what was sent, and what can be subscribed to
func (s *Server) adminSubscriptions() map[string]any {
	type resource struct {
		Pattern    string `json:"pattern"`
		Kind       string `json:"kind"`
		MaxMinutes int    `json:"maxMinutes"`
	}
	var resources []resource
	for _, r := range s.notifier.Resources() {
		resources = append(resources, resource{r.Pattern, r.Kind, r.MaxMinutes})
	}
	return map[string]any{"subscriptions": s.notifier.List(), "deliveries": s.notifier.Deliveries(), "resources": resources}
}

// adminCreateSubscription makes a subscription for the portal. It is the same call as POST /subscriptions, with the same checks (and the validation request to the
// notification URL), as the client of the portal.
func (s *Server) adminCreateSubscription(w http.ResponseWriter, r *http.Request) {
	body, e := api.ReadJSON(r)
	if e != nil {
		writeError(w, e)
		return
	}
	o, err := s.notifier.Create(r.Context(), body, notify.Who{AppID: PortalAppID})
	if err != nil {
		writeError(w, asAPIError(err))
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

// adminEmulate sends one subscription a notification as if an object had changed. The data does not change. For a subscription to all users or all groups, the object
// that the notification names is the first one.
func (s *Server) adminEmulate(w http.ResponseWriter, r *http.Request) {
	body, e := api.ReadJSON(r)
	if e != nil {
		writeError(w, e)
		return
	}
	changeType, _ := body["changeType"].(string)
	pick := func(kind string) string {
		list := s.world.ListUsers()
		if kind == repo.KindGroup {
			list = s.world.ListGroups()
		}
		if len(list) == 0 {
			return ""
		}
		id, _ := list[0]["id"].(string)
		return id
	}
	if err := s.notifier.Emulate(r.PathValue("id"), changeType, pick); err != nil {
		writeError(w, asAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
