package httpapi

import (
	"context"
	"time"
)

func timeBoundContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}
