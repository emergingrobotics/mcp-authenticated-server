package auth

import (
	"context"
	"testing"
)

func TestGroupAuthorizer_NoRequiredGroups(t *testing.T) {
	a := &GroupAuthorizer{
		ToolGroups: map[string][]string{},
	}
	ctx := WithClaims(context.Background(), &Claims{Subject: "user1"})
	if err := a.Authorize(ctx, "search_documents"); err != nil {
		t.Errorf("expected authorized, got: %v", err)
	}
}

func TestGroupAuthorizer_UserHasRequiredGroup(t *testing.T) {
	a := &GroupAuthorizer{
		ToolGroups: map[string][]string{
			"ingest_documents": {"admin"},
		},
	}
	ctx := WithClaims(context.Background(), &Claims{
		Subject: "user1",
		Groups:  []string{"admin", "users"},
	})
	if err := a.Authorize(ctx, "ingest_documents"); err != nil {
		t.Errorf("expected authorized, got: %v", err)
	}
}

func TestGroupAuthorizer_UserLacksRequiredGroup(t *testing.T) {
	a := &GroupAuthorizer{
		ToolGroups: map[string][]string{
			"ingest_documents": {"admin"},
		},
	}
	ctx := WithClaims(context.Background(), &Claims{
		Subject: "user1",
		Groups:  []string{"users"},
	})
	if err := a.Authorize(ctx, "ingest_documents"); err == nil {
		t.Error("expected unauthorized error")
	}
}

func TestGroupAuthorizer_ServerWideGroups_Allowed(t *testing.T) {
	a := &GroupAuthorizer{
		ServerGroups: []string{"premium"},
		ToolGroups:   map[string][]string{},
	}
	ctx := WithClaims(context.Background(), &Claims{
		Subject: "user1",
		Groups:  []string{"premium"},
	})
	if err := a.Authorize(ctx, "search_documents"); err != nil {
		t.Errorf("expected authorized, got: %v", err)
	}
}

func TestGroupAuthorizer_ServerWideGroups_Denied(t *testing.T) {
	a := &GroupAuthorizer{
		ServerGroups: []string{"premium"},
		ToolGroups:   map[string][]string{},
	}
	ctx := WithClaims(context.Background(), &Claims{
		Subject: "user1",
		Groups:  []string{"basic"},
	})
	if err := a.Authorize(ctx, "search_documents"); err == nil {
		t.Error("expected forbidden error for server-wide group check")
	}
}

func TestGroupAuthorizer_EmptyServerGroups_NoRestriction(t *testing.T) {
	a := &GroupAuthorizer{
		ServerGroups: []string{},
		ToolGroups:   map[string][]string{},
	}
	ctx := WithClaims(context.Background(), &Claims{
		Subject: "user1",
		Groups:  []string{},
	})
	if err := a.Authorize(ctx, "search_documents"); err != nil {
		t.Errorf("expected authorized with empty server groups, got: %v", err)
	}
}

func TestGroupAuthorizer_NoClaims(t *testing.T) {
	a := &GroupAuthorizer{}
	if err := a.Authorize(context.Background(), "search_documents"); err == nil {
		t.Error("expected error when no claims in context")
	}
}

func TestGroupAuthorizer_IngestRequiresAdmin(t *testing.T) {
	// AUTH-11: ingest_documents requires explicit group authorization by default
	a := &GroupAuthorizer{
		ToolGroups: map[string][]string{
			"ingest_documents": {"admin"},
		},
	}

	// User without admin group
	ctx := WithClaims(context.Background(), &Claims{
		Subject: "user1",
		Groups:  []string{"users"},
	})
	if err := a.Authorize(ctx, "ingest_documents"); err == nil {
		t.Error("expected ingest_documents to require admin group")
	}

	// User with admin group
	ctx = WithClaims(context.Background(), &Claims{
		Subject: "admin1",
		Groups:  []string{"admin"},
	})
	if err := a.Authorize(ctx, "ingest_documents"); err != nil {
		t.Errorf("admin should be authorized for ingest_documents: %v", err)
	}
}
