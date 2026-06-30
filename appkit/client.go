package appkit

import (
	"context"
	"fmt"

	soothe "github.com/mirasoth/soothe-client-go"
)

// ManagedClient is the subset of the core Client that appkit's ConnectionPool
// and TurnRunner depend on. The concrete *soothe.Client satisfies it; tests
// supply a fake. Defining it as an interface lets appkit be unit-tested
// without a live WebSocket daemon.
//
// Note: loop bootstrap (loop_new + subscribe) is performed via a separate
// BootstrapFunc (see DefaultBootstrapFunc) because soothe.BootstrapLoopSession
// is a package-level function, not a method on *Client.
type ManagedClient interface {
	// Connect dials and handshakes.
	Connect(ctx context.Context) error
	// Reconnect re-dials after a drop.
	Reconnect(ctx context.Context) error
	// ReattachAndProbe resumes a loop by id and probes liveness.
	ReattachAndProbe(ctx context.Context, loopID string) error
	// SendMessage sends a fire-and-forget notification (e.g. loop_input).
	SendMessage(ctx context.Context, msg interface{}) error
	// ReceiveMessages starts the read loop, returning the event stream.
	ReceiveMessages(ctx context.Context) (<-chan interface{}, error)
	// Disconnected returns the drop signal (closed once on drop).
	Disconnected() <-chan soothe.DisconnectCause
	// IsConnected reports connection liveness.
	IsConnected() bool
	// Close tears down the connection.
	Close() error
}

// ClientFactory builds a fresh ManagedClient for a daemon URL and config.
// ConnectionPool calls it per pooled connection. Applications may supply a
// custom factory (e.g. wrapping *soothe.Client with logging/metrics).
type ClientFactory func(url string, cfg *soothe.Config) ManagedClient

// DefaultClientFactory returns a ClientFactory that builds a *soothe.Client.
func DefaultClientFactory() ClientFactory {
	return func(url string, cfg *soothe.Config) ManagedClient {
		return soothe.NewClient(url, cfg)
	}
}

// BootstrapFunc creates a new loop (loop_new + subscribe) on a connected
// client and returns the new loop id. The default implementation calls
// soothe.BootstrapLoopSession; apps may override it (e.g. to inject custom
// LoopSessionOptions).
type BootstrapFunc func(ctx context.Context, client ManagedClient, workspaceID, userID string, cfg *soothe.Config) (string, error)

// DefaultBootstrapFunc calls soothe.BootstrapLoopSession on the underlying
// *soothe.Client. If the client is not a *soothe.Client (e.g. a test fake),
// the app must supply its own BootstrapFunc.
func DefaultBootstrapFunc() BootstrapFunc {
	return func(ctx context.Context, client ManagedClient, workspaceID, userID string, cfg *soothe.Config) (string, error) {
		sc, ok := client.(*soothe.Client)
		if !ok {
			return "", fmt.Errorf("appkit: DefaultBootstrapFunc requires *soothe.Client, got %T", client)
		}
		opts := &soothe.LoopSessionOptions{
			ClientWorkspace:   workspaceID,
			UserID:            userID,
			ClientWorkspaceID: workspaceID,
		}
		return soothe.BootstrapLoopSession(ctx, sc, "", workspaceID, cfg, opts)
	}
}
