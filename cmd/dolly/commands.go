package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/withholm/dolly-cli/graph"
)

func generateCmd(s *settings) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"seed"},
		Short:   "Make new fake data, in place of the data that is there.",
		Long: "Make new fake data, in place of the data that is there: a new seed.json and an empty history.\n" +
			"The old data and every change in it are deleted, and a rollback cannot bring them back, so\n" +
			"the command asks first. Use --force (-f) to go on without asking. Use --users and --seed to\n" +
			"make another world. The same seed always makes the same data.\n\n" +
			"Stop 'dolly graph serve' first: the folder has one server at a time.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if s.dir == "" {
				return errors.New("generate needs a folder: give --dir")
			}
			return generate(os.Stdin, os.Stdout, s, force)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Replace the data without asking")
	return cmd
}

// generate replaces the data in the folder with new data. When there is data, it
// shows what is lost and asks (unless force). It asks in a terminal only.
func generate(in *os.File, out io.Writer, s *settings, force bool) error {
	if _, err := os.Stat(s.dir + "/seed.json"); err == nil && !force {
		old, err := graph.Open(s.dir, graph.OpenOptions{})
		if err != nil {
			return err
		}
		state, changes := old.Snapshot(), len(old.History())
		_ = old.Close()
		fmt.Fprintf(out, "The folder %s has data: %d users, %d groups, and %d changes in the history.\n", s.dir, state.Users, state.Groups, changes)
		if !term.IsTerminal(int(in.Fd())) {
			return errors.New("generate deletes this data, and there is no terminal to ask in. Give --force to go on")
		}
		fmt.Fprintf(out, "Delete it for good, and make new data (%d users, seed %d)? [y/N] ", s.users, s.seed)
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Fprintln(out, "Nothing was changed.")
			return nil
		}
	}
	st, err := graph.Replace(s.dir, graph.OpenOptions{Users: s.users, Seed: s.seed, Profile: s.profile})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Made new data in %s: %d users, %d groups, %d service principals. The history is empty.\n", s.dir, st.Users, st.Groups, st.ServicePrincipals)
	return nil
}

// ---- commands for a running server ----

// address is the address of the server that the commands talk to: the one that
// runs on the data folder, or else the port.
func address(s *settings) string {
	if s.dir != "" {
		if url, ok := graph.Running(s.dir); ok {
			return url
		}
	}
	if s.https() {
		return fmt.Sprintf("https://127.0.0.1:%d", s.port)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

// localClient talks to the dolly on this machine. With --tls its certificate is signed by the local CA of dollycli (tls/ca.pem in
// the data folder), which this machine may not trust yet, so the client checks the certificate against that file as well.
func localClient(s *settings) *http.Client {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if s.root != "" {
		if ca, err := os.ReadFile(filepath.Join(s.root, "tls", "ca.pem")); err == nil {
			pool.AppendCertsFromPEM(ca)
		}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
}

// healthCmd checks that a dolly answers. A container runs it as its health check: the image has no shell, no curl.
func healthCmd(s *settings) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check that a running dolly answers (for the health check of a container)",
		Long: "Ask the running dolly for the list of its APIs. It prints ok and exits with 0 when dolly answers, and\n" +
			"exits with 1 when it does not. It looks for dolly at --dir (the lock file has the address), or at 127.0.0.1:PORT.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
			defer cancel()
			base := address(s)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/_dolly/apis", nil)
			if err != nil {
				return err
			}
			res, err := localClient(s).Do(req)
			if err != nil {
				return fmt.Errorf("dolly does not answer at %s: %w", base, err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("dolly at %s answered %d", base, res.StatusCode)
			}
			fmt.Println("ok")
			return nil
		},
	}
}

// adminCall sends one request to the admin API of the server, and decodes the answer into out.
func adminCall(ctx context.Context, s *settings, method, path string, body, out any) error {
	base := address(s)
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+"/_dolly/graph"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := localClient(s).Do(req)
	if err != nil {
		return fmt.Errorf("dolly cannot reach the server at %s: %w. Run 'dolly graph serve' first", base, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		var e struct{ Error struct{ Message string } }
		if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
			return fmt.Errorf("the server said: %s", e.Error.Message)
		}
		return fmt.Errorf("the server answered with status %d", res.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func printState(st graph.State) {
	fmt.Printf("Users %d, groups %d, service principals %d (seed %d)\n", st.Users, st.Groups, st.ServicePrincipals, st.Seed)
	for _, set := range []struct {
		name string
		vals []graph.ValueCount
	}{{"Departments", st.Departments}, {"Job titles", st.JobTitles}, {"Offices", st.Offices}} {
		fmt.Printf("\n%s\n", set.name)
		for _, v := range set.vals {
			fmt.Printf("  %-34s %d\n", v.Value, v.Count)
		}
	}
}

func showCmd(s *settings) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show what the fake data has: counts, departments, job titles, offices",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var st graph.State
			if err := adminCall(cmd.Context(), s, http.MethodGet, "/state", nil, &st); err != nil {
				return err
			}
			printState(st)
			return nil
		},
	}
}

// fieldNames are the short names for the fields that people change most.
var fieldNames = map[string]string{
	"department": "department", "title": "jobTitle", "jobtitle": "jobTitle", "office": "officeLocation",
	"officelocation": "officeLocation", "city": "city", "country": "country", "company": "companyName",
}

func resolveField(name string) string {
	if f, ok := fieldNames[strings.ToLower(name)]; ok {
		return f
	}
	return name // a Graph name, such as usageLocation
}

func renameCmd(s *settings) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <field> <from> <to>",
		Short: "Change a value for every user that has it",
		Long: "Change a value for every user that has it. The field is department, title, office, city,\n" +
			"country, company, or a Graph name such as usageLocation.",
		Example: "  dolly graph rename department Sales Sales-Global",
		Args:    cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			var out struct{ Changed int }
			body := map[string]string{"field": resolveField(args[0]), "from": args[1], "to": args[2]}
			if err := adminCall(cmd.Context(), s, http.MethodPost, "/rename", body, &out); err != nil {
				return err
			}
			fmt.Printf("Changed %d users: %s %q is now %q\n", out.Changed, body["field"], args[1], args[2])
			return nil
		},
	}
}

