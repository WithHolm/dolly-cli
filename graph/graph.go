package graph

import "github.com/withholm/dolly-cli/graph/repo"

// The data of dolly is in the repo package. These names are how the server, the
// CLI, and the tests reach it.

type (
	// World is the data: users, groups, service principals, and the history of changes.
	World = repo.World
	// OpenOptions sets how a new world is made.
	OpenOptions = repo.OpenOptions
	// Options sets the size of a generated world.
	Options = repo.Options
	// Event is one change in the history.
	Event = repo.Event
	// State is a short account of the world.
	State = repo.State
	// ValueCount is a value and how many users have it.
	ValueCount = repo.ValueCount
	// UserInfo is a user in a list of the page.
	UserInfo = repo.UserInfo
	// Profile says what a generated tenant looks like (see repo.Profile).
	Profile = repo.Profile
	// ProfilePreview is what a profile would make.
	ProfilePreview = repo.ProfilePreview
)

// DefaultProfile is the tenant that dollycli makes when it is given no profile.
func DefaultProfile() *Profile { return repo.DefaultProfile() }

// ParseProfile reads a profile from JSON, and checks it.
var ParseProfile = repo.ParseProfile

// PreviewProfile makes a world from a profile in memory, and says what it has.
func PreviewProfile(p *Profile, users int, seed int64) (*ProfilePreview, error) {
	return repo.PreviewProfile(p, users, seed)
}

// Departments and Offices are the fixed sets of the generated data.
var (
	Departments = repo.Departments
	Offices     = repo.Offices
)

// The flags of a service principal (see repo.SetServicePrincipalFlag).
const (
	FlagExemptFromThrottle = repo.FlagExemptFromThrottle
	FlagHideCalls          = repo.FlagHideCalls
)

// PortalAppID is the client ID of the service principal of the portal.
var PortalAppID = repo.PortalAppID

// FakeTenantID is the ID of the fake tenant.
const FakeTenantID = repo.FakeTenantID

// Generate makes a world in memory.
func Generate(o Options) *World { return repo.Generate(o) }

// NewMemory makes a world that is only in memory, with no history and no files.
func NewMemory(o Options) *World { return repo.NewMemory(o) }

// Open opens the data folder, and makes it when it is not there.
func Open(dir string, o OpenOptions) (*World, error) { return repo.Open(dir, o) }

// Replace makes a new world in the data folder, in place of the old one and its history.
func Replace(dir string, o OpenOptions) (State, error) { return repo.Replace(dir, o) }

// Running says where a dolly runs on this folder, if one does.
func Running(dir string) (string, bool) { return repo.Running(dir) }
