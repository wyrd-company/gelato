package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"

	"charm.land/log/v2"
	"github.com/charmbracelet/colorprofile"
	mcobra "github.com/muesli/mango-cobra"
	"github.com/muesli/roff"
	"github.com/spf13/cobra"
	"github.com/wyrd-company/gelato/cmd/soft/admin"
	"github.com/wyrd-company/gelato/cmd/soft/browse"
	"github.com/wyrd-company/gelato/cmd/soft/hook"
	"github.com/wyrd-company/gelato/cmd/soft/serve"
	"github.com/wyrd-company/gelato/pkg/config"
	logr "github.com/wyrd-company/gelato/pkg/log"
	"github.com/wyrd-company/gelato/pkg/ui/common"
	"github.com/wyrd-company/gelato/pkg/version"
	"go.uber.org/automaxprocs/maxprocs"
)

var (
	// Version contains the application version number. It's set via ldflags
	// when building.
	Version = ""

	// CommitSHA contains the SHA of the commit that this application was built
	// against. It's set via ldflags when building.
	CommitSHA = ""

	// CommitDate contains the date of the commit that this application was
	// built against. It's set via ldflags when building.
	CommitDate = ""

	rootCmd = &cobra.Command{
		Use:          "gelato",
		Short:        "A self-hostable Git server for the command line",
		Long:         "Gelato is a certificate-authenticated Git server with a NATS admin interface.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return browse.Command.RunE(cmd, args)
		},
	}

	manCmd = &cobra.Command{
		Use:    "man",
		Short:  "Generate man pages",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			manPage, err := mcobra.NewManPage(1, rootCmd) //.
			if err != nil {
				return err
			}

			manPage = manPage.WithSection("Copyright", "(C) 2021-2023 Charmbracelet, Inc.\n"+
				"Released under MIT license.")
			fmt.Println(manPage.Build(roff.NewDocument()))
			return nil
		},
	}
)

func init() {
	if noColor, _ := strconv.ParseBool(firstEnv("GELATO_NO_COLOR", "SOFT_SERVE_NO_COLOR")); noColor {
		common.DefaultColorProfile = colorprofile.NoTTY
	}

	rootCmd.AddCommand(
		manCmd,
		serve.Command,
		hook.Command,
		admin.Command,
		browse.Command,
	)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	if len(CommitSHA) >= 7 {
		vt := rootCmd.VersionTemplate()
		rootCmd.SetVersionTemplate(vt[:len(vt)-1] + " (" + CommitSHA[0:7] + ")\n")
	}
	if Version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Sum != "" {
			Version = info.Main.Version
		} else {
			Version = "unknown (built from source)"
		}
	}
	rootCmd.Version = Version

	version.Version = Version
	version.CommitSHA = CommitSHA
	version.CommitDate = CommitDate
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}

func main() {
	ctx := context.Background()
	cfg := config.DefaultConfig()
	if cfg.Exist() {
		if err := cfg.Parse(); err != nil {
			log.Fatal(err)
		}
	}

	if err := cfg.ParseEnv(); err != nil {
		log.Fatal(err)
	}

	ctx = config.WithContext(ctx, cfg)
	logger, f, err := logr.NewLogger(cfg)
	if err != nil {
		log.Errorf("failed to create logger: %v", err)
	}

	ctx = log.WithContext(ctx, logger)
	if f != nil {
		defer f.Close() //nolint: errcheck
	}

	// Set global logger
	log.SetDefault(logger)

	var opts []maxprocs.Option
	if config.IsVerbose() {
		opts = append(opts, maxprocs.Logger(log.Debugf))
	}

	// Set the max number of processes to the number of CPUs
	// This is useful when running Gelato in a container.
	if _, err := maxprocs.Set(opts...); err != nil {
		log.Warn("couldn't set automaxprocs", "error", err)
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
