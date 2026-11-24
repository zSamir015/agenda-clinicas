package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/auth"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/schedule"
)

func (s *Server) scheduleReady() bool {
	return s.schedule.Catalog != nil &&
		s.schedule.Rules != nil &&
		s.schedule.Practitioners != nil &&
		s.schedule.Rooms != nil &&
		s.schedule.Services != nil &&
		s.schedule.RuleRepo != nil &&
		s.schedule.Blocks != nil &&
		s.auth.Clinics != nil
}

func (s *Server) scheduleTenantContext(ctx context.Context, sess auth.Session) (context.Context, error) {
	clinic, err := s.auth.Clinics.ByID(ctx, sess.ClinicID)
	if err != nil {
		return ctx, err
	}
	return platform.WithTenant(ctx, clinic), nil
}

func (s *Server) actorPractitionerID(ctx context.Context, sess auth.Session) *bson.ObjectID {
	if s.schedule.Practitioners == nil {
		return nil
	}
	list, err := s.schedule.Practitioners.List(ctx, false)
	if err != nil {
		return nil
	}
	for _, p := range list {
		if p.UserID != nil && *p.UserID == sess.UserID {
			id := p.ID
			return &id
		}
	}
	return nil
}

func parseObjectIDList(hexes []string) ([]bson.ObjectID, error) {
	out := make([]bson.ObjectID, 0, len(hexes))
	for _, h := range hexes {
		id, err := parseObjectID(h)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func parseOptionalObjectID(hex *string) (*bson.ObjectID, error) {
	if hex == nil || strings.TrimSpace(*hex) == "" {
		return nil, nil
	}
	id, err := parseObjectID(*hex)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func authorizeScheduleRead(grants []rbac.Grant) error {
	if err := rbac.Authorize(grants, rbac.PermScheduleManage, rbac.ScopeOwn); err == nil {
		return nil
	}
	return rbac.Authorize(grants, rbac.PermAppointmentsRead, rbac.ScopeOwn)
}

type mappedProblem struct {
	status int
	prob   Problem
}

func mapScheduleError(err error) mappedProblem {
	switch {
	case errors.Is(err, rbac.ErrForbidden):
		return mappedProblem{
			status: http.StatusForbidden,
			prob:   Problem(forbidden("forbidden", "Permiso insuficiente", "No tienes permiso para realizar esta acción.")),
		}
	case errors.Is(err, schedule.ErrPractitionerNotFound),
		errors.Is(err, schedule.ErrRoomNotFound),
		errors.Is(err, schedule.ErrServiceNotFound),
		errors.Is(err, schedule.ErrRuleNotFound),
		errors.Is(err, schedule.ErrBlockNotFound):
		return mappedProblem{
			status: http.StatusNotFound,
			prob:   Problem(notFound("not_found", "Recurso no encontrado", err.Error())),
		}
	case errors.Is(err, schedule.ErrDuplicateName):
		return mappedProblem{
			status: http.StatusConflict,
			prob:   Problem(conflict("duplicate_name", "Nombre duplicado", "Ya existe un registro con ese nombre en la clínica.")),
		}
	case errors.Is(err, schedule.ErrInvalidPractitioner),
		errors.Is(err, schedule.ErrInvalidRoom),
		errors.Is(err, schedule.ErrInvalidService),
		errors.Is(err, schedule.ErrInvalidRule),
		errors.Is(err, schedule.ErrInvalidBlock),
		errors.Is(err, schedule.ErrInvalidAvailabilityQuery):
		return mappedProblem{
			status: http.StatusBadRequest,
			prob:   Problem(badRequest("validation_failed", "Datos inválidos", err.Error())),
		}
	default:
		return mappedProblem{
			status: http.StatusBadRequest,
			prob:   Problem(badRequest("schedule_error", "No se pudo completar la operación", err.Error())),
		}
	}
}

func toDTOPractitioner(p schedule.Practitioner) Practitioner {
	serviceIDs := make([]string, 0, len(p.ServiceIDs))
	for _, id := range p.ServiceIDs {
		serviceIDs = append(serviceIDs, id.Hex())
	}
	var userID *string
	if p.UserID != nil && !p.UserID.IsZero() {
		s := p.UserID.Hex()
		userID = &s
	}
	return Practitioner{
		Id:         p.ID.Hex(),
		UserId:     userID,
		Name:       p.Name,
		Specialty:  p.Specialty,
		ServiceIds: serviceIDs,
		Active:     p.Active,
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
	}
}

func toDTORoom(r schedule.Room) Room {
	return Room{
		Id:        r.ID.Hex(),
		Name:      r.Name,
		RoomType:  r.RoomType,
		Active:    r.Active,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func toDTOService(sv schedule.Service) Service {
	return Service{
		Id:              sv.ID.Hex(),
		Name:            sv.Name,
		DurationMinutes: sv.DurationMinutes,
		BufferMinutes:   sv.BufferMinutes,
		RoomType:        sv.RoomType,
		PaymentMode:     PaymentMode(sv.PaymentMode),
		PriceCents:      sv.PriceCents,
		DepositCents:    sv.DepositCents,
		DepositPercent:  sv.DepositPercent,
		Active:          sv.Active,
		CreatedAt:       sv.CreatedAt,
		UpdatedAt:       sv.UpdatedAt,
	}
}

func toDTORule(r schedule.AvailabilityRule) AvailabilityRule {
	return AvailabilityRule{
		Id:             r.ID.Hex(),
		PractitionerId: r.PractitionerID.Hex(),
		RoomId:         r.RoomID.Hex(),
		Weekday:        int(r.Weekday),
		StartMinute:    r.StartMinute,
		EndMinute:      r.EndMinute,
		ValidFrom:      r.ValidFrom,
		ValidUntil:     r.ValidUntil,
		Active:         r.Active,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

func toDTOBlock(b schedule.Block) Block {
	var pracID *string
	if b.PractitionerID != nil && !b.PractitionerID.IsZero() {
		s := b.PractitionerID.Hex()
		pracID = &s
	}
	var roomID *string
	if b.RoomID != nil && !b.RoomID.IsZero() {
		s := b.RoomID.Hex()
		roomID = &s
	}
	return Block{
		Id:             b.ID.Hex(),
		Scope:          BlockScope(b.Scope),
		PractitionerId: pracID,
		RoomId:         roomID,
		Reason:         b.Reason,
		Source:         BlockSource(b.Source),
		StartAt:        b.StartAt,
		EndAt:          b.EndAt,
		CreatedAt:      b.CreatedAt,
	}
}

func toDTOAvailableSlot(sl schedule.AvailableSlot) AvailableSlot {
	return AvailableSlot{
		StartAt:        sl.StartAt,
		EndAt:          sl.EndAt,
		PractitionerId: sl.PractitionerID.Hex(),
		RoomId:         sl.RoomID.Hex(),
		RoomName:       sl.RoomName,
	}
}

// GetSchedulePractitioners lists clinic practitioners.
func (s *Server) GetSchedulePractitioners(ctx context.Context, req GetSchedulePractitionersRequestObject) (GetSchedulePractitionersResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return GetSchedulePractitioners403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return GetSchedulePractitioners403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	activeOnly := req.Params.ActiveOnly != nil && *req.Params.ActiveOnly
	list, err := s.schedule.Catalog.ListPractitioners(tctx, sess.Grants, activeOnly)
	if err != nil {
		return GetSchedulePractitioners403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", err.Error())}, nil
	}
	out := make([]Practitioner, 0, len(list))
	for _, p := range list {
		out = append(out, toDTOPractitioner(p))
	}
	return GetSchedulePractitioners200JSONResponse{Practitioners: out}, nil
}

// PostSchedulePractitioners creates a practitioner.
func (s *Server) PostSchedulePractitioners(ctx context.Context, req PostSchedulePractitionersRequestObject) (PostSchedulePractitionersResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return PostSchedulePractitioners403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil {
		return PostSchedulePractitioners400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Cuerpo requerido", "")}, nil
	}
	userID, err := parseOptionalObjectID(req.Body.UserId)
	if err != nil {
		return PostSchedulePractitioners400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "userId inválido", "")}, nil
	}
	serviceIDs, err := parseObjectIDList(req.Body.ServiceIds)
	if err != nil {
		return PostSchedulePractitioners400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "serviceIds inválidos", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return PostSchedulePractitioners403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	created, err := s.schedule.Catalog.CreatePractitioner(tctx, sess.Grants, schedule.Practitioner{
		UserID:     userID,
		Name:       req.Body.Name,
		Specialty:  req.Body.Specialty,
		ServiceIDs: serviceIDs,
		Active:     req.Body.Active,
	})
	if err != nil {
		mp := mapScheduleError(err)
		if mp.status == http.StatusForbidden {
			return PostSchedulePractitioners403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
		return PostSchedulePractitioners400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
	}
	return PostSchedulePractitioners201JSONResponse(toDTOPractitioner(created)), nil
}

// PutSchedulePractitioner updates a practitioner.
func (s *Server) PutSchedulePractitioner(ctx context.Context, req PutSchedulePractitionerRequestObject) (PutSchedulePractitionerResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return PutSchedulePractitioner403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil {
		return PutSchedulePractitioner400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Cuerpo requerido", "")}, nil
	}
	pracID, err := parseObjectID(req.PractitionerId)
	if err != nil {
		return PutSchedulePractitioner400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "practitionerId inválido", "")}, nil
	}
	userID, err := parseOptionalObjectID(req.Body.UserId)
	if err != nil {
		return PutSchedulePractitioner400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "userId inválido", "")}, nil
	}
	serviceIDs, err := parseObjectIDList(req.Body.ServiceIds)
	if err != nil {
		return PutSchedulePractitioner400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "serviceIds inválidos", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return PutSchedulePractitioner403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	updated, err := s.schedule.Catalog.UpdatePractitioner(tctx, sess.Grants, schedule.Practitioner{
		ID:         pracID,
		UserID:     userID,
		Name:       req.Body.Name,
		Specialty:  req.Body.Specialty,
		ServiceIDs: serviceIDs,
		Active:     req.Body.Active,
	})
	if err != nil {
		mp := mapScheduleError(err)
		switch mp.status {
		case http.StatusForbidden:
			return PutSchedulePractitioner403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		case http.StatusNotFound:
			return PutSchedulePractitioner404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse(mp.prob)}, nil
		default:
			return PutSchedulePractitioner400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
	}
	return PutSchedulePractitioner200JSONResponse(toDTOPractitioner(updated)), nil
}

// GetScheduleRooms lists clinic rooms.
func (s *Server) GetScheduleRooms(ctx context.Context, req GetScheduleRoomsRequestObject) (GetScheduleRoomsResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return GetScheduleRooms403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return GetScheduleRooms403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	activeOnly := req.Params.ActiveOnly != nil && *req.Params.ActiveOnly
	list, err := s.schedule.Catalog.ListRooms(tctx, sess.Grants, activeOnly)
	if err != nil {
		return GetScheduleRooms403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", err.Error())}, nil
	}
	out := make([]Room, 0, len(list))
	for _, r := range list {
		out = append(out, toDTORoom(r))
	}
	return GetScheduleRooms200JSONResponse{Rooms: out}, nil
}

// PostScheduleRooms creates a room.
func (s *Server) PostScheduleRooms(ctx context.Context, req PostScheduleRoomsRequestObject) (PostScheduleRoomsResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return PostScheduleRooms403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil {
		return PostScheduleRooms400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Cuerpo requerido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return PostScheduleRooms403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	created, err := s.schedule.Catalog.CreateRoom(tctx, sess.Grants, schedule.Room{
		Name:     req.Body.Name,
		RoomType: req.Body.RoomType,
		Active:   req.Body.Active,
	})
	if err != nil {
		mp := mapScheduleError(err)
		switch mp.status {
		case http.StatusForbidden:
			return PostScheduleRooms403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		case http.StatusConflict:
			return PostScheduleRooms409ApplicationProblemPlusJSONResponse{ConflictApplicationProblemPlusJSONResponse(mp.prob)}, nil
		default:
			return PostScheduleRooms400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
	}
	return PostScheduleRooms201JSONResponse(toDTORoom(created)), nil
}

// PutScheduleRoom updates a room.
func (s *Server) PutScheduleRoom(ctx context.Context, req PutScheduleRoomRequestObject) (PutScheduleRoomResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return PutScheduleRoom403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil {
		return PutScheduleRoom400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Cuerpo requerido", "")}, nil
	}
	roomID, err := parseObjectID(req.RoomId)
	if err != nil {
		return PutScheduleRoom400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "roomId inválido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return PutScheduleRoom403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	updated, err := s.schedule.Catalog.UpdateRoom(tctx, sess.Grants, schedule.Room{
		ID:       roomID,
		Name:     req.Body.Name,
		RoomType: req.Body.RoomType,
		Active:   req.Body.Active,
	})
	if err != nil {
		mp := mapScheduleError(err)
		switch mp.status {
		case http.StatusForbidden:
			return PutScheduleRoom403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		case http.StatusNotFound:
			return PutScheduleRoom404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse(mp.prob)}, nil
		case http.StatusConflict:
			return PutScheduleRoom409ApplicationProblemPlusJSONResponse{ConflictApplicationProblemPlusJSONResponse(mp.prob)}, nil
		default:
			return PutScheduleRoom400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
	}
	return PutScheduleRoom200JSONResponse(toDTORoom(updated)), nil
}

// GetScheduleServices lists clinic services.
func (s *Server) GetScheduleServices(ctx context.Context, req GetScheduleServicesRequestObject) (GetScheduleServicesResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return GetScheduleServices403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return GetScheduleServices403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	activeOnly := req.Params.ActiveOnly != nil && *req.Params.ActiveOnly
	list, err := s.schedule.Catalog.ListServices(tctx, sess.Grants, activeOnly)
	if err != nil {
		return GetScheduleServices403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", err.Error())}, nil
	}
	out := make([]Service, 0, len(list))
	for _, sv := range list {
		out = append(out, toDTOService(sv))
	}
	return GetScheduleServices200JSONResponse{Services: out}, nil
}

// PostScheduleServices creates a service.
func (s *Server) PostScheduleServices(ctx context.Context, req PostScheduleServicesRequestObject) (PostScheduleServicesResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return PostScheduleServices403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil {
		return PostScheduleServices400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Cuerpo requerido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return PostScheduleServices403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	created, err := s.schedule.Catalog.CreateService(tctx, sess.Grants, schedule.Service{
		Name:            req.Body.Name,
		DurationMinutes: req.Body.DurationMinutes,
		BufferMinutes:   req.Body.BufferMinutes,
		RoomType:        req.Body.RoomType,
		PaymentMode:     schedule.PaymentMode(req.Body.PaymentMode),
		PriceCents:      req.Body.PriceCents,
		DepositCents:    req.Body.DepositCents,
		DepositPercent:  req.Body.DepositPercent,
		Active:          req.Body.Active,
	})
	if err != nil {
		mp := mapScheduleError(err)
		switch mp.status {
		case http.StatusForbidden:
			return PostScheduleServices403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		case http.StatusConflict:
			return PostScheduleServices409ApplicationProblemPlusJSONResponse{ConflictApplicationProblemPlusJSONResponse(mp.prob)}, nil
		default:
			return PostScheduleServices400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
	}
	return PostScheduleServices201JSONResponse(toDTOService(created)), nil
}

// PutScheduleService updates a service.
func (s *Server) PutScheduleService(ctx context.Context, req PutScheduleServiceRequestObject) (PutScheduleServiceResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return PutScheduleService403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil {
		return PutScheduleService400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Cuerpo requerido", "")}, nil
	}
	serviceID, err := parseObjectID(req.ServiceId)
	if err != nil {
		return PutScheduleService400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "serviceId inválido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return PutScheduleService403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	updated, err := s.schedule.Catalog.UpdateService(tctx, sess.Grants, schedule.Service{
		ID:              serviceID,
		Name:            req.Body.Name,
		DurationMinutes: req.Body.DurationMinutes,
		BufferMinutes:   req.Body.BufferMinutes,
		RoomType:        req.Body.RoomType,
		PaymentMode:     schedule.PaymentMode(req.Body.PaymentMode),
		PriceCents:      req.Body.PriceCents,
		DepositCents:    req.Body.DepositCents,
		DepositPercent:  req.Body.DepositPercent,
		Active:          req.Body.Active,
	})
	if err != nil {
		mp := mapScheduleError(err)
		switch mp.status {
		case http.StatusForbidden:
			return PutScheduleService403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		case http.StatusNotFound:
			return PutScheduleService404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse(mp.prob)}, nil
		case http.StatusConflict:
			return PutScheduleService409ApplicationProblemPlusJSONResponse{ConflictApplicationProblemPlusJSONResponse(mp.prob)}, nil
		default:
			return PutScheduleService400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
	}
	return PutScheduleService200JSONResponse(toDTOService(updated)), nil
}

// GetScheduleRules lists availability rules.
func (s *Server) GetScheduleRules(ctx context.Context, req GetScheduleRulesRequestObject) (GetScheduleRulesResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return GetScheduleRules403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	pracID, err := parseOptionalObjectID(req.Params.PractitionerId)
	if err != nil {
		return GetScheduleRules400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "practitionerId inválido", "")}, nil
	}
	roomID, err := parseOptionalObjectID(req.Params.RoomId)
	if err != nil {
		return GetScheduleRules400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "roomId inválido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return GetScheduleRules403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	actorPrac := s.actorPractitionerID(tctx, sess)
	rules, err := s.schedule.Rules.ListRules(tctx, sess.Grants, actorPrac, schedule.RuleFilter{
		PractitionerID: pracID,
		RoomID:         roomID,
		ActiveOnly:     req.Params.ActiveOnly != nil && *req.Params.ActiveOnly,
	})
	if err != nil {
		mp := mapScheduleError(err)
		if mp.status == http.StatusForbidden {
			return GetScheduleRules403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
		return GetScheduleRules400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
	}
	out := make([]AvailabilityRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, toDTORule(r))
	}
	return GetScheduleRules200JSONResponse{Rules: out}, nil
}

// PostScheduleRules creates an availability rule.
func (s *Server) PostScheduleRules(ctx context.Context, req PostScheduleRulesRequestObject) (PostScheduleRulesResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return PostScheduleRules403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil {
		return PostScheduleRules400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Cuerpo requerido", "")}, nil
	}
	pracID, err := parseObjectID(req.Body.PractitionerId)
	if err != nil {
		return PostScheduleRules400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "practitionerId inválido", "")}, nil
	}
	roomID, err := parseObjectID(req.Body.RoomId)
	if err != nil {
		return PostScheduleRules400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "roomId inválido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return PostScheduleRules403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	actorPrac := s.actorPractitionerID(tctx, sess)
	created, err := s.schedule.Rules.CreateRule(tctx, sess.Grants, actorPrac, schedule.AvailabilityRule{
		PractitionerID: pracID,
		RoomID:         roomID,
		Weekday:        time.Weekday(req.Body.Weekday),
		StartMinute:    req.Body.StartMinute,
		EndMinute:      req.Body.EndMinute,
		ValidFrom:      req.Body.ValidFrom,
		ValidUntil:     req.Body.ValidUntil,
		Active:         req.Body.Active,
	})
	if err != nil {
		mp := mapScheduleError(err)
		if mp.status == http.StatusForbidden {
			return PostScheduleRules403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
		return PostScheduleRules400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
	}
	return PostScheduleRules201JSONResponse(toDTORule(created)), nil
}

// PutScheduleRule updates an availability rule.
func (s *Server) PutScheduleRule(ctx context.Context, req PutScheduleRuleRequestObject) (PutScheduleRuleResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return PutScheduleRule403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil {
		return PutScheduleRule400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Cuerpo requerido", "")}, nil
	}
	ruleID, err := parseObjectID(req.RuleId)
	if err != nil {
		return PutScheduleRule400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "ruleId inválido", "")}, nil
	}
	pracID, err := parseObjectID(req.Body.PractitionerId)
	if err != nil {
		return PutScheduleRule400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "practitionerId inválido", "")}, nil
	}
	roomID, err := parseObjectID(req.Body.RoomId)
	if err != nil {
		return PutScheduleRule400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "roomId inválido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return PutScheduleRule403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	actorPrac := s.actorPractitionerID(tctx, sess)
	updated, err := s.schedule.Rules.UpdateRule(tctx, sess.Grants, actorPrac, schedule.AvailabilityRule{
		ID:             ruleID,
		PractitionerID: pracID,
		RoomID:         roomID,
		Weekday:        time.Weekday(req.Body.Weekday),
		StartMinute:    req.Body.StartMinute,
		EndMinute:      req.Body.EndMinute,
		ValidFrom:      req.Body.ValidFrom,
		ValidUntil:     req.Body.ValidUntil,
		Active:         req.Body.Active,
	})
	if err != nil {
		mp := mapScheduleError(err)
		switch mp.status {
		case http.StatusForbidden:
			return PutScheduleRule403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		case http.StatusNotFound:
			return PutScheduleRule404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse(mp.prob)}, nil
		default:
			return PutScheduleRule400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
	}
	return PutScheduleRule200JSONResponse(toDTORule(updated)), nil
}

// DeleteScheduleRule deletes an availability rule.
func (s *Server) DeleteScheduleRule(ctx context.Context, req DeleteScheduleRuleRequestObject) (DeleteScheduleRuleResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return DeleteScheduleRule403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	ruleID, err := parseObjectID(req.RuleId)
	if err != nil {
		return DeleteScheduleRule400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "ruleId inválido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return DeleteScheduleRule403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	actorPrac := s.actorPractitionerID(tctx, sess)
	if err := s.schedule.Rules.DeleteRule(tctx, sess.Grants, actorPrac, ruleID); err != nil {
		mp := mapScheduleError(err)
		switch mp.status {
		case http.StatusForbidden:
			return DeleteScheduleRule403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		case http.StatusNotFound:
			return DeleteScheduleRule404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse(mp.prob)}, nil
		default:
			return DeleteScheduleRule400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
	}
	return DeleteScheduleRule204Response{}, nil
}

// GetScheduleBlocks lists blocks overlapping [from, to).
func (s *Server) GetScheduleBlocks(ctx context.Context, req GetScheduleBlocksRequestObject) (GetScheduleBlocksResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return GetScheduleBlocks403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if err := authorizeScheduleRead(sess.Grants); err != nil {
		return GetScheduleBlocks403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return GetScheduleBlocks403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	blocks, err := s.schedule.Blocks.ListInRange(tctx, req.Params.From, req.Params.To)
	if err != nil {
		mp := mapScheduleError(err)
		return GetScheduleBlocks400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
	}
	out := make([]Block, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, toDTOBlock(b))
	}
	return GetScheduleBlocks200JSONResponse{Blocks: out}, nil
}

// PostScheduleBlocks creates a manual schedule block.
func (s *Server) PostScheduleBlocks(ctx context.Context, req PostScheduleBlocksRequestObject) (PostScheduleBlocksResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return PostScheduleBlocks403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil {
		return PostScheduleBlocks400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Cuerpo requerido", "")}, nil
	}
	pracID, err := parseOptionalObjectID(req.Body.PractitionerId)
	if err != nil {
		return PostScheduleBlocks400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "practitionerId inválido", "")}, nil
	}
	roomID, err := parseOptionalObjectID(req.Body.RoomId)
	if err != nil {
		return PostScheduleBlocks400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "roomId inválido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return PostScheduleBlocks403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	actorPrac := s.actorPractitionerID(tctx, sess)
	created, err := s.schedule.Rules.CreateBlock(tctx, sess.Grants, actorPrac, schedule.Block{
		Scope:          schedule.BlockScope(req.Body.Scope),
		PractitionerID: pracID,
		RoomID:         roomID,
		Reason:         req.Body.Reason,
		Source:         schedule.BlockSourceManual,
		StartAt:        req.Body.StartAt,
		EndAt:          req.Body.EndAt,
	})
	if err != nil {
		mp := mapScheduleError(err)
		if mp.status == http.StatusForbidden {
			return PostScheduleBlocks403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
		return PostScheduleBlocks400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
	}
	return PostScheduleBlocks201JSONResponse(toDTOBlock(created)), nil
}

// DeleteScheduleBlock removes a schedule block.
func (s *Server) DeleteScheduleBlock(ctx context.Context, req DeleteScheduleBlockRequestObject) (DeleteScheduleBlockResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return DeleteScheduleBlock403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	blockID, err := parseObjectID(req.BlockId)
	if err != nil {
		return DeleteScheduleBlock400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "blockId inválido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return DeleteScheduleBlock403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}
	actorPrac := s.actorPractitionerID(tctx, sess)
	if err := s.schedule.Rules.DeleteBlock(tctx, sess.Grants, actorPrac, blockID); err != nil {
		mp := mapScheduleError(err)
		switch mp.status {
		case http.StatusForbidden:
			return DeleteScheduleBlock403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse(mp.prob)}, nil
		case http.StatusNotFound:
			return DeleteScheduleBlock404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse(mp.prob)}, nil
		default:
			return DeleteScheduleBlock400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
	}
	return DeleteScheduleBlock204Response{}, nil
}

// GetScheduleAvailability computes bookable slots for a service in [from, to).
func (s *Server) GetScheduleAvailability(ctx context.Context, req GetScheduleAvailabilityRequestObject) (GetScheduleAvailabilityResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil || !s.scheduleReady() {
		return GetScheduleAvailability403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if err := authorizeScheduleRead(sess.Grants); err != nil {
		return GetScheduleAvailability403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", "")}, nil
	}
	serviceID, err := parseObjectID(req.Params.ServiceId)
	if err != nil {
		return GetScheduleAvailability400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "serviceId inválido", "")}, nil
	}
	pracID, err := parseOptionalObjectID(req.Params.PractitionerId)
	if err != nil {
		return GetScheduleAvailability400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "practitionerId inválido", "")}, nil
	}
	tctx, err := s.scheduleTenantContext(ctx, sess)
	if err != nil {
		return GetScheduleAvailability403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Clínica no disponible", "")}, nil
	}

	svc, err := s.schedule.Services.ByID(tctx, serviceID)
	if err != nil {
		mp := mapScheduleError(err)
		if mp.status == http.StatusNotFound {
			return GetScheduleAvailability404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
		return GetScheduleAvailability400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
	}

	practitioners, err := s.schedule.Practitioners.List(tctx, true)
	if err != nil {
		mp := mapScheduleError(err)
		return GetScheduleAvailability400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
	}
	rooms, err := s.schedule.Rooms.List(tctx, true)
	if err != nil {
		mp := mapScheduleError(err)
		return GetScheduleAvailability400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
	}
	rules, err := s.schedule.RuleRepo.List(tctx, schedule.RuleFilter{
		PractitionerID: pracID,
		ActiveOnly:     true,
	})
	if err != nil {
		mp := mapScheduleError(err)
		return GetScheduleAvailability400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
	}
	var blocks []schedule.Block
	if req.Params.To.After(req.Params.From) {
		blocks, err = s.schedule.Blocks.ListInRange(tctx, req.Params.From, req.Params.To)
		if err != nil {
			mp := mapScheduleError(err)
			return GetScheduleAvailability400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
		}
	}

	step := 0
	if req.Params.StepMinutes != nil {
		step = *req.Params.StepMinutes
	}

	slots, err := schedule.ComputeAvailability(schedule.AvailabilityInput{
		From:           req.Params.From,
		To:             req.Params.To,
		StepMinutes:    step,
		Service:        svc,
		PractitionerID: pracID,
		Practitioners:  practitioners,
		Rooms:          rooms,
		Rules:          rules,
		Blocks:         blocks,
	})
	if err != nil {
		mp := mapScheduleError(err)
		return GetScheduleAvailability400ApplicationProblemPlusJSONResponse{BadRequestApplicationProblemPlusJSONResponse(mp.prob)}, nil
	}

	out := make([]AvailableSlot, 0, len(slots))
	for _, sl := range slots {
		out = append(out, toDTOAvailableSlot(sl))
	}
	return GetScheduleAvailability200JSONResponse{Slots: out}, nil
}
