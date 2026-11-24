package schedule

import (
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestComputeAvailability(t *testing.T) {
	loc := PanamaLocation()

	svcGeneralID := bson.NewObjectID()
	svcXRayID := bson.NewObjectID()

	prac1ID := bson.NewObjectID()
	prac2ID := bson.NewObjectID()
	pracInactiveID := bson.NewObjectID()

	roomConsult1ID := bson.NewObjectID()
	roomConsult2ID := bson.NewObjectID()
	roomXRayID := bson.NewObjectID()
	roomInactiveID := bson.NewObjectID()

	svcGeneral := Service{
		ID:              svcGeneralID,
		Name:            "Consulta general",
		DurationMinutes: 30,
		BufferMinutes:   0,
		RoomType:        "consultorio",
		PaymentMode:     PaymentNone,
		Active:          true,
	}
	svcWithBuffer := Service{
		ID:              svcGeneralID,
		Name:            "Consulta con margen",
		DurationMinutes: 30,
		BufferMinutes:   10,
		RoomType:        "consultorio",
		PaymentMode:     PaymentNone,
		Active:          true,
	}
	svcInactive := svcGeneral
	svcInactive.Active = false

	svcXRay := Service{
		ID:              svcXRayID,
		Name:            "Radiografía",
		DurationMinutes: 20,
		BufferMinutes:   0,
		RoomType:        "rayos-x",
		PaymentMode:     PaymentNone,
		Active:          true,
	}

	practitioners := []Practitioner{
		{ID: prac1ID, Name: "Dra. Pérez", Specialty: "Medicina General", ServiceIDs: []bson.ObjectID{svcGeneralID}, Active: true},
		{ID: prac2ID, Name: "Dr. Gómez", Specialty: "Radiología", ServiceIDs: []bson.ObjectID{svcXRayID}, Active: true},
		{ID: pracInactiveID, Name: "Dr. Inactivo", Specialty: "Medicina General", ServiceIDs: []bson.ObjectID{svcGeneralID}, Active: false},
	}
	rooms := []Room{
		{ID: roomConsult1ID, Name: "Consultorio 1", RoomType: "consultorio", Active: true},
		{ID: roomConsult2ID, Name: "Consultorio 2", RoomType: "consultorio", Active: true},
		{ID: roomXRayID, Name: "Sala Rayos X", RoomType: "rayos-x", Active: true},
		{ID: roomInactiveID, Name: "Consultorio Inactivo", RoomType: "consultorio", Active: false},
	}

	// Monday 2026-11-23 in America/Panama (UTC-5).
	panamaTime := func(year int, month time.Month, day, hour, minute int) time.Time {
		return time.Date(year, month, day, hour, minute, 0, 0, loc).UTC()
	}

	dayStart := panamaTime(2026, time.November, 23, 0, 0)
	dayEnd := panamaTime(2026, time.November, 24, 0, 0)

	type wantSlot struct {
		start time.Time
		prac  bson.ObjectID
		room  bson.ObjectID
	}

	tests := []struct {
		name    string
		input   AvailabilityInput
		want    []wantSlot
		wantErr error
	}{
		{
			name: "invalid window to <= from",
			input: AvailabilityInput{
				From:    dayStart,
				To:      dayStart,
				Service: svcGeneral,
			},
			wantErr: ErrInvalidAvailabilityQuery,
		},
		{
			name: "invalid step not multiple of 5",
			input: AvailabilityInput{
				From:        dayStart,
				To:          dayEnd,
				StepMinutes: 7,
				Service:     svcGeneral,
			},
			wantErr: ErrInvalidAvailabilityQuery,
		},
		{
			name: "inactive service returns no slots",
			input: AvailabilityInput{
				From:          dayStart,
				To:            dayEnd,
				StepMinutes:   30,
				Service:       svcInactive,
				Practitioners: practitioners,
				Rooms:         rooms,
				Rules: []AvailabilityRule{
					{PractitionerID: prac1ID, RoomID: roomConsult1ID, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 11 * 60, Active: true},
				},
			},
			want: nil,
		},
		{
			name: "basic 1h rule with 30m service and 30m step",
			input: AvailabilityInput{
				From:          dayStart,
				To:            dayEnd,
				StepMinutes:   30,
				Service:       svcGeneral,
				Practitioners: practitioners,
				Rooms:         rooms,
				Rules: []AvailabilityRule{
					{PractitionerID: prac1ID, RoomID: roomConsult1ID, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 10 * 60, Active: true},
				},
			},
			want: []wantSlot{
				{start: panamaTime(2026, time.November, 23, 9, 0), prac: prac1ID, room: roomConsult1ID},
				{start: panamaTime(2026, time.November, 23, 9, 30), prac: prac1ID, room: roomConsult1ID},
			},
		},
		{
			name: "overlapping and adjacent rules merge across boundary",
			input: AvailabilityInput{
				From:          dayStart,
				To:            dayEnd,
				StepMinutes:   30,
				Service:       svcGeneral,
				Practitioners: practitioners,
				Rooms:         rooms,
				Rules: []AvailabilityRule{
					{PractitionerID: prac1ID, RoomID: roomConsult1ID, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 9*60 + 15, Active: true},
					{PractitionerID: prac1ID, RoomID: roomConsult1ID, Weekday: time.Monday, StartMinute: 9*60 + 15, EndMinute: 10 * 60, Active: true},
					{PractitionerID: prac1ID, RoomID: roomConsult1ID, Weekday: time.Monday, StartMinute: 9*60 + 30, EndMinute: 10*60 + 30, Active: true},
				},
			},
			want: []wantSlot{
				{start: panamaTime(2026, time.November, 23, 9, 0), prac: prac1ID, room: roomConsult1ID},
				{start: panamaTime(2026, time.November, 23, 9, 30), prac: prac1ID, room: roomConsult1ID},
				{start: panamaTime(2026, time.November, 23, 10, 0), prac: prac1ID, room: roomConsult1ID},
			},
		},
		{
			name: "filters out wrong room type, inactive room, and inactive practitioner",
			input: AvailabilityInput{
				From:          dayStart,
				To:            dayEnd,
				StepMinutes:   30,
				Service:       svcXRay,
				Practitioners: practitioners,
				Rooms:         rooms,
				Rules: []AvailabilityRule{
					// Wrong room type for svcXRay
					{PractitionerID: prac2ID, RoomID: roomConsult1ID, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 10 * 60, Active: true},
					// Practitioner doesn't offer svcXRay
					{PractitionerID: prac1ID, RoomID: roomXRayID, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 10 * 60, Active: true},
					// Valid pair
					{PractitionerID: prac2ID, RoomID: roomXRayID, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 9*60 + 40, Active: true},
				},
			},
			want: []wantSlot{
				{start: panamaTime(2026, time.November, 23, 9, 0), prac: prac2ID, room: roomXRayID},
			},
		},
		{
			name: "clinic, practitioner, and room blocks exclude overlapping slots",
			input: AvailabilityInput{
				From:          dayStart,
				To:            dayEnd,
				StepMinutes:   30,
				Service:       svcGeneral,
				Practitioners: practitioners,
				Rooms:         rooms,
				Rules: []AvailabilityRule{
					{PractitionerID: prac1ID, RoomID: roomConsult1ID, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 12 * 60, Active: true},
				},
				Blocks: []Block{
					// Clinic block 09:30-10:00
					{Scope: BlockScopeClinic, Source: BlockSourceManual, Reason: "Reunión", StartAt: panamaTime(2026, time.November, 23, 9, 30), EndAt: panamaTime(2026, time.November, 23, 10, 0)},
					// Practitioner block 10:30-11:00
					{Scope: BlockScopePractitioner, PractitionerID: &prac1ID, Source: BlockSourceManual, Reason: "Descanso", StartAt: panamaTime(2026, time.November, 23, 10, 30), EndAt: panamaTime(2026, time.November, 23, 11, 0)},
					// Room block 11:00-11:30
					{Scope: BlockScopeRoom, RoomID: &roomConsult1ID, Source: BlockSourceManual, Reason: "Limpieza", StartAt: panamaTime(2026, time.November, 23, 11, 0), EndAt: panamaTime(2026, time.November, 23, 11, 30)},
				},
			},
			want: []wantSlot{
				{start: panamaTime(2026, time.November, 23, 9, 0), prac: prac1ID, room: roomConsult1ID},
				{start: panamaTime(2026, time.November, 23, 10, 0), prac: prac1ID, room: roomConsult1ID},
				{start: panamaTime(2026, time.November, 23, 11, 30), prac: prac1ID, room: roomConsult1ID},
			},
		},
		{
			name: "occupied cells and service buffer prevent adjacent collision",
			input: AvailabilityInput{
				From:          dayStart,
				To:            dayEnd,
				StepMinutes:   30,
				Service:       svcWithBuffer, // 30m duration + 10m buffer
				Practitioners: practitioners,
				Rooms:         rooms,
				Rules: []AvailabilityRule{
					{PractitionerID: prac1ID, RoomID: roomConsult1ID, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 11 * 60, Active: true},
				},
				OccupiedCells: []OccupiedCell{
					// Existing booking occupies 10:00..10:05 on roomConsult1ID.
					// 09:30 slot (ends 10:00, buffer until 10:10) must be excluded.
					// 10:00 slot (starts 10:00) must be excluded.
					// 09:00 slot (ends 09:30, buffer until 09:40) is valid.
					// 10:30 slot (starts 10:30) is valid.
					{ResourceID: roomConsult1ID, CellStart: panamaTime(2026, time.November, 23, 10, 0)},
				},
			},
			want: []wantSlot{
				{start: panamaTime(2026, time.November, 23, 9, 0), prac: prac1ID, room: roomConsult1ID},
				{start: panamaTime(2026, time.November, 23, 10, 30), prac: prac1ID, room: roomConsult1ID},
			},
		},
		{
			name: "validFrom, validUntil, and now cutoff constrain slots",
			input: AvailabilityInput{
				From:          dayStart,
				To:            dayEnd,
				Now:           panamaTime(2026, time.November, 23, 9, 15),
				StepMinutes:   30,
				Service:       svcGeneral,
				Practitioners: practitioners,
				Rooms:         rooms,
				Rules: []AvailabilityRule{
					{
						PractitionerID: prac1ID,
						RoomID:         roomConsult1ID,
						Weekday:        time.Monday,
						StartMinute:    9 * 60,
						EndMinute:      11 * 60,
						ValidFrom:      ptrTime(panamaTime(2026, time.November, 23, 9, 30)),
						ValidUntil:     ptrTime(panamaTime(2026, time.November, 23, 10, 30)),
						Active:         true,
					},
				},
			},
			want: []wantSlot{
				{start: panamaTime(2026, time.November, 23, 9, 30), prac: prac1ID, room: roomConsult1ID},
				{start: panamaTime(2026, time.November, 23, 10, 0), prac: prac1ID, room: roomConsult1ID},
			},
		},
		{
			name: "practitioner filter limits results when specified",
			input: AvailabilityInput{
				From:           dayStart,
				To:             dayEnd,
				StepMinutes:    30,
				Service:        svcGeneral,
				PractitionerID: &prac2ID,
				Practitioners:  practitioners,
				Rooms:          rooms,
				Rules: []AvailabilityRule{
					{PractitionerID: prac1ID, RoomID: roomConsult1ID, Weekday: time.Monday, StartMinute: 9 * 60, EndMinute: 10 * 60, Active: true},
				},
			},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ComputeAvailability(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ComputeAvailability() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ComputeAvailability() unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d slots (%+v), want %d (%+v)", len(got), got, len(tc.want), tc.want)
			}
			for i := range tc.want {
				if !got[i].StartAt.Equal(tc.want[i].start) ||
					got[i].PractitionerID != tc.want[i].prac ||
					got[i].RoomID != tc.want[i].room {
					t.Errorf("slot[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
