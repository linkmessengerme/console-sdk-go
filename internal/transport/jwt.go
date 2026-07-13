package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
)

// signEdDSAJWT assembles a compact JWS (EdDSA; no header kid needed — the client
// assertion is verified against the client's registered public key).
// Zero-dep: header.payload.signature is assembled by hand instead of pulling in a JWT library.
func signEdDSAJWT(priv ed25519.PrivateKey, claims map[string]any) (string, error) {
	header := b64url([]byte(`{"alg":"EdDSA","typ":"JWT"}`))
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := header + "." + b64url(payloadJSON)
	sig := ed25519.Sign(priv, []byte(signingInput))
	return signingInput + "." + b64url(sig), nil
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func randomJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b64url(b), nil
}
