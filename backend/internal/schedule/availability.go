package schedule

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrInvalidAvailabilityQuery is returned when an availability query has
// invalid time bounds or step configuration.
var ErrInvalidAvailabilityQuery = errors.New("schedule: invalid availability query")

// OccupiedCell represents a single 5-minute cell already occupied by a
// booking or hold for a resource (practitioner or room), matching slot_locks.
type OccupiedCell struct {
	ResourceID bson.ObjectID
	CellStart  time.Time
}

// AvailabilityInput bundles the query window and clinic schedule data needed
// to compute bookable slots in memory without database side effects.
type AvailabilityInput struct {
	From           time.Time
	To             time.Time
	Now            time.Time
	Location       *time.Location
	StepMinutes    int
	Service        Service
	PractitionerID *bson.ObjectID
	Practitioners  []Practitioner
	Rooms          []Room
	Rules          []AvailabilityRule
	Blocks         []Block
	OccupiedCells  []OccupiedCell
}

// AvailableSlot represents one bookable start time for a practitioner and room.
type AvailableSlot struct {
	StartAt        time.Time
	EndAt          time.Time
	PractitionerID bson.ObjectID
	RoomID         bson.ObjectID
	RoomName       string
}

type timeInterval struct {
	start time.Time
	end   time.Time
}

type resourceIntervalKey struct {
	practitionerID bson.ObjectID
	roomID         bson.ObjectID
}

type occupiedCellKey struct {
	resourceID bson.ObjectID
	cellUnix   int64
}

// ComputeAvailability returns all valid bookable slots in [From, To) sorted
// deterministically by StartAt, PractitionerID, and RoomID.
//
// Rules are evaluated in Location (defaulting to America/Panama) and merged
// per (practitioner, room) when overlapping or adjacent. Candidate slots must
// fit their service DurationMinutes inside the merged rule window, avoid any
// clinic/practitioner/room blocks during [StartAt, EndAt), and have no
// occupied 5-minute cells during [StartAt, EndAt + BufferMinutes).
func ComputeAvailability(in AvailabilityInput) ([]AvailableSlot, error) {
	if !in.To.After(in.From) {
		return nil, fmt.Errorf("%w: to must be after from", ErrInvalidAvailabilityQuery)
	}
	step := in.StepMinutes
	if step == 0 {
		step = gridStepMins
	}
	if step < gridStepMins || step%gridStepMins != 0 {
		return nil, fmt.Errorf("%w: stepMinutes must be a positive multiple of %d", ErrInvalidAvailabilityQuery, gridStepMins)
	}
	if !in.Service.Active || in.Service.ID.IsZero() {
		return nil, nil
	}
	if err := in.Service.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAvailabilityQuery, err)
	}

	loc := in.Location
	if loc == nil {
		loc = PanamaLocation()
	}

	fromUTC := in.From.UTC()
	toUTC := in.To.UTC()
	nowUTC := time.Time{}
	if !in.Now.IsZero() {
		nowUTC = in.Now.UTC()
	}

	eligiblePractitioners := indexEligiblePractitioners(in.Practitioners, in.Service.ID, in.PractitionerID)
	eligibleRooms := indexEligibleRooms(in.Rooms, in.Service.RoomType)
	if len(eligiblePractitioners) == 0 || len(eligibleRooms) == 0 {
		return nil, nil
	}

	occupied := indexOccupiedCells(in.OccupiedCells)
	clinicBlocks, practitionerBlocks, roomBlocks := indexBlocks(in.Blocks)

	windows := buildMergedWindows(fromUTC, toUTC, loc, in.Rules, eligiblePractitioners, eligibleRooms)
	if len(windows) == 0 {
		return nil, nil
	}

	duration := time.Duration(in.Service.DurationMinutes) * time.Minute
	buffer := time.Duration(in.Service.BufferMinutes) * time.Minute
	stepDur := time.Duration(step) * time.Minute

	var slots []AvailableSlot
	for key, intervals := range windows {
		room := eligibleRooms[key.roomID]
		for _, win := range intervals {
			for start := win.start; !start.Add(duration).After(win.end); start = start.Add(stepDur) {
				if start.Before(fromUTC) || !start.Before(toUTC) {
					continue
				}
				if !nowUTC.IsZero() && start.Before(nowUTC) {
					continue
				}
				end := start.Add(duration)
				spanEnd := end.Add(buffer)

				if isBlocked(start, end, clinicBlocks) ||
					isBlocked(start, end, practitionerBlocks[key.practitionerID]) ||
					isBlocked(start, end, roomBlocks[key.roomID]) {
					continue
				}
				if hasOccupiedCells(start, spanEnd, key.practitionerID, key.roomID, occupied) {
					continue
				}

				slots = append(slots, AvailableSlot{
					StartAt:        start,
					EndAt:          end,
					PractitionerID: key.practitionerID,
					RoomID:         key.roomID,
					RoomName:       room.Name,
				})
			}
		}
	}

	slices.SortFunc(slots, func(a, b AvailableSlot) int {
		if !a.StartAt.Equal(b.StartAt) {
			if a.StartAt.Before(b.StartAt) {
				return -1
			}
			return 1
		}
		if c := compareObjectIDs(a.PractitionerID, b.PractitionerID); c != 0 {
			return c
		}
		return compareObjectIDs(a.RoomID, b.RoomID)
	})

	return slots, nil
}

