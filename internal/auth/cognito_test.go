package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

type testKeyPair struct {
	privateKey *rsa.PrivateKey
	kid        string
	jwkSet     jwk.Set
}

func newTestKeyPair(t *testing.T, kid string) *testKeyPair {
	t.Helper()
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	pubJWK, err := jwk.FromRaw(privKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	if err := pubJWK.Set(jwk.KeyIDKey, kid); err != nil {
		t.Fatal(err)
	}
	if err := pubJWK.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		t.Fatal(err)
	}
	if err := pubJWK.Set(jwk.KeyUsageKey, "sig"); err != nil {
		t.Fatal(err)
	}

	set := jwk.NewSet()
	set.AddKey(pubJWK)

	return &testKeyPair{
		privateKey: privKey,
		kid:        kid,
		jwkSet:     set,
	}
}

func (kp *testKeyPair) signToken(t *testing.T, tok jwt.Token) string {
	t.Helper()
	privJWK, err := jwk.FromRaw(kp.privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := privJWK.Set(jwk.KeyIDKey, kp.kid); err != nil {
		t.Fatal(err)
	}
	if err := privJWK.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		t.Fatal(err)
	}

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

func serveJWKS(t *testing.T, set jwk.Set) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, err := json.Marshal(set)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(buf)
	}))
}

func makeValidator(t *testing.T, jwksServer *httptest.Server) *CognitoValidator {
	t.Helper()
	v := &CognitoValidator{
		issuer:   "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_TestPool",
		clientID: "test-client-id",
		tokenUse: "access",
		jwksURL:  jwksServer.URL,
		sfWait:   make(map[string]chan struct{}),
	}
	if err := v.FetchJWKS(context.Background()); err != nil {
		t.Fatal(err)
	}
	return v
}

func buildToken(kp *testKeyPair, issuer, clientID, tokenUse string, expOffset time.Duration) jwt.Token {
	tok, _ := jwt.NewBuilder().
		Issuer(issuer).
		Subject("user-123").
		Expiration(time.Now().Add(expOffset)).
		Build()
	tok.Set("token_use", tokenUse)
	tok.Set("client_id", clientID)
	tok.Set("cognito:groups", []interface{}{"admin", "users"})
	tok.Set("email", "test@example.com")
	tok.Set("scope", "openid profile")
	return tok
}

func TestValidate_ValidToken(t *testing.T) {
	kp := newTestKeyPair(t, "test-kid-1")
	server := serveJWKS(t, kp.jwkSet)
	defer server.Close()

	v := makeValidator(t, server)
	tok := buildToken(kp, v.issuer, v.clientID, "access", time.Hour)
	tokenStr := kp.signToken(t, tok)

	claims, err := v.Validate(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("expected subject user-123, got %s", claims.Subject)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", claims.Email)
	}
	if len(claims.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(claims.Groups))
	}
}

