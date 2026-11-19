package pluginapi

import (
	"context"
	"errors"
	"time"
)

// Retry runs an operation at the plugin's chosen delays until it succeeds
// or its activation ends. A delay applies before the corresponding attempt.
func Retry(ctx context.Context, delays []time.Duration, run func(context.Context) error) error {
	var err error
	for _, delay := range delays {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = run(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			return err
		}
	}
	return err
}
