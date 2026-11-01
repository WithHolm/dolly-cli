package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/withholm/dolly-cli/graph"
)

// loadProfile reads a profile from a file, and checks it. The error says which field is wrong.
func loadProfile(path string) (*graph.Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("--profile: %w", err)
	}
	defer f.Close()
	p, err := graph.ParseProfile(f)
	if err != nil {
		return nil, fmt.Errorf("--profile %s: %w", path, err)
	}
	return p, nil
}

// profileCmd is 'dolly profile': the commands for a profile, which says what a generated tenant looks like.
func profileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Work with a profile of a tenant: print the default, or check a file",
		Long: "A profile says what a generated tenant looks like: the organization, the offices, the departments and their job titles,\n" +
			"the names, how many guests and synced users there are, the formats of ids and phone numbers, the extension attributes, the\n" +
			"groups, and how the admin account of the owner is named. Give it to dolly serve or dolly graph generate with --profile.",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "default",
		Short: "Print the default profile, as JSON. Change it, and give it back with --profile.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(graph.DefaultProfile())
		},
	}, &cobra.Command{
		Use:   "check <file>",
		Short: "Check a profile file, and say what it would make",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := loadProfile(args[0])
			if err != nil {
				return err
			}
			pv, err := graph.PreviewProfile(p, 500, 1)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "The profile is valid: %q.\n", p.Name)
			fmt.Fprintf(out, "With 500 users and seed 1 it makes %d users, %d groups, and %d service principals.\n", pv.Users, pv.Groups, pv.ServicePrincipals)
			fmt.Fprintf(out, "The domain is %s. The owner is %s, and the admin account is %s.\n", pv.Domain, pv.Owner, pv.Admin)
			fmt.Fprintf(out, "People: %d guests, %d synced, %d disabled. Offices: %d, departments: %d.\n", pv.Guests, pv.Synced, pv.Disabled, len(pv.Offices), len(pv.Departments))
			return nil
		},
	})
	return cmd
}
