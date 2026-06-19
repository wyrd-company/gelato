package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/wyrd-company/gelato/pkg/sshutils"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

var binPath = "gelato"

// SSHConfig is the configuration for the SSH server.
type SSHConfig struct {
	// Enabled toggles the SSH server on/off
	Enabled bool `env:"ENABLED" yaml:"enabled"`

	// ListenAddr is the address on which the SSH server will listen.
	ListenAddr string `env:"LISTEN_ADDR" yaml:"listen_addr"`

	// PublicURL is the public URL of the SSH server.
	PublicURL string `env:"PUBLIC_URL" yaml:"public_url"`

	// KeyPath is the path to the SSH server's private key.
	KeyPath string `env:"KEY_PATH" yaml:"key_path"`

	// ClientKeyPath is the path to the server's client private key.
	ClientKeyPath string `env:"CLIENT_KEY_PATH" yaml:"client_key_path"`

	// MaxTimeout is the maximum number of seconds a connection can take.
	MaxTimeout int `env:"MAX_TIMEOUT" yaml:"max_timeout"`

	// IdleTimeout is the number of seconds a connection can be idle before it is closed.
	IdleTimeout int `env:"IDLE_TIMEOUT" yaml:"idle_timeout"`
}

// GitConfig is the Git daemon configuration for the server.
type GitConfig struct {
	// Enabled toggles the Git daemon on/off
	Enabled bool `env:"ENABLED" yaml:"enabled"`

	// ListenAddr is the address on which the Git daemon will listen.
	ListenAddr string `env:"LISTEN_ADDR" yaml:"listen_addr"`

	// PublicURL is the public URL of the Git daemon server.
	PublicURL string `env:"PUBLIC_URL" yaml:"public_url"`

	// MaxTimeout is the maximum number of seconds a connection can take.
	MaxTimeout int `env:"MAX_TIMEOUT" yaml:"max_timeout"`

	// IdleTimeout is the number of seconds a connection can be idle before it is closed.
	IdleTimeout int `env:"IDLE_TIMEOUT" yaml:"idle_timeout"`

	// MaxConnections is the maximum number of concurrent connections.
	MaxConnections int `env:"MAX_CONNECTIONS" yaml:"max_connections"`
}

// CORSConfig is the CORS configuration for the server.
type CORSConfig struct {
	AllowedHeaders []string `env:"ALLOWED_HEADERS" yaml:"allowed_headers"`

	AllowedOrigins []string `env:"ALLOWED_ORIGINS" yaml:"allowed_origins"`

	AllowedMethods []string `env:"ALLOWED_METHODS" yaml:"allowed_methods"`
}

// HTTPConfig is the HTTP configuration for the server.
type HTTPConfig struct {
	// Enabled toggles the HTTP server on/off
	Enabled bool `env:"ENABLED" yaml:"enabled"`

	// ListenAddr is the address on which the HTTP server will listen.
	ListenAddr string `env:"LISTEN_ADDR" yaml:"listen_addr"`

	// TLSKeyPath is the path to the TLS private key.
	TLSKeyPath string `env:"TLS_KEY_PATH" yaml:"tls_key_path"`

	// TLSCertPath is the path to the TLS certificate.
	TLSCertPath string `env:"TLS_CERT_PATH" yaml:"tls_cert_path"`

	// PublicURL is the public URL of the HTTP server.
	PublicURL string `env:"PUBLIC_URL" yaml:"public_url"`

	// CORS is the cross-origin configuration for the HTTP server.
	CORS CORSConfig `envPrefix:"CORS_" yaml:"cors"`
}

// StatsConfig is the configuration for the stats server.
type StatsConfig struct {
	// Enabled toggles the Stats server on/off
	Enabled bool `env:"ENABLED" yaml:"enabled"`

	// ListenAddr is the address on which the stats server will listen.
	ListenAddr string `env:"LISTEN_ADDR" yaml:"listen_addr"`
}

