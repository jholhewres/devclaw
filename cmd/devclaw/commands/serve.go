package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/auth/profiles"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/discord"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/slack"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/telegram"
	"github.com/jholhewres/devclaw/pkg/devclaw/channels/whatsapp"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
	"github.com/jholhewres/devclaw/pkg/devclaw/gateway"
	"github.com/jholhewres/devclaw/pkg/devclaw/media"
	"github.com/jholhewres/devclaw/pkg/devclaw/paths"
	"github.com/jholhewres/devclaw/pkg/devclaw/plugins"
	devtls "github.com/jholhewres/devclaw/pkg/devclaw/tls"
	"github.com/jholhewres/devclaw/pkg/devclaw/updater"
	"github.com/jholhewres/devclaw/pkg/devclaw/webui"
	"github.com/spf13/cobra"
)

// newServeCmd creates the `devclaw serve` command that starts the daemon.
func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the daemon with messaging channels",
		Long: `Start DevClaw as a daemon service, connecting to enabled
channels (WhatsApp, Telegram) and processing messages.

Examples:
  devclaw serve
  devclaw serve --channel whatsapp
  devclaw serve --config ./config.yaml`,
		RunE: runServe,
	}

	cmd.Flags().StringSlice("channel", nil, "channels to enable (whatsapp, telegram, discord, slack)")
	return cmd
}

func runServe(cmd *cobra.Command, _ []string) error {
	// ── Ensure state directories exist ──
	if err := paths.EnsureStateDirs(); err != nil {
		return fmt.Errorf("failed to create state directories: %w", err)
	}
	if err := paths.EnsureWorkspaceTemplates(); err != nil {
		return fmt.Errorf("failed to create workspace templates: %w", err)
	}

	// ── Load config ──
	cfg, configPath, err := resolveConfig(cmd)
	if err != nil {
		// No config? Start in web setup mode.
		return runWebSetupMode()
	}

	// ── Apply PORT env override ──
	// The PM2 ecosystem.config.js passes the user-specified port via the PORT
	// env var, but the config file may still contain the default address.
	// Let the env var win so that --port at install time is honoured.
	if envPort := os.Getenv("PORT"); envPort != "" {
		cfg.WebUI.Address = ":" + strings.TrimLeft(envPort, ":")
	}

	// ── Configure logger ──
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	logLevel := slog.LevelInfo
	if verbose || cfg.Logging.Level == "debug" {
		logLevel = slog.LevelDebug
	}

	var handler slog.Handler
	if cfg.Logging.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	logger := slog.New(handler)

	// ── Resolve secrets ──
	// Audit BEFORE resolving — checks the raw config values for hardcoded keys.
	copilot.AuditSecrets(cfg, logger)
	// Resolve from vault → keyring → env → config.
	// Returns unlocked vault (if available) for agent vault tools.
	vault := copilot.ResolveAPIKey(cfg, logger)

	// Re-resolve secrets now that vault has injected env vars.
	// The initial load ran before vault was unlocked, so channel tokens
	// like ${TELEGRAM_BOT_TOKEN} were not yet in the environment.
	copilot.ResolveSecrets(cfg)

	// ── Run startup verification ──
	verifier := copilot.NewStartupVerifier(cfg, vault, logger)
	startupReport := verifier.RunAll()
	verifier.PrintReport(startupReport)
	if !startupReport.Healthy {
		logger.Error("startup verification failed, some required checks did not pass")
		return fmt.Errorf("startup verification failed")
	}

	// ── Create assistant ──
	assistant := copilot.New(cfg, logger)
	if vault != nil {
		assistant.SetVault(vault)
	}

	// ── Create context ──
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Register channels ──
	channelFilter, _ := cmd.Flags().GetStringSlice("channel")

	// WhatsApp (core channel — supports multiple instances).
	waInstances := make(map[string]*whatsapp.WhatsApp)
	if shouldEnable("whatsapp", channelFilter, true) {
		for instanceID, waCfg := range cfg.Channels.WhatsAppAll() {
			if err := channels.ValidateInstanceID(instanceID); err != nil {
				logger.Error("WhatsApp: invalid instance ID, skipping", "instance", instanceID, "error", err)
				continue
			}
			waCfg.InstanceID = instanceID

			// Use the main devclaw database for WhatsApp sessions.
			if waCfg.DatabasePath == "" && cfg.Database.Path != "" {
				waCfg.DatabasePath = cfg.Database.Path
			}

			// Inherit global access config into WhatsApp when the channel-specific
			// access config is not explicitly set.
			inheritWhatsAppAccess(&waCfg, cfg)

			wa := whatsapp.New(waCfg, logger)
			if err := assistant.ChannelManager().Register(wa); err != nil {
				logger.Error("WhatsApp register failed", "instance", instanceID, "error", err)
				continue
			}
			waInstances[instanceID] = wa
			label := "WhatsApp"
			if instanceID != "" {
				label = "WhatsApp:" + instanceID
			}
			logger.Info(label + " channel registered")
		}
	}

	// Telegram (core channel — supports multiple instances).
	if shouldEnable("telegram", channelFilter, true) {
		for instanceID, tgCfg := range cfg.Channels.TelegramAll() {
			if tgCfg.Token == "" {
				continue
			}
			if err := channels.ValidateInstanceID(instanceID); err != nil {
				logger.Error("Telegram: invalid instance ID, skipping", "instance", instanceID, "error", err)
				continue
			}
			tgCfg.InstanceID = instanceID
			tg := telegram.New(tgCfg, logger)
			if err := assistant.ChannelManager().Register(tg); err != nil {
				logger.Error("Telegram register failed", "instance", instanceID, "error", err)
				continue
			}
			label := "Telegram"
			if instanceID != "" {
				label = "Telegram:" + instanceID
			}
			logger.Info(label + " channel registered")
		}
	}

	// Discord (core channel — supports multiple instances).
	if shouldEnable("discord", channelFilter, false) {
		for instanceID, dcCfg := range cfg.Channels.DiscordAll() {
			if dcCfg.Token == "" {
				continue
			}
			if err := channels.ValidateInstanceID(instanceID); err != nil {
				logger.Error("Discord: invalid instance ID, skipping", "instance", instanceID, "error", err)
				continue
			}
			dcCfg.InstanceID = instanceID
			dc := discord.New(dcCfg, logger)
			if err := assistant.ChannelManager().Register(dc); err != nil {
				logger.Error("Discord register failed", "instance", instanceID, "error", err)
				continue
			}
			label := "Discord"
			if instanceID != "" {
				label = "Discord:" + instanceID
			}
			logger.Info(label + " channel registered")
		}
	}

	// Slack (core channel — supports multiple instances).
	if shouldEnable("slack", channelFilter, false) {
		for instanceID, slCfg := range cfg.Channels.SlackAll() {
			if slCfg.BotToken == "" {
				continue
			}
			if err := channels.ValidateInstanceID(instanceID); err != nil {
				logger.Error("Slack: invalid instance ID, skipping", "instance", instanceID, "error", err)
				continue
			}
			slCfg.InstanceID = instanceID
			sl := slack.New(slCfg, logger)
			if err := assistant.ChannelManager().Register(sl); err != nil {
				logger.Error("Slack register failed", "instance", instanceID, "error", err)
				continue
			}
			label := "Slack"
			if instanceID != "" {
				label = "Slack:" + instanceID
			}
			logger.Info(label + " channel registered")
		}
	}

	// Load plugins (YAML-based + legacy .so).
	pluginLoader := plugins.NewLoader(cfg.Plugins, logger)
	var pluginVault plugins.VaultReader
	if vault != nil {
		pluginVault = vault
	}
	if err := pluginLoader.LoadAll(ctx, pluginVault); err != nil {
		logger.Error("failed to load plugins", "error", err)
	} else if pluginLoader.Count() > 0 {
		// Register native channel plugins with the channel manager.
		if err := pluginLoader.RegisterChannels(assistant.ChannelManager()); err != nil {
			logger.Error("failed to register plugin channels", "error", err)
		}
	}

	// Create plugin registry and wire it into the assistant.
	pluginRegistry := plugins.NewRegistry(logger)
	pluginRegistry.AddLoadedPlugins(pluginLoader)
	assistant.SetPluginRegistry(pluginRegistry)

	// ── Resolve TLS certificates ──
	tlsEnabled := cfg.WebUI.TLS.Enabled || cfg.Gateway.TLS.Enabled
	if tlsEnabled {
		// Resolve default cert/key paths if not set.
		certPath := cfg.WebUI.TLS.CertPath
		if certPath == "" {
			certPath = cfg.Gateway.TLS.CertPath
		}
		if certPath == "" {
			certPath = filepath.Join(paths.ResolveDataDir(), "tls", "devclaw-cert.pem")
		}
		keyPath := cfg.WebUI.TLS.KeyPath
		if keyPath == "" {
			keyPath = cfg.Gateway.TLS.KeyPath
		}
		if keyPath == "" {
			keyPath = filepath.Join(paths.ResolveDataDir(), "tls", "devclaw-key.pem")
		}

		// Share paths to both configs.
		if cfg.WebUI.TLS.Enabled {
			cfg.WebUI.TLS.CertPath = certPath
			cfg.WebUI.TLS.KeyPath = keyPath
		}
		if cfg.Gateway.TLS.Enabled {
			cfg.Gateway.TLS.CertPath = certPath
			cfg.Gateway.TLS.KeyPath = keyPath
		}

		// Auto-generate if configured.
		autoGen := cfg.WebUI.TLS.AutoGenerate || cfg.Gateway.TLS.AutoGenerate
		if autoGen {
			if err := devtls.EnsureSelfSignedCert(certPath, keyPath, logger); err != nil {
				logger.Error("failed to generate TLS certificates", "error", err)
				return fmt.Errorf("TLS certificate generation failed: %w", err)
			}
		}

		// Log fingerprint.
		if fp, err := devtls.CertFingerprint(certPath); err == nil {
			logger.Info("TLS certificate fingerprint", "sha256", fp)
		}
	}

	// ── Start Web UI first (independent of channels) ──
	var webServer *webui.Server
	var adapter *webui.AssistantAdapter
	if cfg.WebUI.Enabled {
		adapter = buildWebUIAdapter(ctx, assistant, cfg, waInstances, configPath, logger)
		webServer = webui.New(cfg.WebUI, adapter, logger)

		// Register restart callback
		webServer.OnRestartRequested(func() error {
			logger.Info("restart requested via web UI")

			// Graceful shutdown
			cancel()

			// Wait a moment for graceful shutdown
			time.Sleep(1 * time.Second)

			// Execute restart
			if err := reloadProcess(); err != nil {
				logger.Error("restart failed", "error", err)
				return err
			}
			return nil
		})

		// Initialize OAuth handlers for OAuth providers (gemini, chatgpt, qwen, minimax, google-*)
		oauthHandlers, err := webui.NewOAuthHandlers(paths.ResolveDataDir(), logger)
		if err != nil {
			logger.Warn("failed to initialize OAuth handlers", "error", err)
		} else {
			// Wire up Hub mode if configured (from YAML config)
			hubConfigured := false
			if cfg.OAuthHub.Mode == "hub" && cfg.OAuthHub.HubURL != "" {
				apiKey := cfg.OAuthHub.APIKey
				if apiKey == "" {
					envVar := cfg.OAuthHub.APIKeyEnvVar
					if envVar == "" {
						envVar = "OAUTH_HUB_API_KEY"
					}
					apiKey = os.Getenv(envVar)
				}
				if apiKey != "" {
					oauthHandlers.SetHubConfig(&webui.HubConfig{
						Enabled: true,
						HubURL:  cfg.OAuthHub.HubURL,
						APIKey:  apiKey,
					})
					os.Setenv("DEVCLAW_HUB_API_KEY", apiKey)
					logger.Info("OAuth Hub mode enabled for WebUI", "hub_url", cfg.OAuthHub.HubURL)
					hubConfigured = true
				}
			}
			// Fallback: load Hub config from hub_config.json (current directory) + vault env var
			if !hubConfigured {
				apiKey := os.Getenv("DEVCLAW_HUB_API_KEY") // Set by vault InjectProviderKeys
				hubURL := ""

				// Read hub_url from local hub_config.json
				if data, err := os.ReadFile(filepath.Join(paths.ResolveStateDir(), "hub_config.json")); err == nil {
					var savedHub struct {
						HubURL string `json:"hub_url"`
						APIKey string `json:"api_key"`
					}
					if json.Unmarshal(data, &savedHub) == nil {
						hubURL = savedHub.HubURL
						// Fallback: use api_key from file if not in vault/env
						if apiKey == "" && savedHub.APIKey != "" {
							apiKey = savedHub.APIKey
						}
					}
				}

				if hubURL != "" && apiKey != "" {
					oauthHandlers.SetHubConfig(&webui.HubConfig{
						Enabled: true,
						HubURL:  hubURL,
						APIKey:  apiKey,
					})
					os.Setenv("DEVCLAW_HUB_API_KEY", apiKey)
					logger.Info("OAuth Hub loaded from hub_config.json + vault", "hub_url", hubURL)
				}
			}
			// Wire up skill installer for Hub skill downloads
			oauthHandlers.SetSkillInstaller(func(name, content string) error {
				dir := filepath.Join(paths.ResolveSkillsDir(), name)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("failed to create skill directory: %w", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
					return fmt.Errorf("failed to write skill file: %w", err)
				}
				if reg := assistant.SkillRegistry(); reg != nil {
					_, _ = reg.Reload(context.Background())
				}
				return nil
			})

			// Wire up bundle installer for multi-file skill installs (gator-hub + references)
			oauthHandlers.SetSkillBundleInstaller(func(name string, files map[string]string) error {
				for relPath, content := range files {
					fullPath := filepath.Join(paths.ResolveSkillsDir(), name, relPath)
					if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
						return fmt.Errorf("failed to create directory for %s: %w", relPath, err)
					}
					if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
						return fmt.Errorf("failed to write %s: %w", relPath, err)
					}
				}
				if reg := assistant.SkillRegistry(); reg != nil {
					_, _ = reg.Reload(context.Background())
				}
				return nil
			})

			// Wire up reference remover for disconnecting services
			oauthHandlers.SetSkillReferenceRemover(func(skillName, refPath string) error {
				fullPath := filepath.Join(paths.ResolveSkillsDir(), skillName, refPath)
				if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("failed to remove %s: %w", refPath, err)
				}
				if reg := assistant.SkillRegistry(); reg != nil {
					_, _ = reg.Reload(context.Background())
				}
				return nil
			})

			// Wire up vault-based secret storage for Hub API key
			if vault != nil && vault.IsUnlocked() {
				oauthHandlers.SetSecretSaver(func(name, value string) error {
					if err := vault.Set(name, value); err != nil {
						return fmt.Errorf("failed to store %s in vault: %w", name, err)
					}
					os.Setenv(name, value)
					return nil
				})
			}

			// Wire up Hub URL persistence to YAML config
			oauthHandlers.SetHubURLSaver(func(hubURL string) error {
				cfg.OAuthHub.HubURL = hubURL
				cfg.OAuthHub.Mode = "hub"
				savePath := configPath
				if savePath == "" {
					savePath = "config.yaml"
				}
				return copilot.SaveConfigToFile(cfg, savePath)
			})

			webServer.SetOAuthHandlers(oauthHandlers)
		}

		// Wire version and auto-update checker
		webServer.SetVersion(cmd.Root().Version)
		if cfg.Update.Enabled {
			assetsURL := cfg.Update.AssetsURL
			if assetsURL == "" {
				assetsURL = "https://github.com/jholhewres/devclaw/releases"
			}
			checkInterval := 1 * time.Hour
			if cfg.Update.CheckInterval != "" {
				if parsed, err := time.ParseDuration(cfg.Update.CheckInterval); err == nil {
					checkInterval = parsed
				}
			}
			checker := updater.NewChecker(cmd.Root().Version, assetsURL, checkInterval, logger)
			checker.Start(ctx)
			webServer.SetUpdateChecker(checker)
			webServer.OnUpdateRequested(func() error {
				logger.Info("update requested via web UI")
				inst := updater.NewInstaller(assetsURL, logger)
				return inst.InstallAndRestart()
			})
		}

		if err := webServer.Start(ctx); err != nil {
			logger.Error("failed to start web UI", "error", err)
		} else {
			logger.Info("web UI running", "address", cfg.WebUI.Address)
		}
	}

	// ── Start assistant (channels, scheduler, heartbeat, etc.) ──
	if err := assistant.Start(ctx); err != nil {
		logger.Warn("assistant started with warnings", "error", err)
		scheme := "http"
		if cfg.WebUI.TLS.Enabled {
			scheme = "https"
		}
		logger.Info("channels pending — connect via web UI", "url", fmt.Sprintf("%s://localhost%s/channels", scheme, cfg.WebUI.Address))
	}

	// ── Start gateway if enabled ──
	var gw *gateway.Gateway
	if cfg.Gateway.Enabled {
		gw = gateway.New(assistant, cfg.Gateway, logger)
		if err := gw.Start(ctx); err != nil {
			logger.Error("failed to start gateway", "error", err)
		} else {
			logger.Info("gateway running", "address", cfg.Gateway.Address)
		}
	}

	// ── Wire domain config to WebUI adapter ──
	if webServer != nil {
		wireDomainAdapter(adapter, cfg, webServer, configPath)
	}

	// ── Wire webhook management to WebUI adapter ──
	if webServer != nil {
		wireWebhookAdapter(adapter, gw)
	}

	// ── Wire media service to WebUI ──
	if webServer != nil {
		wireMediaAdapter(webServer, assistant, logger)
	}

	// ── Start config watcher for hot-reload ──
	if configPath != "" {
		watcher := copilot.NewConfigWatcher(
			configPath,
			5*time.Second,
			assistant.ApplyConfigUpdate,
			logger,
		)
		go watcher.Start(ctx)
		logger.Info("config watcher started", "path", configPath)
	}

	// ── Wait for shutdown ──
	logger.Info("DevClaw Copilot running. Press Ctrl+C to stop.",
		"name", cfg.Name,
		"trigger", cfg.Trigger,
		"policy", cfg.Access.DefaultPolicy,
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutdown signal received, stopping...")

	// Graceful shutdown with timeout.
	done := make(chan struct{})
	go func() {
		pluginLoader.Shutdown()
		if gw != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = gw.Stop(shutdownCtx)
			cancel()
		}
		if webServer != nil {
			webServer.Stop()
		}
		assistant.Stop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("shutdown complete")
	case <-time.After(10 * time.Second):
		logger.Warn("shutdown timed out after 10s, forcing exit")
	}

	return nil
}

