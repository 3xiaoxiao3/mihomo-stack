package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/3xiaoxiao3/mihomo-stack/internal/api"
	"github.com/3xiaoxiao3/mihomo-stack/internal/auth"
	"github.com/3xiaoxiao3/mihomo-stack/internal/candidate"
	"github.com/3xiaoxiao3/mihomo-stack/internal/config"
	"github.com/3xiaoxiao3/mihomo-stack/internal/mihomo"
	"github.com/3xiaoxiao3/mihomo-stack/internal/state"
	"github.com/3xiaoxiao3/mihomo-stack/internal/subscription"
	"github.com/3xiaoxiao3/mihomo-stack/internal/updater"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type runtimeComponents struct {
	config     config.Config
	store      *state.Store
	controller *mihomo.Client
	updates    *updater.Service
	auth       *auth.Manager
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "validate":
		return validate(args[1:])
	case "update":
		return update(args[1:])
	case "doctor":
		return doctor(args[1:])
	case "version":
		fmt.Printf("mihomo-guardian %s (commit %s, built %s)\n", version, commit, date)
		return nil
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText())
	}
}

func serve(args []string) error {
	flags := newFlagSet("serve")
	configPath := flags.String("config", defaultConfigPath(), "path to Guardian YAML configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	components, err := buildRuntime(*configPath)
	if err != nil {
		return err
	}
	logger := newLogger()
	webDir := components.config.Server.WebDir
	if override := os.Getenv("GUARDIAN_WEB_DIR"); override != "" {
		webDir = override
	}
	sourceCount := 0
	for _, source := range components.config.Subscription.Sources {
		if source.IsEnabled() {
			sourceCount++
		}
	}
	handler := api.New(api.Options{
		Version:          version,
		WebDir:           webDir,
		DashboardURL:     components.config.UI.DashboardURL,
		ActiveConfigPath: components.config.Storage.ActiveConfig,
		DataDir:          components.config.Storage.DataDir,
		SchedulerEnabled: components.config.Update.Enabled,
		UpdateInterval:   components.config.Update.Interval.Duration,
		SourceCount:      sourceCount,
	}, components.auth, components.updates, components.store, components.controller, logger).Handler()

	server := &http.Server{
		Addr:         components.config.Server.Listen,
		Handler:      handler,
		ReadTimeout:  components.config.Server.ReadTimeout.Duration,
		WriteTimeout: components.config.Server.WriteTimeout.Duration,
		IdleTimeout:  60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if components.config.Update.Enabled {
		go scheduleUpdates(ctx, components, logger)
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("Guardian listening", "address", server.Addr, "version", version)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func validate(args []string) error {
	flags := newFlagSet("validate")
	configPath := flags.String("config", defaultConfigPath(), "path to Guardian YAML configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	validator := mihomo.CommandValidator{
		Binary:  cfg.Mihomo.Binary,
		DataDir: cfg.Storage.DataDir,
		Timeout: cfg.Mihomo.ValidationTimeout.Duration,
	}
	if err := validator.Validate(context.Background(), cfg.Storage.ActiveConfig); err != nil {
		return err
	}
	fmt.Println("configuration is valid")
	return nil
}

func update(args []string) error {
	flags := newFlagSet("update")
	configPath := flags.String("config", defaultConfigPath(), "path to Guardian YAML configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	components, err := buildRuntime(*configPath)
	if err != nil {
		return err
	}
	record, err := components.updates.Update(context.Background(), "cli")
	_ = json.NewEncoder(os.Stdout).Encode(record)
	return err
}

func doctor(args []string) error {
	flags := newFlagSet("doctor")
	configPath := flags.String("config", defaultConfigPath(), "path to Guardian YAML configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	components, err := buildRuntime(*configPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), components.config.Mihomo.RequestTimeout.Duration)
	defer cancel()
	mihomoVersion, healthErr := components.controller.Version(ctx)
	report := map[string]any{
		"configuration":          "valid",
		"data_dir":               components.config.Storage.DataDir,
		"active_config":          components.config.Storage.ActiveConfig,
		"mihomo_online":          healthErr == nil,
		"mihomo_version":         mihomoVersion,
		"authentication_enabled": components.auth.Enabled(),
	}
	if healthErr != nil {
		report["mihomo_error"] = "controller is unreachable or unhealthy"
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return err
	}
	return healthErr
}

func buildRuntime(configPath string) (*runtimeComponents, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	adminToken, err := config.ReadSecret(cfg.Auth.AdminTokenFile)
	if err != nil {
		return nil, fmt.Errorf("load administrator token: %w", err)
	}
	if adminToken != "" && len(adminToken) < 16 {
		return nil, errors.New("administrator token must contain at least 16 characters")
	}
	controllerSecret, err := config.ReadSecret(cfg.Mihomo.SecretFile)
	if err != nil {
		return nil, fmt.Errorf("load Mihomo controller secret: %w", err)
	}
	store, err := state.Open(cfg.StateFile(), cfg.Storage.HistoryRetention)
	if err != nil {
		return nil, err
	}
	controller, err := mihomo.NewClient(cfg.Mihomo.APIURL, controllerSecret, cfg.Mihomo.RequestTimeout.Duration)
	if err != nil {
		return nil, err
	}
	subscriptionBuilder := subscription.NewBuilder(cfg.Subscription, cfg.Converter)
	builder := candidate.NewOverlay(subscriptionBuilder, candidate.RuntimeSettings{
		Enabled:            cfg.Mihomo.EnforceRuntimeSettings,
		MixedPort:          cfg.Mihomo.MixedPort,
		AllowLAN:           cfg.Mihomo.AllowLAN,
		ExternalController: cfg.Mihomo.ControllerListen,
		ExternalUI:         cfg.Mihomo.ExternalUI,
		Secret:             controllerSecret,
	})
	validator := mihomo.CommandValidator{
		Binary:  cfg.Mihomo.Binary,
		DataDir: cfg.Storage.DataDir,
		Timeout: cfg.Mihomo.ValidationTimeout.Duration,
	}
	updates := updater.New(updater.Config{
		ActiveConfig:    cfg.Storage.ActiveConfig,
		BackupDir:       cfg.BackupDir(),
		BackupRetention: cfg.Storage.BackupRetention,
		HealthDelay:     cfg.Update.HealthDelay.Duration,
		RollbackTimeout: 2*cfg.Mihomo.RequestTimeout.Duration + cfg.Update.HealthDelay.Duration,
	}, builder, validator, controller, store)
	return &runtimeComponents{
		config:     cfg,
		store:      store,
		controller: controller,
		updates:    updates,
		auth:       auth.New(adminToken, 12*time.Hour),
	}, nil
}

func scheduleUpdates(ctx context.Context, components *runtimeComponents, logger *slog.Logger) {
	if components.config.Update.RunOnStart {
		record, err := components.updates.Update(ctx, "startup")
		logUpdateResult(logger, record, err)
	}
	ticker := time.NewTicker(components.config.Update.Interval.Duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			record, err := components.updates.Update(ctx, "schedule")
			logUpdateResult(logger, record, err)
		}
	}
}

func logUpdateResult(logger *slog.Logger, record state.Record, err error) {
	if err != nil {
		logger.Error("scheduled configuration update failed", "stage", record.Stage, "rolled_back", record.RolledBack, "error", err)
		return
	}
	logger.Info("scheduled configuration update completed", "id", record.ID)
}

func defaultConfigPath() string {
	if path := os.Getenv("GUARDIAN_CONFIG"); path != "" {
		return path
	}
	return "config.yaml"
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	return flags
}

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("GUARDIAN_LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

func usageError() error {
	return errors.New(usageText())
}

func printUsage() {
	fmt.Print(usageText())
}

func usageText() string {
	return `Usage: mihomo-guardian <command> [options]

Commands:
  serve      Run the scheduler, API, and management UI
  validate   Validate the current active config with Mihomo
  update     Run one subscription update transaction
  doctor     Check configuration, secrets, and Mihomo connectivity
  version    Print build version information

Each command accepts -config <path>. GUARDIAN_CONFIG changes its default.
`
}
