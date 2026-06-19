package messages

import (
	"fmt"
	"strings"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

const (
	// MediaTypeMessagePack is the required NATS payload media type.
	MediaTypeMessagePack = "application/vnd.msgpack"

	// HeaderContentType carries MediaTypeMessagePack on every Gelato NATS message.
	HeaderContentType = "Content-Type"

	ActionRepoBranch = "repo.branch"
	ActionRepoCreate = "repo.create"
	ActionRepoRename = "repo.rename"
	ActionRepoTag    = "repo.tag"

	EventRepositoryAdded        = "repository.added"
	EventBranchCreated          = "branch.created"
	EventRepositoryRenamed      = "repository.renamed"
	EventRepositoryTagged       = "repository.tagged"
	EventRepositoryRemotePushed = "repository.remote.pushed"
	EventRepositoryRemotePulled = "repository.remote.pulled"
)

// SSHSignature is the SSH wire signature over an admin request payload.
type SSHSignature struct {
	Format string `json:"format" msgpack:"format"`
	Blob   []byte `json:"blob" msgpack:"blob"`
	Rest   []byte `json:"rest,omitempty" msgpack:"rest,omitempty"`
}

// AdminEnvelope is the signed NATS admin message body.
type AdminEnvelope struct {
	Payload     []byte       `json:"payload" msgpack:"payload"`
	Signature   SSHSignature `json:"signature" msgpack:"signature"`
	Certificate string       `json:"certificate" msgpack:"certificate"`
}

// AdminRequestBase is the common JSON payload signed by the caller.
type AdminRequestBase struct {
	RequestID string    `json:"requestId,omitempty"`
	Action    string    `json:"action"`
	Target    string    `json:"target,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type RepoCreateRequest struct {
	AdminRequestBase
	Repository  string `json:"repository"`
	ProjectName string `json:"projectName,omitempty"`
	Description string `json:"description,omitempty"`
	Private     bool   `json:"private,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
	Remote      string `json:"remote,omitempty"`
	Mirror      bool   `json:"mirror,omitempty"`
	LFS         bool   `json:"lfs,omitempty"`
	LFSEndpoint string `json:"lfsEndpoint,omitempty"`
}

type RepoRenameRequest struct {
	AdminRequestBase
	Repository string `json:"repository"`
	NewName    string `json:"newName"`
}

type RepoBranchRequest struct {
	AdminRequestBase
	Repository string `json:"repository"`
	Operation  string `json:"operation"`
	Branch     string `json:"branch,omitempty"`
	FromRef    string `json:"fromRef,omitempty"`
}

type RepoTagRequest struct {
	AdminRequestBase
	Repository string `json:"repository"`
	Operation  string `json:"operation"`
	Tag        string `json:"tag,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Message    string `json:"message,omitempty"`
}

type AdminResponse struct {
	RequestID string      `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
	OK        bool        `json:"ok" msgpack:"ok"`
	Error     string      `json:"error,omitempty" msgpack:"error,omitempty"`
	Result    interface{} `json:"result,omitempty" msgpack:"result,omitempty"`
}

type RepositoryEvent struct {
	ID          string    `json:"id" msgpack:"id"`
	Type        string    `json:"type" msgpack:"type"`
	Repository  string    `json:"repository" msgpack:"repository"`
	ProjectName string    `json:"projectName,omitempty" msgpack:"projectName,omitempty"`
	Remote      string    `json:"remote,omitempty" msgpack:"remote,omitempty"`
	Mirror      bool      `json:"mirror,omitempty" msgpack:"mirror,omitempty"`
	OldName     string    `json:"oldName,omitempty" msgpack:"oldName,omitempty"`
	NewName     string    `json:"newName,omitempty" msgpack:"newName,omitempty"`
	Ref         string    `json:"ref,omitempty" msgpack:"ref,omitempty"`
	Branch      string    `json:"branch,omitempty" msgpack:"branch,omitempty"`
	Tag         string    `json:"tag,omitempty" msgpack:"tag,omitempty"`
	OldSHA      string    `json:"oldSha,omitempty" msgpack:"oldSha,omitempty"`
	NewSHA      string    `json:"newSha,omitempty" msgpack:"newSha,omitempty"`
	Actor       string    `json:"actor,omitempty" msgpack:"actor,omitempty"`
	Timestamp   time.Time `json:"timestamp" msgpack:"timestamp"`
}

func Marshal(v interface{}) ([]byte, error) {
	return msgpack.Marshal(v)
}

func Unmarshal(data []byte, v interface{}) error {
	return msgpack.Unmarshal(data, v)
}

func AdminSubject(prefix, action string) string {
	return joinSubject(prefix, "admin", action)
}

func AdminWildcardSubject(prefix string) string {
	return joinSubject(prefix, "admin", "repo", "*")
}

func EventSubjects(prefix, eventType, repo string) []string {
	eventPart := strings.ReplaceAll(eventType, ".", ".")
	repoPart := strings.Join(RepoSubjectTokens(repo), ".")
	if repoPart == "" {
		repoPart = "_"
	}

	return []string{
		joinSubject(prefix, "events", "repo", repoPart, eventPart),
		joinSubject(prefix, "events", "type", eventPart, repoPart),
	}
}

func RepoSubjectTokens(repo string) []string {
	repo = strings.Trim(repo, "/")
	if repo == "" {
		return nil
	}

	parts := strings.Split(repo, "/")
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		tokens = append(tokens, encodeSubjectToken(part))
	}
	return tokens
}

func joinSubject(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, ".")
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, ".")
}

func encodeSubjectToken(token string) string {
	var b strings.Builder
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			for _, by := range []byte(string(r)) {
				b.WriteString(fmt.Sprintf("%%%02X", by))
			}
		}
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}
