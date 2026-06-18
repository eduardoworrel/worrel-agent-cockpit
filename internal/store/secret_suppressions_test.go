package store

import "testing"

func TestSuppressSecretAndIsSecretSuppressed(t *testing.T) {
	s := newTestStore(t)

	const suppressed = "abc123hash"
	const other = "def456hash"

	// Before suppression, both should return false.
	if s.IsSecretSuppressed(suppressed) {
		t.Fatal("IsSecretSuppressed: want false before suppress, got true")
	}

	// Suppress the secret.
	if err := s.SuppressSecret(suppressed); err != nil {
		t.Fatalf("SuppressSecret: %v", err)
	}

	// Now it should be suppressed.
	if !s.IsSecretSuppressed(suppressed) {
		t.Fatal("IsSecretSuppressed: want true after suppress, got false")
	}

	// A different hash must not be affected.
	if s.IsSecretSuppressed(other) {
		t.Fatal("IsSecretSuppressed: want false for non-suppressed hash, got true")
	}

	// Calling SuppressSecret again must be idempotent (INSERT OR IGNORE).
	if err := s.SuppressSecret(suppressed); err != nil {
		t.Fatalf("SuppressSecret (idempotent): %v", err)
	}
}
