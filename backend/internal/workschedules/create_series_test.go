package workschedules

import (
	"slices"
	"testing"
	"time"
)

func TestOccurrenceDates(t *testing.T) {
	tests := []struct {
		name, repeat, from, until string
		want                      []string
		count                     int
		message                   string
	}{
		{name: "daily across weekend", repeat: daily, from: "2026-06-19", until: "2026-06-23",
			want: []string{"2026-06-19", "2026-06-22", "2026-06-23"}},
		{name: "weekly inclusive", repeat: weekly, from: "2026-06-22", until: "2026-07-06",
			want: []string{"2026-06-22", "2026-06-29", "2026-07-06"}},
		{name: "weekly weekend anchor", repeat: weekly, from: "2026-06-20", until: "2026-07-20",
			message: "作成対象の日付がありません。"},
		{name: "only weekend dates", repeat: daily, from: "2026-06-20", until: "2026-06-21",
			message: "作成対象の日付がありません。"},
		{name: "at the limit", repeat: daily, from: "2026-06-22", until: "2026-09-11", count: 60},
		{name: "past the limit", repeat: daily, from: "2026-06-22", until: "2026-09-14",
			message: "繰り返し作成は最大60件までです。"},
		{name: "last representable day", repeat: daily, from: "9999-12-31", until: "9999-12-31",
			want: []string{"9999-12-31"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			from, err := time.Parse(time.DateOnly, test.from)
			if err != nil {
				t.Fatal(err)
			}
			until, err := time.Parse(time.DateOnly, test.until)
			if err != nil {
				t.Fatal(err)
			}
			dates, err := occurrenceDates(CreateInput{Date: from, RepeatUntil: &until}, test.repeat)
			if test.message != "" {
				if err == nil || err.Error() != test.message || len(dates) != 0 {
					t.Fatalf("occurrenceDates = %v, %v, want %q with no dates", dates, err, test.message)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.count > 0 {
				if len(dates) != test.count || !dates[0].Equal(from) || !dates[len(dates)-1].Equal(until) {
					t.Fatalf("wrong range or occurrence count: %v", dates)
				}
				return
			}
			got := make([]string, len(dates))
			for i, day := range dates {
				got[i] = day.Format(time.DateOnly)
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("dates = %v, want %v", got, test.want)
			}
		})
	}
}