// resolveConfig loads config from file, runs interactive setup if missing.
// Returns (config, configPath, error). configPath is empty if config came from discovery without a known path.
func resolveConfig(cmd *cobra.Command) (*copilot.Config, string, error) {
	configPath, _ := cmd.Root().PersistentFlags().GetString("config")

	// Try explicit path first.
	if configPath != "" {
		cfg, err := copilot.LoadConfigFromFile(configPath)
		if err != nil {
			return nil, "", fmt.Errorf("loading config: %w", err)
		}
		return cfg, configPath, nil
	}

	// Auto-discover config file.
	if found := copilot.FindConfigFile(); found != "" {
		cfg, err := copilot.LoadConfigFromFile(found)
		if err != nil {
			return nil, "", fmt.Errorf("loading config from %s: %w", found, err)
		}
		slog.Info("config loaded", "path", found)
		return cfg, found, nil
	}

	// No config file found — the caller (runServe) will fall back to
	// web setup mode. CLI setup is still available via `copilot setup`.
	return nil, "", fmt.Errorf("no configuration file found")
}

// runWebSetupMode starts a minimal webui server in setup-only mode.
// Blocks until the setup wizard completes or the user cancels.
// After setup, it automatically reloads the process.
func runWebSetupMode() error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Get server IP for display
	serverIP := "localhost"
	if ip := getServerIP(); ip != "" {
		serverIP = ip
	}

	// Respect PORT env var (set by PM2 ecosystem config via --port install flag).
	setupPort := "47716"
	if envPort := os.Getenv("PORT"); envPort != "" {
		setupPort = strings.TrimLeft(envPort, ":")
	}

	fmt.Println()
	fmt.Println("  ╭──────────────────────────────────────────────╮")
	fmt.Println("  │  🐾 DevClaw — First Run Setup                 │")
	fmt.Println("  │                                              │")
	fmt.Println("  │  No config.yaml found.                       │")
	fmt.Println("  │  Starting web setup wizard...                │")
	fmt.Println("  │                                              │")
	fmt.Printf("  │  Open:  http://%s:%s/setup          │\n", serverIP, setupPort)
	fmt.Println("  ╰──────────────────────────────────────────────╯")
	fmt.Println()

	setupDone := make(chan struct{})

	// Start a webui server in setup-only mode (no assistant needed).
	webuiCfg := webui.Config{
		Enabled: true,
		Address: ":" + setupPort,
	}
	webServer := webui.New(webuiCfg, nil, logger)
	webServer.SetSetupMode(true)
	webServer.OnSetupDone(func() {
		close(setupDone)
	})
	webServer.OnVaultInit(func(password string, secrets map[string]string) error {
		vault := copilot.NewVault(copilot.VaultFile)
		if vault.Exists() {
			// Vault already exists — unlock and add secrets.
			if err := vault.Unlock(password); err != nil {
				return fmt.Errorf("failed to unlock existing vault: %w", err)
			}
		} else {
			if err := vault.Create(password); err != nil {
				return fmt.Errorf("failed to create vault: %w", err)
			}
		}
		for name, value := range secrets {
			if err := vault.Set(name, value); err != nil {
				return fmt.Errorf("failed to store %s in vault: %w", name, err)
			}
		}
		return nil
	})

	if err := webServer.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start setup server: %w", err)
	}

	// Wait for setup completion or interrupt.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-setupDone:
		webServer.Stop()
		fmt.Println()
		fmt.Println("Setup complete! config.yaml saved.")
		fmt.Println("Reloading...")
		// Small delay to ensure server is fully stopped
		time.Sleep(500 * time.Millisecond)
		return reloadProcess()
	case <-sigChan:
		webServer.Stop()
		return nil
	}
}

// reloadProcess replaces the current process with a new instance.
// This is used after setup to start the service with the new config.
func reloadProcess() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Get the original arguments (skip the program name)
	args := os.Args[1:]

	// Replace current process with a new instance
	err = syscall.Exec(executable, append([]string{executable}, args...), os.Environ())
	if err != nil {
		return fmt.Errorf("failed to reload process: %w", err)
	}

	return nil
}

// getServerIP returns the first non-loopback IP address of the server.
func getServerIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

// inheritWhatsAppAccess copies global access config fields into a WhatsApp config
// when the channel-specific fields are not explicitly set.
func inheritWhatsAppAccess(waCfg *whatsapp.Config, cfg *copilot.Config) {
	if waCfg.Access.DefaultPolicy == "" && string(cfg.Access.DefaultPolicy) != "" {
		waCfg.Access.DefaultPolicy = string(cfg.Access.DefaultPolicy)
	}
	if len(waCfg.Access.Owners) == 0 && len(cfg.Access.Owners) > 0 {
		waCfg.Access.Owners = cfg.Access.Owners
	}
	if len(waCfg.Access.Admins) == 0 && len(cfg.Access.Admins) > 0 {
		waCfg.Access.Admins = cfg.Access.Admins
	}
	if len(waCfg.Access.AllowedUsers) == 0 && len(cfg.Access.AllowedUsers) > 0 {
		waCfg.Access.AllowedUsers = cfg.Access.AllowedUsers
	}
	if len(waCfg.Access.BlockedUsers) == 0 && len(cfg.Access.BlockedUsers) > 0 {
		waCfg.Access.BlockedUsers = cfg.Access.BlockedUsers
	}
	if len(waCfg.Access.AllowedGroups) == 0 && len(cfg.Access.AllowedGroups) > 0 {
		waCfg.Access.AllowedGroups = cfg.Access.AllowedGroups
	}
	if len(waCfg.Access.BlockedGroups) == 0 && len(cfg.Access.BlockedGroups) > 0 {
		waCfg.Access.BlockedGroups = cfg.Access.BlockedGroups
	}
	if waCfg.Access.PendingMessage == "" && cfg.Access.PendingMessage != "" {
		waCfg.Access.PendingMessage = cfg.Access.PendingMessage
	}
}

// whatsappStatusFromInstance builds a WhatsAppStatus from a WhatsApp instance.
func whatsappStatusFromInstance(wa *whatsapp.WhatsApp) webui.WhatsAppStatus {
	health := wa.Health()
	state := wa.GetState()
	status := webui.WhatsAppStatus{
		Connected:  wa.IsConnected(),
		State:      string(state),
		NeedsQR:    wa.NeedsQR(),
		ErrorCount: health.ErrorCount,
	}
	if jid, ok := health.Details["jid"].(string); ok {
		status.Phone = jid
	}
	if platform, ok := health.Details["platform"].(string); ok {
		status.Platform = platform
	}
	if attempts, ok := health.Details["reconnect_attempts"].(int); ok {
		status.ReconnectAttempts = attempts
	}
	switch state {
	case "connected":
		status.Message = "Connected"
	case "disconnected":
		status.Message = "Disconnected"
	case "connecting":
		status.Message = "Connecting..."
	case "reconnecting":
		status.Message = fmt.Sprintf("Reconnecting (attempt %d)...", status.ReconnectAttempts)
	case "waiting_qr":
		status.Message = "Waiting for QR code scan"
	case "banned":
		status.Message = "Account temporarily banned"
	case "logging_out":
		status.Message = "Logging out..."
	}
	return status
}

// bridgeWhatsAppQR bridges whatsapp.QREvent → webui.WhatsAppQREvent via a channel.
func bridgeWhatsAppQR(wa *whatsapp.WhatsApp) (chan webui.WhatsAppQREvent, func()) {
	ch, unsub := wa.SubscribeQR()
	out := make(chan webui.WhatsAppQREvent, 8)
	go func() {
		defer close(out)
		for evt := range ch {
			out <- webui.WhatsAppQREvent{
				Type:        evt.Type,
				Code:        evt.Code,
				Message:     evt.Message,
				ExpiresAt:   evt.ExpiresAt.Format(time.RFC3339),
				SecondsLeft: evt.SecondsLeft,
			}
		}
	}()
	return out, unsub
}