// LogConfig is the logger configuration.
type LogConfig struct {
	// Format is the format of the logs.
	// Valid values are "json", "logfmt", and "text".
	Format string `env:"FORMAT" yaml:"format"`

	// Time format for the log `ts` field.
	// Format must be described in Golang's time format.
	TimeFormat string `env:"TIME_FORMAT" yaml:"time_format"`

	// Path to a file to write logs to.
	// If not set, logs will be written to stderr.
	Path string `env:"PATH" yaml:"path"`
}

// DBConfig is the database connection configuration.
type DBConfig struct {
	// Driver is the driver for the database.
	Driver string `env:"DRIVER" yaml:"driver"`

	// DataSource is the database data source name.
	DataSource string `env:"DATA_SOURCE" yaml:"data_source"`
}

// LFSConfig is the configuration for Git LFS.
type LFSConfig struct {
	// Enabled is whether or not Git LFS is enabled.
	Enabled bool `env:"ENABLED" yaml:"enabled"`

	// SSHEnabled is whether or not Git LFS over SSH is enabled.
	// This is only used if LFS is enabled.
	SSHEnabled bool `env:"SSH_ENABLED" yaml:"ssh_enabled"`
}

// JobsConfig is the configuration for cron jobs.
type JobsConfig struct {
	MirrorPull string `env:"MIRROR_PULL" yaml:"mirror_pull"`
}

// OpenBaoConfig configures OpenBao SSH CA trust.
type OpenBaoConfig struct {
	// Enabled requires OpenBao-signed SSH certificates for SSH and NATS admin authentication.
	Enabled bool `env:"ENABLED" yaml:"enabled"`

	// PublicKeyURL is the full URL for the OpenBao SSH CA public key endpoint.
	PublicKeyURL string `env:"PUBLIC_KEY_URL" yaml:"public_key_url"`

	// PollInterval is how frequently the public CA key endpoint is refreshed.
	PollInterval time.Duration `env:"POLL_INTERVAL" yaml:"poll_interval"`

	// RequestTimeout is the timeout used while polling OpenBao.
	RequestTimeout time.Duration `env:"REQUEST_TIMEOUT" yaml:"request_timeout"`
}

// NATSConfig configures the NATS admin interface and event publishing.
type NATSConfig struct {
	// Enabled toggles the NATS integration on/off.
	Enabled bool `env:"ENABLED" yaml:"enabled"`

	// URL is the NATS server URL.
	URL string `env:"URL" yaml:"url"`

	// SubjectPrefix is the root subject prefix for Gelato messages.
	SubjectPrefix string `env:"SUBJECT_PREFIX" yaml:"subject_prefix"`

	// AdminQueue is the queue group used by admin subscribers.
	AdminQueue string `env:"ADMIN_QUEUE" yaml:"admin_queue"`

	// RequestMaxSkew is the maximum allowed admin request timestamp skew.
	RequestMaxSkew time.Duration `env:"REQUEST_MAX_SKEW" yaml:"request_max_skew"`
}

// RemotePushConfig configures automatic pushes back to imported repository remotes.
type RemotePushConfig struct {
	// Enabled pushes ref updates back to a configured remote from the update hook.
	Enabled bool `env:"ENABLED" yaml:"enabled"`
}

// Config is the configuration for Gelato.
type Config struct {
	// Name is the name of the server.
	Name string `env:"NAME" yaml:"name"`

	// SSH is the configuration for the SSH server.
	SSH SSHConfig `envPrefix:"SSH_" yaml:"ssh"`

	// Git is the configuration for the Git daemon.
	Git GitConfig `envPrefix:"GIT_" yaml:"git"`

	// HTTP is the configuration for the HTTP server.
	HTTP HTTPConfig `envPrefix:"HTTP_" yaml:"http"`

	// Stats is the configuration for the stats server.
	Stats StatsConfig `envPrefix:"STATS_" yaml:"stats"`

	// Log is the logger configuration.
	Log LogConfig `envPrefix:"LOG_" yaml:"log"`

	// DB is the database configuration.
	DB DBConfig `envPrefix:"DB_" yaml:"db"`

	// LFS is the configuration for Git LFS.
	LFS LFSConfig `envPrefix:"LFS_" yaml:"lfs"`

	// Jobs is the configuration for cron jobs
	Jobs JobsConfig `envPrefix:"JOBS_" yaml:"jobs"`

	// OpenBao configures SSH certificate trust.
	OpenBao OpenBaoConfig `envPrefix:"OPENBAO_" yaml:"openbao"`

	// NATS configures admin requests and event publishing.
	NATS NATSConfig `envPrefix:"NATS_" yaml:"nats"`

	// RemotePush configures push-on-update behavior for imported repositories.
	RemotePush RemotePushConfig `envPrefix:"REMOTE_PUSH_" yaml:"remote_push"`

	// InitialAdminKeys is a list of public keys that will be added to the list of admins.
	InitialAdminKeys []string `env:"INITIAL_ADMIN_KEYS" envSeparator:"\n" yaml:"initial_admin_keys"`

	// DataPath is the path to the directory where Gelato will store its data.
	DataPath string `env:"DATA_PATH" yaml:"-"`
}

