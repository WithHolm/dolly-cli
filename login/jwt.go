package login

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

// The key that signs the tokens. It is fake: it is made for this Dolly, and a folder of data
// keeps it (key.pem), so a token is still good after a restart. Without a folder it lives in memory.

func loadKey(dir string) (*rsa.PrivateKey, error) {
	if dir == "" {
		return rsa.GenerateKey(rand.Reader, 2048)
	}
	path := filepath.Join(dir, "key.pem")
	if b, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(b)
		if block == nil {
			return nil, fmt.Errorf("%s is not a PEM file", path)
		}
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%s is not an RSA key", path)
		}
		return rk, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return k, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600)
}

// keyID is the kid of a key: the start of the hash of its public key.
func keyID(k *rsa.PrivateKey) string {
	sum := sha256.Sum256(k.N.Bytes())
	return base64.RawURLEncoding.EncodeToString(sum[:])[:22]
}

var b64 = base64.RawURLEncoding

// sign makes a JWT (RS256) of the claims.
func (a *API) sign(claims map[string]any) (string, error) {
	h, _ := json.Marshal(map[string]string{"typ": "JWT", "alg": "RS256", "kid": a.kid})
	p, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signing := b64.EncodeToString(h) + "." + b64.EncodeToString(p)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64.EncodeToString(sig), nil
}

// jwtParts splits a JWT and reads its header and payload. It does not check the signature.
func jwtParts(tok string) (header, payload map[string]any, ok bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, nil, false
	}
	for i, dst := range []*map[string]any{&header, &payload} {
		raw, err := b64.DecodeString(parts[i])
		if err != nil || json.Unmarshal(raw, dst) != nil {
			return nil, nil, false
		}
	}
	return header, payload, true
}

// checkSignature reports whether the key of this Dolly signed the JWT.
func (a *API) checkSignature(tok string) bool {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return false
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return false
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	return rsa.VerifyPKCS1v15(&a.key.PublicKey, crypto.SHA256, sum[:], sig) == nil
}

// jwks is the JSON Web Key Set that a client reads to check the tokens.
func (a *API) jwks() map[string]any {
	pub := a.key.PublicKey
	return map[string]any{"keys": []map[string]any{{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": a.kid,
		"n": b64.EncodeToString(pub.N.Bytes()),
		"e": b64.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
}