// shouldEnable checks if a channel should be enabled.
func shouldEnable(name string, filter []string, defaultEnabled bool) bool {
	if len(filter) == 0 {
		return defaultEnabled
	}
	for _, f := range filter {
		if f == name {
			return true
		}
	}
	return false
}

// anySliceToStringSlice converts []any to []string.
func anySliceToStringSlice(items []any) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// wireDomainAdapter connects domain/network config functions to the WebUI adapter.
func wireDomainAdapter(adapter *webui.AssistantAdapter, cfg *copilot.Config, ws *webui.Server, configPath string) {
	adapter.GetDomainConfigFn = func() webui.DomainConfigInfo {
		info := webui.DomainConfigInfo{
			WebuiAddress:   cfg.WebUI.Address,
			WebuiAuthToken: cfg.WebUI.AuthToken != "",

			GatewayEnabled:   cfg.Gateway.Enabled,
			GatewayAddress:   cfg.Gateway.Address,
			GatewayAuthToken: cfg.Gateway.AuthToken != "",
			CORSOrigins:      cfg.Gateway.CORSOrigins,

			WebuiTLSEnabled:   cfg.WebUI.TLS.Enabled,
			GatewayTLSEnabled: cfg.Gateway.TLS.Enabled,
			TLSCertPath:       cfg.WebUI.TLS.CertPath,
		}
		// Include fingerprint if TLS is active.
		if cfg.WebUI.TLS.Enabled || cfg.Gateway.TLS.Enabled {
			certPath := cfg.WebUI.TLS.CertPath
			if certPath == "" {
				certPath = cfg.Gateway.TLS.CertPath
			}
			if fp, err := devtls.CertFingerprint(certPath); err == nil {
				info.TLSFingerprint = fp
			}
		}
		return info
	}

	adapter.UpdateDomainConfigFn = func(update webui.DomainConfigUpdate) error {
		if update.WebuiAddress != nil {
			cfg.WebUI.Address = *update.WebuiAddress
		}
		if update.WebuiAuthToken != nil {
			cfg.WebUI.AuthToken = *update.WebuiAuthToken
			ws.SetAuthToken(*update.WebuiAuthToken)
		}
		if update.GatewayEnabled != nil {
			cfg.Gateway.Enabled = *update.GatewayEnabled
		}
		if update.GatewayAddress != nil {
			cfg.Gateway.Address = *update.GatewayAddress
		}
		if update.GatewayAuthToken != nil {
			cfg.Gateway.AuthToken = *update.GatewayAuthToken
		}
		if update.CORSOrigins != nil {
			cfg.Gateway.CORSOrigins = update.CORSOrigins
		}

		savePath := configPath
		if savePath == "" {
			savePath = "config.yaml"
		}
		return copilot.SaveConfigToFile(cfg, savePath)
	}
}

// wireWebhookAdapter connects webhook management functions to the WebUI adapter.
// Called after the gateway is created (may be nil if gateway is disabled).
func wireWebhookAdapter(adapter *webui.AssistantAdapter, gw *gateway.Gateway) {
	if gw == nil {
		adapter.ListWebhooksFn = func() []webui.WebhookInfo { return nil }
		adapter.CreateWebhookFn = func(string, []string) (webui.WebhookInfo, error) {
			return webui.WebhookInfo{}, fmt.Errorf("Gateway API is not enabled")
		}
		adapter.DeleteWebhookFn = func(string) error {
			return fmt.Errorf("Gateway API is not enabled")
		}
		adapter.ToggleWebhookFn = func(string, bool) error {
			return fmt.Errorf("Gateway API is not enabled")
		}
		adapter.GetValidWebhookEventsFn = func() []string { return gateway.ValidWebhookEvents }
		return
	}

	adapter.ListWebhooksFn = func() []webui.WebhookInfo {
		entries := gw.ListWebhooks()
		result := make([]webui.WebhookInfo, len(entries))
		for i, e := range entries {
			result[i] = webui.WebhookInfo{
				ID:        e.ID,
				URL:       e.URL,
				Events:    e.Events,
				Active:    e.Active,
				CreatedAt: e.CreatedAt,
			}
		}
		return result
	}
	adapter.CreateWebhookFn = func(url string, events []string) (webui.WebhookInfo, error) {
		entry, err := gw.AddWebhook(url, events)
		if err != nil {
			return webui.WebhookInfo{}, err
		}
		return webui.WebhookInfo{
			ID:        entry.ID,
			URL:       entry.URL,
			Events:    entry.Events,
			Active:    entry.Active,
			CreatedAt: entry.CreatedAt,
		}, nil
	}
	adapter.DeleteWebhookFn = func(id string) error {
		if !gw.DeleteWebhook(id) {
			return fmt.Errorf("webhook %q not found", id)
		}
		return nil
	}
	adapter.ToggleWebhookFn = func(id string, active bool) error {
		if !gw.ToggleWebhook(id, active) {
			return fmt.Errorf("webhook %q not found", id)
		}
		return nil
	}
	adapter.GetValidWebhookEventsFn = func() []string {
		return gateway.ValidWebhookEvents
	}
}

// wireMediaAdapter connects the MediaService to the WebUI server.
func wireMediaAdapter(webServer *webui.Server, assistant *copilot.Assistant, logger *slog.Logger) {
	mediaSvc := assistant.GetMediaService()
	if mediaSvc == nil {
		logger.Debug("native media service not available")
		return
	}

	adapter := &webui.MediaAdapter{
		UploadFn: func(r *http.Request, sessionID string) (string, string, string, int64, error) {
			// Parse multipart form
			if err := r.ParseMultipartForm(50 * 1024 * 1024); err != nil {
				return "", "", "", 0, fmt.Errorf("failed to parse form: %w", err)
			}

			file, header, err := r.FormFile("file")
			if err != nil {
				return "", "", "", 0, fmt.Errorf("no file provided: %w", err)
			}
			defer file.Close()

			// Read file data
			data, err := io.ReadAll(file)
			if err != nil {
				return "", "", "", 0, fmt.Errorf("failed to read file: %w", err)
			}

			// Upload to media service
			media, err := mediaSvc.Upload(r.Context(), media.UploadRequest{
				Data:      data,
				Filename:  header.Filename,
				Channel:   "ui",
				SessionID: sessionID,
				Temporary: r.FormValue("temporary") == "true",
			})
			if err != nil {
				return "", "", "", 0, err
			}

			return media.ID, string(media.Type), media.Filename, media.Size, nil
		},
		GetFn: func(mediaID string) ([]byte, string, string, error) {
			data, storedMedia, err := mediaSvc.Get(context.Background(), mediaID)
			if err != nil {
				return nil, "", "", err
			}
			return data, storedMedia.MimeType, storedMedia.Filename, nil
		},
		ListFn: func(sessionID string, mediaType string, limit int) ([]webui.MediaInfo, error) {
			medias, err := mediaSvc.List(context.Background(), media.ListFilter{
				SessionID: sessionID,
				Type:      media.MediaType(mediaType),
				Limit:     limit,
			})
			if err != nil {
				return nil, err
			}

			result := make([]webui.MediaInfo, len(medias))
			for i, m := range medias {
				result[i] = webui.MediaInfo{
					ID:        m.ID,
					Filename:  m.Filename,
					Type:      string(m.Type),
					Size:      m.Size,
					URL:       mediaSvc.URL(m.ID),
					CreatedAt: m.CreatedAt.Format(time.RFC3339),
				}
			}
			return result, nil
		},
		DeleteFn: func(mediaID string) error {
			return mediaSvc.Delete(context.Background(), mediaID)
		},
	}

	webServer.SetMediaAPI(adapter)
	logger.Info("media API wired to web UI")
}

