package tools

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/cron"
)

func newTestCronToolWithConfig(t *testing.T, cfg *config.Config) *CronTool {
	return newTestCronToolWithTimeout(t, cfg, 0)
}

func newTestCronToolWithTimeout(t *testing.T, cfg *config.Config, execTimeout time.Duration) *CronTool {
	t.Helper()
	storePath := filepath.Join(t.TempDir(), "cron.json")
	cronService := cron.NewCronService(storePath, nil)
	msgBus := bus.NewMessageBus()
	tool, err := NewCronTool(cronService, nil, msgBus, t.TempDir(), true, execTimeout, cfg)
	if err != nil {
		t.Fatalf("NewCronTool() error: %v", err)
	}
	return tool
}

func newTestCronTool(t *testing.T) *CronTool {
	t.Helper()
	return newTestCronToolWithConfig(t, config.DefaultConfig())
}

type fakeCronExecutor struct {
	err error
}

func (f fakeCronExecutor) ProcessDirectWithChannel(
	_ context.Context,
	_, _, _, _ string,
) (string, error) {
	return "", f.err
}

type blockingCronExecutor struct{}

func (blockingCronExecutor) ProcessDirectWithChannel(
	ctx context.Context,
	_, _, _, _ string,
) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

// TestCronTool_CommandBlockedFromRemoteChannel verifies command scheduling is restricted to internal channels
func TestCronTool_CommandBlockedFromRemoteChannel(t *testing.T) {
	tool := newTestCronTool(t)
	ctx := WithToolContext(context.Background(), "telegram", "chat-1")
	result := tool.Execute(ctx, map[string]any{
		"action":          "add",
		"message":         "check disk",
		"command":         "df -h",
		"command_confirm": true,
		"at_seconds":      float64(60),
	})

	if !result.IsError {
		t.Fatal("expected command scheduling to be blocked from remote channel")
	}
	if !strings.Contains(result.ForLLM, "restricted to internal channels") {
		t.Errorf("expected 'restricted to internal channels', got: %s", result.ForLLM)
	}
}

func TestCronTool_CommandDoesNotRequireConfirmByDefault(t *testing.T) {
	tool := newTestCronTool(t)
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{
		"action":     "add",
		"message":    "check disk",
		"command":    "df -h",
		"at_seconds": float64(60),
	})

	if result.IsError {
		t.Fatalf("expected command scheduling without confirm to succeed by default, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Cron job added") {
		t.Errorf("expected 'Cron job added', got: %s", result.ForLLM)
	}
}

func TestCronTool_CommandRequiresConfirmWhenAllowCommandDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Cron.AllowCommand = false

	tool := newTestCronToolWithConfig(t, cfg)
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{
		"action":     "add",
		"message":    "check disk",
		"command":    "df -h",
		"at_seconds": float64(60),
	})

	if !result.IsError {
		t.Fatal("expected command scheduling to require confirm when allow_command is disabled")
	}
	if !strings.Contains(result.ForLLM, "command_confirm=true") {
		t.Errorf("expected command_confirm requirement message, got: %s", result.ForLLM)
	}
}

func TestCronTool_CommandAllowedWithConfirmWhenAllowCommandDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Cron.AllowCommand = false

	tool := newTestCronToolWithConfig(t, cfg)
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{
		"action":          "add",
		"message":         "check disk",
		"command":         "df -h",
		"command_confirm": true,
		"at_seconds":      float64(60),
	})

	if result.IsError {
		t.Fatalf(
			"expected command scheduling with confirm to succeed when allow_command is disabled, got: %s",
			result.ForLLM,
		)
	}
	if !strings.Contains(result.ForLLM, "Cron job added") {
		t.Errorf("expected 'Cron job added', got: %s", result.ForLLM)
	}
}

func TestCronTool_CommandBlockedWhenExecDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Exec.Enabled = false

	tool := newTestCronToolWithConfig(t, cfg)
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{
		"action":          "add",
		"message":         "check disk",
		"command":         "df -h",
		"command_confirm": true,
		"at_seconds":      float64(60),
	})

	if !result.IsError {
		t.Fatal("expected command scheduling to be blocked when exec is disabled")
	}
	if !strings.Contains(result.ForLLM, "command execution is disabled") {
		t.Errorf("expected exec disabled message, got: %s", result.ForLLM)
	}
}

