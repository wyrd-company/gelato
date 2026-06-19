package ssh

import (
	"fmt"
	"strconv"
	"time"

	"charm.land/log/v2"
	"charm.land/wish/v2"
	"github.com/charmbracelet/ssh"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/spf13/cobra"
	"github.com/wyrd-company/gelato/pkg/backend"
	"github.com/wyrd-company/gelato/pkg/certauth"
	"github.com/wyrd-company/gelato/pkg/config"
	"github.com/wyrd-company/gelato/pkg/db"
	"github.com/wyrd-company/gelato/pkg/events"
	"github.com/wyrd-company/gelato/pkg/proto"
	"github.com/wyrd-company/gelato/pkg/ssh/cmd"
	"github.com/wyrd-company/gelato/pkg/sshutils"
	"github.com/wyrd-company/gelato/pkg/store"
	gossh "golang.org/x/crypto/ssh"
)

// ErrPermissionDenied is returned when a user is not allowed connect.
var ErrPermissionDenied = fmt.Errorf("permission denied")

// AuthenticationMiddleware handles authentication.
func AuthenticationMiddleware(sh ssh.Handler) ssh.Handler {
	return func(s ssh.Session) {
		// XXX: The authentication key is set in the context but gossh doesn't
		// validate the authentication. We need to verify that the _last_ key
		// that was approved is the one that's being used.

		ctx := s.Context()
		be := backend.FromContext(ctx)

		var pkFp string
		perms := s.Permissions().Permissions
		pk := s.PublicKey()
		if pk != nil {
			// There is no public key stored in the context, public-key auth
			// was never requested, skip
			if perms == nil {
				wish.Fatalln(s, ErrPermissionDenied)
				return
			}

			pkFp = gossh.FingerprintSHA256(pk)
		}

		// Check if the key is the same as the one we have in context
		fp := perms.Extensions["pubkey-fp"]
		if fp != "" && fp != pkFp {
			wish.Fatalln(s, ErrPermissionDenied)
			return
		}

		if cache := certauth.AuthorityCacheFromContext(ctx); cache != nil && cache.Enabled() {
			if pk == nil {
				wish.Fatalln(s, ErrPermissionDenied)
				return
			}

			user, err := cache.VerifyPublicKey(pk)
			if err != nil {
				wish.Fatalln(s, ErrPermissionDenied)
				return
			}
			ctx.SetValue(proto.ContextKeyUser, user)
			sh(s)
			return
		}

		ac := be.AllowKeyless(ctx)
		publicKeyCounter.WithLabelValues(strconv.FormatBool(ac || pk != nil)).Inc()
		if !ac && pk == nil {
			wish.Fatalln(s, ErrPermissionDenied)
			return
		}

		// Set the auth'd user, or anon, in the context
		var user proto.User
		if pk != nil {
			user, _ = be.UserByPublicKey(ctx, pk)
		}
		ctx.SetValue(proto.ContextKeyUser, user)

		sh(s)
	}
}

// ContextMiddleware adds the config, backend, and logger to the session context.
type contextMiddlewareOptions struct {
	cache     *certauth.AuthorityCache
	publisher events.Publisher
}

type ContextMiddlewareOption func(*contextMiddlewareOptions)

func WithAuthorityCache(cache *certauth.AuthorityCache) ContextMiddlewareOption {
	return func(opts *contextMiddlewareOptions) {
		opts.cache = cache
	}
}

func WithEventPublisher(publisher events.Publisher) ContextMiddlewareOption {
	return func(opts *contextMiddlewareOptions) {
		opts.publisher = publisher
	}
}

func ContextMiddleware(cfg *config.Config, dbx *db.DB, datastore store.Store, be *backend.Backend, logger *log.Logger, opts ...ContextMiddlewareOption) func(ssh.Handler) ssh.Handler {
	var options contextMiddlewareOptions
	for _, opt := range opts {
		opt(&options)
	}

	return func(sh ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			ctx := s.Context()
			ctx.SetValue(sshutils.ContextKeySession, s)
			ctx.SetValue(config.ContextKey, cfg)
			ctx.SetValue(db.ContextKey, dbx)
			ctx.SetValue(store.ContextKey, datastore)
			ctx.SetValue(backend.ContextKey, be)
			ctx.SetValue(log.ContextKey, logger.WithPrefix("ssh"))
			if options.cache != nil {
				ctx.SetValue(certauth.ContextKey(), options.cache)
			}
			if options.publisher != nil {
				ctx.SetValue(events.ContextKey(), options.publisher)
			}
			sh(s)
		}
	}
}

var cliCommandCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "soft_serve",
	Subsystem: "cli",
	Name:      "commands_total",
	Help:      "Total times each command was called",
}, []string{"command"})

// CommandMiddleware handles git commands and CLI commands.
// This middleware must be run after the ContextMiddleware.
func CommandMiddleware(sh ssh.Handler) ssh.Handler {
	return func(s ssh.Session) {
		_, _, ptyReq := s.Pty()
		if ptyReq {
			sh(s)
			return
		}

		ctx := s.Context()
		cfg := config.FromContext(ctx)

		args := s.Command()
		cliCommandCounter.WithLabelValues(cmd.CommandName(args)).Inc()
		rootCmd := &cobra.Command{
			Short:        "Gelato is a self-hostable Git server for the command line.",
			SilenceUsage: true,
		}
		rootCmd.CompletionOptions.DisableDefaultCmd = true

		rootCmd.SetUsageTemplate(cmd.UsageTemplate)
		rootCmd.SetUsageFunc(cmd.UsageFunc)
		rootCmd.AddCommand(
			cmd.GitUploadPackCommand(),
			cmd.GitUploadArchiveCommand(),
			cmd.GitReceivePackCommand(),
			cmd.RepoCommand(),
			cmd.SettingsCommand(),
			cmd.InfoCommand(),
		)
		if !cfg.OpenBao.Enabled {
			rootCmd.AddCommand(
				cmd.UserCommand(),
				cmd.PubkeyCommand(),
				cmd.SetUsernameCommand(),
				cmd.JWTCommand(),
				cmd.TokenCommand(),
			)
		}

		if cfg.LFS.Enabled {
			rootCmd.AddCommand(
				cmd.GitLFSAuthenticateCommand(),
			)

			if cfg.LFS.SSHEnabled {
				rootCmd.AddCommand(
					cmd.GitLFSTransfer(),
				)
			}
		}

		rootCmd.SetArgs(args)
		if len(args) == 0 {
			// otherwise it'll default to os.Args, which is not what we want.
			rootCmd.SetArgs([]string{"--help"})
		}
		rootCmd.SetIn(s)
		rootCmd.SetOut(s)
		rootCmd.SetErr(s.Stderr())
		rootCmd.SetContext(ctx)

		if err := rootCmd.ExecuteContext(ctx); err != nil {
			s.Exit(1) //nolint: errcheck
			return
		}
	}
}

// LoggingMiddleware logs the ssh connection and command.
func LoggingMiddleware(sh ssh.Handler) ssh.Handler {
	return func(s ssh.Session) {
		ctx := s.Context()
		logger := log.FromContext(ctx).WithPrefix("ssh")
		ct := time.Now()
		hpk := sshutils.MarshalAuthorizedKey(s.PublicKey())
		ptyReq, _, isPty := s.Pty()
		addr := s.RemoteAddr().String()
		user := proto.UserFromContext(ctx)
		logArgs := []interface{}{
			"addr",
			addr,
			"cmd",
			s.Command(),
		}

		if user != nil {
			logArgs = append([]interface{}{
				"username",
				user.Username(),
			}, logArgs...)
		}

		if isPty {
			logArgs = []interface{}{
				"term", ptyReq.Term,
				"width", ptyReq.Window.Width,
				"height", ptyReq.Window.Height,
			}
		}

		if config.IsVerbose() {
			logArgs = append(logArgs,
				"key", hpk,
				"envs", s.Environ(),
			)
		}

		msg := fmt.Sprintf("user %q", s.User())
		logger.Debug(msg+" connected", logArgs...)
		sh(s)
		logger.Debug(msg+" disconnected", append(logArgs, "duration", time.Since(ct))...)
	}
}
