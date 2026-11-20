package reservations

import (
	"testing"
	"time"
)

func TestIsConflicting(t *testing.T) {
	day := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	nextDay := day.AddDate(0, 0, 1)
	left := Reservation{
		Room:      "A",
		Date:      day,
		StartTime: day.Add(10 * time.Hour),
		EndTime:   day.Add(11 * time.Hour),
	}

	tests := []struct {
		name  string
		right Reservation
		want  bool
	}{
		{
			name: "partial_overlap",
			right: Reservation{
				Room:      "A",
				Date:      day,
				StartTime: left.StartTime.Add(30 * time.Minute),
				EndTime:   left.EndTime.Add(30 * time.Minute),
			},
			want: true,
		},
		{
			name: "adjacent",
			right: Reservation{
				Room:      "A",
				Date:      day,
				StartTime: left.EndTime,
				EndTime:   day.Add(12 * time.Hour),
			},
			want: false,
		},
		{
			name: "different_room",
			right: Reservation{
				Room:      "B",
				Date:      day,
				StartTime: left.StartTime,
				EndTime:   left.EndTime,
			},
			want: false,
		},
		{
			name: "different_day",
			right: Reservation{
				Room:      "A",
				Date:      nextDay,
				StartTime: nextDay.Add(10 * time.Hour),
				EndTime:   nextDay.Add(11 * time.Hour),
			},
			want: false,
		},
		{
			name: "different_date_same_timestamps",
			right: Reservation{
				Room:      "A",
				Date:      nextDay,
				StartTime: left.StartTime,
				EndTime:   left.EndTime,
			},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsConflicting(left, test.right); got != test.want {
				t.Errorf("IsConflicting(%+v, %+v) = %t, want %t", left, test.right, got, test.want)
			}
		})
	}
}
