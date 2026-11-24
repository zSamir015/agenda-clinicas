package httpapi

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/schedule"
)

func TestMapScheduleError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "forbidden", err: rbac.ErrForbidden, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "practitioner not found", err: schedule.ErrPractitionerNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "room not found", err: schedule.ErrRoomNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "service not found", err: schedule.ErrServiceNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "rule not found", err: schedule.ErrRuleNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "block not found", err: schedule.ErrBlockNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "duplicate name", err: schedule.ErrDuplicateName, wantStatus: http.StatusConflict, wantCode: "duplicate_name"},
		{name: "invalid service", err: schedule.ErrInvalidService, wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
		{name: "invalid availability query", err: schedule.ErrInvalidAvailabilityQuery, wantStatus: http.StatusBadRequest, wantCode: "validation_failed"},
		{name: "generic error", err: errors.New("boom"), wantStatus: http.StatusBadRequest, wantCode: "schedule_error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mapScheduleError(tc.err)
			if got.status != tc.wantStatus {
				t.Errorf("status = %d, want %d", got.status, tc.wantStatus)
			}
			if got.prob.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", got.prob.Code, tc.wantCode)
			}
		})
	}
}

func TestScheduleDTORoundTrip(t *testing.T) {
	id := bson.NewObjectID()
	uid := bson.NewObjectID()
	svcID := bson.NewObjectID()
	now := time.Date(2026, time.November, 24, 15, 0, 0, 0, time.UTC)

	pDTO := toDTOPractitioner(schedule.Practitioner{
		ID:         id,
		UserID:     &uid,
		Name:       "Dra. Pérez",
		Specialty:  "Pediatría",
		ServiceIDs: []bson.ObjectID{svcID},
		Active:     true,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if pDTO.Id != id.Hex() || pDTO.UserId == nil || *pDTO.UserId != uid.Hex() || len(pDTO.ServiceIds) != 1 || pDTO.ServiceIds[0] != svcID.Hex() {
		t.Fatalf("unexpected Practitioner DTO: %+v", pDTO)
	}

	rDTO := toDTORoom(schedule.Room{ID: id, Name: "Consultorio 1", RoomType: "consultorio", Active: true, CreatedAt: now, UpdatedAt: now})
	if rDTO.Id != id.Hex() || rDTO.RoomType != "consultorio" {
		t.Fatalf("unexpected Room DTO: %+v", rDTO)
	}

	sDTO := toDTOService(schedule.Service{
		ID:              id,
		Name:            "Consulta",
		DurationMinutes: 30,
		BufferMinutes:   5,
		RoomType:        "consultorio",
		PaymentMode:     schedule.PaymentDeposit,
		PriceCents:      5000,
		DepositCents:    1500,
		Active:          true,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if sDTO.PaymentMode != PaymentMode("deposit") || sDTO.PriceCents != 5000 || sDTO.DepositCents != 1500 {
		t.Fatalf("unexpected Service DTO: %+v", sDTO)
	}
}
