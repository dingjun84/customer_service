// Command smoke runs one real end-to-end chat turn against the configured LLM.
// It is optional and never part of CI: requires LLM_ENABLED=true plus a seeded DB.
//
//	go run ./cmd/smoke -message "我的订单 ORD-10003 到哪了？" -customer CUST-1001
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/example/ecommerce-support/internal/ai"
	"github.com/example/ecommerce-support/internal/config"
	"github.com/example/ecommerce-support/internal/conversation"
	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/repository/sqlite"
	"github.com/example/ecommerce-support/internal/service"
	"github.com/example/ecommerce-support/internal/tools"
)

func main() {
	message := flag.String("message", "你好，我想查一下我的订单物流", "消息内容")
	customerNo := flag.String("customer", "CUST-1001", "演示客户编号")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误: %v\n", err)
		os.Exit(1)
	}
	if !cfg.LLMEnabled {
		fmt.Fprintln(os.Stderr, "LLM_ENABLED=false，无法执行 smoke test")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(ctx, cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "迁移失败: %v\n", err)
		os.Exit(1)
	}

	store := sqlite.New(db)
	customer, err := store.GetCustomerByNo(ctx, *customerNo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "客户 %s 不存在，请先运行 go run ./cmd/seed: %v\n", *customerNo, err)
		os.Exit(1)
	}

	services := service.New(store, service.Options{
		PendingActionTTL: cfg.PendingActionTTL,
		AfterSalesWindow: time.Duration(cfg.AfterSalesWindowDays) * 24 * time.Hour,
	})
	registry := tools.New(services, tools.Options{Timeout: cfg.ToolTimeout})
	model := ai.NewOpenAICompatible(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMTimeout)
	orchestrator := ai.NewOrchestrator(model, registry, store,
		ai.NewAssembler(cfg.ConversationRecentTurns, cfg.ConversationMaxChars),
		ai.Limits{MaxRounds: cfg.LLMMaxToolRounds, MaxToolCalls: cfg.LLMMaxToolCalls}, logger)
	convSvc := conversation.New(store, orchestrator, services.Actions, conversation.Options{Logger: logger})

	conv, err := convSvc.Create(ctx, customer.ID, "smoke")
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建会话失败: %v\n", err)
		os.Exit(1)
	}
	turn, err := convSvc.SendMessage(ctx, conversation.SendInput{
		CustomerID: customer.ID, ConversationID: conv.ID, Text: *message, RequestID: "req-smoke",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "对话失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("用户: %s\n", *message)
	for _, tr := range turn.ToolExecutions {
		fmt.Printf("工具: %s status=%s latency=%dms\n", tr.ToolName, tr.Status, tr.LatencyMS)
	}
	if turn.AssistantMessage != nil {
		fmt.Printf("助手: %s\n", turn.AssistantMessage.Content)
	}
	for _, p := range turn.PendingActions {
		fmt.Printf("待确认: %s (%s) 过期时间 %s\n", p.Summary, p.ActionID, p.ExpiresAt)
	}
}
