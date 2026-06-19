package certauth

import (
	"testing"

	"github.com/wyrd-company/gelato/pkg/access"
	"golang.org/x/crypto/ssh"
)

func TestIdentityMapsAdminPrincipal(t *testing.T) {
	identity := NewIdentity(&ssh.Certificate{
		KeyId:           "key-id",
		ValidPrincipals: []string{"user:alice", "role:admin"},
	})

	if got := identity.Username(); got != "alice" {
		t.Fatalf("Username() = %q, want alice", got)
	}
	if !identity.IsAdmin() {
		t.Fatal("IsAdmin() = false, want true")
	}
	if got := identity.AccessLevel("private/repo"); got != access.AdminAccess {
		t.Fatalf("AccessLevel() = %s, want %s", got, access.AdminAccess)
	}
}

func TestIdentityMapsAccessPrincipals(t *testing.T) {
	identity := NewIdentity(&ssh.Certificate{
		KeyId: "key-id",
		ValidPrincipals: []string{
			"user:bob",
			"access-level:read",
			"repo:platform/api:write",
		},
	})

	if got := identity.AccessLevel("other/repo"); got != access.ReadOnlyAccess {
		t.Fatalf("AccessLevel(other/repo) = %s, want %s", got, access.ReadOnlyAccess)
	}
	if got := identity.AccessLevel("platform/api"); got != access.ReadWriteAccess {
		t.Fatalf("AccessLevel(platform/api) = %s, want %s", got, access.ReadWriteAccess)
	}
}
