package main

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestDetectionOptions(t *testing.T) {
	const enabled = "ROOM_CONFLICT_DETECTION_ENABLED"
	const interval = "ROOM_CONFLICT_DETECTION_INTERVAL_MINUTES"
	const lookback = "ROOM_CONFLICT_DETECTION_LOOKBACK_MINUTES"
	for _, key := range []string{enabled, interval, lookback} {
		t.Setenv(key, "")
	}
	defaults := detectionOptions{interval: time.Minute, lookback: 15 * time.Minute}
	if got, err := readDetectionOptions(); err != nil || got != defaults {
		t.Fatalf("defaults: %+v %v", got, err)
	}
	t.Setenv(interval, "0")
	t.Setenv(lookback, "-1")
	if got, err := readDetectionOptions(); err != nil || got != defaults {
		t.Fatalf("fallbacks: %+v %v", got, err)
	}
	t.Setenv(enabled, "true")
	t.Setenv(interval, "2")
	t.Setenv(lookback, "17")
	if got, err := readDetectionOptions(); err != nil || got != (detectionOptions{true, 2 * time.Minute, 17 * time.Minute}) {
		t.Fatalf("configured: %+v %v", got, err)
	}
	for key, value := range map[string]string{enabled: "yes", interval: "1.5", lookback: "2147483647"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := readDetectionOptions(); err == nil {
				t.Fatal("invalid value accepted")
			}
		})
	}
}

func TestDetectionWorkerSchedule(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		start := time.Now()
		var calls []time.Time
		done := make(chan struct{})
		go func() {
			defer close(done)
			runDetectionWorker(ctx, detectionOptions{true, time.Minute, 15 * time.Minute}, func(ctx context.Context, now time.Time, window time.Duration) (int, error) {
				calls = append(calls, now)
				if window != 15*time.Minute {
					t.Errorf("lookback=%s", window)
				}
				if len(calls) == 1 {
					time.Sleep(2 * time.Second)
					return 0, errors.New("synthetic failure")
				}
				<-ctx.Done()
				return 0, ctx.Err()
			})
		}()
		time.Sleep(61 * time.Second)
		synctest.Wait()
		if len(calls) != 1 || !calls[0].Equal(start) {
			t.Fatalf("first pass: %v", calls)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if len(calls) != 2 || calls[1].Sub(start) != 62*time.Second {
			t.Fatalf("retry: %v", calls)
		}
		cancel()
		<-done
	})
}