func TestValidate_ExpiredToken(t *testing.T) {
	kp := newTestKeyPair(t, "test-kid-1")
	server := serveJWKS(t, kp.jwkSet)
	defer server.Close()

	v := makeValidator(t, server)
	tok := buildToken(kp, v.issuer, v.clientID, "access", -time.Hour) // expired
	tokenStr := kp.signToken(t, tok)

	_, err := v.Validate(context.Background(), tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidate_FutureNBF(t *testing.T) {
	kp := newTestKeyPair(t, "test-kid-1")
	server := serveJWKS(t, kp.jwkSet)
	defer server.Close()

	v := makeValidator(t, server)
	tok := buildToken(kp, v.issuer, v.clientID, "access", time.Hour)
	tok.Set("nbf", time.Now().Add(time.Hour)) // not valid yet
	tokenStr := kp.signToken(t, tok)

	_, err := v.Validate(context.Background(), tokenStr)
	if err == nil {
		t.Fatal("expected error for future nbf")
	}
}

func TestValidate_WrongIssuer(t *testing.T) {
	kp := newTestKeyPair(t, "test-kid-1")
	server := serveJWKS(t, kp.jwkSet)
	defer server.Close()

	v := makeValidator(t, server)
	tok := buildToken(kp, "https://wrong-issuer.example.com", v.clientID, "access", time.Hour)
	tokenStr := kp.signToken(t, tok)

	_, err := v.Validate(context.Background(), tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestValidate_WrongClientID(t *testing.T) {
	kp := newTestKeyPair(t, "test-kid-1")
	server := serveJWKS(t, kp.jwkSet)
	defer server.Close()

	v := makeValidator(t, server)
	tok := buildToken(kp, v.issuer, "wrong-client-id", "access", time.Hour)
	tokenStr := kp.signToken(t, tok)

	_, err := v.Validate(context.Background(), tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong client_id")
	}
}

func TestValidate_WrongTokenUse(t *testing.T) {
	kp := newTestKeyPair(t, "test-kid-1")
	server := serveJWKS(t, kp.jwkSet)
	defer server.Close()

	v := makeValidator(t, server)
	tok := buildToken(kp, v.issuer, v.clientID, "id", time.Hour) // server expects "access"
	tokenStr := kp.signToken(t, tok)

	_, err := v.Validate(context.Background(), tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong token_use")
	}
}

func TestValidate_IDTokenAudienceCheck(t *testing.T) {
	kp := newTestKeyPair(t, "test-kid-1")
	server := serveJWKS(t, kp.jwkSet)
	defer server.Close()

	v := makeValidator(t, server)
	v.tokenUse = "id"

	tok, _ := jwt.NewBuilder().
		Issuer(v.issuer).
		Subject("user-123").
		Audience([]string{v.clientID}).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	tok.Set("token_use", "id")
	tok.Set("email", "test@example.com")
	tokenStr := kp.signToken(t, tok)

	claims, err := v.Validate(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("expected valid id token, got: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("expected subject user-123, got %s", claims.Subject)
	}
}

func TestValidate_UnknownKidTriggersRefetch(t *testing.T) {
	kp1 := newTestKeyPair(t, "kid-1")
	kp2 := newTestKeyPair(t, "kid-2")

	// Start with only kp1 in JWKS
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var set jwk.Set
		if callCount == 1 {
			set = kp1.jwkSet
		} else {
			// After refetch, include both keys
			combined := jwk.NewSet()
			iter := kp1.jwkSet.Keys(context.Background())
			for iter.Next(context.Background()) {
				combined.AddKey(iter.Pair().Value.(jwk.Key))
			}
			iter = kp2.jwkSet.Keys(context.Background())
			for iter.Next(context.Background()) {
				combined.AddKey(iter.Pair().Value.(jwk.Key))
			}
			set = combined
		}
		buf, _ := json.Marshal(set)
		w.Header().Set("Content-Type", "application/json")
		w.Write(buf)
	}))
	defer server.Close()

	v := &CognitoValidator{
		issuer:   "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_TestPool",
		clientID: "test-client-id",
		tokenUse: "access",
		jwksURL:  server.URL,
		sfWait:   make(map[string]chan struct{}),
	}
	if err := v.FetchJWKS(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Reset lastFetch to allow immediate refetch for testing
	v.lastFetch.Store(0)

	// Sign token with kp2 (unknown kid initially)
	tok := buildToken(kp2, v.issuer, v.clientID, "access", time.Hour)
	tokenStr := kp2.signToken(t, tok)

	claims, err := v.Validate(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("expected valid after refetch, got: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("expected subject user-123, got %s", claims.Subject)
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 JWKS fetches (initial + refetch), got %d", callCount)
	}
}

func TestValidate_RateLimitedRefetch(t *testing.T) {
	kp := newTestKeyPair(t, "kid-1")
	server := serveJWKS(t, kp.jwkSet)
	defer server.Close()

	v := makeValidator(t, server)
	// lastFetch is recent, so refetch should be rate-limited
	v.lastFetch.Store(time.Now().Unix())

	// Token with unknown kid
	kp2 := newTestKeyPair(t, "kid-unknown")
	tok := buildToken(kp2, v.issuer, v.clientID, "access", time.Hour)
	tokenStr := kp2.signToken(t, tok)

	_, err := v.Validate(context.Background(), tokenStr)
	if err == nil {
		t.Fatal("expected error for unknown kid with rate-limited refetch")
	}
}

func TestExtractKid(t *testing.T) {
	t.Run("malformed JWT", func(t *testing.T) {
		_, err := extractKid("not.a.jwt.token")
		if err == nil {
			t.Error("expected error for malformed JWT")
		}
	})

	t.Run("missing kid", func(t *testing.T) {
		// Base64 encode a header without kid
		_, err := extractKid("eyJhbGciOiJSUzI1NiJ9.e30.sig")
		if err == nil {
			t.Error("expected error for missing kid")
		}
	})
}
