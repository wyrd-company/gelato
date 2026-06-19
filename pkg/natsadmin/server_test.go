package natsadmin

import (
	"testing"
	"time"

	"github.com/wyrd-company/gelato/pkg/config"
)

func TestRememberRequestIDRejectsReplay(t *testing.T) {
	server := &Server{
		cfg: &config.Config{
			NATS: config.NATSConfig{RequestMaxSkew: time.Minute},
		},
		seen: make(map[string]time.Time),
	}

	if err := server.rememberRequestID("request-1"); err != nil {
		t.Fatalf("rememberRequestID first call returned error: %v", err)
	}
	if err := server.rememberRequestID("request-1"); err == nil {
		t.Fatal("rememberRequestID replay returned nil error")
	}
}

func TestRememberRequestIDPrunesExpiredEntries(t *testing.T) {
	server := &Server{
		cfg: &config.Config{
			NATS: config.NATSConfig{RequestMaxSkew: time.Minute},
		},
		seen: map[string]time.Time{
			"request-1": time.Now().Add(-time.Second),
		},
	}

	if err := server.rememberRequestID("request-1"); err != nil {
		t.Fatalf("rememberRequestID after expiry returned error: %v", err)
	}
}
