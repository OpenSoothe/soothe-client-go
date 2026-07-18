package appkit

import (
	"context"
	"testing"
)

func TestAppKeyContextRoundTrip(t *testing.T) {
	ctx := ContextWithAppKey(context.Background(), "chat-1")
	got, ok := AppKeyFromContext(ctx)
	if !ok || got != "chat-1" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	if _, ok := AppKeyFromContext(context.Background()); ok {
		t.Fatal("empty context should not have AppKey")
	}
}
