package auth

import "testing"

func TestPrincipalHasPermission(t *testing.T) {
	principal := Principal{
		Permissions: map[string]struct{}{"file:create": {}},
	}

	if !principal.HasPermission("file:create") {
		t.Fatal("expected permission")
	}
	if principal.HasPermission("file:delete") {
		t.Fatal("unexpected permission")
	}
}
