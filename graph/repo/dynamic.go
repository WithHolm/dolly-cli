package repo

import (
	"log/slog"
	"strings"

	"github.com/withholm/dolly-cli/graph/rule"
)

// Dynamic groups: the members of a group with dynamic membership come from its rule, as in Entra
// (Learn: "Manage rules for dynamic membership groups"). The rule is checked when the group is made or
// changed (a rule that Entra would refuse is a 400). The members are worked out again when a user
// changes, when a manager changes, and when the rule changes. A group with the processing state "Paused"
// keeps the members that it has. The members are derived data: they are not in the history, and a replay
// works them out again.

const (
	processingOn     = "On"
	processingPaused = "Paused"
)

// processing is the state of the rule of a dynamic group: On when it was never set.
func (g *Group) processing() string {
	if g.RuleProcessing == "" {
		return processingOn
	}
	return g.RuleProcessing
}

// checkRule says whether Entra would take this rule for a group.
func checkRule(text string) *apiError {
	if _, err := rule.Parse(text); err != nil {
		return badRequest("The membership rule is not valid: %s.", err)
	}
	return nil
}

// checkProcessing reads a value of membershipRuleProcessingState.
func checkProcessing(v any) (string, *apiError) {
	s, _ := v.(string)
	switch {
	case strings.EqualFold(s, processingOn):
		return processingOn, nil
	case strings.EqualFold(s, processingPaused):
		return processingPaused, nil
	}
	return "", badRequest("Invalid value specified for property 'membershipRuleProcessingState' of resource 'Group'. Use On or Paused.")
}

func (w *World) ruleEnv() rule.Env {
	return rule.Env{Manager: func(id string) string { return w.Managers[id] }}
}

// refreshDynamic works out the members of every dynamic group again. Call it with the lock held.
func (w *World) refreshDynamic() {
	env := w.ruleEnv()
	for _, g := range w.Groups {
		w.refreshGroup(g, env)
	}
}

// refreshGroup works out the members of one dynamic group: the users that its rule takes, in the
// order of the users. A paused group, and a group with no rule that can be read, keep their members.
func (w *World) refreshGroup(g *Group, env rule.Env) {
	if !g.Dynamic || g.Rule == "" || g.processing() == processingPaused {
		return
	}
	if g.rule == nil || g.ruleText != g.Rule {
		r, err := rule.Parse(g.Rule)
		if err != nil {
			slog.Warn("Dolly cannot read the rule of a dynamic group, so it keeps the members.", "group", g.Name, "err", err)
			return
		}
		g.rule, g.ruleText = r, g.Rule
	}
	members := make([]string, 0, len(g.Members))
	for _, u := range w.Users {
		if g.rule.Match(u, env) {
			members = append(members, strings.ToLower(str(u["id"])))
		}
	}
	g.Members = members
}

// refreshFor works out the groups that an event of the history can change, after a replay did it.
func (w *World) refreshFor(e Event) {
	switch e.Type {
	case evGroupCreate, evGroupUpdate:
		if g := w.groups[strings.ToLower(e.Group)]; g != nil {
			w.refreshGroup(g, w.ruleEnv())
		}
	case evRename, evUser, evUserCreate, evUserDelete, evManagerSet, evManagerRemove:
		w.refreshDynamic()
	}
}
