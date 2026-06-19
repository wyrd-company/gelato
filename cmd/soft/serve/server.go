package serve

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"

	"charm.land/log/v2"

	"github.com/charmbracelet/ssh"
	"github.com/wyrd-company/gelato/pkg/backend"
	"github.com/wyrd-company/gelato/pkg/config"
	"github.com/wyrd-company/gelato/pkg/cron"
	"github.com/wyrd-company/gelato/pkg/daemon"
	"github.com/wyrd-company/gelato/pkg/db"
	"github.com/wyrd-company/gelato/pkg/jobs"
	"github.com/wyrd-company/gelato/pkg/natsadmin"
	sshsrv "github.com/wyrd-company/gelato/pkg/ssh"
	"github.com/wyrd-company/gelato/pkg/stats"
	"github.com/wyrd-company/gelato/pkg/web"
	"golang.org/x/sync/errgroup"
)

// Server is the Gelato server.
type Server struct {
	SSHServer   *sshsrv.SSHServer
	GitDaemon   *daemon.GitDaemon
	HTTPServer  *web.HTTPServer
	StatsServer *stats.StatsServer
	NATSServer  *natsadmin.Server
	CertLoader  *CertReloader
	Cron        *cron.Scheduler
	Config      *config.Config
	Backend     *backend.Backend
	DB          *db.DB

	logger *log.Logger
	ctx    context.Context
}

// NewServer returns a new *Server configured to serve Gelato. The SSH
// server key-pair will be created if none exists.
// It expects a context with *backend.Backend, *db.DB, *log.Logger, and
// *config.Config attached.
func NewServer(ctx context.Context) (*Server, error) {
	var err error
	cfg := config.FromContext(ctx)
	be := backend.FromContext(ctx)
	db := db.FromContext(ctx)
	logger := log.FromContext(ctx).WithPrefix("server")
	srv := &Server{
		Config:  cfg,
		Backend: be,
		DB:      db,
		logger:  log.FromContext(ctx).WithPrefix("server"),
		ctx:     ctx,
	}

	// Add cron jobs.
	sched := cron.NewScheduler(ctx)
	for n, j := range jobs.List() {
		id, err := sched.AddFunc(j.Runner.Spec(ctx), j.Runner.Func(ctx))
		if err != nil {
			logger.Warn("error adding cron job", "job", n, "err", err)
		}

		j.ID = id
	}

	srv.Cron = sched

	srv.SSHServer, err = sshsrv.NewSSHServer(ctx)
	if err != nil {
		return nil, fmt.Errorf("create ssh server: %w", err)
	}

	srv.GitDaemon, err = daemon.NewGitDaemon(ctx)
	if err != nil {
		return nil, fmt.Errorf("create git daemon: %w", err)
	}

	srv.HTTPServer, err = web.NewHTTPServer(ctx)
	if err != nil {
		return nil, fmt.Errorf("create http server: %w", err)
	}

	srv.StatsServer, err = stats.NewStatsServer(ctx)
	if err != nil {
		return nil, fmt.Errorf("create stats server: %w", err)
	}

	srv.NATSServer, err = natsadmin.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create nats admin server: %w", err)
	}

	if cfg.HTTP.TLSKeyPath != "" && cfg.HTTP.TLSCertPath != "" {
		srv.CertLoader, err = NewCertReloader(cfg.HTTP.TLSCertPath, cfg.HTTP.TLSKeyPath, logger)
		if err != nil {
			return nil, fmt.Errorf("create cert reloader: %w", err)
		}

		srv.HTTPServer.SetTLSConfig(&tls.Config{
			GetCertificate: srv.CertLoader.GetCertificateFunc(),
		})
	}

	return srv, nil
}

// ReloadCertificates reloads the TLS certificates for the HTTP server.
func (s *Server) ReloadCertificates() error {
	if s.CertLoader == nil {
		return nil
	}
	return s.CertLoader.Reload()
}

// Start starts the SSH server.
func (s *Server) Start() error {
	errg, _ := errgroup.WithContext(s.ctx)

	// optionally start the SSH server
	if s.Config.SSH.Enabled {
		errg.Go(func() error {
			s.logger.Print("Starting SSH server", "addr", s.Config.SSH.ListenAddr)
			if err := s.SSHServer.ListenAndServe(); !errors.Is(err, ssh.ErrServerClosed) {
				return err
			}
			return nil
		})
	}

	// optionally start the git daemon
	if s.Config.Git.Enabled {
		errg.Go(func() error {
			s.logger.Print("Starting Git daemon", "addr", s.Config.Git.ListenAddr)
			if err := s.GitDaemon.ListenAndServe(); !errors.Is(err, daemon.ErrServerClosed) {
				return err
			}
			return nil
		})
	}

	// optionally start the HTTP server
	if s.Config.HTTP.Enabled {
		errg.Go(func() error {
			s.logger.Print("Starting HTTP server", "addr", s.Config.HTTP.ListenAddr)
			if err := s.HTTPServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
	}

	// optionally start the Stats server
	if s.Config.Stats.Enabled {
		errg.Go(func() error {
			s.logger.Print("Starting Stats server", "addr", s.Config.Stats.ListenAddr)
			if err := s.StatsServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
	}

	if s.Config.NATS.Enabled && s.NATSServer != nil {
		errg.Go(func() error {
			s.logger.Print("Starting NATS admin server", "url", s.Config.NATS.URL, "subject", s.Config.NATS.SubjectPrefix+".admin.repo.*")
			return s.NATSServer.Start()
		})
	}

	errg.Go(func() error {
		s.Cron.Start()
		return nil
	})
	return errg.Wait()
}

// Shutdown lets the server gracefully shutdown.
func (s *Server) Shutdown(ctx context.Context) error {
	errg, ctx := errgroup.WithContext(ctx)
	errg.Go(func() error {
		return s.GitDaemon.Shutdown(ctx)
	})
	errg.Go(func() error {
		return s.HTTPServer.Shutdown(ctx)
	})
	errg.Go(func() error {
		return s.SSHServer.Shutdown(ctx)
	})
	errg.Go(func() error {
		return s.StatsServer.Shutdown(ctx)
	})
	if s.NATSServer != nil {
		errg.Go(func() error {
			return s.NATSServer.Close()
		})
	}
	errg.Go(func() error {
		for _, j := range jobs.List() {
			s.Cron.Remove(j.ID)
		}
		s.Cron.Stop()
		return nil
	})
	// defer s.DB.Close() // nolint: errcheck
	return errg.Wait()
}

// Close closes the SSH server.
func (s *Server) Close() error {
	var errg errgroup.Group
	errg.Go(s.GitDaemon.Close)
	errg.Go(s.HTTPServer.Close)
	errg.Go(s.SSHServer.Close)
	errg.Go(s.StatsServer.Close)
	if s.NATSServer != nil {
		errg.Go(s.NATSServer.Close)
	}
	errg.Go(func() error {
		s.Cron.Stop()
		return nil
	})
	// defer s.DB.Close() // nolint: errcheck
	return errg.Wait()
}
