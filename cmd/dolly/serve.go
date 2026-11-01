package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/permissions"
	"github.com/withholm/dolly-cli/login"
	"github.com/withholm/dolly-cli/shared"
	"github.com/withholm/dolly-cli/ui"
)

// apiDef is one API that the CLI can run. To add an API, add a line to apis.
type apiDef struct {
	name string
	// open opens the data of the API and gives the API, and a function that closes the data. The
	// APIs that are open already are given too, for an API that works on their data.
	open func(s *settings, running []shared.API) (shared.API, func(), error)
}

var apis = []apiDef{{"graph", openGraph}, {"login", openLogin}}

// openLogin opens the sign-in API on the data of the Graph API, and makes Graph check its tokens.
func openLogin(s *settings, running []shared.API) (shared.API, func(), error) {
	for _, r := range running {
		if g, ok := r.(*graph.Server); ok {
			dir := ""
			if s.root != "" {
				dir = filepath.Join(s.root, "login")
			}
			l, err := login.New(g.World(), login.Options{Dir: dir, CORS: shared.CORS(s.allowHosts)})
			if err != nil {
				return nil, nil, err
			}
			g.SetTokenVerifier(l.Verify)
			return l, func() {}, nil
		}
	}
	return nil, nil, errors.New("the sign-in API needs the Graph API")
}

// useTenantPermissions adds the permissions that the person downloaded from their own tenant (docs/permissions.md): the file of --permissions, or
// permissions/graph.json in the data folder when it is there. It stops with a message when the file cannot be read: a catalog with a part missing
// would hide that.
func useTenantPermissions(s *settings) error {
	path := s.permissionsPath
	if path == "" && s.root != "" {
		if p := filepath.Join(s.root, "permissions", "graph.json"); fileExists(p) {
			path = p
		}
	}
	permissions.SetExtra(path)
	c, err := permissions.Load()
	if err != nil {
		return err
	}
	if c.Extra != nil {
		slog.Info("Dolly adds the permissions of your tenant.", "file", c.Extra.Path, "delegated", c.Extra.Delegated, "application", c.Extra.Application, "written", c.Extra.Date.Format(time.DateOnly))
	}
	return nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func openGraph(s *settings, _ []shared.API) (shared.API, func(), error) {
	if err := useTenantPermissions(s); err != nil {
		return nil, nil, err
	}
	var world *graph.World
	if s.dir == "" {
		world = graph.NewMemory(graph.Options{Users: s.users, Seed: s.seed, Profile: s.profile})
	} else {
		var err error
		if world, err = graph.Open(s.dir, graph.OpenOptions{Users: s.users, Seed: s.seed, Profile: s.profile}); err != nil {
			return nil, nil, err
		}
		slog.Info("Dolly keeps its data here.", "api", "graph", "folder", s.dir, "changes", len(world.History()))
	}
	opt := graph.ServerOptions{PageSize: s.pageSize, Throttle: s.throttle, UsageHistory: s.usageHistory}
	if s.docs != "" {
		if _, err := os.Stat(s.docs); err != nil {
			_ = world.Close()
			return nil, nil, fmt.Errorf("--docs: %w", err)
		}
		opt.Docs = os.DirFS(s.docs)
		slog.Info("Dolly reads the documentation from this folder, and not from the program.", "folder", s.docs)
	}
	return graph.New(world, opt), func() { _ = world.Close() }, nil
}

// reachable gives host:port for a listener, as a client on the same machine reaches it. An address that listens on every
// interface (0.0.0.0 or ::) is reached at 127.0.0.1: the lock file and the commands of dolly use it to find the server.
func reachable(a net.Addr) string {
	tcp, ok := a.(*net.TCPAddr)
	if !ok {
		return a.String()
	}
	if tcp.IP.IsUnspecified() {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(tcp.Port))
	}
	return tcp.String()
}

