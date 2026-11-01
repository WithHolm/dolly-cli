package permissions

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

// implies.json is written by hand. It says what a permission is assumed to include, when Microsoft's tables do not say it: a permission that has
// a name like ReadUpdate, and that only one page names. These are assumptions, and the portal says so. The tables of Microsoft are never changed by it.

//go:embed implies.json
var impliesJSON []byte

// Assumption is what a permission is assumed to include.
type Assumption struct {
	Includes []string `json:"includes"`
	Note     string   `json:"note"`
}

// AssumedPermission is a permission with its assumption, for the admin API.
type AssumedPermission struct {
	Permission string `json:"permission"`
	Assumption
}

var implied = sync.OnceValue(func() map[string]Assumption {
	var raw map[string]Assumption
	if err := json.Unmarshal(impliesJSON, &raw); err != nil {
		panic("permissions/implies.json: " + err.Error())
	}
	out := make(map[string]Assumption, len(raw))
	for k, v := range raw {
		out[strings.ToLower(k)] = v
	}
	return out
})

// Assumptions gives the whole table, by name in lower case.
func Assumptions() map[string]Assumption { return implied() }

// Assumed says what a permission is assumed to include, directly.
func Assumed(permission string) (Assumption, bool) {
	a, ok := implied()[strings.ToLower(permission)]
	return a, ok
}

// Includes gives every permission that holding `permission` is assumed to give as well, in lower case, through the chains in implies.json.
func Includes(permission string) []string {
	var out []string
	seen := map[string]bool{strings.ToLower(permission): true}
	queue := []string{strings.ToLower(permission)}
	for len(queue) > 0 {
		a, ok := implied()[queue[0]]
		queue = queue[1:]
		if !ok {
			continue
		}
		for _, p := range a.Includes {
			if l := strings.ToLower(p); !seen[l] {
				seen[l] = true
				out = append(out, l)
				queue = append(queue, l)
			}
		}
	}
	return out
}
