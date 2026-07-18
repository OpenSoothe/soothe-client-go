package appkit

import "context"

// AppKey is the application conversation key used by ConnectionPool, QueryGate,
// TurnRunner, and SessionStore (e.g. Triarch chat_id).
//
// It is NOT a daemon protocol identity. The daemon's first-class ids are
// loop_id (conversation continuity) and client_id (WebSocket connection).
// AppKit maps AppKey → loop_id via SessionStore; that mapping never leaves
// the product process as a wire "session_id".
type AppKey = string

type appKeyContextKey struct{}

// ContextWithAppKey attaches an AppKey to ctx for bootstrap callbacks and
// other helpers that must not take a fake protocol sessionID parameter.
func ContextWithAppKey(ctx context.Context, appKey AppKey) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, appKeyContextKey{}, appKey)
}

// AppKeyFromContext returns the AppKey previously attached with ContextWithAppKey.
func AppKeyFromContext(ctx context.Context) (AppKey, bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(appKeyContextKey{}).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}
