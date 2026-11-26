package appointments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/schedule"
	"github.com/zSamir015/agenda-clinicas/backend/internal/tenantdb"
)

// Repository manages appointments and slot_locks within MongoDB transactions.
type Repository struct {
	client *mongo.Client
	appts  *tenantdb.Repo[Appointment]
	locks  *tenantdb.Repo[SlotLock]
	now    func() time.Time
}

// NewRepository returns an appointments Repository backed by client.
func NewRepository(client *mongo.Client, sharedDB string) *Repository {
	return &Repository{
		client: client,
		appts:  tenantdb.NewRepo[Appointment](client, sharedDB, "appointments"),
		locks:  tenantdb.NewRepo[SlotLock](client, sharedDB, "slot_locks"),
		now:    func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
	}
}

// Book atomically creates a held or confirmed appointment and locks all 5-minute
// cells for both the practitioner and the room. Expired hold locks in the
// target cells are reclaimed inside the same transaction.
func (r *Repository) Book(ctx context.Context, in BookInput) (Appointment, error) {
	if err := validateBookInput(in); err != nil {
		return Appointment{}, err
	}
	clinic, ok := platform.TenantFrom(ctx)
	if !ok || clinic.ID.IsZero() {
		return Appointment{}, tenantdb.ErrNoTenant
	}

	cells, err := BuildSlotCells(in.StartAt, in.EndAt, in.BufferMinutes)
	if err != nil {
		return Appointment{}, err
	}

	var created Appointment
	err = r.runTx(ctx, func(txCtx context.Context) error {
		now := r.now()
		status := StatusHeld
		var holdExpires *time.Time
		if in.Confirm {
			status = StatusConfirmed
		} else {
			ttl := in.HoldTTL
			if ttl <= 0 {
				ttl = DefaultHoldTTL
			}
			exp := now.Add(ttl)
			holdExpires = &exp
		}

		appt := Appointment{
			ID:             bson.NewObjectID(),
			ClinicID:       clinic.ID,
			PractitionerID: in.PractitionerID,
			RoomID:         in.RoomID,
			ServiceID:      in.ServiceID,
			PatientRef:     strings.TrimSpace(in.PatientRef),
			StartAt:        in.StartAt.UTC().Truncate(time.Millisecond),
			EndAt:          in.EndAt.UTC().Truncate(time.Millisecond),
			BufferMinutes:  in.BufferMinutes,
			Status:         status,
			HoldExpiresAt:  holdExpires,
			History: []HistoryEntry{{
				Status:    status,
				ChangedAt: now,
				ActorID:   in.ActorID,
				Note:      strings.TrimSpace(in.Note),
			}},
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := r.reclaimExpiredLocks(txCtx, in.PractitionerID, in.RoomID, cells, now); err != nil {
			return err
		}
		if err := r.insertLocks(txCtx, clinic.ID, appt.ID, in.PractitionerID, in.RoomID, cells, status, holdExpires, now); err != nil {
			return err
		}
		if err := r.appts.Insert(txCtx, &appt); err != nil {
			return err
		}
		created = appt
		return nil
	})
	if err != nil {
		return Appointment{}, err
	}
	return created, nil
}

// ConfirmHold transitions a non-expired held appointment to confirmed and
// removes the TTL expiry from both the appointment and its slot_locks.
func (r *Repository) ConfirmHold(ctx context.Context, appointmentID bson.ObjectID, actorID *bson.ObjectID, note string) (Appointment, error) {
	if appointmentID.IsZero() {
		return Appointment{}, ErrAppointmentNotFound
	}
	var updated Appointment
	err := r.runTx(ctx, func(txCtx context.Context) error {
		appt, err := r.appts.FindOne(txCtx, bson.D{{Key: "_id", Value: appointmentID}})
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrAppointmentNotFound
		}
		if err != nil {
			return err
		}
		if appt.Status != StatusHeld {
			return fmt.Errorf("%w: cannot confirm appointment in status %s", ErrInvalidTransition, appt.Status)
		}

		now := r.now()
		if appt.HoldExpiresAt != nil && !appt.HoldExpiresAt.After(now) {
			return ErrHoldExpired
		}

		entry := HistoryEntry{
			Status:    StatusConfirmed,
			ChangedAt: now,
			ActorID:   actorID,
			Note:      strings.TrimSpace(note),
		}
		if _, err := r.appts.UpdateOne(txCtx, bson.D{{Key: "_id", Value: appointmentID}}, bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "status", Value: StatusConfirmed},
				{Key: "updatedAt", Value: now},
			}},
			{Key: "$unset", Value: bson.D{{Key: "holdExpiresAt", Value: ""}}},
			{Key: "$push", Value: bson.D{{Key: "history", Value: entry}}},
		}); err != nil {
			return err
		}

		cells, err := BuildSlotCells(appt.StartAt, appt.EndAt, appt.BufferMinutes)
		if err != nil {
			return err
		}
		res, err := r.locks.UpdateMany(txCtx, bson.D{{Key: "appointmentId", Value: appointmentID}}, bson.D{
			{Key: "$set", Value: bson.D{{Key: "status", Value: StatusConfirmed}}},
			{Key: "$unset", Value: bson.D{{Key: "expiresAt", Value: ""}}},
		})
		if err != nil {
			return err
		}
		// One lock per cell for the practitioner and one for the room.
		if res.MatchedCount != int64(2*len(cells)) {
			return ErrHoldExpired
		}

		got, err := r.appts.FindOne(txCtx, bson.D{{Key: "_id", Value: appointmentID}})
		if err != nil {
			return err
		}
		updated = got
		return nil
	})
	if err != nil {
		return Appointment{}, err
	}
	return updated, nil
}

