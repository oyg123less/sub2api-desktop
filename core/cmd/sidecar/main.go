// Command sidecar is the Sub2API Desktop network gateway core. It runs as a
// local sidecar process spawned by the Tauri shell (or standalone for testing).
// It exposes:
//   - the OpenAI-compatible API on the user-configured port (default 8080)
//   - a loopback control API (protected by a random token) for the desktop UI
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"sub2api-desktop/core/internal/account"
	"sub2api-desktop/core/internal/apiserver"
	"sub2api-desktop/core/internal/config"
	"sub2api-desktop/core/internal/control"
	"sub2api-desktop/core/internal/crypto"
	"sub2api-desktop/core/internal/diagnostics"
	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

// version is overridable at build time via -ldflags.
var version = "0.2.0-dev"

func main() {
	var (
		dataDir      = flag.String("data-dir", defaultDataDir(), "data directory for database and keys")
		controlPort  = flag.Int("control-port", 0, "control API port (0 = random free port)")
		controlToken = flag.String("control-token", "", "control API token (generated if empty)")
		showVersion  = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(*dataDir, *controlPort, *controlToken, logger); err != nil {
		logger.Error("sidecar exited with error", "error", err)
		os.Exit(1)
	}
}

func run(dataDir string, controlPort int, controlToken string, logger *slog.Logger) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	cipher, err := crypto.LoadOrCreate(filepath.Join(dataDir, "key"))
	if err != nil {
		return fmt.Errorf("init crypto: %w", err)
	}
	st, err := store.Open(filepath.Join(dataDir, "sub2api.db"), cipher)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()
	// Opening SQLite alone does not exercise encrypted columns. Force a full
	// credential read before the readiness handshake so data-dir migration can
	// detect a copied database paired with the wrong key and roll back.
	if _, err := st.ListAccounts(); err != nil {
		return fmt.Errorf("validate encrypted accounts: %w", err)
	}
	if _, err := st.ListProxies(); err != nil {
		return fmt.Errorf("validate encrypted proxies: %w", err)
	}

	holder, err := config.NewHolder(st)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	mgr := account.NewManager(st)
	engine := gateway.New(st, mgr, holder.Get, logger)
	apiHandler := apiserver.New(engine, holder.Get)
	apiManager := apiserver.NewManager(apiHandler, holder.Get)
	diagnosticService := diagnostics.New(st, holder.Get, apiManager, dataDir, version)
	cleanupCtx, stopCleanup := context.WithCancel(context.Background())
	defer stopCleanup()
	go maintainLogs(cleanupCtx, st, holder.Get, logger)
	go engine.MaintainUsageSnapshots(cleanupCtx)

	if controlToken == "" {
		controlToken = randomToken()
	}

	ctrl := control.New(st, mgr, holder, apiManager, engine, diagnosticService, controlToken, version)

	// Control API listener (loopback only).
	controlLn, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", controlPort))
	if err != nil {
		return fmt.Errorf("bind control port: %w", err)
	}
	actualControlPort := controlLn.Addr().(*net.TCPAddr).Port

	controlMux := http.NewServeMux()
	ctrl.Mount(controlMux)
	controlSrv := &http.Server{Handler: control.WithCORS(controlMux), ReadHeaderTimeout: 15 * time.Second}
	go func() {
		if err := controlSrv.Serve(controlLn); err != nil && err != http.ErrServerClosed {
			logger.Error("control server error", "error", err)
		}
	}()

	// Emit handshake so the parent (Tauri) can discover port + token.
	handshake, _ := json.Marshal(map[string]any{
		"control_port":  actualControlPort,
		"control_token": controlToken,
		"version":       version,
	})
	fmt.Println("SUB2API_READY " + string(handshake))

	// Auto-start the API server if configured.
	if holder.Get().AutoStartServer {
		if err := apiManager.Start(); err != nil {
			logger.Warn("auto-start api server failed", "error", err)
		} else {
			logger.Info("api server started", "port", apiManager.Port())
		}
	}

	logger.Info("sidecar ready", "control_port", actualControlPort, "data_dir", dataDir)

	// Wait for termination.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	_ = apiManager.Stop()
	_ = controlSrv.Close()
	return nil
}

func maintainLogs(ctx context.Context, st *store.Store, settings func() store.Settings, logger *slog.Logger) {
	cleanup := func() {
		cfg := settings()
		deleted, err := st.CleanupLogs(cfg.LogRetentionDays, cfg.MaxLogRows)
		if err != nil {
			logger.Warn("request log cleanup failed", "error", err)
		} else if deleted > 0 {
			logger.Info("request logs cleaned", "deleted", deleted)
		}
	}
	cleanup()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func defaultDataDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "sub2api-desktop")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sub2api-desktop")
}
