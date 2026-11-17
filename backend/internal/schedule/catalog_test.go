package schedule_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/migrate"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/schedule"
)

func validService() schedule.Service {
	return schedule.Service{
		Name: "Consulta general", DurationMinutes: 30, BufferMinutes: 10, RoomType: "consultorio",
		PaymentMode: schedule.PaymentDeposit, PriceCents: 3500, DepositCents: 1000, DepositPercent: 0, Active: true,
	}
}

func TestServiceValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*schedule.Service)
		ok     bool
	}{
		{"valid", func(*schedule.Service) {}, true},
		{"duration not multiple of 5", func(s *schedule.Service) { s.DurationMinutes = 32 }, false},
		{"duration zero", func(s *schedule.Service) { s.DurationMinutes = 0 }, false},
		{"duration above 8h", func(s *schedule.Service) { s.DurationMinutes = 485 }, false},
		{"buffer not multiple of 5", func(s *schedule.Service) { s.BufferMinutes = 7 }, false},
		{"buffer negative", func(s *schedule.Service) { s.BufferMinutes = -5 }, false},
		{"buffer above 2h", func(s *schedule.Service) { s.BufferMinutes = 125 }, false},
		{"buffer zero allowed", func(s *schedule.Service) { s.BufferMinutes = 0 }, true},
		{"unknown payment mode", func(s *schedule.Service) { s.PaymentMode = "installments" }, false},
		{"empty payment mode", func(s *schedule.Service) { s.PaymentMode = "" }, false},
		{"negative price", func(s *schedule.Service) { s.PriceCents = -1 }, false},
		{"negative deposit", func(s *schedule.Service) { s.DepositCents = -1 }, false},
		{"deposit above price", func(s *schedule.Service) { s.DepositCents = 4000 }, false},
		{"deposit percent above 100", func(s *schedule.Service) { s.DepositPercent = 101 }, false},
		{"free service", func(s *schedule.Service) { s.PaymentMode, s.PriceCents, s.DepositCents = schedule.PaymentNone, 0, 0 }, true},
		{"blank name", func(s *schedule.Service) { s.Name = "  " }, false},
		{"blank room type", func(s *schedule.Service) { s.RoomType = "" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validService()
			tc.mutate(&s)
			err := s.Validate()
			if tc.ok && err != nil {
				t.Fatalf("want valid, got %v", err)
			}
			if !tc.ok && !errors.Is(err, schedule.ErrInvalidService) {
				t.Fatalf("want ErrInvalidService, got %v", err)
			}
		})
	}
}

func TestPractitionerAndRoomValidation(t *testing.T) {
	dup := bson.NewObjectID()
	for name, p := range map[string]schedule.Practitioner{
		"blank name":      {Specialty: "Pediatría"},
		"blank specialty": {Name: "Dra. Ana"},
		"zero service id": {Name: "Dra. Ana", Specialty: "Pediatría", ServiceIDs: []bson.ObjectID{{}}},
		"duplicate id":    {Name: "Dra. Ana", Specialty: "Pediatría", ServiceIDs: []bson.ObjectID{dup, dup}},
	} {
		if err := p.Validate(); !errors.Is(err, schedule.ErrInvalidPractitioner) {
			t.Fatalf("practitioner %s: got %v", name, err)
		}
	}
	for name, r := range map[string]schedule.Room{
		"blank name": {RoomType: "consultorio"},
		"blank type": {Name: "C1"},
	} {
		if err := r.Validate(); !errors.Is(err, schedule.ErrInvalidRoom) {
			t.Fatalf("room %s: got %v", name, err)
		}
	}
}

type catalogEnv struct {
	svc      *schedule.CatalogService
	clinicA  context.Context
	clinicB  context.Context
	owner    []rbac.Grant
	reader   []rbac.Grant
	noAccess []rbac.Grant
}

func newCatalogEnv(t *testing.T) *catalogEnv {
	t.Helper()
	ctx := context.Background()
	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })

	// Real $jsonSchema validators and unique indexes, as in production.
	runner, err := migrate.NewRunner(cl.Mongo().Database("platform"), migrate.OperationsMigrations())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if err := runner.Run(ctx, cl.Mongo().Database("operations")); err != nil {
		t.Fatalf("run operations migrations: %v", err)
	}

	c := cl.Mongo()
	env := &catalogEnv{
		svc: schedule.NewCatalogService(
			schedule.NewPractitionerRepository(c, "operations"),
			schedule.NewRoomRepository(c, "operations"),
			schedule.NewServiceRepository(c, "operations"),
		),
		clinicA: platform.WithTenant(ctx, platform.Clinic{ID: bson.NewObjectID(), Slug: "cat-a", Tier: platform.TierStandard}),
		clinicB: platform.WithTenant(ctx, platform.Clinic{ID: bson.NewObjectID(), Slug: "cat-b", Tier: platform.TierStandard}),
		owner: []rbac.Grant{
			{Permission: rbac.PermClinicSettings, Scope: rbac.ScopeAll},
			{Permission: rbac.PermScheduleManage, Scope: rbac.ScopeAll},
		},
		reader:   []rbac.Grant{{Permission: rbac.PermScheduleManage, Scope: rbac.ScopeOwn}},
		noAccess: []rbac.Grant{{Permission: rbac.PermPatientsRead, Scope: rbac.ScopeAll}},
	}
	return env
}