// Reschedule atomically releases the appointment's old slot_locks, acquires new
// slot_locks for the target window/resources, and updates the appointment.
func (r *Repository) Reschedule(ctx context.Context, in RescheduleInput) (Appointment, error) {
	if in.AppointmentID.IsZero() {
		return Appointment{}, ErrAppointmentNotFound
	}
	if in.PractitionerID.IsZero() || in.RoomID.IsZero() {
		return Appointment{}, fmt.Errorf("%w: practitionerId and roomId are required", ErrInvalidAppointment)
	}
	cells, err := BuildSlotCells(in.StartAt, in.EndAt, in.BufferMinutes)
	if err != nil {
		return Appointment{}, err
	}

	clinic, ok := platform.TenantFrom(ctx)
	if !ok || clinic.ID.IsZero() {
		return Appointment{}, tenantdb.ErrNoTenant
	}

	var updated Appointment
	err = r.runTx(ctx, func(txCtx context.Context) error {
		appt, err := r.appts.FindOne(txCtx, bson.D{{Key: "_id", Value: in.AppointmentID}})
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrAppointmentNotFound
		}
		if err != nil {
			return err
		}
		if appt.Status != StatusHeld && appt.Status != StatusConfirmed {
			return fmt.Errorf("%w: cannot reschedule appointment in status %s", ErrInvalidTransition, appt.Status)
		}

		now := r.now()
		if appt.Status == StatusHeld && appt.HoldExpiresAt != nil && !appt.HoldExpiresAt.After(now) {
			return ErrHoldExpired
		}

		if _, err := r.locks.DeleteMany(txCtx, bson.D{{Key: "appointmentId", Value: appt.ID}}); err != nil {
			return err
		}
		if err := r.reclaimExpiredLocks(txCtx, in.PractitionerID, in.RoomID, cells, now); err != nil {
			return err
		}
		if err := r.insertLocks(txCtx, clinic.ID, appt.ID, in.PractitionerID, in.RoomID, cells, appt.Status, appt.HoldExpiresAt, now); err != nil {
			return err
		}

		entry := HistoryEntry{
			Status:    appt.Status,
			ChangedAt: now,
			ActorID:   in.ActorID,
			Note:      strings.TrimSpace(in.Note),
		}
		if _, err := r.appts.UpdateOne(txCtx, bson.D{{Key: "_id", Value: appt.ID}}, bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "practitionerId", Value: in.PractitionerID},
				{Key: "roomId", Value: in.RoomID},
				{Key: "startAt", Value: in.StartAt.UTC().Truncate(time.Millisecond)},
				{Key: "endAt", Value: in.EndAt.UTC().Truncate(time.Millisecond)},
				{Key: "bufferMinutes", Value: in.BufferMinutes},
				{Key: "updatedAt", Value: now},
			}},
			{Key: "$push", Value: bson.D{{Key: "history", Value: entry}}},
		}); err != nil {
			return err
		}

		got, err := r.appts.FindOne(txCtx, bson.D{{Key: "_id", Value: appt.ID}})
		if err != nil {
			return err
		}
		updated = got
		return nil
	})
	if err != nil {
		return Appointment{}, err
	}
	return updated, nil
}

