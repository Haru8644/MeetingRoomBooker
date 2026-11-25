package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"
)

type detectionOptions struct {
	enabled            bool
	interval, lookback time.Duration
}

func readDetectionOptions() (detectionOptions, error) {
	var opts detectionOptions
	var err error
	if value := os.Getenv("ROOM_CONFLICT_DETECTION_ENABLED"); value != "" {
		if opts.enabled, err = strconv.ParseBool(value); err != nil {
			return opts, errors.New("ROOM_CONFLICT_DETECTION_ENABLED must be a boolean")
		}
	}
	if opts.interval, err = detectionMinutes("ROOM_CONFLICT_DETECTION_INTERVAL_MINUTES", 1); err != nil {
		return opts, err
	}
	opts.lookback, err = detectionMinutes("ROOM_CONFLICT_DETECTION_LOOKBACK_MINUTES", 15)
	return opts, err
}

func detectionMinutes(name string, fallback int64) (time.Duration, error) {
	value := os.Getenv(name)
	minutes := fallback
	if value != "" {
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil || n > math.MaxInt64/int64(time.Minute) {
			return 0, fmt.Errorf("%s must be an integer minute count within the supported duration range", name)
		}
		if n > 0 {
			minutes = n
		}
	}
	return time.Duration(minutes) * time.Minute, nil
}

func runDetectionWorker(ctx context.Context, opts detectionOptions, detect func(context.Context, time.Time, time.Duration) (int, error)) {
	for ctx.Err() == nil {
		count, err := detect(ctx, time.Now(), opts.lookback)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Print("Room conflict detection pass failed")
		} else if count > 0 {
			log.Printf("Room conflict detection created %d records", count)
		}
		timer := time.NewTimer(opts.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
