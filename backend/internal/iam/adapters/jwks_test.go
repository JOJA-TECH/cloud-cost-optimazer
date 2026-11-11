package adapters

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwksTestServer serves a JWKS document with the given RSA keys.
func jwksTestServer(t *testing.T, keys map[string]*rsa.PublicKey) *httptest.Server {
	t.Helper()
	doc := map[string]interface{}{"keys": []interface{}{}}
	list := doc["keys"].([]interface{})
	for kid, key := range keys {
		nBytes := key.N.Bytes()
		eBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(eBytes, uint32(key.E))
		list = append(list, map[string]string{
			"kty": "RSA",
			"kid": kid,
			"n":   base64.RawURLEncoding.EncodeToString(nBytes),
			"e":   base64.RawURLEncoding.EncodeToString(eBytes[1:]),
			"alg": "RS256",
			"use": "sig",
		})
	}
	doc["keys"] = list
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestJWKSCache(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	srv := jwksTestServer(t, map[string]*rsa.PublicKey{"test-key": &key.PublicKey})
	cache := newJWKSCache(srv.URL)

	// publicKey resolves the kid
	got, err := cache.publicKey(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("publicKey: %v", err)
	}
	if got.N.Cmp(key.N) != 0 || got.E != key.E {
		t.Fatal("resolved key does not match")
	}

	// Unknown kid fails after refresh attempt
	if _, err := cache.publicKey(context.Background(), "nope"); err == nil {
		t.Fatal("expected error for unknown kid")
	}

	// Full jwt.Parse with RS256 keyfunc succeeds for correctly signed token
	claims := jwt.MapClaims{
		"exp": time.Now().Add(5 * time.Minute).Unix(),
		"aud": "client-1",
		"iss": "https://sts.windows.net/tenant-1/",
	}
	signed := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed.Header["kid"] = "test-key"
	tokenStr, err := signed.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	keyfunc := func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok || token.Header["alg"] != "RS256" {
			return nil, jwt.ErrSignatureInvalid
		}
		kid, _ := token.Header["kid"].(string)
		return cache.publicKey(context.Background(), kid)
	}
	parsed, err := jwt.Parse(tokenStr, keyfunc)
	if err != nil || !parsed.Valid {
		t.Fatalf("expected valid token, got err=%v", err)
	}

	// Token signed by a different key fails
	forged, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jwt.Parse(forged, keyfunc); err == nil {
		t.Fatal("expected signature error for forged token")
	}

	// Token signed with HS256 (alg confusion) fails
	hsToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jwt.Parse(hsToken, keyfunc); err == nil {
		t.Fatal("expected alg error for HS256 token")
	}
}
