package chatwork

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestReminderLoopRunsImmediatelyAndWaitsAfterEachPass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, stop := context.WithCancel(t.Context())
		defer stop()
		start := time.Now()
		var calls []time.Time
		done := make(chan struct{})
		go func() {
			defer close(done)
			runReminderLoop(ctx, func(_ context.Context, now time.Time) error {
				calls = append(calls, now)
				time.Sleep(2 * time.Second)
				return errors.New("synthetic poll failure")
			})
		}()
		synctest.Wait()
		if len(calls) != 1 || !calls[0].Equal(start) {
			t.Fatalf("expected startup pass, got %v", calls)
		}
		time.Sleep(61 * time.Second)
		synctest.Wait()
		if len(calls) != 1 {
			t.Fatalf("ran again before one minute had passed after the first pass: %v", calls)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if len(calls) != 2 || calls[1].Sub(start) != 62*time.Second {
			t.Fatalf("expected second pass one minute after completion, got %v", calls)
		}
		time.Sleep(2 * time.Second)
		stop()
		<-done
		time.Sleep(time.Minute)
		if len(calls) != 2 {
			t.Fatalf("ran after cancellation: %v", calls)
		}
	})
}
