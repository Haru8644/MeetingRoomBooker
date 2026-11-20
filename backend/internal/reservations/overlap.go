package reservations

import "time"

// Reservation holds the fields used by the overlap check.
type Reservation struct {
	Room      string
	Date      time.Time // The booking's calendar date is supplied independently of its times.
	StartTime time.Time
	EndTime   time.Time
}

// IsConflicting reports whether two reservations overlap in the same room on the same date.
func IsConflicting(left, right Reservation) bool {
	if left.Room != right.Room {
		return false
	}

	leftYear, leftMonth, leftDay := left.Date.Date()
	rightYear, rightMonth, rightDay := right.Date.Date()
	if leftYear != rightYear || leftMonth != rightMonth || leftDay != rightDay {
		return false
	}

	return left.StartTime.Before(right.EndTime) && left.EndTime.After(right.StartTime)
}
