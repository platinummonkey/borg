package agent

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const shutdownTimeout = 10 * time.Second

// Lifecycle manages the agent's startup, signal handling, and graceful shutdown.
type Lifecycle struct {
	agent *Agent
}

// NewLifecycle creates a Lifecycle bound to the given Agent.
func NewLifecycle(a *Agent) *Lifecycle {
	return &Lifecycle{agent: a}
}

// Wait blocks until the context is cancelled or a termination signal is received,
// then performs a graceful shutdown.
func (lc *Lifecycle) Wait(ctx context.Context) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		slog.Info("received signal, shutting down", "signal", sig)
	case <-ctx.Done():
		slog.Info("context cancelled, shutting down")
	}

	return lc.shutdown()
}

func (lc *Lifecycle) shutdown() error {
	done := make(chan struct{})
	go func() {
		lc.agent.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("shutdown complete")
		return nil
	case <-time.After(shutdownTimeout):
		slog.Warn("shutdown timed out, forcing exit")
		return nil
	}
}