// TestCronTool_CommandAllowedFromInternalChannel verifies command scheduling works from internal channels
func TestCronTool_CommandAllowedFromInternalChannel(t *testing.T) {
	tool := newTestCronTool(t)
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{
		"action":          "add",
		"message":         "check disk",
		"command":         "df -h",
		"command_confirm": true,
		"at_seconds":      float64(60),
	})

	if result.IsError {
		t.Fatalf("expected command scheduling to succeed from internal channel, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Cron job added") {
		t.Errorf("expected 'Cron job added', got: %s", result.ForLLM)
	}
}

// TestCronTool_AddJobRequiresSessionContext verifies fail-closed when channel/chatID missing
func TestCronTool_AddJobRequiresSessionContext(t *testing.T) {
	tool := newTestCronTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"action":     "add",
		"message":    "reminder",
		"at_seconds": float64(60),
	})

	if !result.IsError {
		t.Fatal("expected error when session context is missing")
	}
	if !strings.Contains(result.ForLLM, "no session context") {
		t.Errorf("expected 'no session context' message, got: %s", result.ForLLM)
	}
}

// TestCronTool_NonCommandJobAllowedFromRemoteChannel verifies regular reminders work from any channel
func TestCronTool_NonCommandJobAllowedFromRemoteChannel(t *testing.T) {
	tool := newTestCronTool(t)
	ctx := WithToolContext(context.Background(), "telegram", "chat-1")
	result := tool.Execute(ctx, map[string]any{
		"action":     "add",
		"message":    "time to stretch",
		"at_seconds": float64(600),
	})

	if result.IsError {
		t.Fatalf("expected non-command reminder to succeed from remote channel, got: %s", result.ForLLM)
	}
}

func TestCronTool_NonCommandJobDefaultsDeliverToFalse(t *testing.T) {
	tool := newTestCronTool(t)
	ctx := WithToolContext(context.Background(), "telegram", "chat-1")
	result := tool.Execute(ctx, map[string]any{
		"action":     "add",
		"message":    "send me a poem",
		"at_seconds": float64(600),
	})

	if result.IsError {
		t.Fatalf("expected non-command reminder to succeed, got: %s", result.ForLLM)
	}

	jobs := tool.cronService.ListJobs(false)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Payload.Deliver {
		t.Fatal("expected deliver=false by default for non-command jobs")
	}
}

func TestCronTool_ExecuteJobPublishesErrorWhenExecDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Exec.Enabled = false

	tool := newTestCronToolWithConfig(t, cfg)
	job := &cron.CronJob{}
	job.Payload.Channel = "cli"
	job.Payload.To = "direct"
	job.Payload.Command = "df -h"

	got, err := tool.ExecuteJob(context.Background(), job)
	if err != nil {
		t.Fatalf("ExecuteJob() error = %v", err)
	}
	if got != "ok" {
		t.Fatalf("ExecuteJob() = %q, want ok", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var msg bus.OutboundMessage
	select {
	case msg = <-tool.msgBus.OutboundChan():
		// got message
	case <-ctx.Done():
		t.Fatal("timeout waiting for outbound message")
	}
	if !strings.Contains(msg.Content, "command execution is disabled") {
		t.Fatalf("expected exec disabled message, got: %s", msg.Content)
	}
}

func TestCronTool_ExecuteJobPropagatesAgentError(t *testing.T) {
	tool := newTestCronTool(t)
	tool.executor = fakeCronExecutor{err: errors.New("agent failed")}

	job := &cron.CronJob{}
	job.Payload.Channel = "telegram"
	job.Payload.To = "chat-1"
	job.Payload.Message = "remind me"

	got, err := tool.ExecuteJob(context.Background(), job)
	if err == nil {
		t.Fatal("expected agent error to propagate")
	}
	if got != "" {
		t.Fatalf("ExecuteJob() result = %q, want empty string on error", got)
	}
	if !strings.Contains(err.Error(), "agent failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCronTool_ExecuteJobAppliesExecTimeoutToAgentProcessing(t *testing.T) {
	tool := newTestCronToolWithTimeout(t, config.DefaultConfig(), 25*time.Millisecond)
	tool.executor = blockingCronExecutor{}

	job := &cron.CronJob{}
	job.Payload.Channel = "telegram"
	job.Payload.To = "chat-1"
	job.Payload.Message = "remind me"

	_, err := tool.ExecuteJob(context.Background(), job)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
