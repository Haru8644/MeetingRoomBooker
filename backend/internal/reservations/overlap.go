package reservations

import "time"

type Reservation struct {
	Room      string
	Date      time.Time
	StartTime time.Time
	EndTime   time.Time
}

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
