package service

import (
	"context"
	"errors"
	"net"
	"strings"
)

// IsTimeoutError returns true when an error is very likely caused by an upstream request timeout.
// This is used for retry/circuit-break decisions.
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Context deadline
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// net.Error timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Fallback for common http.Client timeout strings
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "client.timeout exceeded") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "timeout") && strings.Contains(msg, "exceeded") {
		return true
	}

	return false
}

