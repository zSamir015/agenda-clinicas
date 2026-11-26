package appointments

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/zSamir015/agenda-clinicas/backend/internal/migrate"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
)

func TestBuildSlotCells(t *testing.T) {
	base := time.Date(2026, time.November, 24, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		start     time.Time
		end       time.Time
		buffer    int
		wantCount int
		wantErr   error
	}{
		{
			name:      "30m service with 0 buffer produces 6 cells",
			start:     base,
			end:       base.Add(30 * time.Minute),
			buffer:    0,
			wantCount: 6,
		},
		{
			name:      "30m service with 10m buffer produces 8 cells",
			start:     base,
			end:       base.Add(30 * time.Minute),
			buffer:    10,
			wantCount: 8,
		},
		{
			name:    "unaligned start fails",
			start:   base.Add(2 * time.Minute),
			end:     base.Add(32 * time.Minute),
			buffer:  0,
			wantErr: ErrInvalidAppointment,
		},
		{
			name:      "480m appointment is allowed",
			start:     base,
			end:       base.Add(480 * time.Minute),
			wantCount: 96,
		},
		{
			name:    "485m appointment fails",
			start:   base,
			end:     base.Add(485 * time.Minute),
			wantErr: ErrInvalidAppointment,
		},
		{
			name:    "year-long appointment fails",
			start:   base,
			end:     base.AddDate(1, 0, 0),
			wantErr: ErrInvalidAppointment,
		},
		{
			name:    "end <= start fails",
			start:   base,
			end:     base,
			buffer:  0,
			wantErr: ErrInvalidAppointment,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cells, err := BuildSlotCells(tc.start, tc.end, tc.buffer)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cells) != tc.wantCount {
				t.Fatalf("got %d cells, want %d", len(cells), tc.wantCount)
			}
		})
	}
}

func setupAppointmentsRepo(t *testing.T) (*Repository, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })

	client := cl.Mongo()
	const sharedDB = "operations"

	runner, err := migrate.NewRunner(client.Database("platform"), migrate.OperationsMigrations())
	if err != nil {
		t.Fatalf("migrate.NewRunner: %v", err)
	}
	if err := runner.Run(ctx, client.Database(sharedDB)); err != nil {
		t.Fatalf("runner.Run: %v", err)
	}

	clinic := platform.Clinic{
		ID:     bson.NewObjectID(),
		Slug:   "clinica-citas",
		Name:   "Clínica Citas",
		Tier:   platform.TierStandard,
		Status: platform.StatusActive,
	}
	repo := NewRepository(client, sharedDB)
	return repo, platform.WithTenant(ctx, clinic)
}

// futureBase returns 14:00 UTC thirty days from now. Held locks carry a real
// expiresAt, so a fake clock in the past would let the TTL monitor delete them.
func futureBase() time.Time {
	return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 30).Add(14 * time.Hour)
}

