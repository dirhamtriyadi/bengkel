package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIssueAndParseAccess(t *testing.T) {
	manager := NewManager("access-secret-that-is-longer-than-32-chars", "refresh-secret-that-is-longer-than-32", time.Minute, time.Hour)
	userID, branchID := uuid.New(), uuid.New()
	access, refresh, _, err := manager.Issue(userID, &branchID, []string{"owner"})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.ParseAccess(access)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != userID || claims.BranchID == nil || *claims.BranchID != branchID {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if _, err := manager.ParseAccess(refresh); err == nil {
		t.Fatal("refresh token must not be accepted as access token")
	}
}

func TestHashIsDeterministicAndDoesNotExposeToken(t *testing.T) {
	raw := "sensitive-refresh-token"
	if Hash(raw) != Hash(raw) {
		t.Fatal("hash should be deterministic")
	}
	if Hash(raw) == raw {
		t.Fatal("hash should not expose token")
	}
}
