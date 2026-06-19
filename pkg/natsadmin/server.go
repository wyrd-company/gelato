package natsadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/log/v2"
	gitm "github.com/aymanbagabas/git-module"
	nats "github.com/nats-io/nats.go"
	"github.com/wyrd-company/gelato/git"
	"github.com/wyrd-company/gelato/pkg/backend"
	"github.com/wyrd-company/gelato/pkg/certauth"
	"github.com/wyrd-company/gelato/pkg/config"
	"github.com/wyrd-company/gelato/pkg/events"
	"github.com/wyrd-company/gelato/pkg/messages"
	"github.com/wyrd-company/gelato/pkg/proto"
	"github.com/wyrd-company/gelato/pkg/task"
	gossh "golang.org/x/crypto/ssh"
)

type Server struct {
	cfg    *config.Config
	be     *backend.Backend
	cache  *certauth.AuthorityCache
	conn   *nats.Conn
	sub    *nats.Subscription
	logger *log.Logger
	ctx    context.Context
}

func New(ctx context.Context) (*Server, error) {
	cfg := config.FromContext(ctx)
	if cfg == nil || !cfg.NATS.Enabled {
		return nil, nil
	}

	conn, err := nats.Connect(cfg.NATS.URL, nats.Name("gelato-admin"))
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	return &Server{
		cfg:    cfg,
		be:     backend.FromContext(ctx),
		cache:  certauth.AuthorityCacheFromContext(ctx),
		conn:   conn,
		logger: log.FromContext(ctx).WithPrefix("nats.admin"),
		ctx:    ctx,
	}, nil
}

func (s *Server) Start() error {
	if s == nil || s.conn == nil {
		return nil
	}

	subject := messages.AdminWildcardSubject(s.cfg.NATS.SubjectPrefix)
	sub, err := s.conn.QueueSubscribe(subject, s.cfg.NATS.AdminQueue, s.handleMessage)
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", subject, err)
	}
	s.sub = sub
	return s.conn.Flush()
}

func (s *Server) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	if s.sub != nil {
		_ = s.sub.Unsubscribe()
	}
	s.conn.Drain() //nolint:errcheck
	s.conn.Close()
	return nil
}

func (s *Server) handleMessage(msg *nats.Msg) {
	response := s.execute(msg)
	if msg.Reply == "" {
		if !response.OK {
			s.logger.Error("admin request failed without reply subject", "err", response.Error)
		}
		return
	}

	data, err := messages.Marshal(response)
	if err != nil {
		s.logger.Error("failed to marshal admin response", "err", err)
		return
	}

	reply := &nats.Msg{
		Subject: msg.Reply,
		Data:    data,
		Header:  nats.Header{},
	}
	reply.Header.Set(messages.HeaderContentType, messages.MediaTypeMessagePack)
	if err := s.conn.PublishMsg(reply); err != nil {
		s.logger.Error("failed to publish admin response", "err", err)
	}
}

func (s *Server) execute(msg *nats.Msg) messages.AdminResponse {
	requestID, base, identity, err := s.verify(msg)
	if err != nil {
		return messages.AdminResponse{RequestID: requestID, OK: false, Error: err.Error()}
	}

	ctx := proto.WithUserContext(s.ctx, identity)
	switch base.Action {
	case messages.ActionRepoCreate:
		result, err := s.repoCreate(ctx, msg.Data)
		return response(requestID, result, err)
	case messages.ActionRepoRename:
		result, err := s.repoRename(ctx, msg.Data)
		return response(requestID, result, err)
	case messages.ActionRepoBranch:
		result, err := s.repoBranch(ctx, msg.Data)
		return response(requestID, result, err)
	case messages.ActionRepoTag:
		result, err := s.repoTag(ctx, msg.Data)
		return response(requestID, result, err)
	default:
		return messages.AdminResponse{RequestID: requestID, OK: false, Error: "unsupported admin action"}
	}
}

func response(requestID string, result interface{}, err error) messages.AdminResponse {
	if err != nil {
		return messages.AdminResponse{RequestID: requestID, OK: false, Error: err.Error()}
	}
	return messages.AdminResponse{RequestID: requestID, OK: true, Result: result}
}

