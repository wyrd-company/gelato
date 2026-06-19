package certauth

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/wyrd-company/gelato/pkg/access"
	"golang.org/x/crypto/ssh"
)

const (
	principalRolePrefix        = "role:"
	principalAccessLevelPrefix = "access-level:"
	principalRepoPrefix        = "repo:"
	principalTeamPrefix        = "team:"
)

// Identity is a synthetic Gelato user derived from a verified SSH certificate.
type Identity struct {
	cert       *ssh.Certificate
	username   string
	admin      bool
	principals []string
}

func NewIdentity(cert *ssh.Certificate) *Identity {
	principals := append([]string{}, cert.ValidPrincipals...)
	username := usernameFromCertificate(cert)
	return &Identity{
		cert:       cert,
		username:   username,
		admin:      hasAdminPrincipal(principals),
		principals: principals,
	}
}

func (i *Identity) ID() int64 {
	// Negative IDs cannot accidentally match unowned repositories, whose
	// owner ID is 0 in the upstream data model.
	h := fnv.New32a()
	_, _ = h.Write([]byte(i.username))
	return -int64(h.Sum32()) - 1
}

func (i *Identity) Username() string {
	return i.username
}

func (i *Identity) IsAdmin() bool {
	return i.admin
}

func (i *Identity) PublicKeys() []ssh.PublicKey {
	if i == nil || i.cert == nil || i.cert.Key == nil {
		return nil
	}
	return []ssh.PublicKey{i.cert.Key}
}

func (i *Identity) Password() string {
	return ""
}

func (i *Identity) Certificate() *ssh.Certificate {
	return i.cert
}

func (i *Identity) Principals() []string {
	return append([]string{}, i.principals...)
}

func AccessLevelForUser(user interface{}, repo string) (access.AccessLevel, bool) {
	identity, ok := user.(*Identity)
	if !ok || identity == nil {
		return access.NoAccess, false
	}
	return identity.AccessLevel(repo), true
}

func (i *Identity) AccessLevel(repo string) access.AccessLevel {
	if i == nil {
		return access.NoAccess
	}
	if i.admin {
		return access.AdminAccess
	}

	level := access.NoAccess
	for _, principal := range i.principals {
		if parsed, ok := globalAccessPrincipal(principal); ok && parsed > level {
			level = parsed
		}
		if parsed, ok := repoAccessPrincipal(principal, repo); ok && parsed > level {
			level = parsed
		}
	}
	return level
}

func usernameFromCertificate(cert *ssh.Certificate) string {
	for _, principal := range cert.ValidPrincipals {
		for _, prefix := range []string{"user:", "username:", "email:"} {
			if value, ok := strings.CutPrefix(principal, prefix); ok && value != "" {
				return strings.ToLower(sanitizeUsername(value))
			}
		}
	}
	if cert.KeyId != "" {
		return strings.ToLower(sanitizeUsername(cert.KeyId))
	}
	return "cert-" + strings.TrimPrefix(ssh.FingerprintSHA256(cert.Key), "SHA256:")
}

func sanitizeUsername(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "cert-user"
	}
	return b.String()
}

func hasAdminPrincipal(principals []string) bool {
	for _, principal := range principals {
		normalized := strings.ToLower(strings.TrimSpace(principal))
		if normalized == "admin" || normalized == principalRolePrefix+"admin" || normalized == principalTeamPrefix+"admin" {
			return true
		}
		if parsed, ok := globalAccessPrincipal(normalized); ok && parsed >= access.AdminAccess {
			return true
		}
	}
	return false
}

func globalAccessPrincipal(principal string) (access.AccessLevel, bool) {
	normalized := strings.ToLower(strings.TrimSpace(principal))
	value, ok := strings.CutPrefix(normalized, principalAccessLevelPrefix)
	if !ok {
		return access.NoAccess, false
	}
	return parsePrincipalAccess(value)
}

func repoAccessPrincipal(principal, repo string) (access.AccessLevel, bool) {
	normalized := strings.ToLower(strings.TrimSpace(principal))
	repo = strings.ToLower(strings.TrimSuffix(repo, ".git"))
	value, ok := strings.CutPrefix(normalized, principalRepoPrefix)
	if !ok {
		return access.NoAccess, false
	}

	repoName, level, ok := strings.Cut(value, ":")
	if !ok {
		return access.NoAccess, false
	}
	if repoName != repo {
		return access.NoAccess, false
	}
	return parsePrincipalAccess(level)
}

func parsePrincipalAccess(value string) (access.AccessLevel, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "admin", "admin-access":
		return access.AdminAccess, true
	case "write", "read-write", "rw":
		return access.ReadWriteAccess, true
	case "read", "read-only", "ro":
		return access.ReadOnlyAccess, true
	case "none", "no-access":
		return access.NoAccess, true
	default:
		return access.NoAccess, false
	}
}

func PrincipalSummary(principals []string) string {
	return fmt.Sprintf("[%s]", strings.Join(principals, " "))
}