func TestAppointmentLifecycleAndHoldReclaim(t *testing.T) {
	repo, ctx := setupAppointmentsRepo(t)

	now := futureBase()
	repo.now = func() time.Time { return now }

	pracID := bson.NewObjectID()
	roomID := bson.NewObjectID()
	svcID := bson.NewObjectID()
	slotStart := now.Add(24 * time.Hour)
	slotEnd := slotStart.Add(30 * time.Minute)

	// 1. Create a held appointment.
	held, err := repo.Book(ctx, BookInput{
		PractitionerID: pracID,
		RoomID:         roomID,
		ServiceID:      svcID,
		PatientRef:     "patient-1",
		StartAt:        slotStart,
		EndAt:          slotEnd,
		BufferMinutes:  5,
	})
	if err != nil {
		t.Fatalf("Book(held): %v", err)
	}
	if held.Status != StatusHeld || held.HoldExpiresAt == nil {
		t.Fatalf("expected held status with HoldExpiresAt, got %+v", held)
	}

	// OccupiedCellsInRange should return 7 practitioner + 7 room = 14 cells.
	occupied, err := repo.OccupiedCellsInRange(ctx, slotStart, slotEnd.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("OccupiedCellsInRange: %v", err)
	}
	if len(occupied) != 14 {
		t.Fatalf("occupied = %d, want 14", len(occupied))
	}

	// 2. Advance clock past hold expiry: new booking reclaims expired locks atomically.
	now = now.Add(11 * time.Minute)
	confirmed, err := repo.Book(ctx, BookInput{
		PractitionerID: pracID,
		RoomID:         roomID,
		ServiceID:      svcID,
		PatientRef:     "patient-2",
		StartAt:        slotStart,
		EndAt:          slotEnd,
		BufferMinutes:  5,
		Confirm:        true,
	})
	if err != nil {
		t.Fatalf("Book(reclaim): %v", err)
	}
	if confirmed.Status != StatusConfirmed {
		t.Fatalf("status = %s, want confirmed", confirmed.Status)
	}

	// Sweep expired holds should mark the first held appointment cancelled.
	released, err := repo.ReleaseExpiredHolds(ctx)
	if err != nil {
		t.Fatalf("ReleaseExpiredHolds: %v", err)
	}
	if released != 1 {
		t.Fatalf("released = %d, want 1", released)
	}

	// 3. Cancel confirmed appointment and verify slot is free again.
	cancelled, err := repo.Cancel(ctx, confirmed.ID, nil, "Paciente reprogramará")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("status = %s, want cancelled", cancelled.Status)
	}

	rebooked, err := repo.Book(ctx, BookInput{
		PractitionerID: pracID,
		RoomID:         roomID,
		ServiceID:      svcID,
		PatientRef:     "patient-3",
		StartAt:        slotStart,
		EndAt:          slotEnd,
		Confirm:        true,
	})
	if err != nil {
		t.Fatalf("Book(after cancel): %v", err)
	}
	now = slotStart
	if _, err := repo.TransitionStatus(ctx, rebooked.ID, StatusCompleted, nil, "Atendida"); err != nil {
		t.Fatalf("TransitionStatus(completed): %v", err)
	}
}

func Test100ConcurrentBookings(t *testing.T) {
	repo, ctx := setupAppointmentsRepo(t)

	pracID := bson.NewObjectID()
	roomID := bson.NewObjectID()
	svcID := bson.NewObjectID()
	slotStart := time.Date(2026, time.November, 26, 15, 0, 0, 0, time.UTC)
	slotEnd := slotStart.Add(30 * time.Minute)

	const total = 100
	var wins atomic.Int32
	var conflicts atomic.Int32
	var otherErrs sync.Mutex
	var unexpected []error

	startGate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(total)

	for i := range total {
		go func(idx int) {
			defer wg.Done()
			<-startGate
			_, err := repo.Book(ctx, BookInput{
				PractitionerID: pracID,
				RoomID:         roomID,
				ServiceID:      svcID,
				PatientRef:     fmt.Sprintf("patient-%d", idx),
				StartAt:        slotStart,
				EndAt:          slotEnd,
				BufferMinutes:  5,
				Confirm:        true,
			})
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, ErrSlotUnavailable) || mongo.IsDuplicateKeyError(err):
				conflicts.Add(1)
			default:
				otherErrs.Lock()
				unexpected = append(unexpected, err)
				otherErrs.Unlock()
			}
		}(i)
	}

	close(startGate)
	wg.Wait()

	if len(unexpected) > 0 {
		t.Fatalf("unexpected errors in concurrent booking: %v", unexpected)
	}
	if wins.Load() != 1 || conflicts.Load() != total-1 {
		t.Fatalf("wins=%d conflicts=%d, want wins=1 conflicts=%d", wins.Load(), conflicts.Load(), total-1)
	}
}

