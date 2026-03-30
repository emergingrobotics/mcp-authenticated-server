package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const (
	jwksFetchTimeout = 10 * time.Second
	jwksMaxBody      = 1 << 20 // 1 MiB (SEC-04)
	minRefetchInterval int64 = 60 // seconds (AUTH-04)
)

// CognitoValidator validates JWTs against AWS Cognito JWKS.
type CognitoValidator struct {
	issuer   string
	clientID string
	tokenUse string
	jwksURL  string

	mu        sync.RWMutex
	keySet    jwk.Set
	lastFetch atomic.Int64

	sfMu   sync.Mutex
	sfWait map[string]chan struct{}
}

// NewCognitoValidator creates a validator for the given Cognito config.
func NewCognitoValidator(region, userPoolID, clientID, tokenUse string) *CognitoValidator {
	issuer := fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", region, userPoolID)
	return &CognitoValidator{
		issuer:   issuer,
		clientID: clientID,
		tokenUse: tokenUse,
		jwksURL:  issuer + "/.well-known/jwks.json",
		sfWait:   make(map[string]chan struct{}),
	}
}

// FetchJWKS fetches the JWKS from Cognito. Called at startup and on unknown kid.
func (v *CognitoValidator) FetchJWKS(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, jwksFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("creating JWKS request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching JWKS: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, jwksMaxBody))
	if err != nil {
		return fmt.Errorf("reading JWKS response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS fetch returned status %d", resp.StatusCode)
	}

	set, err := jwk.Parse(body)
	if err != nil {
		return fmt.Errorf("parsing JWKS: %w", err)
	}

	v.mu.Lock()
	v.keySet = set
	v.mu.Unlock()
	v.lastFetch.Store(time.Now().Unix())

	return nil
}

// refreshForKid triggers a JWKS refetch for an unknown kid.
// Uses singleflight pattern: one fetch per kid, waiters share the result.
// Rate-limited to one refetch per 60 seconds (AUTH-04).
func (v *CognitoValidator) refreshForKid(ctx context.Context, kid string) error {
	now := time.Now().Unix()
	last := v.lastFetch.Load()
	if now-last < minRefetchInterval {
		return fmt.Errorf("JWKS refetch rate-limited (last fetch %ds ago)", now-last)
	}

	// Singleflight per kid
	v.sfMu.Lock()
	if ch, ok := v.sfWait[kid]; ok {
		v.sfMu.Unlock()
		<-ch // wait for in-flight fetch
		return nil
	}
	ch := make(chan struct{})
	v.sfWait[kid] = ch
	v.sfMu.Unlock()

	err := v.FetchJWKS(ctx)

	v.sfMu.Lock()
	delete(v.sfWait, kid)
	close(ch)
	v.sfMu.Unlock()

	return err
}

// getKey returns the key for a given kid, triggering refetch if needed.
func (v *CognitoValidator) getKey(ctx context.Context, kid string) (jwk.Key, error) {
	v.mu.RLock()
	set := v.keySet
	v.mu.RUnlock()

	if set != nil {
		if key, ok := set.LookupKeyID(kid); ok {
			return key, nil
		}
	}

	// Unknown kid — try refetch
	if err := v.refreshForKid(ctx, kid); err != nil {
		slog.Warn("JWKS refetch failed", "kid", kid, "error", err)
		if set != nil {
			return nil, fmt.Errorf("unknown key ID %q and refetch failed", kid)
		}
		return nil, fmt.Errorf("no JWKS cache and fetch failed: %w", err)
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	if key, ok := v.keySet.LookupKeyID(kid); ok {
		return key, nil
	}
	return nil, fmt.Errorf("key ID %q not found after JWKS refetch", kid)
}

// Validate validates a JWT string and returns the parsed claims.
func (v *CognitoValidator) Validate(ctx context.Context, tokenString string) (*Claims, error) {
	// Extract kid from JWT header without full parse
	kid, err := extractKid(tokenString)
	if err != nil {
		return nil, fmt.Errorf("extracting kid: %w", err)
	}

	key, err := v.getKey(ctx, kid)
	if err != nil {
		return nil, fmt.Errorf("looking up key: %w", err)
	}

	// Get algorithm from key
	var alg jwa.SignatureAlgorithm
	if algVal, ok := key.Get(jwk.AlgorithmKey); ok {
		if a, ok := algVal.(jwa.SignatureAlgorithm); ok {
			alg = a
		} else if s, ok := algVal.(string); ok {
			alg = jwa.SignatureAlgorithm(s)
		}
	}
	if alg == "" {
		alg = jwa.RS256 // default for Cognito
	}

	// Verify signature and parse
	token, err := jwt.Parse([]byte(tokenString),
		jwt.WithKey(alg, key),
		jwt.WithValidate(false), // we validate manually below
	)
	if err != nil {
		return nil, fmt.Errorf("invalid token signature: %w", err)
	}

	// Validate issuer (AUTH-03a)
	if token.Issuer() != v.issuer {
		return nil, fmt.Errorf("invalid issuer: got %q, want %q", token.Issuer(), v.issuer)
	}

	// Validate expiration (AUTH-03d)
	if token.Expiration().Before(time.Now()) {
		return nil, fmt.Errorf("token expired at %s", token.Expiration())
	}

	// Validate nbf (AUTH-03f)
	nbf := token.NotBefore()
	if !nbf.IsZero() && nbf.After(time.Now()) {
		return nil, fmt.Errorf("token not valid before %s", nbf)
	}

	// Validate token_use (AUTH-03e)
	tokenUseClaim, _ := token.Get("token_use")
	tokenUseStr, _ := tokenUseClaim.(string)
	if tokenUseStr != v.tokenUse {
		return nil, fmt.Errorf("invalid token_use: got %q, want %q", tokenUseStr, v.tokenUse)
	}

	// Validate audience/client_id based on token_use (AUTH-03c)
	if v.tokenUse == "id" {
		audiences := token.Audience()
		found := false
		for _, aud := range audiences {
			if aud == v.clientID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("invalid audience: %v does not contain %q", audiences, v.clientID)
		}
	} else {
		// Access tokens use client_id claim
		clientIDClaim, _ := token.Get("client_id")
		clientIDStr, _ := clientIDClaim.(string)
		if clientIDStr != v.clientID {
			return nil, fmt.Errorf("invalid client_id: got %q, want %q", clientIDStr, v.clientID)
		}
	}

	// Extract claims (AUTH-06)
	claims := &Claims{
		Raw: make(map[string]interface{}),
	}

	if sub := token.Subject(); sub != "" {
		claims.Subject = sub
	}

	if email, ok := token.Get("email"); ok {
		claims.Email, _ = email.(string)
	}

	if groups, ok := token.Get("cognito:groups"); ok {
		if groupSlice, ok := groups.([]interface{}); ok {
			for _, g := range groupSlice {
				if s, ok := g.(string); ok {
					claims.Groups = append(claims.Groups, s)
				}
			}
		}
	}

	if scope, ok := token.Get("scope"); ok {
		claims.Scope, _ = scope.(string)
	}

	// Store all claims in Raw
	for iter := token.Iterate(context.Background()); iter.Next(context.Background()); {
		pair := iter.Pair()
		claims.Raw[pair.Key.(string)] = pair.Value
	}

	return claims, nil
}

// extractKid extracts the kid from the JWT JOSE header without full parsing.
func extractKid(tokenString string) (string, error) {
	parts := strings.SplitN(tokenString, ".", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed JWT: expected 3 parts, got %d", len(parts))
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decoding JWT header: %w", err)
	}

	var header struct {
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return "", fmt.Errorf("parsing JWT header: %w", err)
	}

	if header.Kid == "" {
		return "", fmt.Errorf("JWT header missing kid")
	}

	return header.Kid, nil
}
