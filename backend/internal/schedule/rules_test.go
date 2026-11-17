package schedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/zSamir015/agenda-clinicas/backend/internal/migrate"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
)

func setupScheduleTestDB(t *testing.T) (*mongo.Client, platform.Clinic, platform.Clinic) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })

	client := cl.Mongo()
	platformDB := client.Database("platform")
	runner, err := migrate.NewRunner(platformDB, migrate.OperationsMigrations())
	if err != nil {
		t.Fatalf("migrate.NewRunner: %v", err)
	}

	clinicStd := platform.Clinic{ID: bson.NewObjectID(), Slug: "clinica-std", Tier: platform.TierStandard}
	clinicDed := platform.Clinic{ID: bson.NewObjectID(), Slug: "clinica-ded", Tier: platform.TierDedicated}

	if err := runner.Run(ctx, client.Database("operations")); err != nil {
		t.Fatalf("migrate operations: %v", err)
	}
	if err := runner.Run(ctx, client.Database("clinic_"+clinicDed.ID.Hex())); err != nil {
		t.Fatalf("migrate dedicated: %v", err)
	}
	return client, clinicStd, clinicDed
}

func TestRulesAndBlocksCRUDWithSchemaAndRBAC(t *testing.T) {
	client, clinicA, clinicB := setupScheduleTestDB(t)
	ctxA := platform.WithTenant(context.Background(), clinicA)
	ctxB := platform.WithTenant(context.Background(), clinicB)

	rulesRepo := NewRuleRepository(client, "")
	blocksRepo := NewBlockRepository(client, "")
	svc := NewRuleService(rulesRepo, blocksRepo)

	prac1 := bson.NewObjectID()
	prac2 := bson.NewObjectID()
	room1 := bson.NewObjectID()

	ownGrants := []rbac.Grant{{Permission: rbac.PermScheduleManage, Scope: rbac.ScopeOwn}}
	allGrants := []rbac.Grant{{Permission: rbac.PermScheduleManage, Scope: rbac.ScopeAll}}
	readOnly := []rbac.Grant{{Permission: rbac.PermAppointmentsRead, Scope: rbac.ScopeAll}}

	t.Run("validation rejects non-grid times and invalid scopes", func(t *testing.T) {
		badRule := AvailabilityRule{
			PractitionerID: prac1,
			RoomID:         room1,
			Weekday:        time.Monday,
			StartMinute:    482,
			EndMinute:      600,
			Active:         true,
		}
		if _, err := svc.CreateRule(ctxA, allGrants, nil, badRule); !errors.Is(err, ErrInvalidRule) {
			t.Fatalf("non-5-min startMinute: got %v, want ErrInvalidRule", err)
		}

		badWindow := badRule
		badWindow.StartMinute = 600
		badWindow.EndMinute = 600
		if _, err := svc.CreateRule(ctxA, allGrants, nil, badWindow); !errors.Is(err, ErrInvalidRule) {
			t.Fatalf("equal start/end: got %v, want ErrInvalidRule", err)
		}

		badBlock := Block{
			Scope:   BlockScopePractitioner,
			Reason:  "Congreso médico",
			Source:  BlockSourceManual,
			StartAt: time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC),
			EndAt:   time.Date(2026, 6, 1, 17, 0, 0, 0, time.UTC),
		}
		if _, err := svc.CreateBlock(ctxA, allGrants, nil, badBlock); !errors.Is(err, ErrInvalidBlock) {
			t.Fatalf("missing practitionerId for practitioner block: got %v, want ErrInvalidBlock", err)
		}

		reversedBlock := Block{
			Scope:   BlockScopeClinic,
			Reason:  "Cierre",
			Source:  BlockSourceManual,
			StartAt: time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
			EndAt:   time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
		}
		if _, err := svc.CreateBlock(ctxA, allGrants, nil, reversedBlock); !errors.Is(err, ErrInvalidBlock) {
			t.Fatalf("non-positive block window: got %v, want ErrInvalidBlock", err)
		}
	})

	t.Run("RBAC own vs all and CRUD across rules and blocks", func(t *testing.T) {
		vf := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		created, err := svc.CreateRule(ctxA, ownGrants, &prac1, AvailabilityRule{
			PractitionerID: prac1,
			RoomID:         room1,
			Weekday:        time.Monday,
			StartMinute:    480,
			EndMinute:      720,
			ValidFrom:      &vf,
			Active:         true,
		})
		if err != nil {
			t.Fatalf("CreateRule own: %v", err)
		}

		// Practitioner with scope own cannot manage another practitioner or clinic blocks.
		if _, err := svc.CreateRule(ctxA, ownGrants, &prac1, AvailabilityRule{
			PractitionerID: prac2,
			RoomID:         room1,
			Weekday:        time.Tuesday,
			StartMinute:    480,
			EndMinute:      720,
			Active:         true,
		}); !errors.Is(err, rbac.ErrForbidden) {
			t.Fatalf("own creating rule for other practitioner: got %v, want ErrForbidden", err)
		}
		if _, err := svc.CreateRule(ctxA, readOnly, nil, AvailabilityRule{
			PractitionerID: prac1,
			RoomID:         room1,
			Weekday:        time.Wednesday,
			StartMinute:    480,
			EndMinute:      720,
			Active:         true,
		}); !errors.Is(err, rbac.ErrForbidden) {
			t.Fatalf("readOnly creating rule: got %v, want ErrForbidden", err)
		}
		if _, err := svc.CreateBlock(ctxA, ownGrants, &prac1, Block{
			Scope:   BlockScopeClinic,
			Reason:  "Intento clínica",
			Source:  BlockSourceManual,
			StartAt: time.Date(2026, 7, 1, 5, 0, 0, 0, time.UTC),
			EndAt:   time.Date(2026, 7, 2, 5, 0, 0, 0, time.UTC),
		}); !errors.Is(err, rbac.ErrForbidden) {
			t.Fatalf("own creating clinic block: got %v, want ErrForbidden", err)
		}

		created.EndMinute = 780
		updated, err := svc.UpdateRule(ctxA, ownGrants, &prac1, created)
		if err != nil || updated.EndMinute != 780 {
			t.Fatalf("UpdateRule: updated=%+v err=%v", updated, err)
		}

		listed, err := svc.ListRules(ctxA, ownGrants, &prac1, RuleFilter{ActiveOnly: true})
		if err != nil || len(listed) != 1 {
			t.Fatalf("ListRules: len=%d err=%v", len(listed), err)
		}

		// Dedicated clinic B remains completely isolated from clinic A.
		listedB, err := svc.ListRules(ctxB, allGrants, nil, RuleFilter{})
		if err != nil || len(listedB) != 0 {
			t.Fatalf("clinic B leaked clinic A rules: len=%d err=%v", len(listedB), err)
		}

		// Practitioner block + range query.
		blk, err := svc.CreateBlock(ctxA, ownGrants, &prac1, Block{
			Scope:          BlockScopePractitioner,
			PractitionerID: &prac1,
			Reason:         "Jornada de capacitación",
			Source:         BlockSourceManual,
			StartAt:        time.Date(2026, 8, 10, 14, 0, 0, 0, time.UTC),
			EndAt:          time.Date(2026, 8, 10, 18, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("CreateBlock own practitioner: %v", err)
		}

		inWindow, err := blocksRepo.ListInRange(ctxA,
			time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC),
			time.Date(2026, 8, 10, 17, 0, 0, 0, time.UTC),
		)
		if err != nil || len(inWindow) != 1 || inWindow[0].ID != blk.ID {
			t.Fatalf("ListInRange overlapping: got=%+v err=%v", inWindow, err)
		}
		outsideWindow, err := blocksRepo.ListInRange(ctxA,
			time.Date(2026, 8, 10, 18, 0, 0, 0, time.UTC),
			time.Date(2026, 8, 10, 20, 0, 0, 0, time.UTC),
		)
		if err != nil || len(outsideWindow) != 0 {
			t.Fatalf("ListInRange non-overlapping: got=%+v err=%v", outsideWindow, err)
		}

		if err := svc.DeleteBlock(ctxA, ownGrants, &prac1, blk.ID); err != nil {
			t.Fatalf("DeleteBlock: %v", err)
		}
		if err := svc.DeleteRule(ctxA, ownGrants, &prac1, created.ID); err != nil {
			t.Fatalf("DeleteRule: %v", err)
		}
	})
}

