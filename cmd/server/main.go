// Command server starts the ecommerce AI customer-service MVP HTTP service.
//
// It opens (and migrates) the SQLite database, wires application services, the
// tool registry and the LLM adapter, then serves the HTTP API until SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/example/ecommerce-support/internal/admin"
	"github.com/example/ecommerce-support/internal/adminapi"
	"github.com/example/ecommerce-support/internal/ai"
	"github.com/example/ecommerce-support/internal/config"
	"github.com/example/ecommerce-support/internal/conversation"
	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/httpapi"
	"github.com/example/ecommerce-support/internal/repository/sqlite"
	"github.com/example/ecommerce-support/internal/service"
	"github.com/example/ecommerce-support/internal/tools"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}
	logger := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		return fmt.Errorf("执行数据库迁移失败: %w", err)
	}
	version, err := database.SchemaVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("读取迁移版本失败: %w", err)
	}

	store := sqlite.New(db)
	services := service.New(store, service.Options{
		PendingActionTTL:      cfg.PendingActionTTL,
		AfterSalesWindow:      time.Duration(cfg.AfterSalesWindowDays) * 24 * time.Hour,
		PriceProtectionWindow: time.Duration(cfg.PriceProtectionDays) * 24 * time.Hour,
	})

	registry := tools.New(services, tools.Options{Timeout: cfg.ToolTimeout})
	assembler := ai.NewAssembler(cfg.ConversationRecentTurns, cfg.ConversationMaxChars)

	recorder, closeCallLog, err := newCallRecorder(cfg, logger)
	if err != nil {
		return err
	}
	defer closeCallLog()

	var model ai.ChatModel
	switch cfg.LLMMode() {
	case "offline":
		model = ai.NewOfflineModel()
		logger.Warn("LLM_PROVIDER=offline: using the local rule-based demo model (not a language model)")
	case "openai":
		adapter := ai.NewOpenAICompatible(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMTimeout)
		adapter.Recorder = recorder
		model = adapter
		logger.Info("LLM adapter enabled",
			"model", cfg.LLMModel,
			"base_url", cfg.LLMBaseURL,
			"call_log_level", recorder.Level().String(),
			"call_log_file", cfg.LLMLogFile)
	default:
		logger.Warn("LLM disabled: chat endpoint will return llm_unavailable until LLM_ENABLED=true or LLM_PROVIDER=offline")
	}

	orchestrator := ai.NewOrchestrator(model, registry, store, assembler, ai.Limits{
		MaxRounds:    cfg.LLMMaxToolRounds,
		MaxToolCalls: cfg.LLMMaxToolCalls,
	}, logger)

	convService := conversation.New(store, orchestrator, services.Actions, conversation.Options{
		IdempotencyTTL:  24 * time.Hour,
		MaxMessageChars: 2000,
		HistoryLimit:    cfg.ConversationRecentTurns * 8,
		Logger:          logger,
	})

	// Operations console: its own accounts and sessions, sharing the database
	// but never the customer identity header.
	adminAuth := admin.NewAuth(db, cfg.AdminSessionTTL)
	created, err := adminAuth.BootstrapUser(ctx, cfg.AdminUsername, cfg.AdminPassword, "系统管理员")
	if err != nil {
		return fmt.Errorf("初始化后台管理员失败: %w", err)
	}
	if created {
		logger.Warn("built-in console account created",
			"username", cfg.AdminUsername,
			"password_source", "ADMIN_PASSWORD",
			"note", "the console is at /admin/; change ADMIN_PASSWORD before any non-local deployment")
	} else {
		logger.Info("console accounts already exist; ADMIN_PASSWORD from the environment is ignored")
	}
	if purged, err := adminAuth.PurgeExpired(ctx); err == nil && purged > 0 {
		logger.Info("purged expired console sessions", "count", purged)
	}
	adminService := admin.NewService(admin.NewStore(db), adminAuth, admin.Metadata{
		SchemaVersion: version,
		DBPath:        cfg.DBPath,
		AppEnv:        cfg.AppEnv,
		LLMMode:       cfg.LLMMode(),
		ExposeDBPath:  cfg.IsDev(),
		SessionTTL:    cfg.AdminSessionTTL,
	})
	adminHTTP := adminapi.NewServer(adminService, cfg, logger)

	httpServer := httpapi.NewServer(cfg, db, store, services, convService, adminHTTP, logger)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening",
			"addr", cfg.HTTPAddr,
			"app_env", cfg.AppEnv,
			"db_path", cfg.DBPath,
			"schema_version", version,
			"dev_api_enabled", cfg.DevAPIAllowed())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("优雅关闭失败: %w", err)
	}
	if n := recorder.Dropped(); n > 0 {
		logger.Warn("llm call log lines could not be written", "dropped", n, "file", cfg.LLMLogFile)
	}
	return nil
}

// newCallRecorder builds the model-call log sink: a JSONL file for later
// grep/jq, mirrored into the process log. A level of "off" returns a nil
// recorder, which the adapter treats as "do not log".
func newCallRecorder(cfg config.Config, logger *slog.Logger) (*ai.Recorder, func(), error) {
	level, err := ai.ParseCallLogLevel(cfg.LLMLogLevel)
	if err != nil {
		return nil, nil, fmt.Errorf("配置错误: %w", err)
	}
	noop := func() {}
	if level == ai.CallLogOff {
		return nil, noop, nil
	}

	var (
		writer io.Writer
		closer = noop
	)
	switch path := strings.TrimSpace(cfg.LLMLogFile); path {
	case "", "-", "off", "none":
		// File sink disabled; records still go to the process log.
	default:
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, fmt.Errorf("创建调用日志目录失败: %w", err)
			}
		}
		// 0600: the log carries masked-but-still-sensitive customer text.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, fmt.Errorf("打开调用日志文件失败: %w", err)
		}
		writer, closer = f, func() { _ = f.Close() }
		logger.Info("llm call log", "level", level.String(), "file", path)
	}

	recorder := ai.NewRecorder(ai.RecorderOptions{
		Level:     level,
		BodyChars: cfg.LLMLogBodyChars,
		Logger:    logger,
		Writer:    writer,
	})
	return recorder, closer, nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
