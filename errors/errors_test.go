package sootheerrors

import (
	"errors"
	"testing"
)

func TestConnectionError(t *testing.T) {
	err := NewConnectionError("ws://localhost:8765", 3, errors.New("refused"))
	if err.URL != "ws://localhost:8765" {
		t.Errorf("URL: %s", err.URL)
	}
	if err.Attempt != 3 {
		t.Errorf("Attempt: %d", err.Attempt)
	}
	msg := err.Error()
	if msg == "" {
		t.Error("Error() should not be empty")
	}
	if err.Unwrap() == nil {
		t.Error("Unwrap should return inner error")
	}
}

func TestDaemonError(t *testing.T) {
	err := NewDaemonError("not_found", "thread not found")
	if err.Code != "not_found" {
		t.Errorf("Code: %s", err.Code)
	}
	if err.Message != "thread not found" {
		t.Errorf("Message: %s", err.Message)
	}
	msg := err.Error()
	if msg == "" {
		t.Error("Error() should not be empty")
	}
}

func TestTimeoutError(t *testing.T) {
	err := NewTimeoutError("daemon_ready", "10s")
	if err.Operation != "daemon_ready" {
		t.Errorf("Operation: %s", err.Operation)
	}
	if err.Duration != "10s" {
		t.Errorf("Duration: %s", err.Duration)
	}
	msg := err.Error()
	if msg == "" {
		t.Error("Error() should not be empty")
	}
}