func (s *Server) verify(msg *nats.Msg) (string, messages.AdminRequestBase, *certauth.Identity, error) {
	var base messages.AdminRequestBase
	if msg.Header.Get(messages.HeaderContentType) != messages.MediaTypeMessagePack {
		return "", base, nil, fmt.Errorf("missing or invalid %s header", messages.HeaderContentType)
	}

	var envelope messages.AdminEnvelope
	if err := messages.Unmarshal(msg.Data, &envelope); err != nil {
		return "", base, nil, fmt.Errorf("decode admin envelope: %w", err)
	}
	if err := json.Unmarshal(envelope.Payload, &base); err != nil {
		return "", base, nil, fmt.Errorf("decode signed payload: %w", err)
	}
	if base.Action == "" {
		return base.RequestID, base, nil, errors.New("payload action is required")
	}
	if base.Timestamp.IsZero() {
		return base.RequestID, base, nil, errors.New("payload timestamp is required")
	}
	if skew := time.Since(base.Timestamp); skew > s.cfg.NATS.RequestMaxSkew || skew < -s.cfg.NATS.RequestMaxSkew {
		return base.RequestID, base, nil, errors.New("payload timestamp is outside the allowed skew")
	}

	identity, err := s.cache.VerifyAuthorizedCertificate(envelope.Certificate)
	if err != nil {
		return base.RequestID, base, nil, fmt.Errorf("verify certificate: %w", err)
	}
	if !identity.IsAdmin() {
		return base.RequestID, base, nil, errors.New("admin principal is required")
	}

	signature := &gossh.Signature{
		Format: envelope.Signature.Format,
		Blob:   envelope.Signature.Blob,
		Rest:   envelope.Signature.Rest,
	}
	if err := identity.Certificate().Key.Verify(envelope.Payload, signature); err != nil {
		return base.RequestID, base, nil, fmt.Errorf("verify payload signature: %w", err)
	}

	return base.RequestID, base, identity, nil
}

func (s *Server) repoCreate(ctx context.Context, data []byte) (interface{}, error) {
	var envelope messages.AdminEnvelope
	if err := messages.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	var req messages.RepoCreateRequest
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, err
	}
	if req.Repository == "" {
		return nil, errors.New("repository is required")
	}

	opts := proto.RepositoryOptions{
		Private:     req.Private,
		Description: req.Description,
		ProjectName: req.ProjectName,
		Hidden:      req.Hidden,
		Mirror:      req.Mirror,
		LFS:         req.LFS,
		LFSEndpoint: req.LFSEndpoint,
	}

	var repo proto.Repository
	var err error
	if req.Remote != "" {
		ctx = events.WithRepositorySource(ctx, events.RepositorySource{Remote: req.Remote, Mirror: req.Mirror})
		repo, err = s.be.ImportRepository(ctx, req.Repository, proto.UserFromContext(ctx), req.Remote, opts)
		if errors.Is(err, task.ErrAlreadyStarted) {
			err = errors.New("import already in progress")
		}
	} else {
		repo, err = s.be.CreateRepository(ctx, req.Repository, proto.UserFromContext(ctx), opts)
	}
	if err != nil {
		return nil, err
	}

	return map[string]string{"repository": repo.Name(), "projectName": repo.ProjectName()}, nil
}

func (s *Server) repoRename(ctx context.Context, data []byte) (interface{}, error) {
	var envelope messages.AdminEnvelope
	if err := messages.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	var req messages.RepoRenameRequest
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, err
	}
	if req.Repository == "" || req.NewName == "" {
		return nil, errors.New("repository and newName are required")
	}
	if err := s.be.RenameRepository(ctx, req.Repository, req.NewName); err != nil {
		return nil, err
	}
	return map[string]string{"repository": req.NewName, "oldName": req.Repository}, nil
}

