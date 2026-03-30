package auth

import (
	"context"
	"fmt"
)

// Authorizer determines whether an authenticated user can use a specific tool.
type Authorizer interface {
	Authorize(ctx context.Context, toolName string) error
}

// GroupAuthorizer checks tool authorization based on group membership.
type GroupAuthorizer struct {
	// ToolGroups maps tool names to required group lists.
	// Empty or missing entry means any authenticated user is authorized.
	ToolGroups map[string][]string

	// ServerGroups is the server-wide allowed_groups list (AUTH-09).
	// Empty means no server-wide restriction.
	ServerGroups []string
}

// Authorize checks if the user in the context is authorized for the given tool.
func (a *GroupAuthorizer) Authorize(ctx context.Context, toolName string) error {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return fmt.Errorf("no authentication claims in context")
	}

	// Server-wide group check (AUTH-09)
	if len(a.ServerGroups) > 0 {
		if !hasAnyGroup(claims.Groups, a.ServerGroups) {
			return fmt.Errorf("forbidden: user not in any allowed server group")
		}
	}

	// Per-tool group check (AUTH-11)
	requiredGroups, exists := a.ToolGroups[toolName]
	if !exists || len(requiredGroups) == 0 {
		return nil // no per-tool restriction
	}

	if !hasAnyGroup(claims.Groups, requiredGroups) {
		return fmt.Errorf("forbidden: user not authorized for tool %q", toolName)
	}

	return nil
}

func hasAnyGroup(userGroups, requiredGroups []string) bool {
	groupSet := make(map[string]struct{}, len(userGroups))
	for _, g := range userGroups {
		groupSet[g] = struct{}{}
	}
	for _, g := range requiredGroups {
		if _, ok := groupSet[g]; ok {
			return true
		}
	}
	return false
}

// NoopAuthorizer always authorizes. Used in testing.
type NoopAuthorizer struct{}

func (n *NoopAuthorizer) Authorize(ctx context.Context, toolName string) error {
	return nil
}
