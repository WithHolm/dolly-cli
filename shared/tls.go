package shared

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scheme is http, or https when the request came in over TLS. The addresses that an API writes in its answers (the issuer of a token,
// the link to the next page) start with it.
func Scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

const (
	caLife   = 365 * 24 * time.Hour // the CA is made again when it has less than caRenew left
	caRenew  = 30 * 24 * time.Hour
	leafLife = 90 * 24 * time.Hour // a new server certificate is made at each start
)

// LocalCert makes the certificate that dollycli serves with --tls. A local CA signs it, and the CA is kept in dir (ca.pem and
// ca-key.pem), so a person trusts it once and it is still good at the next start. With no dir the CA lives in memory, and nothing
// can trust it. The certificate is good for localhost, the loopback addresses, and the hosts that are given (names or addresses).
//
// The CA is **name-constrained**: it can sign for localhost, the loopback addresses, and the hosts that were given when it was made, and for
// nothing else. A client that trusts it still refuses a certificate that it signs for a real domain, so a leak of its key cannot be used to
// intercept another site. When a host is given that the CA does not allow (a new --allow-host), or the CA is not constrained (made by an
// older version), or it has less than 30 days left, a new CA is made, and the person trusts the new ca.pem.
// It gives the certificate, and the CA as PEM.
func LocalCert(dir string, hosts []string) (tls.Certificate, []byte, error) {
	names, ips := hostsOf(hosts)
	for _, n := range names {
		if LooksPublic(n) {
			slog.Warn("A host for --tls looks like a real domain. The CA can sign for every host that is given, so do not give it a domain that is not yours to impersonate.", "host", n)
		}
	}
	ca, caKey, caPEM, err := loadCA(dir, names, ips)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	serial, err := newSerial()
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	notAfter := time.Now().Add(leafLife)
	if notAfter.After(ca.NotAfter) {
		notAfter = ca.NotAfter
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "dollycli", Organization: []string{"dollycli"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     append([]string{"localhost"}, names...),
		IPAddresses:  append([]net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}, ips...),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, caPEM, nil
}

// localSuffixes are the endings of names that are not on the internet: a name with a dot that does not end in one of them looks like a real domain.
var localSuffixes = []string{".localhost", ".local", ".internal", ".test", ".example", ".invalid", ".lan", ".home.arpa", ".localdomain", ".corp", ".home", ".intranet", ".private"}

// LooksPublic says whether a host name looks like a real domain (it has a dot, and does not end in a name that is kept for local use). A name with
// no dot (a service name in a compose file) does not. The CA can sign for every name that is given, so dollycli warns about such a name.
func LooksPublic(name string) bool {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if !strings.Contains(name, ".") || net.ParseIP(name) != nil {
		return false
	}
	for _, s := range localSuffixes {
		if strings.HasSuffix(name, s) {
			return false
		}
	}
	return true
}

// hostsOf splits the hosts that are given into names and addresses. An empty one and an unspecified address (0.0.0.0) are left out.
func hostsOf(hosts []string) (names []string, ips []net.IP) {
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || h == "localhost" {
			continue
		}
		if ip := net.ParseIP(h); ip != nil {
			if !ip.IsUnspecified() && !ip.IsLoopback() {
				ips = append(ips, ip)
			}
		} else {
			names = append(names, h)
		}
	}
	return names, ips
}

// permits says whether the name constraints of the CA allow a name or an address.
func permits(ca *x509.Certificate, names []string, ips []net.IP) bool {
	for _, n := range names {
		ok := false
		for _, c := range ca.PermittedDNSDomains {
			if n == c || strings.HasSuffix(n, "."+c) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	for _, ip := range ips {
		ok := false
		for _, r := range ca.PermittedIPRanges {
			if r.Contains(ip) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// usable says whether a CA that was read is good to use again for these hosts, and why not.
func usable(ca *x509.Certificate, names []string, ips []net.IP) (bool, string) {
	switch {
	case !ca.PermittedDNSDomainsCritical || len(ca.PermittedDNSDomains) == 0:
		return false, "it can sign for any name (it was made by an older version, and has no name constraints)"
	case time.Until(ca.NotAfter) < caRenew:
		return false, "it ends within 30 days"
	case !permits(ca, names, ips):
		return false, "a host was given that it is not allowed to sign for"
	}
	return true, ""
}

// loadCA reads the CA from dir, or makes it (and keeps it, when there is a dir).
func loadCA(dir string, names []string, ips []net.IP) (*x509.Certificate, *ecdsa.PrivateKey, []byte, error) {
	if dir != "" {
		certPEM, err1 := os.ReadFile(filepath.Join(dir, "ca.pem"))
		keyPEM, err2 := os.ReadFile(filepath.Join(dir, "ca-key.pem"))
		if err1 == nil && err2 == nil {
			cb, _ := pem.Decode(certPEM)
			kb, _ := pem.Decode(keyPEM)
			if cb == nil || kb == nil {
				return nil, nil, nil, fmt.Errorf("%s: ca.pem and ca-key.pem must be PEM files", dir)
			}
			ca, err := x509.ParseCertificate(cb.Bytes)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("%s/ca.pem: %w", dir, err)
			}
			k, err := x509.ParseECPrivateKey(kb.Bytes)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("%s/ca-key.pem: %w", dir, err)
			}
			ok, why := usable(ca, names, ips)
			if ok {
				// a key that anyone can read is no key: a folder from before this check, or one that was copied
				_ = os.Chmod(dir, 0o700)
				_ = os.Chmod(filepath.Join(dir, "ca-key.pem"), 0o600)
				return ca, k, certPEM, nil
			}
			slog.Warn("dollycli makes a new CA for --tls: the old one is no good now. Trust the new ca.pem, and take the old CA out of your trust store ('dolly tls remove' says how).",
				"why", why, "folder", dir)
		} else if err1 != nil && !errors.Is(err1, os.ErrNotExist) {
			return nil, nil, nil, err1
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	serial, err := newSerial()
	if err != nil {
		return nil, nil, nil, err
	}
	// what the CA may sign for: localhost, the loopback addresses, and the hosts given now
	_, loopback4, _ := net.ParseCIDR("127.0.0.0/8")
	_, loopback6, _ := net.ParseCIDR("::1/128")
	permittedIPs := []*net.IPNet{loopback4, loopback6}
	for _, ip := range ips {
		bits := 8 * len(ip)
		if v4 := ip.To4(); v4 != nil {
			ip, bits = v4, 32
		}
		permittedIPs = append(permittedIPs, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	tmpl := &x509.Certificate{
		SerialNumber:                serial,
		Subject:                     pkix.Name{CommonName: "dollycli local CA", Organization: []string{"dollycli"}},
		NotBefore:                   time.Now().Add(-time.Hour),
		NotAfter:                    time.Now().Add(caLife),
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid:       true,
		IsCA:                        true,
		MaxPathLenZero:              true,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         append([]string{"localhost"}, names...),
		PermittedIPRanges:           permittedIPs,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if dir != "" {
		keyDER, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, nil, nil, err
		}
		_ = os.Chmod(dir, 0o700)
		if err := os.WriteFile(filepath.Join(dir, "ca-key.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
			return nil, nil, nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, "ca.pem"), certPEM, 0o644); err != nil {
			return nil, nil, nil, err
		}
	}
	return ca, key, certPEM, nil
}

// RemoveLocalCA deletes the CA and the key from dir (the folder is the one that LocalCert was given). It says whether there was anything to delete.
// It cannot take the CA out of the trust store of the system or of a browser: that is the person's, and 'dolly tls remove' says how.
func RemoveLocalCA(dir string) (bool, error) {
	if dir == "" {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(dir, "ca.pem")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	for _, f := range []string{"ca-key.pem", "ca.pem"} {
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	_ = os.Remove(dir) // only when it is empty
	return true, nil
}

func newSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
}