// Cancel transitions a held or confirmed appointment to cancelled and deletes
// all of its slot_locks inside a transaction.
func (r *Repository) Cancel(ctx context.Context, appointmentID bson.ObjectID, actorID *bson.ObjectID, reason string) (Appointment, error) {
	if appointmentID.IsZero() {
		return Appointment{}, ErrAppointmentNotFound
	}
	var updated Appointment
	err := r.runTx(ctx, func(txCtx context.Context) error {
		appt, err := r.appts.FindOne(txCtx, bson.D{{Key: "_id", Value: appointmentID}})
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrAppointmentNotFound
		}
		if err != nil {
			return err
		}
		if appt.Status != StatusHeld && appt.Status != StatusConfirmed {
			return fmt.Errorf("%w: cannot cancel appointment in status %s", ErrInvalidTransition, appt.Status)
		}

		now := r.now()
		trimmedReason := strings.TrimSpace(reason)
		entry := HistoryEntry{
			Status:    StatusCancelled,
			ChangedAt: now,
			ActorID:   actorID,
			Note:      trimmedReason,
		}
		set := bson.D{
			{Key: "status", Value: StatusCancelled},
			{Key: "updatedAt", Value: now},
		}
		if trimmedReason != "" {
			set = append(set, bson.E{Key: "cancellationReason", Value: trimmedReason})
		}

		if _, err := r.appts.UpdateOne(txCtx, bson.D{{Key: "_id", Value: appointmentID}}, bson.D{
			{Key: "$set", Value: set},
			{Key: "$unset", Value: bson.D{{Key: "holdExpiresAt", Value: ""}}},
			{Key: "$push", Value: bson.D{{Key: "history", Value: entry}}},
		}); err != nil {
			return err
		}
		if _, err := r.locks.DeleteMany(txCtx, bson.D{{Key: "appointmentId", Value: appointmentID}}); err != nil {
			return err
		}

		got, err := r.appts.FindOne(txCtx, bson.D{{Key: "_id", Value: appointmentID}})
		if err != nil {
			return err
		}
		updated = got
		return nil
	})
	if err != nil {
		return Appointment{}, err
	}
	return updated, nil
}

// TransitionStatus transitions a confirmed appointment to completed or no_show.
func (r *Repository) TransitionStatus(ctx context.Context, appointmentID bson.ObjectID, target Status, actorID *bson.ObjectID, note string) (Appointment, error) {
	if appointmentID.IsZero() {
		return Appointment{}, ErrAppointmentNotFound
	}
	if target != StatusCompleted && target != StatusNoShow {
		return Appointment{}, fmt.Errorf("%w: unsupported target status %s", ErrInvalidTransition, target)
	}
	appt, err := r.appts.FindOne(ctx, bson.D{{Key: "_id", Value: appointmentID}})
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Appointment{}, ErrAppointmentNotFound
	}
	if err != nil {
		return Appointment{}, err
	}
	if appt.Status != StatusConfirmed {
		return Appointment{}, fmt.Errorf("%w: must be confirmed before %s (was %s)", ErrInvalidTransition, target, appt.Status)
	}

	now := r.now()
	if now.Before(appt.StartAt) {
		return Appointment{}, fmt.Errorf("%w: cannot mark %s before the appointment starts", ErrInvalidTransition, target)
	}
	entry := HistoryEntry{
		Status:    target,
		ChangedAt: now,
		ActorID:   actorID,
		Note:      strings.TrimSpace(note),
	}
	if err := r.applyTransition(ctx, appointmentID, entry); err != nil {
		return Appointment{}, err
	}
	return r.ByID(ctx, appointmentID)
}