func TestCatalogCRUDRBACAndIsolation(t *testing.T) {
	e := newCatalogEnv(t)

	// Services: create, unique name, schema-valid round trip.
	svc, err := e.svc.CreateService(e.clinicA, e.owner, validService())
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if svc.ID.IsZero() || svc.CreatedAt.IsZero() || svc.DurationMinutes != 30 {
		t.Fatalf("unexpected service: %+v", svc)
	}
	if _, err := e.svc.CreateService(e.clinicA, e.owner, validService()); !errors.Is(err, schedule.ErrDuplicateName) {
		t.Fatalf("duplicate service name: got %v, want ErrDuplicateName", err)
	}
	// The same name is fine in another clinic.
	if _, err := e.svc.CreateService(e.clinicB, e.owner, validService()); err != nil {
		t.Fatalf("same service name in clinic B: %v", err)
	}

	// Rooms.
	room, err := e.svc.CreateRoom(e.clinicA, e.owner, schedule.Room{Name: "Consultorio 1", RoomType: "consultorio", Active: true})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := e.svc.CreateRoom(e.clinicA, e.owner, schedule.Room{Name: "Consultorio 1", RoomType: "consultorio"}); !errors.Is(err, schedule.ErrDuplicateName) {
		t.Fatalf("duplicate room name: got %v", err)
	}

	// Practitioners: nil service list is stored as an array, services must exist.
	prac, err := e.svc.CreatePractitioner(e.clinicA, e.owner, schedule.Practitioner{Name: "Dra. Ana Ruiz", Specialty: "Pediatría", Active: true})
	if err != nil {
		t.Fatalf("CreatePractitioner without services: %v", err)
	}
	if prac.ServiceIDs == nil || len(prac.ServiceIDs) != 0 {
		t.Fatalf("serviceIds = %#v, want empty non-nil", prac.ServiceIDs)
	}
	if _, err := e.svc.CreatePractitioner(e.clinicA, e.owner, schedule.Practitioner{
		Name: "Dr. Luis", Specialty: "Cardiología", ServiceIDs: []bson.ObjectID{bson.NewObjectID()},
	}); !errors.Is(err, schedule.ErrInvalidPractitioner) {
		t.Fatalf("unknown serviceId: got %v", err)
	}
	prac.ServiceIDs = []bson.ObjectID{svc.ID}
	if prac, err = e.svc.UpdatePractitioner(e.clinicA, e.owner, prac); err != nil || len(prac.ServiceIDs) != 1 {
		t.Fatalf("UpdatePractitioner: %+v err=%v", prac, err)
	}

	// A service of clinic A cannot be attached to a practitioner of clinic B.
	if _, err := e.svc.CreatePractitioner(e.clinicB, e.owner, schedule.Practitioner{
		Name: "Dra. Eva", Specialty: "Pediatría", ServiceIDs: []bson.ObjectID{svc.ID},
	}); !errors.Is(err, schedule.ErrInvalidPractitioner) {
		t.Fatalf("cross-clinic serviceId: got %v", err)
	}

	// Deactivating hides a record from the active listing but keeps it.
	room.Active = false
	if _, err := e.svc.UpdateRoom(e.clinicA, e.owner, room); err != nil {
		t.Fatalf("UpdateRoom: %v", err)
	}
	if rooms, _ := e.svc.ListRooms(e.clinicA, e.owner, true); len(rooms) != 0 {
		t.Fatalf("active rooms = %d, want 0", len(rooms))
	}
	if rooms, _ := e.svc.ListRooms(e.clinicA, e.owner, false); len(rooms) != 1 {
		t.Fatalf("all rooms = %d, want 1", len(rooms))
	}

	// Isolation: clinic B sees none of clinic A's catalog and cannot update it.
	if rooms, _ := e.svc.ListRooms(e.clinicB, e.owner, false); len(rooms) != 0 {
		t.Fatalf("clinic B sees %d rooms of clinic A", len(rooms))
	}
	if pracs, _ := e.svc.ListPractitioners(e.clinicB, e.owner, false); len(pracs) != 0 {
		t.Fatalf("clinic B sees %d practitioners of clinic A", len(pracs))
	}
	svc.Name = "Hackeado"
	if _, err := e.svc.UpdateService(e.clinicB, e.owner, svc); !errors.Is(err, schedule.ErrServiceNotFound) {
		t.Fatalf("cross-clinic update: got %v, want ErrServiceNotFound", err)
	}

	// RBAC: readers list but cannot write; others do neither.
	if _, err := e.svc.ListServices(e.clinicA, e.reader, true); err != nil {
		t.Fatalf("reader ListServices: %v", err)
	}
	for name, err := range map[string]error{
		"reader creates room": func() error {
			_, err := e.svc.CreateRoom(e.clinicA, e.reader, schedule.Room{Name: "X", RoomType: "y"})
			return err
		}(),
		"reader updates service": func() error { _, err := e.svc.UpdateService(e.clinicA, e.reader, svc); return err }(),
		"no access lists":        func() error { _, err := e.svc.ListRooms(e.clinicA, e.noAccess, false); return err }(),
		"no access creates":      func() error { _, err := e.svc.CreateService(e.clinicA, e.noAccess, validService()); return err }(),
	} {
		if !errors.Is(err, rbac.ErrForbidden) {
			t.Fatalf("%s: got %v, want ErrForbidden", name, err)
		}
	}

	// Without a tenant in the context nothing runs.
	if _, err := e.svc.ListServices(context.Background(), e.owner, false); err == nil || !strings.Contains(err.Error(), "tenant") {
		t.Fatalf("no tenant: got %v", err)
	}
}
