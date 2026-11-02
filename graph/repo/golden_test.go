package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

type golden struct {
	users int
	seed  int64
	want  string
}

// hashWorld is the hash of the data of a world. The profile and the admin account are left out: the seed file keeps them, and these tests are about the people and the
// groups. The app registrations came later (applications.go), they draw nothing, and they are left out too.
func hashWorld(t *testing.T, w *World) string {
	t.Helper()
	w.Profile, w.Admin, w.Applications = nil, "", nil
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// The same seed makes the same world. These are the hashes of the data that the generator made before Fjordvik was the default (and before that, before it read a
// profile): Acme, with Nora Helmer. The profile acme.json must make exactly this data, so a seed folder that was made then, and a test that depends on a seed, are
// still what they were. If a hash changes on purpose (a new property in every user, for example), say so in the commit and replace the hash.
func TestGenerate_AcmeMakesTheSameDataAsBefore(t *testing.T) {
	for _, c := range []golden{
		// 2026-11-02: replaced on purpose, twice. First for Nora Helmer (seed.go makeNora): she is the generated user three from the end, and no other user
		// or other part of the world changed (checked by comparing the world before and after for each of these seeds). Then for the rest of the cast
		// (seed.go cast): Torvald Helmer, Nils Krogstad, and Dr. Rank are the users four, five, and six from the end. Their departments, offices, and the
		// boss links change the department and office groups and the manager tree a little. No random number is drawn for any of them.
		{10, 7, "c1e22c8b6a034bcdd713c0bcf80480ed6ae46427a62ea7f53c060d47ad70bfcf"},
		{60, 1, "d7646a429c94667d01b0fad6dd4c96aa81ac00e255867f09d58d7c3a3e27be29"},
		{200, 5, "1eda59f0f4b8b558b2e4bf82d85001e9a49134a487345f9a027a8dafa4afd865"},
		{500, 1, "3f73f3cc2ee5761199f5b1ddf610b2d5b40bd0264b05e58e916404839482b550"},
	} {
		if got := hashWorld(t, Generate(Options{Users: c.users, Seed: c.seed, Profile: AcmeProfile()})); got != c.want {
			t.Errorf("acme users=%d seed=%d: hash %s, want %s", c.users, c.seed, got, c.want)
		}
	}
}

// The default tenant is Fjordvik. These are the hashes of what it makes now (2026-11-02, when it became the default): the same seed makes the same data.
func TestGenerate_DefaultMakesTheSameDataAsBefore(t *testing.T) {
	for _, c := range []golden{
		// replaced on purpose the same day for the rest of the cast (see the Acme test above)
		{10, 7, "b1e590be11ac0fb5790260589cd6a912a797647480caba125ae6337424d81fae"},
		{60, 1, "e2baa1117855affdea58576ff26de560c75fd1074eaaa2bdca17e11e37bf92c5"},
		{200, 5, "4aac13308558040f36844e8f650efc5edb8f81e17b209feed7748a0d463377c1"},
		{500, 1, "5839bb34992e01823cae70b09dcaefccaf3b84532cd91b7f92fa4721124f42c6"},
	} {
		if got := hashWorld(t, Generate(Options{Users: c.users, Seed: c.seed})); got != c.want {
			t.Errorf("default users=%d seed=%d: hash %s, want %s", c.users, c.seed, got, c.want)
		}
	}
}
