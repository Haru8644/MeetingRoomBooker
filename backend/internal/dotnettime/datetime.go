// Package dotnettime converts .NET DateTime ticks and zone-free wire values.
package dotnettime

import "time"

const (
	Layout         = "2006-01-02T15:04:05.9999999"
	TicksPerDay    = 864_000_000_000
	unixEpochTicks = 621355968000000000
	ticksPerSecond = 10_000_000
)

func FromTicks(ticks int64) time.Time {
	unixTicks := ticks - unixEpochTicks
	return time.Unix(unixTicks/ticksPerSecond, unixTicks%ticksPerSecond*100).UTC()
}

func Ticks(value time.Time) int64 {
	return unixEpochTicks + value.Unix()*ticksPerSecond + int64(value.Nanosecond())/100
}

// OnDate combines the calendar date and time of day without changing either to another zone.
func OnDate(date, clock time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), clock.Second(), clock.Nanosecond(), time.UTC)
}