func indexEligiblePractitioners(
	practitioners []Practitioner,
	serviceID bson.ObjectID,
	filterID *bson.ObjectID,
) map[bson.ObjectID]Practitioner {
	out := make(map[bson.ObjectID]Practitioner)
	for _, p := range practitioners {
		if !p.Active || p.ID.IsZero() {
			continue
		}
		if filterID != nil && !filterID.IsZero() && p.ID != *filterID {
			continue
		}
		if slices.Contains(p.ServiceIDs, serviceID) {
			out[p.ID] = p
		}
	}
	return out
}

func indexEligibleRooms(rooms []Room, requiredType string) map[bson.ObjectID]Room {
	out := make(map[bson.ObjectID]Room)
	for _, r := range rooms {
		if !r.Active || r.ID.IsZero() {
			continue
		}
		if r.RoomType == requiredType {
			out[r.ID] = r
		}
	}
	return out
}

func indexOccupiedCells(cells []OccupiedCell) map[occupiedCellKey]struct{} {
	out := make(map[occupiedCellKey]struct{}, len(cells))
	for _, c := range cells {
		if c.ResourceID.IsZero() {
			continue
		}
		out[occupiedCellKey{
			resourceID: c.ResourceID,
			cellUnix:   c.CellStart.UTC().Unix(),
		}] = struct{}{}
	}
	return out
}

func indexBlocks(blocks []Block) (
	clinic []timeInterval,
	practitioner map[bson.ObjectID][]timeInterval,
	room map[bson.ObjectID][]timeInterval,
) {
	practitioner = make(map[bson.ObjectID][]timeInterval)
	room = make(map[bson.ObjectID][]timeInterval)
	for _, b := range blocks {
		if !b.EndAt.After(b.StartAt) {
			continue
		}
		iv := timeInterval{start: b.StartAt.UTC(), end: b.EndAt.UTC()}
		switch b.Scope {
		case BlockScopeClinic:
			clinic = append(clinic, iv)
		case BlockScopePractitioner:
			if b.PractitionerID != nil && !b.PractitionerID.IsZero() {
				practitioner[*b.PractitionerID] = append(practitioner[*b.PractitionerID], iv)
			}
		case BlockScopeRoom:
			if b.RoomID != nil && !b.RoomID.IsZero() {
				room[*b.RoomID] = append(room[*b.RoomID], iv)
			}
		}
	}
	return clinic, practitioner, room
}