func TestConcurrentReschedule(t *testing.T) {
	repo, ctx := setupAppointmentsRepo(t)

	pracID := bson.NewObjectID()
	roomID := bson.NewObjectID()
	svcID := bson.NewObjectID()

	slotA := time.Date(2026, time.November, 27, 14, 0, 0, 0, time.UTC)
	slotB := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	target := time.Date(2026, time.November, 27, 16, 0, 0, 0, time.UTC)

	apptA, err := repo.Book(ctx, BookInput{
		PractitionerID: pracID,
		RoomID:         roomID,
		ServiceID:      svcID,
		PatientRef:     "patient-a",
		StartAt:        slotA,
		EndAt:          slotA.Add(30 * time.Minute),
		Confirm:        true,
	})
	if err != nil {
		t.Fatalf("Book A: %v", err)
	}
	apptB, err := repo.Book(ctx, BookInput{
		PractitionerID: pracID,
		RoomID:         roomID,
		ServiceID:      svcID,
		PatientRef:     "patient-b",
		StartAt:        slotB,
		EndAt:          slotB.Add(30 * time.Minute),
		Confirm:        true,
	})
	if err != nil {
		t.Fatalf("Book B: %v", err)
	}

	startGate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	var errA, errB error

	go func() {
		defer wg.Done()
		<-startGate
		_, errA = repo.Reschedule(ctx, RescheduleInput{
			AppointmentID:  apptA.ID,
			PractitionerID: pracID,
			RoomID:         roomID,
			StartAt:        target,
			EndAt:          target.Add(30 * time.Minute),
		})
	}()
	go func() {
		defer wg.Done()
		<-startGate
		_, errB = repo.Reschedule(ctx, RescheduleInput{
			AppointmentID:  apptB.ID,
			PractitionerID: pracID,
			RoomID:         roomID,
			StartAt:        target,
			EndAt:          target.Add(30 * time.Minute),
		})
	}()

	close(startGate)
	wg.Wait()

	aWon := errA == nil && errors.Is(errB, ErrSlotUnavailable)
	bWon := errB == nil && errors.Is(errA, ErrSlotUnavailable)
	if !aWon && !bWon {
		t.Fatalf("expected exactly one reschedule win and one ErrSlotUnavailable, got errA=%v errB=%v", errA, errB)
	}

	// Verify the loser kept its original slot and its locks are still intact.
	gotA, _ := repo.ByID(ctx, apptA.ID)
	gotB, _ := repo.ByID(ctx, apptB.ID)
	if aWon {
		if !gotA.StartAt.Equal(target) || !gotB.StartAt.Equal(slotB) {
			t.Fatalf("unexpected times after A won: A=%v B=%v", gotA.StartAt, gotB.StartAt)
		}
	} else {
		if !gotB.StartAt.Equal(target) || !gotA.StartAt.Equal(slotA) {
			t.Fatalf("unexpected times after B won: A=%v B=%v", gotA.StartAt, gotB.StartAt)
		}
	}
}

