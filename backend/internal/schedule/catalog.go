// Package schedule manages the clinic catalog (practitioners, rooms and
// services) that the availability engine and booking flow build on.
package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	// catalogGridMins is the granularity of durations and buffers.
	catalogGridMins = 5

	minServiceMinutes = 5
	maxServiceMinutes = 480
	maxBufferMinutes  = 120
)

var (
	// ErrInvalidPractitioner is returned when a practitioner fails validation.
	ErrInvalidPractitioner = errors.New("schedule: invalid practitioner")
	// ErrInvalidRoom is returned when a room fails validation.
	ErrInvalidRoom = errors.New("schedule: invalid room")
	// ErrInvalidService is returned when a service fails validation.
	ErrInvalidService = errors.New("schedule: invalid service")

	// ErrPractitionerNotFound is returned when a practitioner is not in the clinic.
	ErrPractitionerNotFound = errors.New("schedule: practitioner not found")
	// ErrRoomNotFound is returned when a room is not in the clinic.
	ErrRoomNotFound = errors.New("schedule: room not found")
	// ErrServiceNotFound is returned when a service is not in the clinic.
	ErrServiceNotFound = errors.New("schedule: service not found")

	// ErrDuplicateName is returned when a room or service name already exists in the clinic.
	ErrDuplicateName = errors.New("schedule: name already in use in this clinic")
)

// PaymentMode says what a patient pays at booking time.
type PaymentMode string

// Supported payment modes.
const (
	PaymentNone    PaymentMode = "none"
	PaymentDeposit PaymentMode = "deposit"
	PaymentFull    PaymentMode = "full"
)

// Valid reports whether m is a supported payment mode.
func (m PaymentMode) Valid() bool {
	switch m {
	case PaymentNone, PaymentDeposit, PaymentFull:
		return true
	}
	return false
}

// Practitioner is a doctor or other professional who receives patients.
type Practitioner struct {
	ID         bson.ObjectID   `bson:"_id,omitempty"`
	ClinicID   bson.ObjectID   `bson:"clinicId"`
	UserID     *bson.ObjectID  `bson:"userId,omitempty"`
	Name       string          `bson:"name"`
	Specialty  string          `bson:"specialty"`
	ServiceIDs []bson.ObjectID `bson:"serviceIds"`
	Active     bool            `bson:"active"`
	CreatedAt  time.Time       `bson:"createdAt"`
	UpdatedAt  time.Time       `bson:"updatedAt"`
}

func (p *Practitioner) normalize() {
	p.Name = strings.TrimSpace(p.Name)
	p.Specialty = strings.TrimSpace(p.Specialty)
	if p.ServiceIDs == nil {
		p.ServiceIDs = []bson.ObjectID{}
	}
}

// Validate checks the domain invariants of a Practitioner.
func (p Practitioner) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidPractitioner)
	}
	if strings.TrimSpace(p.Specialty) == "" {
		return fmt.Errorf("%w: specialty is required", ErrInvalidPractitioner)
	}
	if p.UserID != nil && p.UserID.IsZero() {
		return fmt.Errorf("%w: userId must not be the zero id", ErrInvalidPractitioner)
	}
	seen := make(map[bson.ObjectID]struct{}, len(p.ServiceIDs))
	for _, id := range p.ServiceIDs {
		if id.IsZero() {
			return fmt.Errorf("%w: serviceIds must not contain the zero id", ErrInvalidPractitioner)
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("%w: duplicate serviceId %s", ErrInvalidPractitioner, id.Hex())
		}
		seen[id] = struct{}{}
	}
	return nil
}

// Room is a consulting room or other bookable space.
type Room struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	ClinicID  bson.ObjectID `bson:"clinicId"`
	Name      string        `bson:"name"`
	RoomType  string        `bson:"roomType"`
	Active    bool          `bson:"active"`
	CreatedAt time.Time     `bson:"createdAt"`
	UpdatedAt time.Time     `bson:"updatedAt"`
}