func buildMergedWindows(
	fromUTC, toUTC time.Time,
	loc *time.Location,
	rules []AvailabilityRule,
	practitioners map[bson.ObjectID]Practitioner,
	rooms map[bson.ObjectID]Room,
) map[resourceIntervalKey][]timeInterval {
	grouped := make(map[resourceIntervalKey][]timeInterval)

	// Expand local calendar days covering [fromUTC, toUTC).
	fromLocal := fromUTC.In(loc)
	toLocal := toUTC.In(loc)
	dayStart := time.Date(fromLocal.Year(), fromLocal.Month(), fromLocal.Day(), 0, 0, 0, 0, loc)
	lastDay := time.Date(toLocal.Year(), toLocal.Month(), toLocal.Day(), 0, 0, 0, 0, loc)

	for d := dayStart; !d.After(lastDay); d = d.AddDate(0, 0, 1) {
		wd := d.Weekday()
		for _, r := range rules {
			if !r.Active || r.Weekday != wd {
				continue
			}
			if _, ok := practitioners[r.PractitionerID]; !ok {
				continue
			}
			if _, ok := rooms[r.RoomID]; !ok {
				continue
			}
			if err := r.Validate(); err != nil {
				continue
			}

			winStart := d.Add(time.Duration(r.StartMinute) * time.Minute).UTC()
			winEnd := d.Add(time.Duration(r.EndMinute) * time.Minute).UTC()

			if r.ValidFrom != nil {
				vf := r.ValidFrom.UTC()
				if vf.After(winStart) {
					winStart = vf
				}
			}
			if r.ValidUntil != nil {
				vu := r.ValidUntil.UTC()
				if vu.Before(winEnd) {
					winEnd = vu
				}
			}
			if !winEnd.After(winStart) {
				continue
			}
			if !winEnd.After(fromUTC) || !winStart.Before(toUTC) {
				continue
			}

			key := resourceIntervalKey{
				practitionerID: r.PractitionerID,
				roomID:         r.RoomID,
			}
			grouped[key] = append(grouped[key], timeInterval{start: winStart, end: winEnd})
		}
	}

	for key, intervals := range grouped {
		grouped[key] = mergeIntervals(intervals)
	}
	return grouped
}

func mergeIntervals(intervals []timeInterval) []timeInterval {
	if len(intervals) <= 1 {
		return intervals
	}
	slices.SortFunc(intervals, func(a, b timeInterval) int {
		if a.start.Before(b.start) {
			return -1
		}
		if a.start.After(b.start) {
			return 1
		}
		if a.end.Before(b.end) {
			return -1
		}
		if a.end.After(b.end) {
			return 1
		}
		return 0
	})

	merged := []timeInterval{intervals[0]}
	for _, cur := range intervals[1:] {
		last := &merged[len(merged)-1]
		if !cur.start.After(last.end) {
			if cur.end.After(last.end) {
				last.end = cur.end
			}
			continue
		}
		merged = append(merged, cur)
	}
	return merged
}

func isBlocked(start, end time.Time, blocks []timeInterval) bool {
	for _, b := range blocks {
		if start.Before(b.end) && end.After(b.start) {
			return true
		}
	}
	return false
}

func hasOccupiedCells(
	start, spanEnd time.Time,
	practitionerID, roomID bson.ObjectID,
	occupied map[occupiedCellKey]struct{},
) bool {
	if len(occupied) == 0 {
		return false
	}
	cellDur := time.Duration(gridStepMins) * time.Minute
	for c := start; c.Before(spanEnd); c = c.Add(cellDur) {
		unix := c.Unix()
		if _, ok := occupied[occupiedCellKey{resourceID: practitionerID, cellUnix: unix}]; ok {
			return true
		}
		if _, ok := occupied[occupiedCellKey{resourceID: roomID, cellUnix: unix}]; ok {
			return true
		}
	}
	return false
}

func compareObjectIDs(a, b bson.ObjectID) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