// Environ returns the config as a list of environment variables.
func (c *Config) Environ() []string {
	envs := []string{
		fmt.Sprintf("SOFT_SERVE_BIN_PATH=%s", binPath),
	}
	if c == nil {
		return envs
	}

	// TODO: do this dynamically
	envs = append(envs, []string{
		fmt.Sprintf("SOFT_SERVE_CONFIG_LOCATION=%s", c.ConfigPath()),
		fmt.Sprintf("SOFT_SERVE_DATA_PATH=%s", c.DataPath),
		fmt.Sprintf("SOFT_SERVE_NAME=%s", c.Name),
		fmt.Sprintf("SOFT_SERVE_INITIAL_ADMIN_KEYS=%s", strings.Join(c.InitialAdminKeys, "\n")),
		fmt.Sprintf("SOFT_SERVE_SSH_ENABLED=%t", c.SSH.Enabled),
		fmt.Sprintf("SOFT_SERVE_SSH_LISTEN_ADDR=%s", c.SSH.ListenAddr),
		fmt.Sprintf("SOFT_SERVE_SSH_PUBLIC_URL=%s", c.SSH.PublicURL),
		fmt.Sprintf("SOFT_SERVE_SSH_KEY_PATH=%s", c.SSH.KeyPath),
		fmt.Sprintf("SOFT_SERVE_SSH_CLIENT_KEY_PATH=%s", c.SSH.ClientKeyPath),
		fmt.Sprintf("SOFT_SERVE_SSH_MAX_TIMEOUT=%d", c.SSH.MaxTimeout),
		fmt.Sprintf("SOFT_SERVE_SSH_IDLE_TIMEOUT=%d", c.SSH.IdleTimeout),
		fmt.Sprintf("SOFT_SERVE_GIT_ENABLED=%t", c.Git.Enabled),
		fmt.Sprintf("SOFT_SERVE_GIT_LISTEN_ADDR=%s", c.Git.ListenAddr),
		fmt.Sprintf("SOFT_SERVE_GIT_PUBLIC_URL=%s", c.Git.PublicURL),
		fmt.Sprintf("SOFT_SERVE_GIT_MAX_TIMEOUT=%d", c.Git.MaxTimeout),
		fmt.Sprintf("SOFT_SERVE_GIT_IDLE_TIMEOUT=%d", c.Git.IdleTimeout),
		fmt.Sprintf("SOFT_SERVE_GIT_MAX_CONNECTIONS=%d", c.Git.MaxConnections),
		fmt.Sprintf("SOFT_SERVE_HTTP_ENABLED=%t", c.HTTP.Enabled),
		fmt.Sprintf("SOFT_SERVE_HTTP_LISTEN_ADDR=%s", c.HTTP.ListenAddr),
		fmt.Sprintf("SOFT_SERVE_HTTP_TLS_KEY_PATH=%s", c.HTTP.TLSKeyPath),
		fmt.Sprintf("SOFT_SERVE_HTTP_TLS_CERT_PATH=%s", c.HTTP.TLSCertPath),
		fmt.Sprintf("SOFT_SERVE_HTTP_PUBLIC_URL=%s", c.HTTP.PublicURL),
		fmt.Sprintf("SOFT_SERVE_HTTP_CORS_ALLOWED_HEADERS=%s", strings.Join(c.HTTP.CORS.AllowedHeaders, ",")),
		fmt.Sprintf("SOFT_SERVE_HTTP_CORS_ALLOWED_ORIGINS=%s", strings.Join(c.HTTP.CORS.AllowedOrigins, ",")),
		fmt.Sprintf("SOFT_SERVE_HTTP_CORS_ALLOWED_METHODS=%s", strings.Join(c.HTTP.CORS.AllowedMethods, ",")),
		fmt.Sprintf("SOFT_SERVE_STATS_ENABLED=%t", c.Stats.Enabled),
		fmt.Sprintf("SOFT_SERVE_STATS_LISTEN_ADDR=%s", c.Stats.ListenAddr),
		fmt.Sprintf("SOFT_SERVE_LOG_FORMAT=%s", c.Log.Format),
		fmt.Sprintf("SOFT_SERVE_LOG_TIME_FORMAT=%s", c.Log.TimeFormat),
		fmt.Sprintf("SOFT_SERVE_DB_DRIVER=%s", c.DB.Driver),
		fmt.Sprintf("SOFT_SERVE_DB_DATA_SOURCE=%s", c.DB.DataSource),
		fmt.Sprintf("SOFT_SERVE_LFS_ENABLED=%t", c.LFS.Enabled),
		fmt.Sprintf("SOFT_SERVE_LFS_SSH_ENABLED=%t", c.LFS.SSHEnabled),
		fmt.Sprintf("SOFT_SERVE_JOBS_MIRROR_PULL=%s", c.Jobs.MirrorPull),
		fmt.Sprintf("SOFT_SERVE_OPENBAO_ENABLED=%t", c.OpenBao.Enabled),
		fmt.Sprintf("SOFT_SERVE_OPENBAO_PUBLIC_KEY_URL=%s", c.OpenBao.PublicKeyURL),
		fmt.Sprintf("SOFT_SERVE_OPENBAO_POLL_INTERVAL=%s", c.OpenBao.PollInterval),
		fmt.Sprintf("SOFT_SERVE_OPENBAO_REQUEST_TIMEOUT=%s", c.OpenBao.RequestTimeout),
		fmt.Sprintf("SOFT_SERVE_NATS_ENABLED=%t", c.NATS.Enabled),
		fmt.Sprintf("SOFT_SERVE_NATS_URL=%s", c.NATS.URL),
		fmt.Sprintf("SOFT_SERVE_NATS_SUBJECT_PREFIX=%s", c.NATS.SubjectPrefix),
		fmt.Sprintf("SOFT_SERVE_NATS_ADMIN_QUEUE=%s", c.NATS.AdminQueue),
		fmt.Sprintf("SOFT_SERVE_NATS_REQUEST_MAX_SKEW=%s", c.NATS.RequestMaxSkew),
		fmt.Sprintf("SOFT_SERVE_REMOTE_PUSH_ENABLED=%t", c.RemotePush.Enabled),
	}...)

	return envs
}

