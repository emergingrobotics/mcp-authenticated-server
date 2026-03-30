package auth

import "context"

type contextKey int

const claimsKey contextKey = iota

// Claims holds the parsed JWT claims stored in request context.
type Claims struct {
	Subject string
	Email   string
	Groups  []string
	Scope   string
	Raw     map[string]interface{}
}

// WithClaims stores claims in the context.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// ClaimsFromContext retrieves claims from the context. Returns nil if not present.
func ClaimsFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}

// SubjectFromContext retrieves the user subject (sub) from the context.
func SubjectFromContext(ctx context.Context) string {
	c := ClaimsFromContext(ctx)
	if c == nil {
		return ""
	}
	return c.Subject
}

// GroupsFromContext retrieves the user's groups from the context.
func GroupsFromContext(ctx context.Context) []string {
	c := ClaimsFromContext(ctx)
	if c == nil {
		return nil
	}
	return c.Groups
}