func (r *Room) normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.RoomType = strings.TrimSpace(r.RoomType)
}

// Validate checks the domain invariants of a Room.
func (r Room) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidRoom)
	}
	if strings.TrimSpace(r.RoomType) == "" {
		return fmt.Errorf("%w: roomType is required", ErrInvalidRoom)
	}
	return nil
}

// Service is something a patient can book: its length, the room type it needs
// and how it is paid for.
type Service struct {
	ID              bson.ObjectID `bson:"_id,omitempty"`
	ClinicID        bson.ObjectID `bson:"clinicId"`
	Name            string        `bson:"name"`
	DurationMinutes int           `bson:"durationMinutes"`
	BufferMinutes   int           `bson:"bufferMinutes"`
	RoomType        string        `bson:"roomType"`
	PaymentMode     PaymentMode   `bson:"paymentMode"`
	PriceCents      int64         `bson:"priceCents"`
	DepositCents    int64         `bson:"depositCents"`
	DepositPercent  int           `bson:"depositPercent"`
	Active          bool          `bson:"active"`
	CreatedAt       time.Time     `bson:"createdAt"`
	UpdatedAt       time.Time     `bson:"updatedAt"`
}

func (s *Service) normalize() {
	s.Name = strings.TrimSpace(s.Name)
	s.RoomType = strings.TrimSpace(s.RoomType)
}

// Validate checks the domain invariants of a Service.
func (s Service) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidService)
	}
	if strings.TrimSpace(s.RoomType) == "" {
		return fmt.Errorf("%w: roomType is required", ErrInvalidService)
	}
	if s.DurationMinutes < minServiceMinutes || s.DurationMinutes > maxServiceMinutes || s.DurationMinutes%catalogGridMins != 0 {
		return fmt.Errorf("%w: durationMinutes must be a multiple of %d in [%d, %d]", ErrInvalidService, catalogGridMins, minServiceMinutes, maxServiceMinutes)
	}
	if s.BufferMinutes < 0 || s.BufferMinutes > maxBufferMinutes || s.BufferMinutes%catalogGridMins != 0 {
		return fmt.Errorf("%w: bufferMinutes must be a multiple of %d in [0, %d]", ErrInvalidService, catalogGridMins, maxBufferMinutes)
	}
	if !s.PaymentMode.Valid() {
		return fmt.Errorf("%w: paymentMode must be none, deposit or full", ErrInvalidService)
	}
	if s.PriceCents < 0 {
		return fmt.Errorf("%w: priceCents must not be negative", ErrInvalidService)
	}
	if s.DepositCents < 0 {
		return fmt.Errorf("%w: depositCents must not be negative", ErrInvalidService)
	}
	if s.DepositCents > s.PriceCents {
		return fmt.Errorf("%w: depositCents must not exceed priceCents", ErrInvalidService)
	}
	if s.DepositPercent < 0 || s.DepositPercent > 100 {
		return fmt.Errorf("%w: depositPercent must be in [0, 100]", ErrInvalidService)
	}
	switch s.PaymentMode {
	case PaymentNone:
		if s.DepositCents != 0 || s.DepositPercent != 0 {
			return fmt.Errorf("%w: depositCents and depositPercent must be zero when paymentMode is none", ErrInvalidService)
		}
	case PaymentFull:
		if s.PriceCents <= 0 {
			return fmt.Errorf("%w: priceCents must be positive when paymentMode is full", ErrInvalidService)
		}
		if s.DepositCents != 0 || s.DepositPercent != 0 {
			return fmt.Errorf("%w: depositCents and depositPercent must be zero when paymentMode is full", ErrInvalidService)
		}
	case PaymentDeposit:
		if s.PriceCents <= 0 {
			return fmt.Errorf("%w: priceCents must be positive when paymentMode is deposit", ErrInvalidService)
		}
		if (s.DepositCents > 0) == (s.DepositPercent > 0) {
			return fmt.Errorf("%w: set either depositCents or depositPercent when paymentMode is deposit", ErrInvalidService)
		}
	}
	return nil
}