// IsDebug returns true if the server is running in debug mode.
func IsDebug() bool {
	debug, _ := strconv.ParseBool(firstEnv("GELATO_DEBUG", "SOFT_SERVE_DEBUG"))
	return debug
}

// IsVerbose returns true if the server is running in verbose mode.
// Verbose mode is only enabled if debug mode is enabled.
func IsVerbose() bool {
	verbose, _ := strconv.ParseBool(firstEnv("GELATO_VERBOSE", "SOFT_SERVE_VERBOSE"))
	return IsDebug() && verbose
}

// parseFile parses the given file as a configuration file.
// The file must be in YAML format.
func parseFile(cfg *Config, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer f.Close() //nolint: errcheck
	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}

	return cfg.Validate()
}

// ParseFile parses the config from the default file path.
// This also calls Validate() on the config.
func (c *Config) ParseFile() error {
	return parseFile(c, c.ConfigPath())
}

// parseEnv parses the environment variables as a configuration file.
func parseEnv(cfg *Config) error {
	// Merge initial admin keys from both config file and environment variables.
	initialAdminKeys := append([]string{}, cfg.InitialAdminKeys...)

	// Preserve upstream Soft Serve variables as a compatibility layer, then let
	// Gelato variables override them.
	if err := env.ParseWithOptions(cfg, env.Options{
		Prefix: "SOFT_SERVE_",
	}); err != nil {
		return fmt.Errorf("parse environment variables: %w", err)
	}
	if err := env.ParseWithOptions(cfg, env.Options{
		Prefix: "GELATO_",
	}); err != nil {
		return fmt.Errorf("parse Gelato environment variables: %w", err)
	}

	// Merge initial admin keys from environment variables.
	if initialAdminKeysEnv := firstEnv("GELATO_INITIAL_ADMIN_KEYS", "SOFT_SERVE_INITIAL_ADMIN_KEYS"); initialAdminKeysEnv != "" {
		cfg.InitialAdminKeys = append(cfg.InitialAdminKeys, initialAdminKeys...)
	}

	return cfg.Validate()
}

