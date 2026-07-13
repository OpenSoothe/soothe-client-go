package soothe_test

import (
	"context"
	"fmt"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_requestAuth submits access_key/secret_key credentials to the daemon
// via the blocking RequestAuth helper.
func Example_requestAuth() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	resp, err := soothe.RequestAuth(ctx, client, "my-access-key", "my-secret-key", 15*time.Second)
	if err != nil {
		fmt.Printf("RequestAuth error: %v\n", err)
		return
	}
	fmt.Printf("Auth response: %v\n", resp)
	// Output:
	// Auth response: map[access_token:mock-access-token expires_in:3600 refresh_token:mock-refresh-token success:true]
}

// Example_requestAuthRefresh submits a refresh_token to the daemon to obtain
// a new access token without re-sending credentials.
func Example_requestAuthRefresh() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	resp, err := soothe.RequestAuthRefresh(ctx, client, "my-refresh-token", 15*time.Second)
	if err != nil {
		fmt.Printf("RequestAuthRefresh error: %v\n", err)
		return
	}
	fmt.Printf("Auth refresh response: %v\n", resp)
	// Output:
	// Auth refresh response: map[access_token:mock-access-token-2 expires_in:3600 refresh_token:mock-refresh-token-2 success:true]
}

// Example_sendAuth uses the low-level SendAuth / SendAuthRefresh methods
// (fire-and-forget request envelopes) when the event reader handles responses.
func Example_sendAuth() {
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

	// Fire-and-forget: send auth request with credentials.
	if err := client.SendAuth(ctx, "my-access-key", "my-secret-key"); err != nil {
		fmt.Printf("SendAuth error: %v\n", err)
	}

	// Fire-and-forget: send auth_refresh with a refresh token.
	if err := client.SendAuthRefresh(ctx, "my-refresh-token"); err != nil {
		fmt.Printf("SendAuthRefresh error: %v\n", err)
	}

	// With explicit request IDs for correlation.
	if err := client.SendAuth(ctx, "key", "secret", "auth-req-1"); err != nil {
		fmt.Printf("SendAuth error: %v\n", err)
	}
	fmt.Println("Auth requests sent")
	// Output:
	// Auth requests sent
}