// applyTransition writes the status carried by entry, only if the appointment
// is still confirmed, and frees the cells that have not started yet.
func (r *Repository) applyTransition(ctx context.Context, appointmentID bson.ObjectID, entry HistoryEntry) error {
	return r.runTx(ctx, func(txCtx context.Context) error {
		res, err := r.appts.UpdateOne(txCtx, bson.D{
			{Key: "_id", Value: appointmentID},
			{Key: "status", Value: StatusConfirmed},
		}, bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "status", Value: entry.Status},
				{Key: "updatedAt", Value: entry.ChangedAt},
			}},
			{Key: "$push", Value: bson.D{{Key: "history", Value: entry}}},
		})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return fmt.Errorf("%w: appointment is no longer confirmed", ErrInvalidTransition)
		}
		_, err = r.locks.DeleteMany(txCtx, bson.D{
			{Key: "appointmentId", Value: appointmentID},
			{Key: "cellStart", Value: bson.D{{Key: "$gt", Value: entry.ChangedAt}}},
		})
		return err
	})
}

// ByID returns the appointment identified by id in the context clinic.
func (r *Repository) ByID(ctx context.Context, id bson.ObjectID) (Appointment, error) {
	got, err := r.appts.FindOne(ctx, bson.D{{Key: "_id", Value: id}})
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Appointment{}, ErrAppointmentNotFound
	}
	return got, err
}

// ListInRange returns appointments overlapping [from, to) ordered by startAt.
func (r *Repository) ListInRange(ctx context.Context, from, to time.Time, practitionerID *bson.ObjectID) ([]Appointment, error) {
	if !to.After(from) {
		return nil, fmt.Errorf("%w: to must be after from", ErrInvalidAppointment)
	}
	filter := bson.D{
		{Key: "startAt", Value: bson.D{{Key: "$lt", Value: to.UTC()}}},
		{Key: "endAt", Value: bson.D{{Key: "$gt", Value: from.UTC()}}},
	}
	if practitionerID != nil && !practitionerID.IsZero() {
		filter = append(filter, bson.E{Key: "practitionerId", Value: *practitionerID})
	}
	return r.appts.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "startAt", Value: 1}}))
}

// OccupiedCellsInRange returns active (non-expired) slot_locks in [from, to)
// formatted for schedule.ComputeAvailability.
func (r *Repository) OccupiedCellsInRange(ctx context.Context, from, to time.Time) ([]schedule.OccupiedCell, error) {
	if !to.After(from) {
		return nil, fmt.Errorf("%w: to must be after from", ErrInvalidAppointment)
	}
	now := r.now()
	filter := bson.D{
		{Key: "cellStart", Value: bson.D{
			{Key: "$gte", Value: from.UTC()},
			{Key: "$lt", Value: to.UTC()},
		}},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "status", Value: StatusConfirmed}},
			bson.D{{Key: "status", Value: StatusHeld}, {Key: "expiresAt", Value: bson.D{{Key: "$gt", Value: now}}}},
		}},
	}
	locks, err := r.locks.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]schedule.OccupiedCell, 0, len(locks))
	for _, lk := range locks {
		out = append(out, schedule.OccupiedCell{
			ResourceID: lk.ResourceID,
			CellStart:  lk.CellStart,
		})
	}
	return out, nil
}

// ReleaseExpiredHolds cancels expired held appointments and deletes their slot_locks.
func (r *Repository) ReleaseExpiredHolds(ctx context.Context) (int, error) {
	now := r.now()
	expired, err := r.appts.Find(ctx, bson.D{
		{Key: "status", Value: StatusHeld},
		{Key: "holdExpiresAt", Value: bson.D{{Key: "$lte", Value: now}}},
	})
	if err != nil {
		return 0, err
	}
	released := 0
	for _, appt := range expired {
		ok, err := r.releaseHold(ctx, appt.ID, now)
		if err != nil {
			return released, err
		}
		if ok {
			released++
		}
	}
	return released, nil
}

// releaseHold cancels one expired hold and deletes its locks, reporting whether
// the appointment was still an expired hold.
func (r *Repository) releaseHold(ctx context.Context, appointmentID bson.ObjectID, now time.Time) (bool, error) {
	released := false
	err := r.runTx(ctx, func(txCtx context.Context) error {
		res, err := r.appts.UpdateOne(txCtx, bson.D{
			{Key: "_id", Value: appointmentID},
			{Key: "status", Value: StatusHeld},
			{Key: "holdExpiresAt", Value: bson.D{{Key: "$lte", Value: now}}},
		}, bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "status", Value: StatusCancelled},
				{Key: "cancellationReason", Value: "hold_expired"},
				{Key: "updatedAt", Value: now},
			}},
			{Key: "$unset", Value: bson.D{{Key: "holdExpiresAt", Value: ""}}},
			{Key: "$push", Value: bson.D{{Key: "history", Value: HistoryEntry{
				Status:    StatusCancelled,
				ChangedAt: now,
				Note:      "hold_expired",
			}}}},
		})
		if err != nil {
			return err
		}
		released = res.ModifiedCount > 0
		if !released {
			return nil
		}
		if _, err := r.locks.DeleteMany(txCtx, bson.D{{Key: "appointmentId", Value: appointmentID}}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return released, nil
}

