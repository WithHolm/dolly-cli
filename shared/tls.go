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
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
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

// LocalCert makes the certificate that dollycli serves with --tls. A local CA signs it, and the CA is kept in dir (ca.pem and
// ca-key.pem), so a person trusts it once and it is still good at the next start. With no dir the CA lives in memory, and nothing
// can trust it. The certificate is good for localhost, the loopback addresses, and the hosts that are given (names or addresses).
// It gives the certificate, and the CA as PEM.
func LocalCert(dir string, hosts []string) (tls.Certificate, []byte, error) {
	ca, caKey, caPEM, err := loadCA(dir)
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
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "dollycli", Organization: []string{"dollycli"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(397 * 24 * time.Hour), // the longest life that browsers accept
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	for _, h := range hosts {
		if h == "" {
			continue
		}
		if ip := net.ParseIP(h); ip != nil {
			if !ip.IsUnspecified() {
				tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
			}
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, caPEM, nil
}

// loadCA reads the CA from dir, or makes it (and keeps it, when there is a dir).
func loadCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, []byte, error) {
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
			return ca, k, certPEM, nil
		} else if !errors.Is(err1, os.ErrNotExist) && err1 != nil {
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
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "dollycli local CA", Organization: []string{"dollycli"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
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
		if err := os.WriteFile(filepath.Join(dir, "ca-key.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
			return nil, nil, nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, "ca.pem"), certPEM, 0o644); err != nil {
			return nil, nil, nil, err
		}
	}
	return ca, key, certPEM, nil
}

func newSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
}
