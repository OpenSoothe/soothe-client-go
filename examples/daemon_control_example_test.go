package soothe_test

import (
	"context"
	"fmt"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_checkDaemonStatus queries the daemon's status via the blocking
// CheckDaemonStatus helper (sends daemon_status, waits for response).
func Example_checkDaemonStatus() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	status, err := soothe.CheckDaemonStatus(ctx, client, 5*time.Second)
	if err != nil {
		fmt.Printf("CheckDaemonStatus error: %v\n", err)
		return
	}
	fmt.Printf("Daemon status: %v\n", status)
	// Output:
	// Daemon status: map[active_loops:2 port_live:true running:true]
}

// Example_isDaemonLive performs a composite health check: connect + status RPC.
// Returns true if the daemon is live and responsive. Useful for readiness probes.
func Example_isDaemonLive() {
	md := NewMockDaemon(nil)
	defer md.Close()

	// This helper creates its own client, connects, checks status, and closes.
	live := soothe.IsDaemonLive(md.URL, 5*time.Second)
	if live {
		fmt.Println("Daemon is live")
	} else {
		fmt.Println("Daemon is not reachable")
	}
	// Output:
	// Daemon is live
}

// Example_waitForDaemonReady waits for the protocol-1 connection_ack handshake
// to report readiness_state "ready". Fast-paths if Connect() already completed it.
func Example_waitForDaemonReady() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		// Connect already performs the handshake; this shows the explicit wait
		// for out-of-band readiness if Connect did not complete it.
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	// WaitForDaemonReady returns immediately if handshake completed during Connect.
	result, err := client.WaitForDaemonReady(10*time.Second)
	if err != nil {
		fmt.Printf("WaitForDaemonReady error: %v\n", err)
		return
	}
	fmt.Printf("Daemon ready: %v\n", result)
	// Output:
	// Daemon ready: map[readiness_state:ready]
}

// Example_daemonShutdown requests a graceful daemon shutdown via RPC.
// The daemon acknowledges with status "acknowledged".
func Example_daemonShutdown() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	if err := soothe.RequestDaemonShutdown(ctx, client, 10*time.Second); err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
		return
	}
	fmt.Println("Daemon shutdown acknowledged")
	// Output:
	// Daemon shutdown acknowledged
}

// Example_configGetReload fetches a config section and triggers a config
// reload on the daemon.
func Example_configGetReload() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	// Fetch the "models" config section via the package-level helper.
	sec, err := soothe.FetchConfigSection(ctx, client, "models", 5*time.Second)
	if err != nil {
		fmt.Printf("FetchConfigSection error: %v\n", err)
	} else {
		fmt.Printf("Models config: %v\n", sec)
	}

	// Request a config reload (e.g. after editing daemon config files).
	if err := soothe.RequestDaemonConfigReload(ctx, client, 10*time.Second); err != nil {
		fmt.Printf("ConfigReload error: %v\n", err)
	} else {
		fmt.Println("ConfigReload: ok")
	}
	// Output:
	// Models config: map[key:value]
	// ConfigReload: ok
}

// Example_sendDaemonStatus uses the low-level SendDaemonStatus (fire-and-forget
// request envelope) when the event reader handles the response.
func Example_sendDaemonStatus() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	_, _ = client.ReceiveMessages(ctx)

	// Fire-and-forget: send daemon_status request.
	if err := client.SendDaemonStatus(ctx); err != nil {
		fmt.Printf("SendDaemonStatus error: %v\n", err)
	}

	// Fire-and-forget: send daemon_shutdown request (with explicit request ID).
	if err := client.SendDaemonShutdown(ctx, "my-shutdown-req-1"); err != nil {
		fmt.Printf("SendDaemonShutdown error: %v\n", err)
	}

	// Fire-and-forget: send config_get for the "models" section.
	if err := client.SendConfigGet(ctx, "models"); err != nil {
		fmt.Printf("SendConfigGet error: %v\n", err)
	}

	// Fire-and-forget: send config_reload.
	if err := client.SendConfigReload(ctx); err != nil {
		fmt.Printf("SendConfigReload error: %v\n", err)
	}
	fmt.Println("Daemon control requests sent")
	// Output:
	// Daemon control requests sent
}

// Example_mcpStatus queries the MCP (Model Context Protocol) server status.
func Example_mcpStatus() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	// Blocking: MCPStatus sends mcp_status and waits for the response.
	result, err := client.MCPStatus(ctx, 15*time.Second)
	if err != nil {
		fmt.Printf("MCPStatus error: %v\n", err)
		return
	}
	fmt.Printf("MCP status: %v\n", result)

	// Low-level alternative: SendMCPStatus (fire-and-forget).
	if err := client.SendMCPStatus(ctx); err != nil {
		fmt.Printf("SendMCPStatus error: %v\n", err)
	}
	// Output:
	// MCP status: map[active:1 servers:[map[name:filesystem status:running]]]
}
