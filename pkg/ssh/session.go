package ssh

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/wish/v2"
	bm "charm.land/wish/v2/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/wyrd-company/gelato/pkg/access"
	"github.com/wyrd-company/gelato/pkg/backend"
	"github.com/wyrd-company/gelato/pkg/config"
	"github.com/wyrd-company/gelato/pkg/proto"
	"github.com/wyrd-company/gelato/pkg/ui/common"
)

var tuiSessionCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "soft_serve",
	Subsystem: "ssh",
	Name:      "tui_session_total",
	Help:      "The total number of TUI sessions",
}, []string{"repo", "term"})

var tuiSessionDuration = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "soft_serve",
	Subsystem: "ssh",
	Name:      "tui_session_seconds_total",
	Help:      "The total time spent in TUI sessions",
}, []string{"repo", "term"})

// SessionHandler is the Gelato Bubble Tea SSH session handler.
// This middleware must be run after the ContextMiddleware.
func SessionHandler(s ssh.Session) *tea.Program {
	pty, _, active := s.Pty()
	if !active {
		return nil
	}

	ctx := s.Context()
	be := backend.FromContext(ctx)
	cfg := config.FromContext(ctx)
	cmd := s.Command()
	initialRepo := ""
	if len(cmd) == 1 {
		initialRepo = cmd[0]
	}

	auth := be.AccessLevelByPublicKey(ctx, initialRepo, s.PublicKey())
	if auth < access.ReadOnlyAccess {
		wish.Fatalln(s, proto.ErrUnauthorized)
		return nil
	}

	opts := bm.MakeOptions(s)
	opts = append(opts,
		tea.WithoutCatchPanics(),
		tea.WithContext(ctx),
		tea.WithColorProfile(common.DefaultColorProfile),
	)

	c := common.NewCommon(ctx, pty.Window.Width, pty.Window.Height)
	c.SetValue(common.ConfigKey, cfg)
	m := NewUI(c, initialRepo)
	p := tea.NewProgram(m, opts...)

	tuiSessionCounter.WithLabelValues(initialRepo, pty.Term).Inc()

	start := time.Now()
	go func() {
		<-ctx.Done()
		tuiSessionDuration.WithLabelValues(initialRepo, pty.Term).Add(time.Since(start).Seconds())
	}()

	return p
}
