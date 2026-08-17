package config

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxSecretBytes = 64 << 10

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	value, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	d.Duration = value
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return d.String(), nil
}

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Auth         AuthConfig         `yaml:"auth"`
	Storage      StorageConfig      `yaml:"storage"`
	Mihomo       MihomoConfig       `yaml:"mihomo"`
	Update       UpdateConfig       `yaml:"update"`
	Subscription SubscriptionConfig `yaml:"subscription"`
	Converter    ConverterConfig    `yaml:"converter"`
	UI           UIConfig           `yaml:"ui"`
}

type ServerConfig struct {
	Listen       string   `yaml:"listen"`
	WebDir       string   `yaml:"web_dir"`
	ReadTimeout  Duration `yaml:"read_timeout"`
	WriteTimeout Duration `yaml:"write_timeout"`
}

type AuthConfig struct {
	AdminTokenFile             string `yaml:"admin_token_file"`
	AllowUnauthenticatedRemote bool   `yaml:"allow_unauthenticated_remote"`
}

type StorageConfig struct {
	DataDir          string `yaml:"data_dir"`
	ActiveConfig     string `yaml:"active_config"`
	BackupRetention  int    `yaml:"backup_retention"`
	HistoryRetention int    `yaml:"history_retention"`
}

type MihomoConfig struct {
	Binary                 string   `yaml:"binary"`
	APIURL                 string   `yaml:"api_url"`
	SecretFile             string   `yaml:"secret_file"`
	EnforceRuntimeSettings bool     `yaml:"enforce_runtime_settings"`
	ControllerListen       string   `yaml:"controller_listen"`
	ExternalUI             string   `yaml:"external_ui"`
	MixedPort              int      `yaml:"mixed_port"`
	AllowLAN               bool     `yaml:"allow_lan"`
	ValidationTimeout      Duration `yaml:"validation_timeout"`
	RequestTimeout         Duration `yaml:"request_timeout"`
}

type UpdateConfig struct {
	Enabled     bool     `yaml:"enabled"`
	RunOnStart  bool     `yaml:"run_on_start"`
	Interval    Duration `yaml:"interval"`
	HealthDelay Duration `yaml:"health_delay"`
}

type SubscriptionConfig struct {
	MaxBytes       int64                `yaml:"max_bytes"`
	RequestTimeout Duration             `yaml:"request_timeout"`
	UserAgent      string               `yaml:"user_agent"`
	Sources        []SubscriptionSource `yaml:"sources"`
}

type SubscriptionSource struct {
	Name    string            `yaml:"name"`
	URLFile string            `yaml:"url_file"`
	Enabled *bool             `yaml:"enabled"`
	Headers map[string]string `yaml:"headers"`
}

func (s SubscriptionSource) IsEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

type ConverterConfig struct {
	Enabled     bool   `yaml:"enabled"`
	APIURL      string `yaml:"api_url"`
	AllowRemote bool   `yaml:"allow_remote"`
}

type UIConfig struct {
	DashboardURL string `yaml:"dashboard_url"`
}

func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Listen:       "127.0.0.1:8080",
			ReadTimeout:  Duration{Duration: 10 * time.Second},
			WriteTimeout: Duration{Duration: 2 * time.Minute},
		},
		Storage: StorageConfig{
			DataDir:          "./data",
			BackupRetention:  10,
			HistoryRetention: 100,
		},
		Mihomo: MihomoConfig{
			Binary:                 "mihomo",
			APIURL:                 "http://127.0.0.1:9090",
			EnforceRuntimeSettings: true,
			ControllerListen:       "127.0.0.1:9090",
			MixedPort:              7890,
			ValidationTimeout:      Duration{Duration: 30 * time.Second},
			RequestTimeout:         Duration{Duration: 10 * time.Second},
		},
		Update: UpdateConfig{
			Enabled:     true,
			Interval:    Duration{Duration: 6 * time.Hour},
			HealthDelay: Duration{Duration: 2 * time.Second},
		},
		Subscription: SubscriptionConfig{
			MaxBytes:       16 << 20,
			RequestTimeout: Duration{Duration: 30 * time.Second},
			UserAgent:      "mihomo-guardian/1",
		},
	}
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	cfg := Defaults()
	decoder := yaml.NewDecoder(io.LimitReader(file, 2<<20))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Config{}, errors.New("decode config: multiple YAML documents are not supported")
	}

	baseDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return Config{}, fmt.Errorf("resolve config directory: %w", err)
	}
	cfg.resolvePaths(baseDir)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) resolvePaths(baseDir string) {
	c.Storage.DataDir = resolvePath(baseDir, c.Storage.DataDir)
	if c.Storage.ActiveConfig == "" {
		c.Storage.ActiveConfig = filepath.Join(c.Storage.DataDir, "config.yaml")
	} else {
		c.Storage.ActiveConfig = resolvePath(baseDir, c.Storage.ActiveConfig)
	}
	c.Server.WebDir = resolvePath(baseDir, c.Server.WebDir)
	c.Auth.AdminTokenFile = resolvePath(baseDir, c.Auth.AdminTokenFile)
	c.Mihomo.SecretFile = resolvePath(baseDir, c.Mihomo.SecretFile)
	if strings.ContainsRune(c.Mihomo.Binary, filepath.Separator) {
		c.Mihomo.Binary = resolvePath(baseDir, c.Mihomo.Binary)
	}
	c.Mihomo.ExternalUI = resolvePath(baseDir, c.Mihomo.ExternalUI)
	for i := range c.Subscription.Sources {
		c.Subscription.Sources[i].URLFile = resolvePath(baseDir, c.Subscription.Sources[i].URLFile)
	}
}

