package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyHMAC(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"type":"payment","action":"payment.updated"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		payload   []byte
		signature string
		secret    string
		want      bool
	}{
		{"valid signature", payload, validSig, secret, true},
		{"wrong signature", payload, "deadbeef", secret, false},
		{"empty signature", payload, "", secret, false},
		{"tampered payload", []byte(`{"tampered":true}`), validSig, secret, false},
		{"wrong secret", payload, validSig, "other-secret", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyHMAC(tt.payload, tt.signature, tt.secret)
			if got != tt.want {
				t.Errorf("verifyHMAC(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestResolveStatus(t *testing.T) {
	tests := []struct {
		action     string
		wantStatus string
		wantTime   bool // whether confirmedAt should be non-nil
	}{
		{"payment.updated", "confirmed", true},
		{"payment.created", "pending", false},
		{"payment.cancelled", "failed", false},
		{"unknown_action", "failed", false},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			status, confirmedAt := resolveStatus(tt.action)
			if status != tt.wantStatus {
				t.Errorf("resolveStatus(%q) status = %q, want %q", tt.action, status, tt.wantStatus)
			}
			if tt.wantTime && confirmedAt == nil {
				t.Error("expected confirmedAt to be non-nil")
			}
			if !tt.wantTime && confirmedAt != nil {
				t.Error("expected confirmedAt to be nil")
			}
		})
	}
}