// ParseEnv parses the config from the environment variables.
// This also calls Validate() on the config.
func (c *Config) ParseEnv() error {
	return parseEnv(c)
}

// Parse parses the config from the default file path and environment variables.
// This also calls Validate() on the config.
func (c *Config) Parse() error {
	if err := c.ParseFile(); err != nil {
		return err
	}

	return c.ParseEnv()
}

// writeConfig writes the configuration to the given file.
func writeConfig(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(newConfigFile(cfg)), 0o644) //nolint: errcheck, gosec
}

// WriteConfig writes the configuration to the default file.
func (c *Config) WriteConfig() error {
	return writeConfig(c, c.ConfigPath())
}

// DefaultDataPath returns the path to the data directory.
// It uses GELATO_DATA_PATH or the compatible SOFT_SERVE_DATA_PATH environment
// variable if set, otherwise it uses "data".
func DefaultDataPath() string {
	dp := firstEnv("GELATO_DATA_PATH", "SOFT_SERVE_DATA_PATH")
	if dp == "" {
		dp = "data"
	}

	return dp
}

// ConfigPath returns the path to the config file.
func (c *Config) ConfigPath() string { //nolint:revive
	// If we have a custom config location set, then use that.
	if path := firstEnv("GELATO_CONFIG_LOCATION", "SOFT_SERVE_CONFIG_LOCATION"); exist(path) {
		return path
	}

	// Otherwise, look in the data path.
	return filepath.Join(c.DataPath, "config.yaml")
}

