// Package appointments implements double-booking-free appointment holds,
// confirmations, atomic rescheduling, and slot_locks management.
package appointments

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	// CellMinutes is the 5-minute grid granularity for all appointments and locks.
	CellMinutes = 5

	// DefaultHoldTTL is the default duration of a held slot before expiry.
	DefaultHoldTTL = 10 * time.Minute
	// CheckoutHoldTTL is the extended hold duration when a payment checkout is active.
	CheckoutHoldTTL = 15 * time.Minute
)

// Status represents the lifecycle state of an appointment.
type Status string

// Supported appointment statuses.
const (
	StatusHeld      Status = "held"
	StatusConfirmed Status = "confirmed"
	StatusCompleted Status = "completed"
	StatusCancelled Status = "cancelled"
	StatusNoShow    Status = "no_show"
)

// Valid reports whether s is a recognized appointment status.
func (s Status) Valid() bool {
	switch s {
	case StatusHeld, StatusConfirmed, StatusCompleted, StatusCancelled, StatusNoShow:
		return true
	}
	return false
}

// IsTerminal reports whether no further state transitions are allowed.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusCancelled, StatusNoShow:
		return true
	}
	return false
}

// ResourceType identifies the locked resource in slot_locks.
type ResourceType string

// Supported locked resource types.
const (
	ResourcePractitioner ResourceType = "practitioner"
	ResourceRoom         ResourceType = "room"
)

var (
	// ErrSlotUnavailable is returned when one or more 5-minute cells are already locked.
	ErrSlotUnavailable = errors.New("appointments: slot unavailable")
	// ErrAppointmentNotFound is returned when the appointment does not exist in the clinic.
	ErrAppointmentNotFound = errors.New("appointments: appointment not found")
	// ErrInvalidAppointment is returned when booking or rescheduling parameters fail validation.
	ErrInvalidAppointment = errors.New("appointments: invalid appointment")
	// ErrInvalidTransition is returned when a status transition is not permitted.
	ErrInvalidTransition = errors.New("appointments: invalid status transition")
	// ErrHoldExpired is returned when confirming or rescheduling an expired hold.
	ErrHoldExpired = errors.New("appointments: hold expired")
)

// HistoryEntry records a status change on an appointment.
type HistoryEntry struct {
	Status    Status         `bson:"status"`
	ChangedAt time.Time      `bson:"changedAt"`
	ActorID   *bson.ObjectID `bson:"actorId,omitempty"`
	Note      string         `bson:"note,omitempty"`
}

// Appointment is a booked, held, or historical clinic appointment.
type Appointment struct {
	ID                 bson.ObjectID  `bson:"_id,omitempty"`
	ClinicID           bson.ObjectID  `bson:"clinicId"`
	PractitionerID     bson.ObjectID  `bson:"practitionerId"`
	RoomID             bson.ObjectID  `bson:"roomId"`
	ServiceID          bson.ObjectID  `bson:"serviceId"`
	PatientRef         string         `bson:"patientRef"`
	StartAt            time.Time      `bson:"startAt"`
	EndAt              time.Time      `bson:"endAt"`
	BufferMinutes      int            `bson:"bufferMinutes"`
	Status             Status         `bson:"status"`
	HoldExpiresAt      *time.Time     `bson:"holdExpiresAt,omitempty"`
	CancellationReason string         `bson:"cancellationReason,omitempty"`
	History            []HistoryEntry `bson:"history"`
	CreatedAt          time.Time      `bson:"createdAt"`
	UpdatedAt          time.Time      `bson:"updatedAt"`
}

// SlotLock represents a single 5-minute cell locked for a practitioner or room.
type SlotLock struct {
	ID            bson.ObjectID `bson:"_id,omitempty"`
	ClinicID      bson.ObjectID `bson:"clinicId"`
	ResourceID    bson.ObjectID `bson:"resourceId"`
	ResourceType  ResourceType  `bson:"resourceType"`
	CellStart     time.Time     `bson:"cellStart"`
	AppointmentID bson.ObjectID `bson:"appointmentId"`
	Status        Status        `bson:"status"`
	ExpiresAt     *time.Time    `bson:"expiresAt,omitempty"`
	CreatedAt     time.Time     `bson:"createdAt"`
}

// BookInput specifies the parameters for creating a held or confirmed appointment.
type BookInput struct {
	PractitionerID bson.ObjectID
	RoomID         bson.ObjectID
	ServiceID      bson.ObjectID
	PatientRef     string
	StartAt        time.Time
	EndAt          time.Time
	BufferMinutes  int
	Confirm        bool
	HoldTTL        time.Duration
	ActorID        *bson.ObjectID
	Note           string
}

// RescheduleInput specifies the target window and resources for atomic rescheduling.
type RescheduleInput struct {
	AppointmentID  bson.ObjectID
	PractitionerID bson.ObjectID
	RoomID         bson.ObjectID
	StartAt        time.Time
	EndAt          time.Time
	// BufferMinutes overrides the stored buffer; nil keeps it.
	BufferMinutes *int
	ActorID       *bson.ObjectID
	Note          string
}

// BuildSlotCells validates 5-minute alignment and returns every 5-minute cell
// timestamp in [startAt, endAt + bufferMinutes).
func BuildSlotCells(startAt, endAt time.Time, bufferMinutes int) ([]time.Time, error) {
	startUTC := startAt.UTC().Truncate(time.Millisecond)
	endUTC := endAt.UTC().Truncate(time.Millisecond)

	if !endUTC.After(startUTC) {
		return nil, fmt.Errorf("%w: endAt must be after startAt", ErrInvalidAppointment)
	}
	if !isAlignedToGrid(startUTC) || !isAlignedToGrid(endUTC) {
		return nil, fmt.Errorf("%w: startAt and endAt must align to %d-minute boundaries", ErrInvalidAppointment, CellMinutes)
	}
	if bufferMinutes < 0 || bufferMinutes > 120 || bufferMinutes%CellMinutes != 0 {
		return nil, fmt.Errorf("%w: bufferMinutes must be a %d-minute multiple in [0, 120]", ErrInvalidAppointment, CellMinutes)
	}

	spanEnd := endUTC.Add(time.Duration(bufferMinutes) * time.Minute)
	step := time.Duration(CellMinutes) * time.Minute
	count := int(spanEnd.Sub(startUTC) / step)
	cells := make([]time.Time, 0, count)
	for c := startUTC; c.Before(spanEnd); c = c.Add(step) {
		cells = append(cells, c)
	}
	return cells, nil
}

func isAlignedToGrid(t time.Time) bool {
	return t.Second() == 0 && t.Nanosecond() == 0 && t.Minute()%CellMinutes == 0
}

func validateBookInput(in BookInput) error {
	if in.PractitionerID.IsZero() || in.RoomID.IsZero() || in.ServiceID.IsZero() {
		return fmt.Errorf("%w: practitionerId, roomId, and serviceId are required", ErrInvalidAppointment)
	}
	if strings.TrimSpace(in.PatientRef) == "" {
		return fmt.Errorf("%w: patientRef is required", ErrInvalidAppointment)
	}
	_, err := BuildSlotCells(in.StartAt, in.EndAt, in.BufferMinutes)
	return err
}
