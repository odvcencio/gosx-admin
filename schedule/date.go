package schedule

import (
	"fmt"
	"time"
)

// Date is a calendar date with no time or zone.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the local date of t in loc. A nil loc means UTC.
func DateOf(t time.Time, loc *time.Location) Date {
	if loc == nil {
		loc = time.UTC
	}
	t = t.In(loc)
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// ParseDate parses "2006-01-02". It rejects dates that do not exist.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil || t.Format("2006-01-02") != s {
		return Date{}, fmt.Errorf("schedule: invalid date %q", s)
	}
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}, nil
}

// String formats d as "2006-01-02". The zero Date formats as "".
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// IsZero reports whether d is the zero Date.
func (d Date) IsZero() bool { return d.Year == 0 && d.Month == 0 && d.Day == 0 }

// AddDays returns d plus n days (n may be negative).
func (d Date) AddDays(n int) Date {
	if d.IsZero() {
		return Date{}
	}
	t := time.Date(d.Year, d.Month, d.Day+n, 0, 0, 0, 0, time.UTC)
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// Compare returns -1, 0, or +1 as d is before, equal to, or after o.
func (d Date) Compare(o Date) int {
	if d.Year != o.Year {
		if d.Year < o.Year {
			return -1
		}
		return 1
	}
	if d.Month != o.Month {
		if d.Month < o.Month {
			return -1
		}
		return 1
	}
	if d.Day < o.Day {
		return -1
	}
	if d.Day > o.Day {
		return 1
	}
	return 0
}

// Before reports whether d is before o.
func (d Date) Before(o Date) bool { return d.Compare(o) < 0 }

// After reports whether d is after o.
func (d Date) After(o Date) bool { return d.Compare(o) > 0 }

// Weekday returns d's day of the week.
func (d Date) Weekday() time.Weekday {
	if d.IsZero() {
		return time.Sunday
	}
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).Weekday()
}

// Start returns the earliest instant whose local date in loc is d. It is
// local midnight except on a day when a clock change skips midnight; then it
// is the first instant after the gap. A nil loc means UTC.
func (d Date) Start(loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	if d.IsZero() {
		return time.Time{}
	}
	midnight := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, loc)
	if DateOf(midnight, loc) == d {
		return midnight
	}

	// time.Date may normalize a skipped midnight to the previous civil day.
	// Search instants around the civil date for its first representable time.
	center := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
	lo := center.Add(-48 * time.Hour).UnixNano()
	hi := center.Add(48 * time.Hour).UnixNano()
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		if DateOf(time.Unix(0, mid), loc).Before(d) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return time.Unix(0, hi).In(loc)
}

// MarshalText implements encoding.TextMarshaler as String.
func (d Date) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler with ParseDate. Empty
// text yields the zero Date.
func (d *Date) UnmarshalText(text []byte) error {
	if d == nil {
		return fmt.Errorf("schedule: nil Date receiver")
	}
	if len(text) == 0 {
		*d = Date{}
		return nil
	}
	parsed, err := ParseDate(string(text))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

func validDate(d Date) bool {
	if d.IsZero() || d.Year < 1 || d.Month < time.January || d.Month > time.December || d.Day < 1 || d.Day > 31 {
		return false
	}
	t := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
	return t.Year() == d.Year && t.Month() == d.Month && t.Day() == d.Day
}
