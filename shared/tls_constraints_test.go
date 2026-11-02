package shared

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func readCA(t *testing.T, dir string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	certPEM, _ := os.ReadFile(filepath.Join(dir, "ca.pem"))
	keyPEM, _ := os.ReadFile(filepath.Join(dir, "ca-key.pem"))
	cb, _ := pem.Decode(certPEM)
	kb, _ := pem.Decode(keyPEM)
	ca, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.ParseECPrivateKey(kb.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return ca, key
}

// signFor makes a certificate for a host, signed with the CA and its key, as someone who got the key would.
func signFor(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, host string) *x509.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := newSerial()
	tmpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	return leaf
}

// A leak of the key of the CA cannot be used against another site: a client that trusts the CA refuses a certificate that it signs for a real domain.
func TestLocalCA_CanSignForLocalNamesOnly(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := LocalCert(dir, []string{"dolly", "10.1.2.3"}); err != nil {
		t.Fatal(err)
	}
	ca, key := readCA(t, dir)
	if !ca.PermittedDNSDomainsCritical || len(ca.PermittedDNSDomains) == 0 || len(ca.PermittedIPRanges) == 0 {
		t.Fatalf("the CA has no name constraints, or they are not critical: %+v", ca)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	for _, host := range []string{"localhost", "app.localhost", "dolly", "x.dolly", "127.0.0.1", "127.9.9.9", "::1", "10.1.2.3"} {
		if _, err := signFor(t, ca, key, host).Verify(x509.VerifyOptions{Roots: pool, DNSName: host}); err != nil {
			t.Errorf("%s must be allowed: %v", host, err)
		}
	}
	for _, host := range []string{"login.microsoftonline.com", "graph.microsoft.com", "example.com", "evil-localhost.com", "dolly.example.com", "8.8.8.8", "10.1.2.4"} {
		_, err := signFor(t, ca, key, host).Verify(x509.VerifyOptions{Roots: pool, DNSName: host})
		if err == nil {
			t.Errorf("%s: the CA signed for it, and a client accepts it", host)
		} else if !strings.Contains(err.Error(), "not permitted") && !strings.Contains(err.Error(), "constraint") {
			t.Logf("%s refused: %v", host, err) // refused for another reason: still refused
		}
	}
}

func TestLocalCA_LivesNotAsLongAsBefore(t *testing.T) {
	dir := t.TempDir()
	cert, _, err := LocalCert(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := readCA(t, dir)
	leaf, _ := x509.ParseCertificate(cert.Certificate[0])
	if life := time.Until(ca.NotAfter); life > 366*24*time.Hour || life < 360*24*time.Hour {
		t.Errorf("the CA is good for %s, and it should be a year", life)
	}
	if life := time.Until(leaf.NotAfter); life > 91*24*time.Hour || leaf.NotAfter.After(ca.NotAfter) {
		t.Errorf("the server certificate is good for %s: it must be 90 days at most, and not longer than the CA", life)
	}
}

// The CA stays the same while the hosts are ones that it may sign for. A new host makes a new CA, and so does a CA that is not constrained.
func TestLocalCA_IsMadeAgainWhenItMust(t *testing.T) {
	dir := t.TempDir()
	_, first, _ := LocalCert(dir, []string{"dolly.internal"})
	if _, same, _ := LocalCert(dir, []string{"dolly.internal", "127.0.0.1", "0.0.0.0", "localhost", "api.dolly.internal"}); string(first) != string(same) {
		t.Error("the same hosts, a host below one that it may sign for, and the loopback: the CA must stay")
	}
	cert, second, err := LocalCert(dir, []string{"dolly.internal", "other.example"})
	if err != nil || string(first) == string(second) {
		t.Fatalf("a new host needs a new CA: %v", err)
	}
	leaf, _ := x509.ParseCertificate(cert.Certificate[0])
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(second)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "other.example"}); err != nil {
		t.Errorf("the new CA signs for the new host: %v", err)
	}

	// a CA from before the constraints: no names are limited, so it is replaced
	old := t.TempDir()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "dollycli local CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true, IsCA: true}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	keyDER, _ := x509.MarshalECPrivateKey(key)
	oldPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	_ = os.WriteFile(filepath.Join(old, "ca.pem"), oldPEM, 0o644)
	_ = os.WriteFile(filepath.Join(old, "ca-key.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
	_, replaced, err := LocalCert(old, nil)
	if err != nil || string(replaced) == string(oldPEM) {
		t.Fatalf("a CA with no name constraints must be replaced: %v", err)
	}
	if ca, _ := readCA(t, old); !ca.PermittedDNSDomainsCritical {
		t.Error("the new CA has no constraints")
	}
}

func TestLocalCA_KeyIsNotReadableByOthers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no unix modes")
	}
	dir := filepath.Join(t.TempDir(), "tls")
	if _, _, err := LocalCert(dir, nil); err != nil {
		t.Fatal(err)
	}
	// a folder that was copied, with the modes of a download
	_ = os.Chmod(dir, 0o755)
	_ = os.Chmod(filepath.Join(dir, "ca-key.pem"), 0o644)
	if _, _, err := LocalCert(dir, nil); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, "ca-key.pem"): 0o600} {
		if st, _ := os.Stat(path); st.Mode().Perm() != want {
			t.Errorf("%s has mode %v, want %v", path, st.Mode().Perm(), want)
		}
	}
}

// The CA signs for the hosts that it is given, so a name that looks like a real domain gets a warning. A service name and a local name do not.
func TestLooksPublic(t *testing.T) {
	for name, want := range map[string]bool{
		"login.microsoftonline.com": true, "example.com": true, "Graph.Microsoft.com.": true, "dolly.example.co": true,
		"dolly": false, "dolly.internal": false, "api.dolly.local": false, "app.localhost": false, "box.lan": false, "x.home.arpa": false,
		"10.1.2.3": false, "::1": false, "": false,
	} {
		if got := LooksPublic(name); got != want {
			t.Errorf("%q: %v, want %v", name, got, want)
		}
	}
}

func TestRemoveLocalCA(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	if removed, err := RemoveLocalCA(dir); removed || err != nil {
		t.Errorf("nothing there: %v %v", removed, err)
	}
	_, _, _ = LocalCert(dir, nil)
	if removed, err := RemoveLocalCA(dir); !removed || err != nil {
		t.Fatalf("%v %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ca-key.pem")); !os.IsNotExist(err) {
		t.Error("the key is still there")
	}
	if removed, _ := RemoveLocalCA(""); removed {
		t.Error("an empty folder is the CA in memory: nothing to remove")
	}
}