// bookAt books a 30-minute appointment on fresh resources starting at start.
func bookAt(ctx context.Context, t *testing.T, repo *Repository, start time.Time, confirm bool) Appointment {
	t.Helper()
	appt, err := repo.Book(ctx, BookInput{
		PractitionerID: bson.NewObjectID(),
		RoomID:         bson.NewObjectID(),
		ServiceID:      bson.NewObjectID(),
		PatientRef:     "patient",
		StartAt:        start,
		EndAt:          start.Add(30 * time.Minute),
		Confirm:        confirm,
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	return appt
}

func countLocks(ctx context.Context, t *testing.T, repo *Repository, appointmentID bson.ObjectID) int {
	t.Helper()
	locks, err := repo.locks.Find(ctx, bson.D{{Key: "appointmentId", Value: appointmentID}})
	if err != nil {
		t.Fatalf("find locks: %v", err)
	}
	return len(locks)
}

// A sweep that listed a hold just before it was confirmed must leave it alone.
func TestReleaseHoldKeepsLocksOfConfirmedAppointment(t *testing.T) {
	repo, ctx := setupAppointmentsRepo(t)
	now := futureBase()
	repo.now = func() time.Time { return now }
	held := bookAt(ctx, t, repo, now.Add(24*time.Hour), false)
	if _, err := repo.ConfirmHold(ctx, held.ID, nil, ""); err != nil {
		t.Fatalf("ConfirmHold: %v", err)
	}

	released, err := repo.releaseHold(ctx, held.ID, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("releaseHold: %v", err)
	}
	if released {
		t.Fatal("released = true, want false")
	}
	if got := countLocks(ctx, t, repo, held.ID); got != 12 {
		t.Fatalf("locks = %d, want 12", got)
	}
}

// Locks can vanish before the hold expires on this clock: the TTL monitor and
// other API instances run on their own.
func TestConfirmHoldRequiresEveryLock(t *testing.T) {
	repo, ctx := setupAppointmentsRepo(t)
	held := bookAt(ctx, t, repo, futureBase(), false)
	if _, err := repo.locks.DeleteMany(ctx, bson.D{
		{Key: "appointmentId", Value: held.ID},
		{Key: "cellStart", Value: held.StartAt},
	}); err != nil {
		t.Fatalf("delete locks: %v", err)
	}

	if _, err := repo.ConfirmHold(ctx, held.ID, nil, ""); !errors.Is(err, ErrHoldExpired) {
		t.Fatalf("error = %v, want ErrHoldExpired", err)
	}
	got, err := repo.ByID(ctx, held.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.Status != StatusHeld {
		t.Fatalf("status = %s, want held", got.Status)
	}
}

// A transition decided on a stale read must not overwrite a cancellation.
func TestApplyTransitionRejectsCancelledAppointment(t *testing.T) {
	repo, ctx := setupAppointmentsRepo(t)
	appt := bookAt(ctx, t, repo, futureBase(), true)
	if _, err := repo.Cancel(ctx, appt.ID, nil, ""); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	err := repo.applyTransition(ctx, appt.ID, HistoryEntry{Status: StatusCompleted, ChangedAt: repo.now()})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("error = %v, want ErrInvalidTransition", err)
	}
	got, err := repo.ByID(ctx, appt.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.Status != StatusCancelled {
		t.Fatalf("status = %s, want cancelled", got.Status)
	}
}

func TestTransitionStatusRejectsFutureAppointment(t *testing.T) {
	for _, target := range []Status{StatusCompleted, StatusNoShow} {
		t.Run(string(target), func(t *testing.T) {
			repo, ctx := setupAppointmentsRepo(t)
			appt := bookAt(ctx, t, repo, futureBase(), true)

			if _, err := repo.TransitionStatus(ctx, appt.ID, target, nil, ""); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v, want ErrInvalidTransition", err)
			}
		})
	}
}

func TestTransitionStatusReleasesRemainingCells(t *testing.T) {
	repo, ctx := setupAppointmentsRepo(t)
	start := futureBase()
	appt := bookAt(ctx, t, repo, start, true)
	repo.now = func() time.Time { return start.Add(12 * time.Minute) }

	if _, err := repo.TransitionStatus(ctx, appt.ID, StatusCompleted, nil, ""); err != nil {
		t.Fatalf("TransitionStatus: %v", err)
	}
	// Cells 14:00, 14:05 and 14:10 have started; one lock per resource each.
	if got := countLocks(ctx, t, repo, appt.ID); got != 6 {
		t.Fatalf("locks = %d, want 6", got)
	}
}

func TestRescheduleBuffer(t *testing.T) {
	zero := 0
	tests := []struct {
		name       string
		buffer     *int
		wantBuffer int
		wantLocks  int
	}{
		{name: "nil keeps the stored buffer", buffer: nil, wantBuffer: 10, wantLocks: 16},
		{name: "explicit zero removes it", buffer: &zero, wantBuffer: 0, wantLocks: 12},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, ctx := setupAppointmentsRepo(t)
			start := futureBase()
			appt, err := repo.Book(ctx, BookInput{
				PractitionerID: bson.NewObjectID(),
				RoomID:         bson.NewObjectID(),
				ServiceID:      bson.NewObjectID(),
				PatientRef:     "patient",
				StartAt:        start,
				EndAt:          start.Add(30 * time.Minute),
				BufferMinutes:  10,
				Confirm:        true,
			})
			if err != nil {
				t.Fatalf("Book: %v", err)
			}

			moved, err := repo.Reschedule(ctx, RescheduleInput{
				AppointmentID:  appt.ID,
				PractitionerID: appt.PractitionerID,
				RoomID:         appt.RoomID,
				StartAt:        start.Add(2 * time.Hour),
				EndAt:          start.Add(2*time.Hour + 30*time.Minute),
				BufferMinutes:  tc.buffer,
			})
			if err != nil {
				t.Fatalf("Reschedule: %v", err)
			}
			if moved.BufferMinutes != tc.wantBuffer {
				t.Fatalf("buffer = %d, want %d", moved.BufferMinutes, tc.wantBuffer)
			}
			if got := countLocks(ctx, t, repo, appt.ID); got != tc.wantLocks {
				t.Fatalf("locks = %d, want %d", got, tc.wantLocks)
			}
		})
	}
}

// A slot starting just before the window end needs the cells that follow it.
func TestOccupiedCellsCoverSlotsStartingBeforeWindowEnd(t *testing.T) {
	repo, ctx := setupAppointmentsRepo(t)
	start := futureBase()
	bookAt(ctx, t, repo, start.Add(time.Hour), true)

	occupied, err := repo.OccupiedCellsInRange(ctx, start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("OccupiedCellsInRange: %v", err)
	}
	if len(occupied) != 12 {
		t.Fatalf("occupied = %d, want 12", len(occupied))
	}
}
