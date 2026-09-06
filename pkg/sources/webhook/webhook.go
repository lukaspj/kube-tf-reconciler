// Package webhook translates SCM webhook payloads into push events.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Push is a normalized webhook push event.
type Push struct {
	// RepoURL is the clone URL of the pushed repository.
	RepoURL string
	// Branch is the pushed branch (refs/heads/<branch>).
	Branch string
	// SHA is the new head commit of the push (informational; the watcher
	// always re-resolves the current revision itself).
	SHA string
}

// Provider parses and verifies webhook payloads of a specific SCM system.
type Provider interface {
	// Name identifies the provider (used for logging/metrics).
	Name() string
	// Match reports whether the request belongs to this provider.
	Match(r *http.Request, body []byte) bool
	// Parse extracts the push event. It returns nil (without error) when
	// the payload is valid but not a push to a branch.
	Parse(r *http.Request, body []byte) (*Push, error)
	// Verify checks the request signature when a secret is configured.
	Verify(r *http.Request, body []byte) error
}

// VerifyHMACSHA256 validates a X-Hub-Signature-256 style signature header of
// the form "sha256=<hex hmac>" against the raw request body.
func VerifyHMACSHA256(signatureHeader string, body []byte, secret string) error {
	if secret == "" {
		return nil
	}
	if signatureHeader == "" {
		return errors.New("missing signature header")
	}
	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return fmt.Errorf("unsupported signature scheme in %q", signatureHeader)
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(signatureHeader, prefix))
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return errors.New("signature mismatch")
	}
	return nil
}
