package backend

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wyrd-company/gelato/git"
	"github.com/wyrd-company/gelato/pkg/certauth"
	"github.com/wyrd-company/gelato/pkg/hooks"
	"github.com/wyrd-company/gelato/pkg/messages"
	"github.com/wyrd-company/gelato/pkg/proto"
	"github.com/wyrd-company/gelato/pkg/sshutils"
	"github.com/wyrd-company/gelato/pkg/webhook"
	gossh "golang.org/x/crypto/ssh"
)

var _ hooks.Hooks = (*Backend)(nil)

// PostReceive is called by the git post-receive hook.
//
// It implements Hooks.
func (d *Backend) PostReceive(_ context.Context, _ io.Writer, _ io.Writer, repo string, args []hooks.HookArg) {
	d.logger.Debug("post-receive hook called", "repo", repo, "args", args)
}

// PreReceive is called by the git pre-receive hook.
//
// It implements Hooks.
func (d *Backend) PreReceive(_ context.Context, _ io.Writer, _ io.Writer, repo string, args []hooks.HookArg) {
	d.logger.Debug("pre-receive hook called", "repo", repo, "args", args)
}

// Update is called by the git update hook.
//
// It implements Hooks.
func (d *Backend) Update(ctx context.Context, _ io.Writer, _ io.Writer, repo string, arg hooks.HookArg) {
	d.logger.Debug("update hook called", "repo", repo, "arg", arg)

	// Find user
	var user proto.User
	if pubkey := os.Getenv("SOFT_SERVE_PUBLIC_KEY"); pubkey != "" {
		pk, _, err := sshutils.ParseAuthorizedKey(pubkey)
		if err != nil {
			d.logger.Error("error parsing public key", "err", err)
			return
		}

		if cert, ok := pk.(*gossh.Certificate); ok {
			user = certauth.NewIdentity(cert)
		} else {
			user, err = d.UserByPublicKey(ctx, pk)
			if err != nil {
				d.logger.Error("error finding user from public key", "key", pubkey, "err", err)
				return
			}
		}
	} else if username := os.Getenv("SOFT_SERVE_USERNAME"); username != "" {
		var err error
		user, err = d.User(ctx, username)
		if err != nil {
			d.logger.Error("error finding user from username", "username", username, "err", err)
			return
		}
	} else {
		d.logger.Error("error finding user")
		return
	}

	// Get repo
	r, err := d.Repository(ctx, repo)
	if err != nil {
		d.logger.Error("error finding repository", "repo", repo, "err", err)
		return
	}

	// TODO: run this async
	// This would probably need something like an RPC server to communicate with the hook process.
	if git.IsZeroHash(arg.OldSha) || git.IsZeroHash(arg.NewSha) {
		wh, err := webhook.NewBranchTagEvent(ctx, user, r, arg.RefName, arg.OldSha, arg.NewSha)
		if err != nil {
			d.logger.Error("error creating branch_tag webhook", "err", err)
		} else if err := webhook.SendEvent(ctx, wh); err != nil {
			d.logger.Error("error sending branch_tag webhook", "err", err)
		}
		if git.IsZeroHash(arg.OldSha) && !git.IsZeroHash(arg.NewSha) {
			switch {
			case strings.HasPrefix(arg.RefName, git.RefsHeads):
				d.publishRepositoryEvent(ctx, messages.RepositoryEvent{
					Type:       messages.EventBranchCreated,
					Repository: r.Name(),
					Ref:        arg.RefName,
					Branch:     strings.TrimPrefix(arg.RefName, git.RefsHeads),
					OldSHA:     arg.OldSha,
					NewSHA:     arg.NewSha,
				})
			case strings.HasPrefix(arg.RefName, git.RefsTags):
				d.publishRepositoryEvent(ctx, messages.RepositoryEvent{
					Type:       messages.EventRepositoryTagged,
					Repository: r.Name(),
					Ref:        arg.RefName,
					Tag:        strings.TrimPrefix(arg.RefName, git.RefsTags),
					OldSHA:     arg.OldSha,
					NewSHA:     arg.NewSha,
				})
			}
		}
	}
	wh, err := webhook.NewPushEvent(ctx, user, r, arg.RefName, arg.OldSha, arg.NewSha)
	if err != nil {
		d.logger.Error("error creating push webhook", "err", err)
	} else if err := webhook.SendEvent(ctx, wh); err != nil {
		d.logger.Error("error sending push webhook", "err", err)
	}

	if d.cfg.RemotePush.Enabled {
		if err := d.pushUpdatedRef(ctx, r, arg); err != nil {
			d.logger.Error("error pushing repository ref to remote", "repo", repo, "ref", arg.RefName, "err", err)
		}
	}
}

// PostUpdate is called by the git post-update hook.
//
// It implements Hooks.
func (d *Backend) PostUpdate(ctx context.Context, _ io.Writer, _ io.Writer, repo string, args ...string) {
	d.logger.Debug("post-update hook called", "repo", repo, "args", args)

	var wg sync.WaitGroup

	// Populate last-modified file.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := populateLastModified(ctx, d, repo); err != nil {
			d.logger.Error("error populating last-modified", "repo", repo, "err", err)
			return
		}
	}()

	wg.Wait()
}

func populateLastModified(ctx context.Context, d *Backend, name string) error {
	var rr *repo
	_rr, err := d.Repository(ctx, name)
	if err != nil {
		return err
	}

	if r, ok := _rr.(*repo); ok {
		rr = r
	} else {
		return proto.ErrRepoNotFound
	}

	r, err := rr.Open()
	if err != nil {
		return err
	}

	c, err := r.LatestCommitTime()
	if err != nil {
		return err
	}

	return rr.writeLastModified(c)
}

func (d *Backend) pushUpdatedRef(ctx context.Context, repo proto.Repository, arg hooks.HookArg) error {
	r, err := repo.Open()
	if err != nil {
		return err
	}

	remoteOutput, err := git.NewCommand("remote").WithContext(ctx).RunInDir(r.Path)
	if err != nil {
		return err
	}
	remotes := strings.Fields(string(remoteOutput))
	if len(remotes) == 0 {
		return nil
	}
	remote := remotes[0]

	args := []string{"push"}
	if repo.IsMirror() {
		args = append(args, "--mirror", remote)
	} else {
		refspec := arg.RefName + ":" + arg.RefName
		if git.IsZeroHash(arg.NewSha) {
			refspec = ":" + arg.RefName
		}
		args = append(args, remote, refspec)
	}

	cmd := git.NewCommand(args...).WithContext(ctx)
	cmd.AddEnvs(fmt.Sprintf(`GIT_SSH_COMMAND=ssh -o UserKnownHostsFile="%s" -o StrictHostKeyChecking=no -i "%s"`,
		filepath.Join(d.cfg.DataPath, "ssh", "known_hosts"),
		d.cfg.SSH.ClientKeyPath,
	))
	if _, err := cmd.RunInDir(r.Path); err != nil {
		return err
	}

	d.publishRepositoryEvent(ctx, messages.RepositoryEvent{
		Type:       messages.EventRepositoryRemotePushed,
		Repository: repo.Name(),
		Ref:        arg.RefName,
		OldSHA:     arg.OldSha,
		NewSHA:     arg.NewSha,
		Remote:     remote,
		Mirror:     repo.IsMirror(),
	})
	return nil
}