func resolvePath(baseDir, value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(filepath.Join(baseDir, value))
}

func (c Config) StateFile() string {
	return filepath.Join(c.Storage.DataDir, "state.json")
}

func (c Config) BackupDir() string {
	return filepath.Join(c.Storage.DataDir, "backups")
}

func (c Config) Validate() error {
	var errs []error
	if err := validateListen(c.Server.Listen, c.Auth); err != nil {
		errs = append(errs, err)
	}
	if c.Server.ReadTimeout.Duration <= 0 || c.Server.WriteTimeout.Duration <= 0 {
		errs = append(errs, errors.New("server timeouts must be positive"))
	}
	if c.Storage.DataDir == "" || c.Storage.ActiveConfig == "" {
		errs = append(errs, errors.New("storage data_dir and active_config are required"))
	}
	if c.Storage.BackupRetention < 1 || c.Storage.BackupRetention > 1000 {
		errs = append(errs, errors.New("storage backup_retention must be between 1 and 1000"))
	}
	if c.Storage.HistoryRetention < 1 || c.Storage.HistoryRetention > 10000 {
		errs = append(errs, errors.New("storage history_retention must be between 1 and 10000"))
	}
	if c.Mihomo.Binary == "" {
		errs = append(errs, errors.New("mihomo binary is required"))
	}
	if err := validateHTTPURL("mihomo api_url", c.Mihomo.APIURL); err != nil {
		errs = append(errs, err)
	}
	if c.Mihomo.ValidationTimeout.Duration <= 0 || c.Mihomo.RequestTimeout.Duration <= 0 {
		errs = append(errs, errors.New("mihomo timeouts must be positive"))
	}
	if c.Mihomo.EnforceRuntimeSettings {
		host, _, err := net.SplitHostPort(c.Mihomo.ControllerListen)
		if err != nil {
			errs = append(errs, errors.New("mihomo controller_listen must be a host:port address"))
		} else if !isLoopbackHost(host) && c.Mihomo.SecretFile == "" {
			errs = append(errs, errors.New("a remote Mihomo controller_listen requires mihomo.secret_file"))
		}
		if c.Mihomo.MixedPort < 1 || c.Mihomo.MixedPort > 65535 {
			errs = append(errs, errors.New("mihomo mixed_port must be between 1 and 65535"))
		}
	}
	if c.Update.Enabled && c.Update.Interval.Duration <= 0 {
		errs = append(errs, errors.New("update interval must be positive when updates are enabled"))
	}
	if c.Update.HealthDelay.Duration < 0 {
		errs = append(errs, errors.New("update health_delay cannot be negative"))
	}
	if c.Subscription.MaxBytes < 1024 || c.Subscription.MaxBytes > 128<<20 {
		errs = append(errs, errors.New("subscription max_bytes must be between 1 KiB and 128 MiB"))
	}
	if c.Subscription.RequestTimeout.Duration <= 0 {
		errs = append(errs, errors.New("subscription request_timeout must be positive"))
	}
	enabled := 0
	names := make(map[string]struct{}, len(c.Subscription.Sources))
	for _, source := range c.Subscription.Sources {
		if !source.IsEnabled() {
			continue
		}
		enabled++
		name := strings.TrimSpace(source.Name)
		if name == "" {
			errs = append(errs, errors.New("enabled subscription name is required"))
		} else if _, exists := names[name]; exists {
			errs = append(errs, fmt.Errorf("duplicate subscription name %q", name))
		}
		names[name] = struct{}{}
		if source.URLFile == "" {
			errs = append(errs, fmt.Errorf("subscription %q requires url_file", name))
		}
	}
	if enabled == 0 {
		errs = append(errs, errors.New("at least one subscription must be enabled"))
	}
	if c.Converter.Enabled {
		if err := validateConverter(c.Converter); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func validateListen(listen string, auth AuthConfig) error {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("invalid server listen address: %w", err)
	}
	if auth.AdminTokenFile != "" || auth.AllowUnauthenticatedRemote || isLoopbackHost(host) {
		return nil
	}
	return errors.New("remote listener requires auth.admin_token_file or explicit allow_unauthenticated_remote")
}

func validateHTTPURL(name, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute HTTP(S) URL", name)
	}
	return nil
}

func validateConverter(converter ConverterConfig) error {
	if err := validateHTTPURL("converter api_url", converter.APIURL); err != nil {
		return err
	}
	if converter.AllowRemote {
		return nil
	}
	parsed, _ := url.Parse(converter.APIURL)
	host := parsed.Hostname()
	if isLoopbackHost(host) || net.ParseIP(host).IsPrivate() || !strings.Contains(host, ".") {
		return nil
	}
	return errors.New("remote converter is disabled; use a loopback/private endpoint or set converter.allow_remote")
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func ReadSecret(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("secret path must be a regular file")
	}
	if info.Size() > maxSecretBytes {
		return "", errors.New("secret file exceeds 64 KiB")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
	}
	secret := strings.TrimSuffix(strings.TrimSuffix(string(value), "\n"), "\r")
	if strings.TrimSpace(secret) == "" {
		return "", errors.New("secret file is empty")
	}
	if strings.ContainsAny(secret, "\r\n") {
		return "", errors.New("secret file must contain exactly one line")
	}
	return secret, nil
}
