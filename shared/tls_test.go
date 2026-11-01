package shared

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The certificate is good for localhost and the loopback addresses and the hosts that are given, a local CA signs it, and the CA is
// the same at the next start (so a person trusts it once).
func TestLocalCert(t *testing.T) {
	dir := t.TempDir()
	cert, ca1, err := LocalCert(dir, []string{"dolly", "10.1.2.3", "0.0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca1) {
		t.Fatal("the CA is not PEM")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "dolly", "10.1.2.3"} {
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: host}); err != nil {
			t.Errorf("%s: %v", host, err)
		}
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "example.com"}); err == nil {
		t.Error("a host that was not given must fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "ca-key.pem")); err != nil {
		t.Error("the key of the CA is kept:", err)
	}

	_, ca2, err := LocalCert(dir, nil)
	if err != nil || string(ca1) != string(ca2) {
		t.Errorf("the CA must stay the same at the next start: %v", err)
	}
	if _, ca3, _ := LocalCert("", nil); string(ca3) == string(ca1) {
		t.Error("with no folder the CA is new")
	}
}

func TestScheme(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if Scheme(r) != "http" {
		t.Error("plain request")
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(Scheme(r))) }))
	defer srv.Close()
	res, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b := make([]byte, 5)
	n, _ := res.Body.Read(b)
	if string(b[:n]) != "https" {
		t.Errorf("TLS request: %q", b[:n])
	}
}