func TestSyncPanamaHolidaysIsIdempotentAndUTCAligned(t *testing.T) {
	client, clinicA, _ := setupScheduleTestDB(t)
	ctx := platform.WithTenant(context.Background(), clinicA)

	holidays, err := LoadDefaultPanamaHolidays()
	if err != nil {
		t.Fatalf("LoadDefaultPanamaHolidays: %v", err)
	}

	confirmedExpected := 0
	for _, h := range holidays {
		if h.Confirmed {
			confirmedExpected++
		}
	}
	if confirmedExpected == 0 || len(holidays) <= confirmedExpected {
		t.Fatalf("expected both confirmed and decree holidays, got total=%d confirmed=%d", len(holidays), confirmedExpected)
	}

	blocksRepo := NewBlockRepository(client, "")

	inserted, err := SyncPanamaHolidays(ctx, blocksRepo, holidays, HolidaySyncOptions{})
	if err != nil {
		t.Fatalf("first SyncPanamaHolidays: %v", err)
	}
	if inserted != confirmedExpected {
		t.Fatalf("inserted=%d, want confirmedExpected=%d", inserted, confirmedExpected)
	}

	// Second run must insert zero duplicates.
	again, err := SyncPanamaHolidays(ctx, blocksRepo, holidays, HolidaySyncOptions{})
	if err != nil {
		t.Fatalf("second SyncPanamaHolidays: %v", err)
	}
	if again != 0 {
		t.Fatalf("second sync inserted %d duplicates, want 0", again)
	}

	// Verify 2026-01-01 maps to 2026-01-01T05:00:00Z .. 2026-01-02T05:00:00Z (America/Panama UTC-5).
	janFirstBlocks, err := blocksRepo.ListInRange(ctx,
		time.Date(2026, 1, 1, 5, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 2, 5, 0, 0, 0, time.UTC),
	)
	if err != nil || len(janFirstBlocks) != 1 {
		t.Fatalf("2026-01-01 holiday block: got=%+v err=%v", janFirstBlocks, err)
	}
	if janFirstBlocks[0].Reason != "Año Nuevo" || janFirstBlocks[0].Source != BlockSourceHoliday {
		t.Fatalf("unexpected 2026-01-01 block: %+v", janFirstBlocks[0])
	}
	if !janFirstBlocks[0].StartAt.Equal(time.Date(2026, 1, 1, 5, 0, 0, 0, time.UTC)) ||
		!janFirstBlocks[0].EndAt.Equal(time.Date(2026, 1, 2, 5, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected UTC window for 2026-01-01: %s .. %s", janFirstBlocks[0].StartAt, janFirstBlocks[0].EndAt)
	}

	// Enabling IncludeUnconfirmed imports only the remaining decree-pending dates.
	decreeInserted, err := SyncPanamaHolidays(ctx, blocksRepo, holidays, HolidaySyncOptions{IncludeUnconfirmed: true})
	if err != nil {
		t.Fatalf("IncludeUnconfirmed sync: %v", err)
	}
	if decreeInserted != len(holidays)-confirmedExpected {
		t.Fatalf("decreeInserted=%d, want %d", decreeInserted, len(holidays)-confirmedExpected)
	}
}
