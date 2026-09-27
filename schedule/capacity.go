package schedule

import "time"

// Occupancy is the seat count of one event at one instant.
type Occupancy struct {
	Capacity  int
	Unlimited bool // Capacity == 0
	Confirmed int  // seats in confirmed registrations
	Held      int  // seats in holds with at before HoldUntil
	Used      int  // Confirmed + Held
	// Remaining is max(0, Capacity-Used); 0 when Unlimited.
	Remaining  int
	Waitlisted int // seats waiting
	Requested  int // seats waiting for review
	// Full is !Unlimited && Used >= Capacity.
	Full bool
	// Over is max(0, Used-Capacity): non-zero only after an override.
	Over int
}

// Occupy counts e's registrations at instant at. Registrations for other
// events are ignored.
func Occupy(e Event, regs []Registration, at time.Time) Occupancy {
	o := Occupancy{Capacity: e.Capacity, Unlimited: e.Capacity == 0}
	for _, r := range regs {
		if r.EventID != e.ID {
			continue
		}
		switch r.State {
		case StateConfirmed:
			o.Confirmed += r.Seats
		case StateHeld:
			if r.Counts(at) {
				o.Held += r.Seats
			}
		case StateWaitlisted:
			o.Waitlisted += r.Seats
		case StateRequested:
			o.Requested += r.Seats
		}
	}
	o.Used = o.Confirmed + o.Held
	if o.Unlimited {
		o.Remaining = 0
	} else {
		o.Remaining = max(0, o.Capacity-o.Used)
		o.Full = o.Used >= o.Capacity
		o.Over = max(0, o.Used-o.Capacity)
	}
	return o
}

// Availability is what a booker can do at an instant.
type Availability string

const (
	AvailabilityOpen     Availability = "open"
	AvailabilityWaitlist Availability = "waitlist"
	AvailabilityFull     Availability = "full"
	AvailabilityClosed   Availability = "closed"
)

// AvailabilityAt returns, in order: closed when e is not published, at is
// before BookingOpens, at is at or after BookingCloses, or at is at or after
// Start; open when o is not Full; waitlist when e.Waitlist; full otherwise.
func AvailabilityAt(e Event, o Occupancy, at time.Time) Availability {
	if e.Status != EventPublished || (!e.BookingOpens.IsZero() && at.Before(e.BookingOpens)) ||
		(!e.BookingCloses.IsZero() && !at.Before(e.BookingCloses)) || !at.Before(e.Start) {
		return AvailabilityClosed
	}
	if !o.Full {
		return AvailabilityOpen
	}
	if e.Waitlist {
		return AvailabilityWaitlist
	}
	return AvailabilityFull
}

// WaitlistOrder returns the waitlisted registrations in regs, ordered by
// RequestedAt, then ID. Admin views may show the order; public views must
// not show a position.
func WaitlistOrder(regs []Registration) []Registration {
	out := make([]Registration, 0, len(regs))
	for _, r := range regs {
		if r.State == StateWaitlisted {
			out = append(out, r)
		}
	}
	sortRegistrations(out)
	return out
}

// Summary totals several rosters at one instant.
type Summary struct {
	Events    int
	FullCount int
	Capacity  int // sum of limited capacities
	Used      int
}

// Summarize totals rosters at instant at.
func Summarize(rosters []Roster, at time.Time) Summary {
	s := Summary{Events: len(rosters)}
	for _, r := range rosters {
		o := Occupy(r.Event, r.Registrations, at)
		if !o.Unlimited {
			s.Capacity += o.Capacity
		}
		s.Used += o.Used
		if o.Full {
			s.FullCount++
		}
	}
	return s
}
