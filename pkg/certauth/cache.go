package certauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wyrd-company/gelato/pkg/config"
	"github.com/wyrd-company/gelato/pkg/sshutils"
	"golang.org/x/crypto/ssh"
)

var (
	ErrDisabled          = errors.New("OpenBao certificate authentication is disabled")
	ErrNoTrustedKeys     = errors.New("no OpenBao SSH CA public keys are cached")
	ErrNotCertificate    = errors.New("public key is not an SSH certificate")
	ErrUntrustedCert     = errors.New("certificate is not signed by a trusted OpenBao CA")
	ErrInvalidCACertData = errors.New("OpenBao response did not contain an SSH public key")
)

type contextKey string

const authorityCacheContextKey contextKey = "gelato-openbao-authority-cache"

func ContextKey() interface{} {
	return authorityCacheContextKey
}

// AuthorityCache polls and caches OpenBao SSH CA public keys.
type AuthorityCache struct {
	cfg    config.OpenBaoConfig
	client *http.Client

	mu   sync.RWMutex
	keys []ssh.PublicKey
}

func NewAuthorityCache(cfg config.OpenBaoConfig) *AuthorityCache {
	return &AuthorityCache{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

func WithAuthorityCache(ctx context.Context, cache *AuthorityCache) context.Context {
	return context.WithValue(ctx, authorityCacheContextKey, cache)
}

func AuthorityCacheFromContext(ctx context.Context) *AuthorityCache {
	cache, _ := ctx.Value(authorityCacheContextKey).(*AuthorityCache)
	return cache
}

func (c *AuthorityCache) Enabled() bool {
	return c != nil && c.cfg.Enabled
}

func (c *AuthorityCache) Refresh(ctx context.Context) error {
	if c == nil || !c.cfg.Enabled {
		return ErrDisabled
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.PublicKeyURL, nil)
	if err != nil {
		return fmt.Errorf("create OpenBao public key request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch OpenBao public key: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("fetch OpenBao public key: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read OpenBao public key response: %w", err)
	}

	keys, err := parsePublicKeys(body)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.keys = keys
	c.mu.Unlock()
	return nil
}

func (c *AuthorityCache) Start(ctx context.Context, report func(error)) {
	if c == nil || !c.cfg.Enabled {
		return
	}

	interval := c.cfg.PollInterval
	if interval <= 0 {
		interval = time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.Refresh(ctx); err != nil && report != nil {
					report(err)
				}
			}
		}
	}()
}

func (c *AuthorityCache) VerifyPublicKey(pk ssh.PublicKey) (*Identity, error) {
	cert, ok := pk.(*ssh.Certificate)
	if !ok {
		return nil, ErrNotCertificate
	}
	return c.VerifyCertificate(cert)
}

func (c *AuthorityCache) VerifyAuthorizedCertificate(authorizedKey string) (*Identity, error) {
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(authorizedKey)))
	if err != nil {
		return nil, fmt.Errorf("parse SSH certificate: %w", err)
	}
	return c.VerifyPublicKey(pk)
}

func (c *AuthorityCache) VerifyCertificate(cert *ssh.Certificate) (*Identity, error) {
	if c == nil || !c.cfg.Enabled {
		return nil, ErrDisabled
	}
	if cert == nil {
		return nil, ErrNotCertificate
	}
	if cert.CertType != ssh.UserCert {
		return nil, fmt.Errorf("ssh certificate has type %d, want user certificate", cert.CertType)
	}
	if !c.isTrustedAuthority(cert.SignatureKey) {
		return nil, ErrUntrustedCert
	}

	checker := ssh.CertChecker{
		IsUserAuthority: c.isTrustedAuthority,
	}
	principal := ""
	if len(cert.ValidPrincipals) > 0 {
		principal = cert.ValidPrincipals[0]
	}
	if err := checker.CheckCert(principal, cert); err != nil {
		return nil, err
	}

	return NewIdentity(cert), nil
}

func (c *AuthorityCache) isTrustedAuthority(key ssh.PublicKey) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, trusted := range c.keys {
		if sshutils.KeysEqual(trusted, key) {
			return true
		}
	}
	return false
}

func (c *AuthorityCache) TrustedKeys() []ssh.PublicKey {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]ssh.PublicKey, len(c.keys))
	copy(keys, c.keys)
	return keys
}

func parsePublicKeys(body []byte) ([]ssh.PublicKey, error) {
	values := publicKeyValues(body)
	keys := make([]ssh.PublicKey, 0, len(values))
	for _, value := range values {
		for len(value) > 0 {
			key, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(value))
			if err != nil {
				break
			}
			keys = append(keys, key)
			value = strings.TrimSpace(string(rest))
		}
	}
	if len(keys) == 0 {
		return nil, ErrInvalidCACertData
	}
	return keys, nil
}

func publicKeyValues(body []byte) []string {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "{") {
		return []string{raw}
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return []string{raw}
	}

	var values []string
	collectPublicKeys(decoded, &values)
	return values
}

func collectPublicKeys(value interface{}, values *[]string) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			if key == "public_key" {
				collectPublicKeys(child, values)
				continue
			}
			if key == "keys" {
				collectPublicKeys(child, values)
				continue
			}
			if key == "data" {
				collectPublicKeys(child, values)
			}
		}
	case []interface{}:
		for _, child := range typed {
			collectPublicKeys(child, values)
		}
	case string:
		if strings.TrimSpace(typed) != "" {
			*values = append(*values, typed)
		}
	}
}