func exist(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Exist returns true if the config file exists.
func (c *Config) Exist() bool {
	return exist(c.ConfigPath())
}

// DefaultConfig returns the default Config. All the path values are relative
// to the data directory.
// Use Validate() to validate the config and ensure absolute paths.
func DefaultConfig() *Config {
	return &Config{
		Name:     "Gelato",
		DataPath: DefaultDataPath(),
		SSH: SSHConfig{
			Enabled:       true,
			ListenAddr:    ":23231",
			PublicURL:     "ssh://localhost:23231",
			KeyPath:       filepath.Join("ssh", "gelato_host_ed25519"),
			ClientKeyPath: filepath.Join("ssh", "gelato_client_ed25519"),
			MaxTimeout:    0,
			IdleTimeout:   10 * 60, // 10 minutes
		},
		Git: GitConfig{
			Enabled:        true,
			ListenAddr:     ":9418",
			PublicURL:      "git://localhost",
			MaxTimeout:     0,
			IdleTimeout:    3,
			MaxConnections: 32,
		},
		HTTP: HTTPConfig{
			Enabled:    true,
			ListenAddr: ":23232",
			PublicURL:  "http://localhost:23232",
			CORS: CORSConfig{
				AllowedHeaders: []string{"Accept", "Accept-Language", "Content-Language", "Content-Type", "Origin", "X-Requested-With", "User-Agent", "Authorization", "Access-Control-Request-Method", "Access-Control-Allow-Origin"},
				AllowedMethods: []string{"GET", "HEAD", "POST", "PUT", "OPTIONS"},
				AllowedOrigins: []string{"http://localhost:23232"},
			},
		},
		Stats: StatsConfig{
			Enabled:    true,
			ListenAddr: "localhost:23233",
		},
		Log: LogConfig{
			Format:     "text",
			TimeFormat: time.DateTime,
		},
		DB: DBConfig{
			Driver: "sqlite",
			DataSource: "gelato.db" +
				"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)",
		},
		LFS: LFSConfig{
			Enabled:    true,
			SSHEnabled: false,
		},
		Jobs: JobsConfig{
			MirrorPull: "@every 10m",
		},
		OpenBao: OpenBaoConfig{
			Enabled:        false,
			PublicKeyURL:   "http://openbao:8200/v1/ssh/public_key",
			PollInterval:   time.Minute,
			RequestTimeout: 5 * time.Second,
		},
		NATS: NATSConfig{
			Enabled:        false,
			URL:            "nats://localhost:4222",
			SubjectPrefix:  "gelato",
			AdminQueue:     "gelato-admin",
			RequestMaxSkew: 5 * time.Minute,
		},
		RemotePush: RemotePushConfig{
			Enabled: false,
		},
	}
}

// Validate validates the configuration.
// It updates the configuration with absolute paths.
func (c *Config) Validate() error {
	// Use absolute paths
	if !filepath.IsAbs(c.DataPath) {
		dp, err := filepath.Abs(c.DataPath)
		if err != nil {
			return err
		}
		c.DataPath = dp
	}

	c.SSH.PublicURL = strings.TrimSuffix(c.SSH.PublicURL, "/")
	c.HTTP.PublicURL = strings.TrimSuffix(c.HTTP.PublicURL, "/")

	if c.SSH.KeyPath != "" && !filepath.IsAbs(c.SSH.KeyPath) {
		c.SSH.KeyPath = filepath.Join(c.DataPath, c.SSH.KeyPath)
	}

	if c.SSH.ClientKeyPath != "" && !filepath.IsAbs(c.SSH.ClientKeyPath) {
		c.SSH.ClientKeyPath = filepath.Join(c.DataPath, c.SSH.ClientKeyPath)
	}

	if c.HTTP.TLSKeyPath != "" && !filepath.IsAbs(c.HTTP.TLSKeyPath) {
		c.HTTP.TLSKeyPath = filepath.Join(c.DataPath, c.HTTP.TLSKeyPath)
	}

	if c.HTTP.TLSCertPath != "" && !filepath.IsAbs(c.HTTP.TLSCertPath) {
		c.HTTP.TLSCertPath = filepath.Join(c.DataPath, c.HTTP.TLSCertPath)
	}

	if strings.HasPrefix(c.DB.Driver, "sqlite") && !filepath.IsAbs(c.DB.DataSource) {
		c.DB.DataSource = filepath.Join(c.DataPath, c.DB.DataSource)
	}

	// Validate keys
	pks := make([]string, 0)
	for _, key := range parseAuthKeys(c.InitialAdminKeys) {
		ak := sshutils.MarshalAuthorizedKey(key)
		pks = append(pks, ak)
	}

	c.InitialAdminKeys = pks

	c.HTTP.CORS.AllowedOrigins = append([]string{c.HTTP.PublicURL}, c.HTTP.CORS.AllowedOrigins...)

	return nil
}

// parseAuthKeys parses authorized keys from either file paths or string authorized_keys.
func parseAuthKeys(aks []string) []ssh.PublicKey {
	exist := make(map[string]struct{}, 0)
	pks := make([]ssh.PublicKey, 0)
	for _, key := range aks {
		if bts, err := os.ReadFile(key); err == nil {
			// key is a file
			key = strings.TrimSpace(string(bts))
		}

		if pk, _, err := sshutils.ParseAuthorizedKey(key); err == nil {
			if _, ok := exist[key]; !ok {
				pks = append(pks, pk)
				exist[key] = struct{}{}
			}
		}
	}
	return pks
}

// AdminKeys returns the server admin keys.
func (c *Config) AdminKeys() []ssh.PublicKey {
	return parseAuthKeys(c.InitialAdminKeys)
}

func init() {
	if ex, err := os.Executable(); err == nil {
		binPath = filepath.ToSlash(ex)
	}
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}