func (s *Server) repoBranch(ctx context.Context, data []byte) (interface{}, error) {
	var envelope messages.AdminEnvelope
	if err := messages.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	var req messages.RepoBranchRequest
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, err
	}
	if req.Repository == "" {
		return nil, errors.New("repository is required")
	}

	repo, r, err := openRepository(ctx, s.be, req.Repository)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(req.Operation) {
	case "list", "":
		branches, err := r.Branches()
		return map[string]interface{}{"repository": repo.Name(), "branches": branches}, err
	case "default":
		if req.Branch == "" {
			head, err := r.HEAD()
			if err != nil {
				return nil, err
			}
			return map[string]string{"repository": repo.Name(), "branch": head.Name().Short()}, nil
		}
		if _, err := r.SymbolicRef(git.HEAD, git.RefsHeads+req.Branch, gitm.SymbolicRefOptions{
			CommandOptions: gitm.CommandOptions{Context: ctx},
		}); err != nil {
			return nil, err
		}
		return map[string]string{"repository": repo.Name(), "branch": req.Branch}, nil
	case "create":
		if req.Branch == "" {
			return nil, errors.New("branch is required")
		}
		from := req.FromRef
		if from == "" {
			from = "HEAD"
		}
		if err := git.NewCommand("update-ref", git.RefsHeads+req.Branch, from).WithContext(ctx).RunInDirWithOptions(r.Path); err != nil {
			return nil, err
		}
		_ = events.PublishRepositoryEvent(ctx, messages.RepositoryEvent{
			Type:       messages.EventBranchCreated,
			Repository: repo.Name(),
			Ref:        git.RefsHeads + req.Branch,
			Branch:     req.Branch,
		})
		return map[string]string{"repository": repo.Name(), "branch": req.Branch, "fromRef": from}, nil
	case "delete", "remove", "rm":
		if req.Branch == "" {
			return nil, errors.New("branch is required")
		}
		if err := r.DeleteBranch(req.Branch, gitm.DeleteBranchOptions{Force: true}); err != nil {
			return nil, err
		}
		return map[string]string{"repository": repo.Name(), "branch": req.Branch}, nil
	default:
		return nil, fmt.Errorf("unsupported branch operation %q", req.Operation)
	}
}

func (s *Server) repoTag(ctx context.Context, data []byte) (interface{}, error) {
	var envelope messages.AdminEnvelope
	if err := messages.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	var req messages.RepoTagRequest
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return nil, err
	}
	if req.Repository == "" {
		return nil, errors.New("repository is required")
	}

	repo, r, err := openRepository(ctx, s.be, req.Repository)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(req.Operation) {
	case "list", "":
		tags, err := r.Tags()
		return map[string]interface{}{"repository": repo.Name(), "tags": tags}, err
	case "create":
		if req.Tag == "" {
			return nil, errors.New("tag is required")
		}
		ref := req.Ref
		if ref == "" {
			ref = "HEAD"
		}
		args := []string{"tag"}
		if req.Message != "" {
			args = append(args, "-a", req.Tag, ref, "-m", req.Message)
		} else {
			args = append(args, req.Tag, ref)
		}
		if err := git.NewCommand(args...).WithContext(ctx).RunInDirWithOptions(r.Path); err != nil {
			return nil, err
		}
		_ = events.PublishRepositoryEvent(ctx, messages.RepositoryEvent{
			Type:       messages.EventRepositoryTagged,
			Repository: repo.Name(),
			Ref:        git.RefsTags + req.Tag,
			Tag:        req.Tag,
		})
		return map[string]string{"repository": repo.Name(), "tag": req.Tag, "ref": ref}, nil
	case "delete", "remove", "rm":
		if req.Tag == "" {
			return nil, errors.New("tag is required")
		}
		if err := r.DeleteTag(req.Tag); err != nil {
			return nil, err
		}
		return map[string]string{"repository": repo.Name(), "tag": req.Tag}, nil
	default:
		return nil, fmt.Errorf("unsupported tag operation %q", req.Operation)
	}
}

func openRepository(ctx context.Context, be *backend.Backend, name string) (proto.Repository, *git.Repository, error) {
	repo, err := be.Repository(ctx, strings.TrimSuffix(name, ".git"))
	if err != nil {
		return nil, nil, err
	}
	r, err := repo.Open()
	if err != nil {
		return nil, nil, err
	}
	return repo, r, nil
}