func (r *Repository) reclaimExpiredLocks(
	ctx context.Context,
	practitionerID, roomID bson.ObjectID,
	cells []time.Time,
	now time.Time,
) error {
	_, err := r.locks.DeleteMany(ctx, bson.D{
		{Key: "resourceId", Value: bson.D{{Key: "$in", Value: bson.A{practitionerID, roomID}}}},
		{Key: "cellStart", Value: bson.D{{Key: "$in", Value: cells}}},
		{Key: "status", Value: StatusHeld},
		{Key: "expiresAt", Value: bson.D{{Key: "$lte", Value: now}}},
	})
	return err
}

func (r *Repository) insertLocks(
	ctx context.Context,
	clinicID, appointmentID, practitionerID, roomID bson.ObjectID,
	cells []time.Time,
	status Status,
	expiresAt *time.Time,
	now time.Time,
) error {
	for _, cell := range cells {
		for _, res := range []struct {
			id  bson.ObjectID
			typ ResourceType
		}{
			{id: practitionerID, typ: ResourcePractitioner},
			{id: roomID, typ: ResourceRoom},
		} {
			lk := SlotLock{
				ID:            bson.NewObjectID(),
				ClinicID:      clinicID,
				ResourceID:    res.id,
				ResourceType:  res.typ,
				CellStart:     cell,
				AppointmentID: appointmentID,
				Status:        status,
				ExpiresAt:     expiresAt,
				CreatedAt:     now,
			}
			if err := r.locks.Insert(ctx, &lk); err != nil {
				if mongo.IsDuplicateKeyError(err) {
					return ErrSlotUnavailable
				}
				return err
			}
		}
	}
	return nil
}

func (r *Repository) runTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	sess, err := r.client.StartSession()
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(txCtx context.Context) (any, error) {
		if err := fn(txCtx); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}

func init() {
	tenantdb.Register(tenantdb.Registration{
		Name:       "appointments",
		Collection: "appointments",
		NewProbe: func(client *mongo.Client) tenantdb.ScopedProbe {
			return &apptProbe{repo: tenantdb.NewRepo[probeAppointment](client, "", "appointments")}
		},
	})
	tenantdb.Register(tenantdb.Registration{
		Name:       "slot_locks",
		Collection: "slot_locks",
		NewProbe: func(client *mongo.Client) tenantdb.ScopedProbe {
			return &apptProbe{repo: tenantdb.NewRepo[probeAppointment](client, "", "slot_locks")}
		},
	})
}

type probeAppointment struct {
	ID       bson.ObjectID `bson:"_id,omitempty"`
	ClinicID bson.ObjectID `bson:"clinicId,omitempty"`
	Marker   string        `bson:"marker"`
}

type apptProbe struct {
	repo *tenantdb.Repo[probeAppointment]
}

func (p *apptProbe) InsertSample(ctx context.Context, marker string) error {
	return p.repo.Insert(ctx, &probeAppointment{Marker: marker})
}

func (p *apptProbe) CountVisible(ctx context.Context, filter any) (int64, error) {
	items, err := p.repo.Find(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int64(len(items)), nil
}

func (p *apptProbe) UpdateSample(ctx context.Context, filter any) (int64, error) {
	res, err := p.repo.UpdateMany(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "marker", Value: "updated"}}}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

func (p *apptProbe) DeleteSample(ctx context.Context, filter any) (int64, error) {
	res, err := p.repo.DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

func (p *apptProbe) AggregateCount(ctx context.Context) (int64, error) {
	rows, err := p.repo.Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$count", Value: "n"}},
	})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	switch v := rows[0]["n"].(type) {
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	default:
		return 0, fmt.Errorf("unexpected count type %T", v)
	}
}
