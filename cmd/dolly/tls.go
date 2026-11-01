package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/withholm/dolly-cli/shared"
)

// tlsCmd is 'dolly tls': the commands for the CA that --tls makes.
func tlsCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tls",
		Short: "The CA that --tls makes",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "remove",
		Short: "Delete the CA and its key, and say how to stop trusting it",
		Long: "Deletes the CA and its private key from the data folder (tls/ca.pem and tls/ca-key.pem). The next 'dolly serve --tls' makes a new CA.\n" +
			"dollycli never put the CA in the trust store of your system or of a browser, so it cannot take it out: the command prints how.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if s.root == "" {
				return errors.New("there is no data folder (--dir is empty): with no folder the CA is in memory, and nothing is kept")
			}
			dir := filepath.Join(s.root, "tls")
			removed, err := shared.RemoveLocalCA(dir)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if removed {
				fmt.Fprintf(out, "Deleted the CA and its key in %s.\n\n", dir)
			} else {
				fmt.Fprintf(out, "There is no CA in %s.\n\n", dir)
			}
			fmt.Fprintln(out, trustRemoval)
			return nil
		},
	})
	return cmd
}

// trustRemoval says how to take the CA out of the places where a person put it. The name of the CA is "dollycli local CA". These commands are the
// usual ones, and they are not run for this text: check them for your system.
const trustRemoval = `If you trusted the CA, take it out of the places where you put it. Its name is "dollycli local CA".

  Debian, Ubuntu   sudo rm /usr/local/share/ca-certificates/dollycli-ca.crt && sudo update-ca-certificates --fresh
  Fedora, RHEL     sudo rm /etc/pki/ca-trust/source/anchors/dollycli-ca.pem && sudo update-ca-trust
  macOS            sudo security delete-certificate -c "dollycli local CA" /Library/Keychains/System.keychain
  Windows          certutil -delstore Root "dollycli local CA"     (an administrator shell; add -user for the store of one user)
  A browser        Settings, certificates, authorities: delete "dollycli local CA"
  A tool           stop setting NODE_EXTRA_CA_CERTS, REQUESTS_CA_BUNDLE, or --cacert to the old ca.pem`
