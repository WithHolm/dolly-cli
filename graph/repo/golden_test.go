package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// The same seed makes the same world. These are the hashes of the data that the generator made before it read a profile. The default profile
// must make exactly this data, so a seed folder, a test, and a demo that depend on a seed do not change. If a hash changes on purpose
// (a new property in every user, for example), say so in the commit and replace the hash.
func TestGenerate_DefaultProfileMakesTheSameDataAsBefore(t *testing.T) {
	for _, c := range []struct {
		users int
		seed  int64
		want  string
	}{
		{10, 7, "ce756ba1a97fb803c5b7018e332efb5c24b1445805459b48a752a7fcb0aeffb5"},
		{60, 1, "db390eae0cd160c52f675f8bfa591806a3bbc94379078efd91c253c85e0587d5"},
		{200, 5, "bc0db5c63a05098dd5fd7bbc9c7505a3d1ac387224b53e5a2da6bc00fa9631ef"},
		{500, 1, "3fefe84969aee8ea6cf0c55748b405fdf996b57858b0e48b13dcf99c69988c17"},
	} {
		w := Generate(Options{Users: c.users, Seed: c.seed})
		w.Applications = nil // the app registrations came later (applications.go): the rest of the data must be what it was, and they draw nothing
		b, err := json.Marshal(w)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != c.want {
			t.Errorf("users=%d seed=%d: hash %s, want %s", c.users, c.seed, got, c.want)
		}
	}
}