// isLoopback says whether an address to listen on is this machine only.
func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// tlsConfig gives the TLS settings for --tls or --tls-cert, and the CA (PEM) that signed the certificate, if dollycli made it.
func tlsConfig(s *settings) (*tls.Config, []byte, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}
	if s.tlsCert != "" {
		cert, err := tls.LoadX509KeyPair(s.tlsCert, s.tlsKey)
		if err != nil {
			return nil, nil, fmt.Errorf("--tls-cert: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
		return cfg, nil, nil
	}
	dir := ""
	if s.root != "" {
		dir = filepath.Join(s.root, "tls")
	}
	cert, ca, err := shared.LocalCert(dir, append([]string{s.host}, s.allowHosts...))
	if err != nil {
		return nil, nil, fmt.Errorf("--tls: %w", err)
	}
	cfg.Certificates = []tls.Certificate{cert}
	if dir != "" {
		slog.Info("Dolly serves https. Trust this file once, and a client trusts it.", "ca", filepath.Join(dir, "ca.pem"))
	} else {
		slog.Warn("Dolly serves https with a CA in memory (no data folder). A new CA is made at each start, so nothing can trust it.")
	}
	return cfg, ca, nil
}

// serveCmd runs the APIs on one address. With only set, it runs that API alone.
func serveCmd(s *settings, only string) *cobra.Command {
	short, long := "Run all the fake APIs", "Run all the fake APIs on one address, HOST:PORT (127.0.0.1:8650 as it comes). Any token works.\n\n"
	if only != "" {
		short, long = "Run the fake "+only, "Run the fake "+only+" on HOST:PORT (127.0.0.1:8650 as it comes). Any token works.\n\n"
	}
	return &cobra.Command{
		Use:   "serve",
		Short: short,
		Long: long +
			"The Graph address is http://127.0.0.1:PORT/v1.0. The address / has the status page, with a tab\n" +
			"for each API: every call a tool makes, live, and the data, the history, and the API. The data is\n" +
			"kept in the folder --dir, one folder for each API: seed.json, and state.jsonl with every change.\n" +
			"With --page-size, every page has that many items. A small number makes a tool follow\n" +
			"the next-page links often, which is a way to test paging.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var running []shared.API
			for _, d := range apis {
				if only != "" && d.name != only {
					continue
				}
				api, closeData, err := d.open(s, running)
				if err != nil {
					return err
				}
				defer closeData()
				running = append(running, api)
			}
			ln, err := net.Listen("tcp", net.JoinHostPort(s.host, strconv.Itoa(s.port)))
			if err != nil {
				return fmt.Errorf("dolly cannot listen on %s: %w. Use --port to pick another port, or --host to pick another address", net.JoinHostPort(s.host, strconv.Itoa(s.port)), err)
			}
			scheme := "http"
			var caPEM []byte
			if s.https() {
				scheme = "https"
				cfg, ca, err := tlsConfig(s)
				if err != nil {
					_ = ln.Close()
					return err
				}
				caPEM = ca
				ln = tls.NewListener(ln, cfg)
			}
			addr := scheme + "://" + reachable(ln.Addr())
			if !isLoopback(s.host) {
				slog.Warn("Dolly listens on an address that other machines can reach. Anyone who can reach it can read and change the data.",
					"host", s.host, "adminHosts", append([]string{"localhost", "127.0.0.1"}, s.allowHosts...))
			}
			for _, a := range running {
				if g, ok := a.(*graph.Server); ok {
					g.SetGraphURL(addr + graph.BasePath) // the lock file says where this server listens
					st := g.World().Snapshot()
					slog.Info("Dolly Graph.", "graph", addr+graph.BasePath, "users", st.Users, "groups", st.Groups, "servicePrincipals", st.ServicePrincipals)
				}
			}
			router := shared.RouterWith(shared.Options{AllowedHosts: s.allowHosts}, ui.Handler(), running...)
			hs := &http.Server{Handler: router, ReadHeaderTimeout: 10 * time.Second}
			if caPEM != nil {
				// the CA is public: a browser or a script can fetch it from here, to trust it
				hs.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/dolly-ca.pem" && r.Method == http.MethodGet {
						w.Header().Set("Content-Type", "application/x-pem-file")
						w.Header().Set("Content-Disposition", `attachment; filename="dollycli-ca.pem"`)
						_, _ = w.Write(caPEM)
						return
					}
					router.ServeHTTP(w, r)
				})
			}

			// Ctrl+C is os.Interrupt. 'docker stop' sends SIGTERM: without this the data is not closed and the lock stays.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			go func() { <-ctx.Done(); _ = hs.Shutdown(context.Background()) }()

			slog.Info("Dolly is running. Press Ctrl+C to stop.", "status page", addr+"/")
			if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
}