// buildWebUIAdapter creates the adapter that bridges the Assistant to the WebUI.
func buildWebUIAdapter(ctx context.Context, assistant *copilot.Assistant, cfg *copilot.Config, waInstances map[string]*whatsapp.WhatsApp, configPath string, logger *slog.Logger) *webui.AssistantAdapter {
	adapter := &webui.AssistantAdapter{
		GetConfigMapFn: func() map[string]any {
			media := cfg.Media.Effective()
			return map[string]any{
				"name":               cfg.Name,
				"trigger":            cfg.Trigger,
				"model":              cfg.Model,
				"language":           cfg.Language,
				"timezone":           cfg.Timezone,
				"provider":           cfg.API.Provider,
				"base_url":           cfg.API.BaseURL,
				"api_key_configured": cfg.API.APIKey != "",
				"params":             cfg.API.Params,
				"media": map[string]any{
					"vision_enabled":         media.VisionEnabled,
					"vision_model":           media.VisionModel,
					"vision_detail":          media.VisionDetail,
					"transcription_enabled":  media.TranscriptionEnabled,
					"transcription_model":    media.TranscriptionModel,
					"transcription_base_url": media.TranscriptionBaseURL,
					"transcription_api_key":  media.TranscriptionAPIKey != "",
					"transcription_language": media.TranscriptionLanguage,
				},
				"access": map[string]any{
					"default_policy":  cfg.Access.DefaultPolicy,
					"owners":          cfg.Access.Owners,
					"admins":          cfg.Access.Admins,
					"allowed_users":   cfg.Access.AllowedUsers,
					"blocked_users":   cfg.Access.BlockedUsers,
					"pending_message": cfg.Access.PendingMessage,
				},
				"channels": map[string]any{
					"telegram": map[string]any{
						"token_configured":  cfg.Channels.Telegram.Token != "",
						"respond_to_groups": cfg.Channels.Telegram.RespondToGroups,
						"respond_to_dms":    cfg.Channels.Telegram.RespondToDMs,
						"send_typing":       cfg.Channels.Telegram.SendTyping,
					},
				},
			}
		},
		UpdateConfigMapFn: func(updates map[string]any) error {
			// Update identity fields.
			if v, ok := updates["name"].(string); ok {
				cfg.Name = v
			}
			if v, ok := updates["trigger"].(string); ok {
				cfg.Trigger = v
			}
			if v, ok := updates["language"].(string); ok {
				cfg.Language = v
			}
			if v, ok := updates["timezone"].(string); ok {
				cfg.Timezone = v
			}

			// Update provider & model.
			llmChanged := false
			if v, ok := updates["provider"].(string); ok && v != "" {
				cfg.API.Provider = v
				llmChanged = true
			}
			if v, ok := updates["model"].(string); ok && v != "" {
				cfg.Model = v
				llmChanged = true
			}
			if v, ok := updates["base_url"].(string); ok {
				cfg.API.BaseURL = v
				llmChanged = true
			}
			if v, ok := updates["api_key"].(string); ok && v != "" {
				// Store in vault if available and unlocked (preferred for security)
				if vault := assistant.Vault(); vault != nil && vault.IsUnlocked() {
					providerKey := copilot.GetProviderKeyName(cfg.API.Provider)
					if err := vault.Set(providerKey, v); err != nil {
						return fmt.Errorf("failed to store API key in vault: %w", err)
					}
					// Inject into current process environment for immediate use
					os.Setenv(providerKey, v)
				}
				// Set in config for immediate use (will be sanitized on save)
				cfg.API.APIKey = v
				llmChanged = true
			}
			// Update API params (provider-specific settings like context1m, tool_stream).
			if paramsRaw, ok := updates["params"]; ok {
				if paramsMap, ok := paramsRaw.(map[string]any); ok {
					if cfg.API.Params == nil {
						cfg.API.Params = make(map[string]any)
					}
					maps.Copy(cfg.API.Params, paramsMap)
					llmChanged = true
				}
			}

			// Hot-reload LLM client when provider/model/key settings changed.
			if llmChanged {
				assistant.UpdateLLMClient(cfg)
			}

			// Update media config.
			if mediaRaw, ok := updates["media"]; ok {
				if mediaMap, ok := mediaRaw.(map[string]any); ok {
					media := cfg.Media
					if v, ok := mediaMap["vision_enabled"].(bool); ok {
						media.VisionEnabled = v
					}
					if v, ok := mediaMap["vision_model"].(string); ok {
						media.VisionModel = v
					}
					if v, ok := mediaMap["vision_detail"].(string); ok {
						media.VisionDetail = v
					}
					if v, ok := mediaMap["transcription_enabled"].(bool); ok {
						media.TranscriptionEnabled = v
					}
					if v, ok := mediaMap["transcription_model"].(string); ok {
						media.TranscriptionModel = v
					}
					if v, ok := mediaMap["transcription_base_url"].(string); ok {
						media.TranscriptionBaseURL = v
					}
					if v, ok := mediaMap["transcription_api_key"].(string); ok && v != "" {
						media.TranscriptionAPIKey = v
					}
					if v, ok := mediaMap["transcription_language"].(string); ok {
						media.TranscriptionLanguage = v
					}
					cfg.Media = media
					assistant.UpdateMediaConfig(media)
				}
			}

			// Update access config.
			if accessRaw, ok := updates["access"]; ok {
				if accessMap, ok := accessRaw.(map[string]any); ok {
					if v, ok := accessMap["default_policy"].(string); ok {
						cfg.Access.DefaultPolicy = copilot.AccessPolicy(v)
					}
					if v, ok := accessMap["owners"].([]any); ok {
						cfg.Access.Owners = anySliceToStringSlice(v)
					}
					if v, ok := accessMap["admins"].([]any); ok {
						cfg.Access.Admins = anySliceToStringSlice(v)
					}
					if v, ok := accessMap["allowed_users"].([]any); ok {
						cfg.Access.AllowedUsers = anySliceToStringSlice(v)
					}
					if v, ok := accessMap["blocked_users"].([]any); ok {
						cfg.Access.BlockedUsers = anySliceToStringSlice(v)
					}
					if v, ok := accessMap["pending_message"].(string); ok {
						cfg.Access.PendingMessage = v
					}
				}
			}

			// Update channel tokens (store in vault, set env, update config).
			if channelsRaw, ok := updates["channels"]; ok {
				if channelsMap, ok := channelsRaw.(map[string]any); ok {
					vault := assistant.Vault()
					vaultOK := vault != nil && vault.IsUnlocked()

					// Telegram
					if tgRaw, ok := channelsMap["telegram"]; ok {
						if tgMap, ok := tgRaw.(map[string]any); ok {
							if v, ok := tgMap["token"].(string); ok && v != "" {
								if vaultOK {
									if err := vault.Set("TELEGRAM_BOT_TOKEN", v); err != nil {
										return fmt.Errorf("failed to store Telegram token in vault: %w", err)
									}
									os.Setenv("TELEGRAM_BOT_TOKEN", v)
								}
								cfg.Channels.Telegram.Token = v

								// Hot-reload: unregister stale channel, then register with new config.
								if _, exists := assistant.ChannelManager().Channel("telegram"); exists {
									_ = assistant.ChannelManager().UnregisterChannel("telegram")
								}
								tg := telegram.New(cfg.Channels.Telegram, logger)
								if err := assistant.ChannelManager().RegisterAndConnect(tg); err != nil {
									logger.Error("failed to hot-reload Telegram channel", "error", err)
								} else {
									logger.Info("Telegram channel connected via hot-reload")
								}
							}
						}
					}
				}
			}

			savePath := configPath
			if savePath == "" {
				savePath = "config.yaml"
			}
			return copilot.SaveConfigToFile(cfg, savePath)
		},
		ListSessionsFn: func() []webui.SessionInfo {
			sessions := assistant.SessionStore().ListAllSessions()
			result := make([]webui.SessionInfo, len(sessions))
			for i, s := range sessions {
				result[i] = webui.SessionInfo{
					ID:            s.ID,
					Channel:       s.Channel,
					ChatID:        s.ChatID,
					Title:         s.Title,
					MessageCount:  s.MessageCount,
					CreatedAt:     s.CreatedAt,
					LastMessageAt: s.LastActiveAt,
				}
			}
			return result
		},
		GetSessionMessagesFn: func(sessionID string) []webui.MessageInfo {
			store := assistant.SessionStore()

			// First, try direct hash-ID lookup (handles bookmarked hash URLs).
			session := store.GetByID(sessionID)

			if session == nil {
				// Parse channel from sessionID (e.g. "webui:abc123" → channel="webui").
				channel := "webui"
				if idx := strings.IndexByte(sessionID, ':'); idx > 0 {
					channel = sessionID[:idx]
				}
				session = store.Get(channel, sessionID)
			}

			if session == nil {
				// Session not in memory. Search all sessions (memory + persistence)
				// matching by hash ID or chatID.
				for _, meta := range store.ListAllSessions() {
					if meta.ID == sessionID || meta.ChatID == sessionID {
						session = store.GetOrCreate(meta.Channel, meta.ChatID)
						break
					}
				}
			}
			if session == nil {
				return nil
			}
			entries := session.RecentHistory(50)
			result := make([]webui.MessageInfo, 0, len(entries)*2)
			for _, e := range entries {
				result = append(result, webui.MessageInfo{
					Role:      "user",
					Content:   e.UserMessage,
					Timestamp: e.Timestamp,
				})
				if e.AssistantResponse != "" {
					result = append(result, webui.MessageInfo{
						Role:      "assistant",
						Content:   e.AssistantResponse,
						Timestamp: e.Timestamp,
					})
				}
			}
			return result
		},
		GetUsageGlobalFn: func() webui.UsageInfo {
			usage := assistant.UsageTracker().GetGlobal()
			if usage == nil {
				return webui.UsageInfo{}
			}
			return webui.UsageInfo{
				TotalInputTokens:  usage.PromptTokens,
				TotalOutputTokens: usage.CompletionTokens,
				TotalCost:         usage.EstimatedCostUSD,
				RequestCount:      usage.Requests,
			}
		},
		GetChannelHealthFn: func() []webui.ChannelHealthInfo {
			healthMap := assistant.ChannelManager().HealthAll()

			// Collect all registered channels from the manager.
			seen := make(map[string]bool)
			result := make([]webui.ChannelHealthInfo, 0, len(healthMap)+2)

			for name, h := range healthMap {
				seen[name] = true
				result = append(result, webui.ChannelHealthInfo{
					Name:       name,
					FullID:     name,
					Configured: true,
					Connected:  h.Connected,
					ErrorCount: h.ErrorCount,
					LastMsgAt:  h.LastMessageAt,
				})
			}

			// Always include default core channels even if not registered,
			// so the UI shows the "connect" option.
			coreDefaults := []struct {
				name       string
				configured bool
			}{
				{"whatsapp", true},
				{"telegram", cfg.Channels.Telegram.Token != ""},
			}
			for _, ch := range coreDefaults {
				if !seen[ch.name] {
					result = append(result, webui.ChannelHealthInfo{
						Name:       ch.name,
						FullID:     ch.name,
						Configured: ch.configured,
					})
				}
			}

			return result
		},
		GetSchedulerJobsFn: func() []webui.JobInfo {
			sched := assistant.Scheduler()
			if sched == nil {
				return nil
			}
			jobs := sched.List()
			result := make([]webui.JobInfo, len(jobs))
			for i, j := range jobs {
				var lastRun time.Time
				if j.LastRunAt != nil {
					lastRun = *j.LastRunAt
				}
				result[i] = webui.JobInfo{
					ID:        j.ID,
					Schedule:  j.Schedule,
					Type:      j.Type,
					Command:   j.Command,
					Enabled:   j.Enabled,
					RunCount:  j.RunCount,
					LastRunAt: lastRun,
					LastError: j.LastError,
				}
			}
			return result
		},
		ToggleJobFn: func(id string, enabled bool) error {
			sched := assistant.Scheduler()
			if sched == nil {
				return fmt.Errorf("scheduler not available")
			}
			return sched.Toggle(id, enabled)
		},
		RemoveJobFn: func(id string) error {
			sched := assistant.Scheduler()
			if sched == nil {
				return fmt.Errorf("scheduler not available")
			}
			return sched.Remove(id)
		},
		ListSkillsFn: func() []webui.SkillInfo {
			reg := assistant.SkillRegistry()
			if reg == nil {
				return nil
			}
			metas := reg.List()
			result := make([]webui.SkillInfo, len(metas))
			for i, m := range metas {
				result[i] = webui.SkillInfo{
					Name:        m.Name,
					Description: m.Description,
					Enabled:     reg.IsEnabled(m.Name),
				}
			}
			return result
		},
		ToggleSkillFn: func(name string, enabled bool) error {
			reg := assistant.SkillRegistry()
			if reg == nil {
				return fmt.Errorf("skill registry not available")
			}
			if enabled {
				return reg.Enable(name)
			}
			return reg.Disable(name)
		},
		RemoveSkillFn: func(name string) error {
			reg := assistant.SkillRegistry()
			if reg == nil {
				return fmt.Errorf("skill registry not available")
			}
			reg.Remove(name)
			// Remove skill directory from disk.
			skillDir := filepath.Join("./skills", name)
			if err := os.RemoveAll(skillDir); err != nil {
				return fmt.Errorf("failed to remove skill directory: %w", err)
			}
			return nil
		},
		ReloadSkillsFn: func() error {
			reg := assistant.SkillRegistry()
			if reg == nil {
				return fmt.Errorf("skill registry not available")
			}
			_, err := reg.Reload(context.Background())
			return err
		},
		GetProfileManagerFn: func() profiles.ProfileManager {
			return assistant.ProfileManager()
		},
		SendChatMessageFn: func(sessionID, content string) (string, error) {
			store := assistant.SessionStore()
			session := store.GetByID(sessionID)
			if session == nil {
				channel := "webui"
				if idx := strings.IndexByte(sessionID, ':'); idx > 0 {
					channel = sessionID[:idx]
				}
				session = store.GetOrCreate(channel, sessionID)
			}
			prompt := assistant.ComposePrompt(session, content)
			ctx := copilot.ContextWithDelivery(context.Background(), "webui", sessionID)
			resp := assistant.ExecuteAgent(ctx, prompt, session, content)
			session.AddMessage(content, resp)
			return resp, nil
		},
		StartChatStreamFn: func(_ context.Context, sessionID, content string) (*webui.RunHandle, error) {
			store := assistant.SessionStore()

			// Resolve session: try hash-ID first, then channel:chatID.
			session := store.GetByID(sessionID)
			if session == nil {
				// Parse channel from sessionID (e.g. "webui:abc123" → channel="webui").
				channel := "webui"
				if idx := strings.IndexByte(sessionID, ':'); idx > 0 {
					channel = sessionID[:idx]
				}
				session = store.GetOrCreate(channel, sessionID)
			}
			prompt := assistant.ComposePrompt(session, content)

			runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
			events := make(chan webui.StreamEvent, 256)

			// FIX: Use context.Background() — the agent must outlive the POST /send
			// request. The context is cancelled by handle.Cancel() when the SSE
			// client disconnects or the user aborts.
			runCtx, cancel := context.WithCancel(context.Background())

			handle := &webui.RunHandle{
				RunID:     runID,
				SessionID: sessionID,
				Events:    events,
				Cancel:    cancel,
			}

			// Run the agent in a goroutine, streaming events via the channel.
			go func() {
				defer close(events)
				defer cancel() // Ensure context is always cleaned up.

				history := session.RecentHistory(10)
				agent := copilot.NewAgentRunWithConfig(
					assistant.LLMClient(),
					assistant.ToolExecutor(),
					cfg.Agent,
					slog.Default(),
				)

				// Propagate caller context via context.Context (goroutine-safe).
				runCtx = copilot.ContextWithCaller(runCtx, copilot.AccessOwner, "webui")
				runCtx = copilot.ContextWithSession(runCtx, sessionID)
				runCtx = copilot.ContextWithDelivery(runCtx, "webui", sessionID)

				// Media emitter: pushes media events to the SSE stream.
				runCtx = copilot.ContextWithMediaEmitter(runCtx, func(evt copilot.MediaEvent) {
					select {
					case events <- webui.StreamEvent{Type: "media", Data: evt}:
					case <-runCtx.Done():
					}
				})

				// Stream text tokens to the SSE channel.
				agent.SetStreamCallback(func(chunk string) {
					// Strip internal tags like [[reply_to_current]] before sending to UI
					cleanChunk := copilot.StripInternalTags(chunk)
					if cleanChunk == "" {
						return // Skip empty chunks after stripping
					}
					select {
					case events <- webui.StreamEvent{
						Type: "delta",
						Data: map[string]string{"content": cleanChunk},
					}:
					case <-runCtx.Done():
					}
				})

				// Record token usage.
				if assistant.UsageTracker() != nil {
					agent.SetUsageRecorder(func(model string, usage copilot.LLMUsage) {
						assistant.UsageTracker().Record(session.ID, model, usage)
					})
				}

				resp, usage, err := agent.RunWithUsage(runCtx, prompt, history, content)
				if err != nil {
					// FIX: Non-blocking sends to avoid deadlock when client is gone.
					if runCtx.Err() != nil {
						select {
						case events <- webui.StreamEvent{
							Type: "error",
							Data: map[string]string{"message": "Execution cancelled"},
						}:
						default:
						}
						return
					}
					select {
					case events <- webui.StreamEvent{
						Type: "error",
						Data: map[string]string{"message": err.Error()},
					}:
					case <-runCtx.Done():
					}
					return
				}

				// Persist the conversation.
				session.AddMessage(content, resp)

				if usage != nil {
					session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens)
				}

				// Send done event with usage stats (non-blocking).
				usageData := map[string]int{"input_tokens": 0, "output_tokens": 0}
				if usage != nil {
					usageData["input_tokens"] = usage.PromptTokens
					usageData["output_tokens"] = usage.CompletionTokens
				}
				select {
				case events <- webui.StreamEvent{
					Type: "done",
					Data: map[string]any{"usage": usageData},
				}:
				case <-runCtx.Done():
				}
			}()

			return handle, nil
		},
		AbortRunFn: func(sessionID string) bool {
			// First try to stop via the assistant's active runs (channel-driven).
			if assistant.StopActiveRun("default", "webui:"+sessionID) {
				return true
			}
			// Web UI runs are cancelled via RunHandle.Cancel() in the SSE handler.
			// This path is a fallback — the primary abort is via the webui server
			// which calls handle.Cancel() directly.
			return false
		},
		DeleteSessionFn: func(sessionID string) error {
			store := assistant.SessionStore()

			// If sessionID contains ":", it's a chatID from the chat view (e.g. "webui:abc123").
			// Compute the hash key first (matching the GetOrCreate/Get pattern).
			if idx := strings.IndexByte(sessionID, ':'); idx > 0 {
				channel := sessionID[:idx]
				hashKey := copilot.MakeSessionID(channel, sessionID)
				if store.DeleteByID(hashKey) {
					return nil
				}
			}

			// Try as hash ID directly (from /sessions listing page).
			if store.DeleteByID(sessionID) {
				return nil
			}

			// Fallback: search all sessions by chatID to find correct channel
			// (handles whatsapp, scheduler, etc. without colon prefix).
			for _, meta := range store.ListAllSessions() {
				if meta.ChatID == sessionID {
					hashKey := copilot.MakeSessionID(meta.Channel, meta.ChatID)
					if store.DeleteByID(hashKey) {
						return nil
					}
				}
			}

			return fmt.Errorf("session not found: %s", sessionID)
		},
	}

	// ── Security: Audit Log ──
	adapter.GetAuditLogFn = func(limit int) []webui.AuditEntry {
		guard := assistant.ToolExecutor().Guard()
		if guard == nil {
			return nil
		}
		audit := guard.SQLiteAudit()
		if audit == nil {
			return nil
		}
		records := audit.RecentRecords(limit)
		entries := make([]webui.AuditEntry, len(records))
		for i, r := range records {
			entries[i] = webui.AuditEntry{
				ID:            r.ID,
				Tool:          r.Tool,
				Caller:        r.Caller,
				Level:         r.Level,
				Allowed:       r.Allowed,
				ArgsSummary:   r.ArgsSummary,
				ResultSummary: r.ResultSummary,
				CreatedAt:     r.CreatedAt,
			}
		}
		return entries
	}
	adapter.GetAuditCountFn = func() int {
		guard := assistant.ToolExecutor().Guard()
		if guard == nil {
			return 0
		}
		audit := guard.SQLiteAudit()
		if audit == nil {
			return 0
		}
		return audit.Count()
	}

	// ── Security: Tool Guard ──
	adapter.GetToolGuardStatusFn = func() webui.ToolGuardStatus {
		gc := cfg.Security.ToolGuard
		return webui.ToolGuardStatus{
			Enabled:             gc.Enabled,
			AllowDestructive:    gc.AllowDestructive,
			AllowSudo:           gc.AllowSudo,
			AllowReboot:         gc.AllowReboot,
			AutoApprove:         gc.AutoApprove,
			RequireConfirmation: gc.RequireConfirmation,
			ProtectedPaths:      gc.ProtectedPaths,
			SSHAllowedHosts:     gc.SSHAllowedHosts,
			DangerousCommands:   gc.DangerousCommands,
			ToolPermissions:     gc.ToolPermissions,
		}
	}
	adapter.UpdateToolGuardFn = func(update webui.ToolGuardStatus) error {
		cfg.Security.ToolGuard.AllowDestructive = update.AllowDestructive
		cfg.Security.ToolGuard.AllowSudo = update.AllowSudo
		cfg.Security.ToolGuard.AllowReboot = update.AllowReboot
		if update.AutoApprove != nil {
			cfg.Security.ToolGuard.AutoApprove = update.AutoApprove
		}
		if update.RequireConfirmation != nil {
			cfg.Security.ToolGuard.RequireConfirmation = update.RequireConfirmation
		}
		if update.ProtectedPaths != nil {
			cfg.Security.ToolGuard.ProtectedPaths = update.ProtectedPaths
		}
		if update.SSHAllowedHosts != nil {
			cfg.Security.ToolGuard.SSHAllowedHosts = update.SSHAllowedHosts
		}
		// Apply hot-reload to the running tool guard.
		assistant.ToolExecutor().UpdateGuardConfig(cfg.Security.ToolGuard)
		return nil
	}

	// ── Security: Vault ──
	adapter.GetVaultStatusFn = func() webui.VaultStatus {
		v := assistant.Vault()
		if v == nil {
			return webui.VaultStatus{Exists: false}
		}
		status := webui.VaultStatus{
			Exists:   v.Exists(),
			Unlocked: v.IsUnlocked(),
		}
		if v.IsUnlocked() {
			status.Keys = v.List()
			if status.Keys == nil {
				status.Keys = []string{}
			}
		}
		return status
	}

	// ── Security: Overview ──
	adapter.GetSecurityStatusFn = func() webui.SecurityStatus {
		s := webui.SecurityStatus{
			GatewayAuthConfigured: cfg.Gateway.AuthToken != "",
			WebUIAuthConfigured:   cfg.WebUI.AuthToken != "",
			ToolGuardEnabled:      cfg.Security.ToolGuard.Enabled,
		}
		if v := assistant.Vault(); v != nil {
			s.VaultExists = v.Exists()
			s.VaultUnlocked = v.IsUnlocked()
		}
		if guard := assistant.ToolExecutor().Guard(); guard != nil {
			if audit := guard.SQLiteAudit(); audit != nil {
				s.AuditEntryCount = audit.Count()
			}
		}
		return s
	}

	// ── Hooks (Lifecycle) ──
	adapter.ListHooksFn = func() []webui.HookInfo {
		hm := assistant.HookManager()
		if hm == nil {
			return nil
		}
		summaries := hm.ListDetailed()
		result := make([]webui.HookInfo, len(summaries))
		for i, s := range summaries {
			events := make([]string, len(s.Events))
			for j, ev := range s.Events {
				events[j] = string(ev)
			}
			result[i] = webui.HookInfo{
				Name:        s.Name,
				Description: s.Description,
				Source:      s.Source,
				Events:      events,
				Priority:    s.Priority,
				Enabled:     s.Enabled,
			}
		}
		return result
	}
	adapter.ToggleHookFn = func(name string, enabled bool) error {
		hm := assistant.HookManager()
		if hm == nil {
			return fmt.Errorf("hook manager not available")
		}
		if !hm.SetEnabled(name, enabled) {
			return fmt.Errorf("hook %q not found", name)
		}
		return nil
	}
	adapter.UnregisterHookFn = func(name string) error {
		hm := assistant.HookManager()
		if hm == nil {
			return fmt.Errorf("hook manager not available")
		}
		if !hm.Unregister(name) {
			return fmt.Errorf("hook %q not found", name)
		}
		return nil
	}
	adapter.GetHookEventsFn = func() []webui.HookEventInfo {
		hm := assistant.HookManager()
		if hm == nil {
			return nil
		}
		hooksByEvent := hm.ListHooks()
		result := make([]webui.HookEventInfo, 0, len(copilot.AllHookEvents))
		for _, ev := range copilot.AllHookEvents {
			names := hooksByEvent[ev]
			if names == nil {
				names = []string{}
			}
			result = append(result, webui.HookEventInfo{
				Event:       string(ev),
				Description: copilot.HookEventDescription(ev),
				Hooks:       names,
			})
		}
		return result
	}

	// Wire up channel instance listing.
	adapter.ListChannelInstancesFn = func(channelType string) []webui.ChannelInstanceInfo {
		chs := assistant.ChannelManager().ChannelsByType(channelType)
		result := make([]webui.ChannelInstanceInfo, 0, len(chs))
		for _, ch := range chs {
			info := webui.ChannelInstanceInfo{
				Type:      channelType,
				FullName:  ch.Name(),
				Connected: ch.IsConnected(),
				ErrorCount: ch.Health().ErrorCount,
				Configured: true,
			}
			if ia, ok := ch.(channels.InstanceAware); ok {
				info.InstanceID = ia.InstanceID()
			}
			if qr, ok := ch.(interface{ NeedsQR() bool }); ok {
				info.NeedsQR = qr.NeedsQR()
			}
			label := strings.ToUpper(channelType[:1]) + channelType[1:]
			if info.InstanceID == "" {
				info.Label = label
			} else {
				info.Label = label + " (" + info.InstanceID + ")"
			}
			result = append(result, info)
		}
		return result
	}

	// Mutex protects waInstances and cfg during runtime instance creation/deletion.
	var instanceMu sync.Mutex

	adapter.CreateChannelInstanceFn = func(channelType, instanceID string, config map[string]any) error {
		if err := channels.ValidateInstanceID(instanceID); err != nil {
			return err
		}
		if instanceID == "" {
			return fmt.Errorf("instance ID cannot be empty")
		}

		instanceMu.Lock()
		defer instanceMu.Unlock()

		fullName := channelType + ":" + instanceID

		// Check for duplicate.
		existing := assistant.ChannelManager().ChannelsByType(channelType)
		for _, ch := range existing {
			if ch.Name() == fullName {
				return fmt.Errorf("instance %q already exists", instanceID)
			}
		}

		switch channelType {
		case "whatsapp":
			waCfg := whatsapp.DefaultConfig()
			waCfg.InstanceID = instanceID
			if cfg.Database.Path != "" {
				waCfg.DatabasePath = cfg.Database.Path
			}
			inheritWhatsAppAccess(&waCfg, cfg)
			wa := whatsapp.New(waCfg, logger)
			if err := assistant.ChannelManager().RegisterAndConnect(wa); err != nil {
				return fmt.Errorf("register: %w", err)
			}
			waInstances[instanceID] = wa

			// Persist to config.
			if cfg.Channels.WhatsAppInstances == nil {
				cfg.Channels.WhatsAppInstances = make(map[string]whatsapp.Config)
			}
			cfg.Channels.WhatsAppInstances[instanceID] = waCfg

		case "telegram":
			tgCfg := telegram.DefaultConfig()
			tgCfg.InstanceID = instanceID
			if tok, ok := config["token"].(string); ok {
				tgCfg.Token = tok
			}
			tg := telegram.New(tgCfg, logger)
			if err := assistant.ChannelManager().RegisterAndConnect(tg); err != nil {
				return fmt.Errorf("register: %w", err)
			}
			if cfg.Channels.TelegramInstances == nil {
				cfg.Channels.TelegramInstances = make(map[string]telegram.Config)
			}
			cfg.Channels.TelegramInstances[instanceID] = tgCfg

		case "discord":
			dcCfg := discord.Config{InstanceID: instanceID}
			if tok, ok := config["token"].(string); ok {
				dcCfg.Token = tok
			}
			dc := discord.New(dcCfg, logger)
			if err := assistant.ChannelManager().RegisterAndConnect(dc); err != nil {
				return fmt.Errorf("register: %w", err)
			}
			if cfg.Channels.DiscordInstances == nil {
				cfg.Channels.DiscordInstances = make(map[string]discord.Config)
			}
			cfg.Channels.DiscordInstances[instanceID] = dcCfg

		case "slack":
			slCfg := slack.Config{InstanceID: instanceID}
			if tok, ok := config["bot_token"].(string); ok {
				slCfg.BotToken = tok
			}
			if tok, ok := config["app_token"].(string); ok {
				slCfg.AppToken = tok
			}
			sl := slack.New(slCfg, logger)
			if err := assistant.ChannelManager().RegisterAndConnect(sl); err != nil {
				return fmt.Errorf("register: %w", err)
			}
			if cfg.Channels.SlackInstances == nil {
				cfg.Channels.SlackInstances = make(map[string]slack.Config)
			}
			cfg.Channels.SlackInstances[instanceID] = slCfg

		default:
			return fmt.Errorf("unsupported channel type: %s", channelType)
		}

		// Persist config to disk.
		if configPath != "" {
			if err := copilot.SaveConfigToFile(cfg, configPath); err != nil {
				logger.Error("failed to save config after instance creation", "error", err)
				return fmt.Errorf("config save: %w", err)
			}
		}

		logger.Info("channel instance created", "type", channelType, "instance", instanceID)
		return nil
	}

	adapter.DeleteChannelInstanceFn = func(channelType, instanceID string) error {
		if instanceID == "" {
			return fmt.Errorf("cannot delete default instance")
		}
		if err := channels.ValidateInstanceID(instanceID); err != nil {
			return err
		}

		instanceMu.Lock()
		defer instanceMu.Unlock()

		fullName := channelType + ":" + instanceID

		// Unregister from channel manager (disconnects if connected).
		if err := assistant.ChannelManager().UnregisterChannel(fullName); err != nil {
			return fmt.Errorf("unregister: %w", err)
		}

		// Remove from runtime maps.
		switch channelType {
		case "whatsapp":
			delete(waInstances, instanceID)
			delete(cfg.Channels.WhatsAppInstances, instanceID)
		case "telegram":
			delete(cfg.Channels.TelegramInstances, instanceID)
		case "discord":
			delete(cfg.Channels.DiscordInstances, instanceID)
		case "slack":
			delete(cfg.Channels.SlackInstances, instanceID)
		default:
			return fmt.Errorf("unsupported channel type: %s", channelType)
		}

		// Persist config to disk.
		if configPath != "" {
			if err := copilot.SaveConfigToFile(cfg, configPath); err != nil {
				logger.Error("failed to save config after instance deletion", "error", err)
				return fmt.Errorf("config save: %w", err)
			}
		}

		logger.Info("channel instance deleted", "type", channelType, "instance", instanceID)
		return nil
	}

	// Wire up WhatsApp QR callbacks if WhatsApp channel is available.
	// Use the default instance for backward-compatible adapter functions.
	wa := waInstances[""]
	if wa != nil {
		adapter.GetWhatsAppStatusFn = func() webui.WhatsAppStatus {
			return whatsappStatusFromInstance(wa)
		}
		adapter.SubscribeWhatsAppQRFn = func() (chan webui.WhatsAppQREvent, func()) {
			return bridgeWhatsAppQR(wa)
		}
		adapter.RequestWhatsAppQRFn = func() error {
			return wa.RequestNewQR(ctx)
		}
		adapter.DisconnectWhatsAppFn = func() error {
			return wa.Disconnect()
		}

		// Helper function to convert []any to []string
		anySliceToStringSlice := func(slice []any) []string {
			result := make([]string, 0, len(slice))
			for _, v := range slice {
				if s, ok := v.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}

		// Type assert WhatsApp to WhatsAppAccessManager interface
		waManager, ok := any(wa).(channels.WhatsAppAccessManager)
		if !ok {
			slog.Warn("whatsapp channel does not implement WhatsAppAccessManager, access control features disabled")
			return adapter
		}

		// WhatsApp Access & Groups
		adapter.GetWhatsAppAccessConfigFn = func() webui.WhatsAppAccessConfig {
			result := webui.WhatsAppAccessConfig{
				DefaultPolicy:  "deny",
				PendingMessage: "Aguardando aprovação do proprietário.",
			}

			if m, ok := waManager.GetAccessConfig().(map[string]any); ok {
				if v, ok := m["default_policy"].(string); ok && v != "" {
					result.DefaultPolicy = v
				}
				if v, ok := m["owners"].([]string); ok {
					result.Owners = v
				} else if v, ok := m["owners"].([]any); ok {
					result.Owners = anySliceToStringSlice(v)
				}
				if v, ok := m["admins"].([]string); ok {
					result.Admins = v
				} else if v, ok := m["admins"].([]any); ok {
					result.Admins = anySliceToStringSlice(v)
				}
				if v, ok := m["allowed_users"].([]string); ok {
					result.AllowedUsers = v
				} else if v, ok := m["allowed_users"].([]any); ok {
					result.AllowedUsers = anySliceToStringSlice(v)
				}
				if v, ok := m["blocked_users"].([]string); ok {
					result.BlockedUsers = v
				} else if v, ok := m["blocked_users"].([]any); ok {
					result.BlockedUsers = anySliceToStringSlice(v)
				}
				if v, ok := m["allowed_groups"].([]string); ok {
					result.AllowedGroups = v
				} else if v, ok := m["allowed_groups"].([]any); ok {
					result.AllowedGroups = anySliceToStringSlice(v)
				}
				if v, ok := m["blocked_groups"].([]string); ok {
					result.BlockedGroups = v
				} else if v, ok := m["blocked_groups"].([]any); ok {
					result.BlockedGroups = anySliceToStringSlice(v)
				}
				if v, ok := m["pending_message"].(string); ok && v != "" {
					result.PendingMessage = v
				}
			}

			// Ensure slices are never nil (for JSON)
			if result.Owners == nil {
				result.Owners = []string{}
			}
			if result.Admins == nil {
				result.Admins = []string{}
			}
			if result.AllowedUsers == nil {
				result.AllowedUsers = []string{}
			}
			if result.BlockedUsers == nil {
				result.BlockedUsers = []string{}
			}
			if result.AllowedGroups == nil {
				result.AllowedGroups = []string{}
			}
			if result.BlockedGroups == nil {
				result.BlockedGroups = []string{}
			}

			return result
		}

		adapter.GrantWhatsAppUserAccessFn = func(jid, level string) error {
			waManager.GrantAccess(jid, level)
			// Sync runtime state back to config and persist
			return syncWhatsAppAccessToConfig(cfg, waManager, configPath)
		}

		adapter.RevokeWhatsAppUserAccessFn = func(jid string) error {
			waManager.RevokeAccess(jid)
			// Sync runtime state back to config and persist
			return syncWhatsAppAccessToConfig(cfg, waManager, configPath)
		}

		adapter.BlockWhatsAppUserFn = func(jid string) error {
			waManager.BlockUser(jid)
			// Sync runtime state back to config and persist
			return syncWhatsAppAccessToConfig(cfg, waManager, configPath)
		}

		adapter.UnblockWhatsAppUserFn = func(jid string) error {
			waManager.UnblockUser(jid)
			// Sync runtime state back to config and persist
			return syncWhatsAppAccessToConfig(cfg, waManager, configPath)
		}

		adapter.UpdateWhatsAppAccessDefaultPolicyFn = func(policy string) error {
			cfg.Channels.WhatsApp.Access.DefaultPolicy = policy
			waManager.SetDefaultPolicy(policy)

			savePath := configPath
			if savePath == "" {
				savePath = "config.yaml"
			}
			return copilot.SaveConfigToFile(cfg, savePath)
		}

		adapter.GetWhatsAppGroupPoliciesFn = func() webui.WhatsAppGroupPolicies {
			result := webui.WhatsAppGroupPolicies{
				DefaultPolicy: "mention",
				Groups:        []webui.WhatsAppGroupPolicy{},
			}

			gpResult := waManager.GetGroupPolicies()

			m, ok := gpResult.(map[string]any)
			if ok {
				if v, ok := m["default_policy"].(string); ok {
					result.DefaultPolicy = v
				}
				if v, ok := m["groups"].([]any); ok {
					result.Groups = make([]webui.WhatsAppGroupPolicy, 0, len(v))
					for _, g := range v {
						if gm, ok := g.(map[string]any); ok {
							gp := webui.WhatsAppGroupPolicy{}
							if id, ok := gm["id"].(string); ok {
								gp.ID = id
							}
							if name, ok := gm["name"].(string); ok {
								gp.Name = name
							}
							if policy, ok := gm["policy"].(string); ok {
								gp.Policy = policy
							}
							if policies, ok := gm["policies"].([]any); ok {
								gp.Policies = anySliceToStringSlice(policies)
							} else if policies, ok := gm["policies"].([]string); ok {
								gp.Policies = policies
							}
							if keywords, ok := gm["keywords"].([]any); ok {
								gp.Keywords = anySliceToStringSlice(keywords)
							} else if keywords, ok := gm["keywords"].([]string); ok {
								gp.Keywords = keywords
							}
							if users, ok := gm["allowed_users"].([]any); ok {
								gp.AllowedUsers = anySliceToStringSlice(users)
							} else if users, ok := gm["allowed_users"].([]string); ok {
								gp.AllowedUsers = users
							}
							if ws, ok := gm["workspace"].(string); ok {
								gp.Workspace = ws
							}
							result.Groups = append(result.Groups, gp)
						}
					}
				}
			}

			return result
		}

		adapter.SetWhatsAppGroupPolicyFn = func(jid string, policy any) error {
			waManager.SetGroupPolicy(jid, policy)
			// Sync runtime state back to config and persist
			return syncWhatsAppGroupPoliciesToConfig(cfg, waManager, configPath)
		}

		adapter.UpdateWhatsAppGroupDefaultPolicyFn = func(policy string) error {
			cfg.Channels.WhatsApp.GroupPolicies.DefaultPolicy = policy
			waManager.SetGroupDefaultPolicy(policy)

			savePath := configPath
			if savePath == "" {
				savePath = "config.yaml"
			}
			return copilot.SaveConfigToFile(cfg, savePath)
		}

		adapter.UpdateWhatsAppConfigFn = func(config map[string]any) error {
			if autoRead, ok := config["auto_read"].(bool); ok {
				waManager.SetAutoRead(autoRead)
				cfg.Channels.WhatsApp.AutoRead = autoRead
			}
			if sendTyping, ok := config["send_typing"].(bool); ok {
				waManager.SetSendTyping(sendTyping)
				cfg.Channels.WhatsApp.SendTyping = sendTyping
			}
			if trigger, ok := config["trigger"].(string); ok {
				waManager.SetTrigger(trigger)
				cfg.Channels.WhatsApp.Trigger = trigger
			}
			if respondToGroups, ok := config["respond_to_groups"].(bool); ok {
				cfg.Channels.WhatsApp.RespondToGroups = respondToGroups
			}
			if respondToDMs, ok := config["respond_to_dms"].(bool); ok {
				cfg.Channels.WhatsApp.RespondToDMs = respondToDMs
			}
			savePath := configPath
			if savePath == "" {
				savePath = "config.yaml"
			}
			return copilot.SaveConfigToFile(cfg, savePath)
		}

		adapter.GetWhatsAppConfigFn = func() map[string]any {
			return map[string]any{
				"trigger":           cfg.Channels.WhatsApp.Trigger,
				"respond_to_groups": cfg.Channels.WhatsApp.RespondToGroups,
				"respond_to_dms":    cfg.Channels.WhatsApp.RespondToDMs,
				"auto_read":         cfg.Channels.WhatsApp.AutoRead,
				"send_typing":       cfg.Channels.WhatsApp.SendTyping,
			}
		}

		adapter.GetWhatsAppJoinedGroupsFn = func() ([]webui.WhatsAppJoinedGroup, error) {
			groups, err := waManager.GetJoinedGroups()
			if err != nil {
				return nil, err
			}
			result := make([]webui.WhatsAppJoinedGroup, 0, len(groups))
			for _, g := range groups {
				result = append(result, webui.WhatsAppJoinedGroup(g))
			}
			return result, nil
		}
	}

	// Instance-aware WhatsApp variants (work independently of default wa instance).
	adapter.GetWhatsAppStatusByInstanceFn = func(instanceID string) webui.WhatsAppStatus {
		instanceMu.Lock()
		inst := waInstances[instanceID]
		instanceMu.Unlock()
		if inst == nil {
			return webui.WhatsAppStatus{State: "not_configured", Message: "Instance not found"}
		}
		return whatsappStatusFromInstance(inst)
	}
	adapter.SubscribeWhatsAppQRByInstanceFn = func(instanceID string) (chan webui.WhatsAppQREvent, func()) {
		instanceMu.Lock()
		inst := waInstances[instanceID]
		instanceMu.Unlock()
		if inst == nil {
			ch := make(chan webui.WhatsAppQREvent, 1)
			ch <- webui.WhatsAppQREvent{Type: "error", Message: "instance not found"}
			close(ch)
			return ch, func() {}
		}
		return bridgeWhatsAppQR(inst)
	}
	adapter.RequestWhatsAppQRByInstanceFn = func(instanceID string) error {
		instanceMu.Lock()
		inst := waInstances[instanceID]
		instanceMu.Unlock()
		if inst == nil {
			return fmt.Errorf("whatsapp instance %q not found", instanceID)
		}
		return inst.RequestNewQR(ctx)
	}
	adapter.DisconnectWhatsAppByInstanceFn = func(instanceID string) error {
		instanceMu.Lock()
		inst := waInstances[instanceID]
		instanceMu.Unlock()
		if inst == nil {
			return fmt.Errorf("whatsapp instance %q not found", instanceID)
		}
		return inst.Disconnect()
	}

	// ── MCP Servers ──
	adapter.ListMCPServersFn = func() []webui.MCPServerInfo {
		mcpCfg := cfg.MCP
		if mcpCfg.Servers == nil {
			return nil
		}
		result := make([]webui.MCPServerInfo, 0, len(mcpCfg.Servers))
		for _, srv := range mcpCfg.Servers {
			env := make(map[string]string)
			for k, v := range srv.Env {
				// Mask sensitive values
				if strings.Contains(strings.ToLower(k), "token") ||
					strings.Contains(strings.ToLower(k), "key") ||
					strings.Contains(strings.ToLower(k), "secret") ||
					strings.Contains(strings.ToLower(k), "password") {
					env[k] = "***"
				} else {
					env[k] = v
				}
			}
			info := webui.MCPServerInfo{
				Name:    srv.Name,
				Command: srv.Command,
				Args:    srv.Args,
				Env:     env,
				Enabled: srv.Enabled,
				Status:  "stopped", // Default status, actual status requires runtime tracking
			}
			result = append(result, info)
		}
		return result
	}
	adapter.CreateMCPServerFn = func(name, command string, args []string, env map[string]string) error {
		newServer := copilot.ManagedMCPServerConfig{
			Name:      name,
			Type:      copilot.MCPTypeStdio,
			Command:   command,
			Args:      args,
			Env:       env,
			Enabled:   true,
			AutoStart: true,
		}
		cfg.MCP.Servers = append(cfg.MCP.Servers, newServer)
		savePath := configPath
		if savePath == "" {
			savePath = "config.yaml"
		}
		return copilot.SaveConfigToFile(cfg, savePath)
	}
	adapter.UpdateMCPServerFn = func(name string, enabled bool) error {
		found := false
		for i, srv := range cfg.MCP.Servers {
			if srv.Name == name {
				cfg.MCP.Servers[i].Enabled = enabled
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("MCP server %q not found", name)
		}
		savePath := configPath
		if savePath == "" {
			savePath = "config.yaml"
		}
		return copilot.SaveConfigToFile(cfg, savePath)
	}
	adapter.DeleteMCPServerFn = func(name string) error {
		found := false
		newServers := make([]copilot.ManagedMCPServerConfig, 0, len(cfg.MCP.Servers))
		for _, srv := range cfg.MCP.Servers {
			if srv.Name == name {
				found = true
				continue
			}
			newServers = append(newServers, srv)
		}
		if !found {
			return fmt.Errorf("MCP server %q not found", name)
		}
		cfg.MCP.Servers = newServers
		savePath := configPath
		if savePath == "" {
			savePath = "config.yaml"
		}
		return copilot.SaveConfigToFile(cfg, savePath)
	}
	adapter.StartMCPServerFn = func(name string) error {
		// MCP server start/stop requires runtime management
		// For now, we just validate the server exists
		found := false
		for _, srv := range cfg.MCP.Servers {
			if srv.Name == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("MCP server %q not found", name)
		}
		// TODO: Implement actual start via MCP manager when available
		return nil
	}
	adapter.StopMCPServerFn = func(name string) error {
		// MCP server start/stop requires runtime management
		found := false
		for _, srv := range cfg.MCP.Servers {
			if srv.Name == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("MCP server %q not found", name)
		}
		// TODO: Implement actual stop via MCP manager when available
		return nil
	}

	// ── Database Status ──
	adapter.GetDatabaseStatusFn = func() webui.DatabaseStatusInfo {
		dbCfg := cfg.Database.Effective()
		status := webui.DatabaseStatusInfo{
			Name:         string(dbCfg.Backend),
			Healthy:      true, // Assume healthy if we got here
			Latency:      1,    // Placeholder, actual value requires runtime check
			Version:      "1.0",
			MaxOpenConns: 25, // Default value
		}

		// Try to get actual database connection and stats
		// For SQLite, check if the database file exists
		if dbCfg.Backend == "sqlite" {
			if info, err := os.Stat(dbCfg.SQLite.Path); err == nil {
				status.Version = "3.x"
				status.OpenConns = 1
				_ = info // File exists, database is healthy
			} else {
				status.Healthy = false
				status.Error = err.Error()
			}
		}

		return status
	}

	// ── Telegram adapter wiring ──
	// Get Telegram channel from the channel manager if registered.
	adapter.GetTelegramConfigFn = func() webui.TelegramConfig {
		result := webui.TelegramConfig{
			Configured:            cfg.Channels.Telegram.Token != "",
			RespondToGroups:       cfg.Channels.Telegram.RespondToGroups,
			RespondToDMs:          cfg.Channels.Telegram.RespondToDMs,
			SendTyping:            cfg.Channels.Telegram.SendTyping,
			AllowedChats:          cfg.Channels.Telegram.AllowedChats,
			ReactionNotifications: cfg.Channels.Telegram.ReactionNotifications,
		}
		ch, exists := assistant.ChannelManager().Channel("telegram")
		if exists {
			result.Connected = ch.IsConnected()
			if tg, ok := ch.(*telegram.Telegram); ok {
				result.BotUsername = tg.BotUsername()
				result.BotID = tg.BotID()
			}
		}
		return result
	}

	adapter.UpdateTelegramConfigFn = func(config map[string]any) error {
		changed := false
		if v, ok := config["respond_to_groups"].(bool); ok {
			cfg.Channels.Telegram.RespondToGroups = v
			changed = true
		}
		if v, ok := config["respond_to_dms"].(bool); ok {
			cfg.Channels.Telegram.RespondToDMs = v
			changed = true
		}
		if v, ok := config["send_typing"].(bool); ok {
			cfg.Channels.Telegram.SendTyping = v
			changed = true
		}
		if v, ok := config["reaction_notifications"].(string); ok {
			cfg.Channels.Telegram.ReactionNotifications = v
			changed = true
		}
		if !changed {
			return nil
		}

		// Hot-reload settings on the live Telegram instance.
		if ch, exists := assistant.ChannelManager().Channel("telegram"); exists {
			if tg, ok := ch.(*telegram.Telegram); ok {
				if v, ok := config["respond_to_groups"].(bool); ok {
					tg.SetRespondToGroups(v)
				}
				if v, ok := config["respond_to_dms"].(bool); ok {
					tg.SetRespondToDMs(v)
				}
				if v, ok := config["send_typing"].(bool); ok {
					tg.SetSendTyping(v)
				}
				if v, ok := config["reaction_notifications"].(string); ok {
					tg.SetReactionNotifications(v)
				}
			}
		}

		savePath := configPath
		if savePath == "" {
			savePath = "config.yaml"
		}
		return copilot.SaveConfigToFile(cfg, savePath)
	}

	adapter.ConnectTelegramFn = func(token string) error {
		// Store token in vault if available.
		v := assistant.Vault()
		if v != nil && v.IsUnlocked() {
			if err := v.Set("TELEGRAM_BOT_TOKEN", token); err != nil {
				return fmt.Errorf("failed to store Telegram token in vault: %w", err)
			}
			os.Setenv("TELEGRAM_BOT_TOKEN", token)
		}
		cfg.Channels.Telegram.Token = token

		// Persist to config file.
		savePath := configPath
		if savePath == "" {
			savePath = "config.yaml"
		}
		if err := copilot.SaveConfigToFile(cfg, savePath); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		// Unregister stale channel if present (e.g. from a previous disconnect that
		// left it registered, or from boot registration with an old token).
		if _, exists := assistant.ChannelManager().Channel("telegram"); exists {
			_ = assistant.ChannelManager().UnregisterChannel("telegram")
		}

		// Register and connect with the new token.
		tg := telegram.New(cfg.Channels.Telegram, logger)
		if err := assistant.ChannelManager().RegisterAndConnect(tg); err != nil {
			return fmt.Errorf("failed to connect Telegram: %w", err)
		}
		logger.Info("Telegram channel connected via UI")
		return nil
	}

	adapter.DisconnectTelegramFn = func() error {
		// Unregister (disconnects + removes) the channel so it can be re-registered later.
		if _, exists := assistant.ChannelManager().Channel("telegram"); exists {
			if err := assistant.ChannelManager().UnregisterChannel("telegram"); err != nil {
				return err
			}
		}

		// Clear token from config and persist.
		cfg.Channels.Telegram.Token = ""
		savePath := configPath
		if savePath == "" {
			savePath = "config.yaml"
		}
		if err := copilot.SaveConfigToFile(cfg, savePath); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		// Clear from vault if available.
		v := assistant.Vault()
		if v != nil && v.IsUnlocked() {
			_ = v.Delete("TELEGRAM_BOT_TOKEN")
		}
		os.Unsetenv("TELEGRAM_BOT_TOKEN")

		logger.Info("Telegram channel disconnected and token removed via UI")
		return nil
	}

	// ── Telegram Access adapter wiring ──
	// These operate on the global AccessManager with @telegram-suffixed IDs.
	adapter.GetTelegramAccessConfigFn = func() webui.TelegramAccessConfig {
		accessMgr := assistant.AccessManager()
		entries := accessMgr.ListUsersByChannel("@telegram")

		result := webui.TelegramAccessConfig{
			DefaultPolicy: string(accessMgr.DefaultPolicy()),
		}
		for _, e := range entries {
			// Strip @telegram suffix for the UI.
			id := strings.TrimSuffix(e.JID, "@telegram")
			switch e.Level {
			case copilot.AccessOwner:
				result.Owners = append(result.Owners, id)
			case copilot.AccessAdmin:
				result.Admins = append(result.Admins, id)
			case copilot.AccessUser:
				result.AllowedUsers = append(result.AllowedUsers, id)
			case copilot.AccessBlocked:
				result.BlockedUsers = append(result.BlockedUsers, id)
			}
		}
		return result
	}

	adapter.UpdateTelegramAccessDefaultPolicyFn = func(policy string) error {
		accessMgr := assistant.AccessManager()
		accessMgr.SetDefaultPolicy(copilot.AccessPolicy(policy))
		return nil
	}

	adapter.GrantTelegramUserAccessFn = func(id, level string) error {
		accessMgr := assistant.AccessManager()
		normalized := copilot.NormalizeTelegramID(id)
		return accessMgr.Grant(normalized, copilot.AccessLevel(level), "webui")
	}

	adapter.RevokeTelegramUserAccessFn = func(id string) error {
		accessMgr := assistant.AccessManager()
		normalized := copilot.NormalizeTelegramID(id)
		accessMgr.Revoke(normalized, "webui")
		return nil
	}

	adapter.BlockTelegramUserFn = func(id string) error {
		accessMgr := assistant.AccessManager()
		normalized := copilot.NormalizeTelegramID(id)
		accessMgr.Block(normalized, "webui")
		return nil
	}

	adapter.UnblockTelegramUserFn = func(id string) error {
		accessMgr := assistant.AccessManager()
		normalized := copilot.NormalizeTelegramID(id)
		accessMgr.Unblock(normalized, "webui")
		return nil
	}

	// ── Models ──
	adapter.ListModelsFn = func() []webui.ModelInfo {
		seen := make(map[string]bool)
		var models []webui.ModelInfo

		add := func(id, name, provider string) {
			if id == "" || seen[id] {
				return
			}
			seen[id] = true
			models = append(models, webui.ModelInfo{ID: id, Name: name, Provider: provider})
		}

		// Primary model from config
		provider := cfg.API.Provider
		if provider == "" {
			provider = "default"
		}
		add(cfg.Model, cfg.Model, provider)

		// Fallback models
		for _, m := range cfg.Fallback.Models {
			add(m, m, provider)
		}
		for _, entry := range cfg.Fallback.Chain {
			p := entry.Provider
			if p == "" {
				p = provider
			}
			add(entry.Model, entry.Model, p)
		}

		// Discovered models (Ollama, vLLM)
		if pd := assistant.ProviderDiscovery(); pd != nil {
			for _, dm := range pd.ListModels() {
				add(dm.Name, dm.Name, dm.Provider)
			}
		}

		return models
	}

	// ── Agents ──
	wireAgentAdapter(adapter, assistant, cfg, configPath, logger)

	// ── Plugins ──
	adapter.ListPluginsFn = func() []webui.PluginInfoAPI {
		reg := assistant.PluginRegistry()
		if reg == nil {
			return nil
		}
		return reg.List()
	}
	adapter.GetPluginInfoFn = func(id string) *webui.PluginInfoAPI {
		reg := assistant.PluginRegistry()
		if reg == nil {
			return nil
		}
		inst := reg.Get(id)
		if inst == nil {
			return nil
		}
		info := inst.Info()
		return &info
	}
	adapter.ConfigurePluginFn = func(id string, updates map[string]any) error {
		reg := assistant.PluginRegistry()
		if reg == nil {
			return fmt.Errorf("plugin registry not available")
		}
		return reg.ConfigurePlugin(id, updates)
	}
	adapter.TogglePluginFn = func(id string, enabled bool) error {
		reg := assistant.PluginRegistry()
		if reg == nil {
			return fmt.Errorf("plugin registry not available")
		}
		if enabled {
			return reg.Enable(id)
		}
		return reg.Disable(id)
	}
	adapter.InstallPluginFn = func(source string) (*plugins.PluginInstallResult, error) {
		dirs := cfg.Plugins.EffectiveDirs()
		if len(dirs) == 0 {
			return nil, fmt.Errorf("no plugins directory configured")
		}
		installer := plugins.NewPluginInstaller(dirs[0], logger)
		return installer.Install(context.Background(), source)
	}
	adapter.RemovePluginFn = func(name string) error {
		dirs := cfg.Plugins.EffectiveDirs()
		if len(dirs) == 0 {
			return fmt.Errorf("no plugins directory configured")
		}
		installer := plugins.NewPluginInstaller(dirs[0], logger)
		return installer.Remove(name)
	}

	return adapter
}

// syncWhatsAppAccessToConfig syncs the WhatsApp runtime access state back to the config
// and persists it to disk. This ensures access changes survive server restarts.
func syncWhatsAppAccessToConfig(cfg *copilot.Config, waManager channels.WhatsAppAccessManager, configPath string) error {
	// Get current access config from runtime
	accessConfig := waManager.GetAccessConfig()
	if m, ok := accessConfig.(map[string]any); ok {
		// Sync owners
		if v, ok := m["owners"].([]string); ok {
			cfg.Channels.WhatsApp.Access.Owners = v
		}
		// Sync admins
		if v, ok := m["admins"].([]string); ok {
			cfg.Channels.WhatsApp.Access.Admins = v
		}
		// Sync allowed users
		if v, ok := m["allowed_users"].([]string); ok {
			cfg.Channels.WhatsApp.Access.AllowedUsers = v
		}
		// Sync blocked users
		if v, ok := m["blocked_users"].([]string); ok {
			cfg.Channels.WhatsApp.Access.BlockedUsers = v
		}
	}

	// Save to file
	savePath := configPath
	if savePath == "" {
		savePath = "config.yaml"
	}
	return copilot.SaveConfigToFile(cfg, savePath)
}

// wireAgentAdapter connects agent management functions to the WebUI adapter.
func wireAgentAdapter(adapter *webui.AssistantAdapter, assistant *copilot.Assistant, cfg *copilot.Config, configPath string, logger *slog.Logger) {
	wsMgr := assistant.WorkspaceManager()

	// Helper: convert Workspace to AgentInfoAPI
	wsToAgent := func(ws *copilot.Workspace) webui.AgentInfoAPI {
		info := webui.AgentInfoAPI{
			ID:           ws.ID,
			Name:         ws.Name,
			Description:  ws.Description,
			Model:        ws.Model,
			Instructions: ws.Instructions,
			Soul:         ws.Soul,
			Language:     ws.Language,
			Timezone:     ws.Timezone,
			Trigger:      ws.Trigger,
			Skills:       ws.Skills,
			Channels:     ws.Channels,
			Members:      ws.Members,
			Groups:       ws.Groups,
			ToolProfile:  ws.ToolProfile,
			ToolsAllow:   ws.ToolsAllow,
			ToolsDeny:    ws.ToolsDeny,
			MaxTurns:     ws.MaxTurns,
			RunTimeout:   ws.RunTimeout,
			Default:      ws.Default,
			Active:       ws.Active,
			Source:       ws.Source,
			MemberCount:  len(ws.Members),
			GroupCount:   len(ws.Groups),
			SessionCount: wsMgr.SessionCountForWorkspace(ws.ID),
		}
		// Read soul from workspace file for file-backed agents
		if ws.ID != wsMgr.DefaultID() {
			wsDir := paths.ResolveWorkspaceDir(ws.ID)
			if fi, err := os.Stat(wsDir); err == nil && fi.IsDir() {
				info.FileBacked = true
				info.WorkspaceDir = wsDir
				if soul, err := os.ReadFile(filepath.Join(wsDir, "SOUL.md")); err == nil {
					info.Soul = strings.TrimSpace(string(soul))
				}
			}
		}
		if !ws.CreatedAt.IsZero() {
			info.CreatedAt = ws.CreatedAt.Format("2006-01-02T15:04:05Z")
		}
		if ws.Identity != nil {
			info.Identity = &webui.AgentIdentity{
				Name:     ws.Identity.Name,
				Emoji:    ws.Identity.Emoji,
				Theme:    ws.Identity.Theme,
				Avatar:   ws.Identity.Avatar,
				Vibe:     ws.Identity.Vibe,
				Creature: ws.Identity.Creature,
			}
		}
		// Ensure slices are never nil for JSON
		if info.Skills == nil {
			info.Skills = []string{}
		}
		if info.Channels == nil {
			info.Channels = []string{}
		}
		if info.Members == nil {
			info.Members = []string{}
		}
		if info.Groups == nil {
			info.Groups = []string{}
		}
		if info.ToolsAllow == nil {
			info.ToolsAllow = []string{}
		}
		if info.ToolsDeny == nil {
			info.ToolsDeny = []string{}
		}
		return info
	}

	adapter.ListAgentsFn = func() []webui.AgentInfoAPI {
		workspaces := wsMgr.List()
		result := make([]webui.AgentInfoAPI, 0, len(workspaces))
		for _, ws := range workspaces {
			result = append(result, wsToAgent(ws))
		}

		// Include plugin agents as read-only
		reg := assistant.PluginRegistry()
		if reg != nil {
			for _, pi := range reg.List() {
				if !pi.Enabled {
					continue
				}
				for _, agentName := range pi.Agents {
					result = append(result, webui.AgentInfoAPI{
						ID:       pi.ID + ":" + agentName,
						Name:     agentName,
						Source:   "plugin",
						Active:   pi.Enabled,
						Skills:   []string{},
						Channels: []string{},
						Members:  []string{},
						Groups:   []string{},
						ToolsAllow: []string{},
						ToolsDeny:  []string{},
					})
				}
			}
		}

		return result
	}

	adapter.CreateAgentFn = func(req webui.CreateAgentRequest) (string, error) {
		// Auto-generate ID from name if not provided; always slugify for safety.
		id := req.ID
		if id == "" {
			id = copilot.Slugify(req.Name)
		} else {
			id = copilot.Slugify(id)
		}

		ws := copilot.Workspace{
			ID:           id,
			Name:         req.Name,
			Description:  req.Description,
			Model:        req.Model,
			Instructions: req.Instructions,
			Soul:         req.Soul,
			Language:     req.Language,
			Skills:       req.Skills,
			Channels:     req.Channels,
			ToolProfile:  req.ToolProfile,
			MaxTurns:     req.MaxTurns,
			RunTimeout:   req.RunTimeout,
			Source:       "api",
		}
		if req.Identity != nil {
			ws.Identity = &copilot.IdentityConfig{
				Name:     req.Identity.Name,
				Emoji:    req.Identity.Emoji,
				Theme:    req.Identity.Theme,
				Avatar:   req.Identity.Avatar,
				Vibe:     req.Identity.Vibe,
				Creature: req.Identity.Creature,
			}
		}

		if err := wsMgr.Create(ws, "webui"); err != nil {
			return "", err
		}

		// Persist to config
		if err := persistWorkspaces(wsMgr, cfg, configPath); err != nil {
			return "", err
		}
		return id, nil
	}

	adapter.GetAgentFn = func(id string) (*webui.AgentInfoAPI, error) {
		ws, ok := wsMgr.Get(id)
		if !ok {
			return nil, fmt.Errorf("agent %q: %w", id, webui.ErrAgentNotFound)
		}
		info := wsToAgent(ws)
		return &info, nil
	}

	adapter.UpdateAgentFn = func(id string, req webui.UpdateAgentRequest) error {
		err := wsMgr.Update(id, func(ws *copilot.Workspace) {
			if req.Name != nil {
				ws.Name = *req.Name
			}
			if req.Description != nil {
				ws.Description = *req.Description
			}
			if req.Model != nil {
				ws.Model = *req.Model
			}
			if req.Instructions != nil {
				ws.Instructions = *req.Instructions
			}
			if req.Soul != nil {
				ws.Soul = *req.Soul
			}
			if req.Language != nil {
				ws.Language = *req.Language
			}
			if req.Timezone != nil {
				ws.Timezone = *req.Timezone
			}
			if req.Trigger != nil {
				ws.Trigger = *req.Trigger
			}
			if req.Identity != nil {
				ws.Identity = &copilot.IdentityConfig{
					Name:     req.Identity.Name,
					Emoji:    req.Identity.Emoji,
					Theme:    req.Identity.Theme,
					Avatar:   req.Identity.Avatar,
					Vibe:     req.Identity.Vibe,
					Creature: req.Identity.Creature,
				}
			}
			if req.Skills != nil {
				ws.Skills = req.Skills
			}
			if req.Channels != nil {
				ws.Channels = req.Channels
			}
			if req.Members != nil {
				ws.Members = req.Members
			}
			if req.Groups != nil {
				ws.Groups = req.Groups
			}
			if req.ToolProfile != nil {
				ws.ToolProfile = *req.ToolProfile
			}
			if req.ToolsAllow != nil {
				ws.ToolsAllow = req.ToolsAllow
			}
			if req.ToolsDeny != nil {
				ws.ToolsDeny = req.ToolsDeny
			}
			if req.MaxTurns != nil {
				ws.MaxTurns = *req.MaxTurns
			}
			if req.RunTimeout != nil {
				ws.RunTimeout = *req.RunTimeout
			}
			if req.Active != nil {
				ws.Active = *req.Active
			}
		})
		if err != nil {
			return err
		}

		// Rebuild routing maps after update
		wsMgr.RebuildMaps()

		// Sync changes to workspace files for file-backed agents
		if id != wsMgr.DefaultID() {
			wsDir := paths.ResolveWorkspaceDir(id)
			if fi, statErr := os.Stat(wsDir); statErr == nil && fi.IsDir() {
				if req.Soul != nil && *req.Soul != "" {
					if err := os.WriteFile(filepath.Join(wsDir, "SOUL.md"), []byte(*req.Soul), 0600); err != nil {
						logger.Warn("failed to sync SOUL.md", "agent", id, "error", err)
					}
				}
				if req.Identity != nil {
					content := copilot.FormatIdentityMd(&copilot.IdentityConfig{
						Name:     req.Identity.Name,
						Emoji:    req.Identity.Emoji,
						Theme:    req.Identity.Theme,
						Avatar:   req.Identity.Avatar,
						Vibe:     req.Identity.Vibe,
						Creature: req.Identity.Creature,
					})
					if content != "" {
						if err := os.WriteFile(filepath.Join(wsDir, "IDENTITY.md"), []byte(content), 0600); err != nil {
							logger.Warn("failed to sync IDENTITY.md", "agent", id, "error", err)
						}
					}
				}
			}
		}

		return persistWorkspaces(wsMgr, cfg, configPath)
	}

	adapter.DeleteAgentFn = func(id string) error {
		if err := wsMgr.Delete(id, "webui"); err != nil {
			return err
		}
		return persistWorkspaces(wsMgr, cfg, configPath)
	}

	adapter.SetDefaultAgentFn = func(id string) error {
		if err := wsMgr.SetDefault(id); err != nil {
			return err
		}
		return persistWorkspaces(wsMgr, cfg, configPath)
	}

	adapter.ToggleAgentFn = func(id string, active bool) error {
		err := wsMgr.Update(id, func(ws *copilot.Workspace) {
			ws.Active = active
		})
		if err != nil {
			return err
		}
		return persistWorkspaces(wsMgr, cfg, configPath)
	}

	adapter.ListAgentFilesFn = func(id string) (*webui.AgentFilesResponse, error) {
		if _, ok := wsMgr.Get(id); !ok {
			return nil, fmt.Errorf("agent %q: %w", id, webui.ErrAgentNotFound)
		}
		wsDir := paths.ResolveWorkspaceDir(id)
		allFiles := []string{"SOUL.md", "IDENTITY.md", "TOOLS.md", "MEMORY.md", "AGENTS.md", "HEARTBEAT.md"}

		result := &webui.AgentFilesResponse{
			WorkspaceDir: wsDir,
			Files:        make(map[string]*string),
			Inherited:    make(map[string]string),
		}

		for _, name := range allFiles {
			// Check workspace dir first
			wsPath := filepath.Join(wsDir, name)
			if content, err := os.ReadFile(wsPath); err == nil {
				s := strings.TrimSpace(string(content))
				result.Files[name] = &s
				continue
			}
			// Check global fallback
			for _, fallback := range []string{"configs/bootstrap", "configs"} {
				globalPath := filepath.Join(fallback, name)
				if _, err := os.Stat(globalPath); err == nil {
					result.Inherited[name] = globalPath
					break
				}
			}
			result.Files[name] = nil // not present
		}
		return result, nil
	}

	adapter.UpdateAgentFileFn = func(id, filename, content string) error {
		// Defense in depth: validate filename even though the HTTP handler also checks.
		allowed := map[string]bool{
			"SOUL.md": true, "IDENTITY.md": true, "TOOLS.md": true,
			"MEMORY.md": true, "AGENTS.md": true, "HEARTBEAT.md": true,
		}
		if !allowed[filename] {
			return fmt.Errorf("invalid filename: %q", filename)
		}
		if _, ok := wsMgr.Get(id); !ok {
			return fmt.Errorf("agent %q: %w", id, webui.ErrAgentNotFound)
		}
		wsDir := paths.ResolveWorkspaceDir(id)
		if err := os.MkdirAll(wsDir, 0700); err != nil {
			return fmt.Errorf("ensure workspace dir: %w", err)
		}
		return os.WriteFile(filepath.Join(wsDir, filename), []byte(content), 0600)
	}
}

// persistWorkspaces saves the current workspace state back to the config file.
func persistWorkspaces(wsMgr *copilot.WorkspaceManager, cfg *copilot.Config, configPath string) error {
	// Rebuild workspace config from live state.
	workspaces := wsMgr.List()
	cfg.Workspaces.Workspaces = make([]copilot.Workspace, len(workspaces))
	for i, ws := range workspaces {
		cfg.Workspaces.Workspaces[i] = *ws
	}
	cfg.Workspaces.DefaultWorkspace = wsMgr.DefaultID()

	savePath := configPath
	if savePath == "" {
		savePath = "config.yaml"
	}
	return copilot.SaveConfigToFile(cfg, savePath)
}

// syncWhatsAppGroupPoliciesToConfig syncs the WhatsApp runtime group policies
// back to the config and persists it to disk. This ensures group policy changes
// survive server restarts.
func syncWhatsAppGroupPoliciesToConfig(cfg *copilot.Config, waManager channels.WhatsAppAccessManager, configPath string) error {
	// Get current group policies from runtime
	policies := waManager.ListGroupPoliciesForConfig()

	// Convert to config format (whatsapp.GroupPolicyConfig)
	cfg.Channels.WhatsApp.GroupPolicies.Groups = make([]whatsapp.GroupPolicyConfig, 0, len(policies))
	for _, p := range policies {
		cfg.Channels.WhatsApp.GroupPolicies.Groups = append(cfg.Channels.WhatsApp.GroupPolicies.Groups, whatsapp.GroupPolicyConfig{
			ID:           p.ID,
			Name:         p.Name,
			Policy:       p.Policy,
			Policies:     p.Policies,
			Keywords:     p.Keywords,
			AllowedUsers: p.AllowedUsers,
			Workspace:    p.Workspace,
		})
	}

	// Save to file
	savePath := configPath
	if savePath == "" {
		savePath = "config.yaml"
	}
	return copilot.SaveConfigToFile(cfg, savePath)
}
