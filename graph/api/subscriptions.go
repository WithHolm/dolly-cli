package api

import (
	"context"
	"net/http"
)

// Subscribable says that a client can subscribe to the changes of a resource: the change notifications of Graph. A resource declares it in its Register, next to its routes, as
// it declares what delta query tracks. /subscriptions refuses a resource that nothing has declared.
type Subscribable struct {
	// Pattern is the path that a subscription names, without the version: users, users/{id}, groups/{id}.
	Pattern string
	// Kind is the kind of object that a change names (user, group). It is what the server tells the engine of notifications.
	Kind string
	// MaxMinutes is the longest life of a subscription to it (the page "Subscription lifetime" of Microsoft Learn).
	MaxMinutes int
}

// Subscribable declares what a client can subscribe to.
func (r *Router) Subscribable(s ...Subscribable) { r.subscribables = append(r.subscribables, s...) }

// Subscribables gives what the resources declared, in the order that they did.
func (r *Router) Subscribables() []Subscribable {
	return append([]Subscribable(nil), r.subscribables...)
}

// directoryLifetime is the longest subscription to a user, a group, or another directory resource: 41,760 minutes, under 29 days (Learn: Subscription lifetime).
const directoryLifetime = 41760

// SubscriptionsBackend is what /subscriptions needs. The engine of notifications is behind it. A call that makes or changes a subscription names the request, because
// the application and the user of the token go in the subscription.
type SubscriptionsBackend interface {
	ListSubscriptions() []Object
	GetSubscription(id string) (Object, *Error)
	CreateSubscription(ctx context.Context, props Object, r *http.Request) (Object, *Error)
	UpdateSubscription(ctx context.Context, id string, props Object) (Object, *Error)
	DeleteSubscription(id string) *Error
}

// Subscriptions is the resource for /subscriptions.
func Subscriptions(b SubscriptionsBackend) Resource { return subscriptions{b} }

type subscriptions struct{ b SubscriptionsBackend }

func (s subscriptions) Register(r *Router) {
	env := r.Env()
	sc := r.Scope("/subscriptions", "Subscriptions")
	sc.Handle("GET", "", "List the subscriptions: the clients that are told when a user or a group changes.", []string{"$select", "$top"},
		func(w http.ResponseWriter, req *http.Request) {
			ServeList(w, req, env, s.b.ListSubscriptions(), ListOptions{Context: "subscriptions", NoConsistency: true})
		})
	sc.Handle("GET", "/{id}", "Get one subscription.", []string{"$select"},
		func(w http.ResponseWriter, req *http.Request) {
			o, e := s.b.GetSubscription(req.PathValue("id"))
			if e != nil {
				WriteError(w, e)
				return
			}
			WriteJSON(w, http.StatusOK, one("subscriptions", o, req, nil, nil, "subscription"))
		})
	sc.Handle("POST", "", "Subscribe to the changes of users or groups. The notification URL is validated first. Gives 201.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				var o Object
				if o, e = s.b.CreateSubscription(req.Context(), props, req); e == nil {
					o["@odata.context"] = contextURL("subscriptions", nil, true)
					WriteJSON(w, http.StatusCreated, o)
					return
				}
			}
			WriteError(w, e)
		})
	sc.Handle("PATCH", "/{id}", "Renew a subscription (expirationDateTime), or give it another notificationUrl, which is validated. Nothing else can change. Gives 200 with the subscription.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				var o Object
				if o, e = s.b.UpdateSubscription(req.Context(), req.PathValue("id"), props); e == nil {
					o["@odata.context"] = contextURL("subscriptions", nil, true)
					WriteJSON(w, http.StatusOK, o)
					return
				}
			}
			WriteError(w, e)
		})
	sc.Handle("DELETE", "/{id}", "Delete a subscription. Gives 204.", nil, func(w http.ResponseWriter, req *http.Request) {
		respond(w, s.b.DeleteSubscription(req.PathValue("id")))
	})
}
