package auth

import (
	"context"
	"testing"
)

func TestPrincipalContext(t *testing.T) {
	principal := Principal{TenantID: "tenant-a", SubjectID: "user-a"}
	ctx := WithPrincipal(context.Background(), principal)

	got, ok := PrincipalFromContext(ctx)
	if !ok {
		t.Fatal("principal not found")
	}
	if got.TenantID != principal.TenantID || got.SubjectID != principal.SubjectID {
		t.Fatalf("principal = %#v", got)
	}
}
