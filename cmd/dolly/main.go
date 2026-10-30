// Command dolly runs fake APIs with fake data (Microsoft Graph for now), and changes their data.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Settings come from flags. A flag that is not given takes its value from the
// environment (DOLLY_DIR, DOLLY_PORT, DOLLY_USERS, DOLLY_SEED, DOLLY_PAGE_SIZE).
type settings struct {
	root string // the data folder of all the APIs
	// dir is the data folder of the API of the subcommand: root/graph for 'dolly graph'.
	// It is empty when the data stays in memory.
	dir                   string
	port, users, pageSize int
	seed                  int64
	throttle              bool
	host                  string   // the address to listen on: 127.0.0.1 by default, 0.0.0.0 in a container
	allowHosts            []string // names (not loopback) that the admin API accepts in the Host header
	docs                  string   // a folder of documentation files, for a dev build (see below)
}

// version is "dev" for a build from source. A release build sets it:
//
//	go build -ldflags "-X main.version=1.2.3" ./cmd/dolly
//
// Only a dev build has the flag --docs.
var version = "dev"

func envInt[T int | int64](name string, def T) T {
	if v, err := strconv.ParseInt(os.Getenv(name), 10, 64); err == nil {
		return T(v)
	}
	return def
}

func envString(name, def string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	return def
}

// envList reads a list of names with commas, such as DOLLY_ALLOW_HOSTS=dolly,dolly.internal.
func envList(name string) []string {
	var out []string
	for _, p := range strings.Split(os.Getenv(name), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	var s settings
	root := &cobra.Command{
		Use:           "dolly",
		Short:         "Fake APIs with fake data, for development and demos: Microsoft Graph for now",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	f := root.PersistentFlags()
	if version == "dev" {
		// The documentation is in the program. A person who writes it can read a folder instead, and see a change
		// at the next reload of the page. A release build does not have this flag.
		f.StringVar(&s.docs, "docs", os.Getenv("DOLLY_DOCS"), "Dev builds only: read the markdown files of the API page from this folder, not from the program (env DOLLY_DOCS). For example graph/docs.")
	}
	f.StringVar(&s.root, "dir", os.Getenv("DOLLY_DIR"), "The folder of the data (env DOLLY_DIR). Default: dolly-data. Each API has its own folder in it. An empty value keeps the data in memory.")
	f.IntVar(&s.port, "port", envInt("DOLLY_PORT", 8650), "The port of the server (env DOLLY_PORT).")
	f.StringVar(&s.host, "host", envString("DOLLY_HOST", "127.0.0.1"), "The address to listen on (env DOLLY_HOST). 127.0.0.1 keeps Dolly on this machine. In a container use 0.0.0.0, and publish the port to 127.0.0.1 only (-p 127.0.0.1:8650:8650).")
	f.StringSliceVar(&s.allowHosts, "allow-host", envList("DOLLY_ALLOW_HOSTS"), "A host name that the admin API accepts, besides localhost and 127.0.0.1 (env DOLLY_ALLOW_HOSTS, with commas). For a service name in a compose file, such as dolly.")
	f.IntVar(&s.users, "users", envInt("DOLLY_USERS", 500), "How many users the seed data has (env DOLLY_USERS). It counts when Dolly makes the seed.")
	f.Int64Var(&s.seed, "seed", envInt[int64]("DOLLY_SEED", 1), "The seed of the fake people (env DOLLY_SEED). The same seed makes the same people.")
	f.IntVar(&s.pageSize, "page-size", envInt("DOLLY_PAGE_SIZE", 0), "Force the size of every page, whatever $top says (env DOLLY_PAGE_SIZE). 0 uses $top.")
	f.BoolVar(&s.throttle, "throttle", os.Getenv("DOLLY_THROTTLE") == "1", "Answer 429 when a service principal passes the limits of Graph (env DOLLY_THROTTLE=1). The cost of each call is counted in any case. A person can change it while Dolly runs.")
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed("dir") {
			if _, ok := os.LookupEnv("DOLLY_DIR"); !ok {
				s.root = "dolly-data"
			}
		}
		s.dir = ""
		if s.root != "" {
			s.dir = filepath.Join(s.root, "graph") // the commands below are for Graph
		}
		if s.port < 1 || s.port > 65535 {
			return fmt.Errorf("--port must be a whole number from 1 to 65535")
		}
		if s.users < 1 || s.users > 100000 {
			return fmt.Errorf("--users must be a whole number from 1 to 100000")
		}
		if s.pageSize < 0 || s.pageSize > 999 {
			return fmt.Errorf("--page-size must be a whole number from 0 to 999")
		}
		return nil
	}

	graph := &cobra.Command{
		Use:   "graph",
		Short: "The fake Microsoft Graph: run it, and change its data",
		Long: "Dolly Graph is a small server that answers Microsoft Graph requests from fake data.\n" +
			"Point your own code at it. It never talks to a real tenant.\n\n" +
			"The data is kept in the folder graph in --dir. The file seed.json is the data that Dolly made on\n" +
			"its first start. The file state.jsonl has every change since, with the time and who made it.\n" +
			"'dolly graph history' shows it, and 'dolly graph rollback 1h' takes the data back one hour.\n" +
			"Delete the folder to start again. The folder has one server at a time.",
	}
	graph.AddCommand(serveCmd(&s, "graph"), showCmd(&s), renameCmd(&s), moveCmd(&s), resetCmd(&s),
		historyCmd(&s), rollbackCmd(&s), generateCmd(&s))
	root.AddCommand(serveCmd(&s, ""), graph, healthCmd(&s))

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "dolly:", err)
		os.Exit(1)
	}
}