func moveCmd(s *settings) *cobra.Command {
	var department, title, office string
	cmd := &cobra.Command{
		Use:   "move <user>",
		Short: "Change the department, title, or office of one user",
		Long:  "Change the department, title, or office of one user. The user is a user principal name or an object ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set := map[string]any{}
			for field, v := range map[string]string{"department": department, "jobTitle": title, "officeLocation": office} {
				if v != "" {
					set[field] = v
				}
			}
			if len(set) == 0 {
				return errors.New("give at least one of --department, --title, --office")
			}
			if err := adminCall(cmd.Context(), s, http.MethodPost, "/user", map[string]any{"user": args[0], "set": set}, nil); err != nil {
				return err
			}
			fmt.Printf("Changed %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&department, "department", "", "The new department")
	cmd.Flags().StringVar(&title, "title", "", "The new job title")
	cmd.Flags().StringVar(&office, "office", "", "The new office location")
	return cmd
}

func resetCmd(s *settings) *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Go back to the seed data. Every change stays in the history.",
		Long: "Go back to the seed data, with none of the changes. The changes stay in the history, and\n" +
			"'dolly graph rollback --before N' can bring them back.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var st graph.State
			if err := adminCall(cmd.Context(), s, http.MethodPost, "/reset", nil, &st); err != nil {
				return err
			}
			fmt.Printf("The data is the seed again: %d users, %d groups.\n", st.Users, st.Groups)
			return nil
		},
	}
}

// who says in a few words who made a change.
func who(a graph.Actor) string {
	if a.Source == "graph" {
		if a.Agent != "" {
			return "tool " + a.Agent
		}
		return "tool"
	}
	return "admin"
}

func historyCmd(s *settings) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show every change of the data: when, who, and what",
		Long: "Show every change of the data, oldest first: the number, the time, who made it, and what it did.\n" +
			"A change that a tool made through Graph names the tool. Use the number with\n" +
			"'dolly graph rollback --before N'.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var out struct {
				Events []graph.HistoryEntry `json:"events"`
				Total  int                  `json:"total"`
				Kept   bool                 `json:"kept"`
			}
			if err := adminCall(cmd.Context(), s, http.MethodGet, fmt.Sprintf("/history?limit=%d", limit), nil, &out); err != nil {
				return err
			}
			if !out.Kept {
				fmt.Println("This server keeps no history: its data is only in memory.")
				return nil
			}
			if out.Total == 0 {
				fmt.Println("No change yet. The data is the seed.")
				return nil
			}
			if out.Total > len(out.Events) {
				fmt.Printf("The last %d of %d changes:\n", len(out.Events), out.Total)
			}
			for i := len(out.Events) - 1; i >= 0; i-- { // oldest first
				e := out.Events[i]
				fmt.Printf("%5d  %s  %-30s  %s\n", e.Seq, e.Time.Local().Format("2006-01-02 15:04:05"), who(e.Actor), e.Text)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "The most changes to show (the newest)")
	return cmd
}

func rollbackCmd(s *settings) *cobra.Command {
	var before int
	var toSeed bool
	cmd := &cobra.Command{
		Use:   "rollback [1h | 2026-10-10T08:00:00Z]",
		Short: "Take the data back to an earlier time",
		Long: "Take the data back to how it was at an earlier time. Give a time span (30m, 2h, 36h), or a\n" +
			"time in the form 2026-10-10T08:00:00Z. Or use --before N to go back to before change N\n" +
			"(see 'dolly graph history'), or --seed to go back to the seed.\n\n" +
			"A rollback is a change too. It is in the history, so it can be undone: roll back to\n" +
			"before the rollback.",
		Example: "  dolly graph rollback 1h\n  dolly graph rollback --before 17",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			switch {
			case toSeed:
				body["seed"] = true
			case before > 0:
				body["before"] = before
			case len(args) == 1:
				if _, err := time.ParseDuration(args[0]); err == nil {
					body["ago"] = args[0]
				} else {
					body["to"] = args[0]
				}
			default:
				return errors.New("say where to go: a time span like 1h, a time, --before N, or --seed")
			}
			var out struct {
				Target int `json:"target"`
				State  graph.State
			}
			if err := adminCall(cmd.Context(), s, http.MethodPost, "/rollback", body, &out); err != nil {
				return err
			}
			if out.Target == 0 {
				fmt.Printf("The data is the seed again: %d users.\n", out.State.Users)
			} else {
				fmt.Printf("The data is as it was after change %d: %d users.\n", out.Target, out.State.Users)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&before, "before", 0, "Go back to before this change number")
	cmd.Flags().BoolVar(&toSeed, "seed", false, "Go back to the seed")
	return cmd
}
