package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/auth"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/users"
)

func badRequest(code, title, detail string) BadRequestApplicationProblemPlusJSONResponse {
	return BadRequestApplicationProblemPlusJSONResponse(newProblem(http.StatusBadRequest, title, code, detail))
}

func unauthorized(code, title, detail string) UnauthorizedApplicationProblemPlusJSONResponse {
	return UnauthorizedApplicationProblemPlusJSONResponse(newProblem(http.StatusUnauthorized, title, code, detail))
}

func forbidden(code, title, detail string) ForbiddenApplicationProblemPlusJSONResponse {
	return ForbiddenApplicationProblemPlusJSONResponse(newProblem(http.StatusForbidden, title, code, detail))
}

func notFound(code, title, detail string) NotFoundApplicationProblemPlusJSONResponse {
	return NotFoundApplicationProblemPlusJSONResponse(newProblem(http.StatusNotFound, title, code, detail))
}

func conflict(code, title, detail string) ConflictApplicationProblemPlusJSONResponse {
	return ConflictApplicationProblemPlusJSONResponse(newProblem(http.StatusConflict, title, code, detail))
}

func gone(code, title, detail string) GoneApplicationProblemPlusJSONResponse {
	return GoneApplicationProblemPlusJSONResponse(newProblem(http.StatusGone, title, code, detail))
}

func (s *Server) sessionCookieValue(rawToken string, ttl time.Duration) *string {
	name := s.auth.CookieName
	if name == "" {
		name = auth.DefaultCookieName
	}
	v := (&http.Cookie{ //nolint:gosec // Secure controlled by environment config
		Name:     name,
		Value:    rawToken,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}).String()
	return &v
}

func (s *Server) clearSessionCookieValue() *string {
	name := s.auth.CookieName
	if name == "" {
		name = auth.DefaultCookieName
	}
	v := (&http.Cookie{ //nolint:gosec // Secure controlled by environment config
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}).String()
	return &v
}

func (s *Server) requireStaff(ctx context.Context) (auth.Session, error) {
	sess, ok := auth.SessionFrom(ctx)
	if !ok || sess.Kind != auth.SessionStaff {
		return auth.Session{}, auth.ErrSessionInvalid
	}
	return sess, nil
}

func (s *Server) buildSessionDTO(ctx context.Context, sess auth.Session) (Session, error) {
	clinic, err := s.auth.Clinics.ByID(ctx, sess.ClinicID)
	if err != nil {
		return Session{}, err
	}
	out := Session{
		Kind:       SessionKind(sess.Kind),
		ClinicId:   clinic.ID.Hex(),
		ClinicSlug: clinic.Slug,
		ClinicName: clinic.Name,
		CsrfToken:  sess.CSRFToken,
		ExpiresAt:  sess.ExpiresAt,
	}
	if sess.Kind == auth.SessionPatient {
		contact := sess.PatientContact
		out.PatientContact = &contact
		return out, nil
	}

	user, err := s.auth.Users.UserByID(ctx, sess.UserID)
	if err != nil {
		return Session{}, err
	}
	role, err := s.auth.Roles.ByID(ctx, sess.ClinicID, sess.RoleID)
	if err != nil {
		return Session{}, err
	}

	grants := toDTOGrants(sess.Grants)
	uid := user.ID.Hex()
	rid := role.ID.Hex()
	out.UserId = &uid
	out.Email = &user.Email
	out.Name = &user.Name
	out.RoleId = &rid
	out.RoleName = &role.Name
	out.Grants = &grants

	if s.auth.Creds != nil {
		creds, credErr := s.auth.Creds.ByUserID(ctx, user.ID)
		if credErr == nil {
			hasPasskey := len(creds.WebAuthnCredentials) > 0
			totpEnabled := creds.TOTPEnabled
			out.HasPasskey = &hasPasskey
			out.TotpEnabled = &totpEnabled
		}
	}
	return out, nil
}

func (s *Server) resolveStaffForClinicLogin(ctx context.Context, clinicSlug, email string) (platform.Clinic, users.User, users.Membership, rbac.Role, error) {
	clinic, err := s.auth.Clinics.BySlug(ctx, strings.TrimSpace(strings.ToLower(clinicSlug)))
	if err != nil || clinic.Status != platform.StatusActive {
		return platform.Clinic{}, users.User{}, users.Membership{}, rbac.Role{}, auth.ErrInvalidCredentials
	}
	user, err := s.auth.Users.UserByEmail(ctx, email)
	if err != nil || user.Status != users.UserActive {
		return platform.Clinic{}, users.User{}, users.Membership{}, rbac.Role{}, auth.ErrInvalidCredentials
	}
	membership, err := s.auth.Users.MembershipByUserAndClinic(ctx, user.ID, clinic.ID)
	if err != nil || membership.Status != users.MembershipActive {
		return platform.Clinic{}, users.User{}, users.Membership{}, rbac.Role{}, auth.ErrInvalidCredentials
	}
	role, err := s.auth.Roles.ByID(ctx, clinic.ID, membership.RoleID)
	if err != nil {
		return platform.Clinic{}, users.User{}, users.Membership{}, rbac.Role{}, auth.ErrInvalidCredentials
	}
	return clinic, user, membership, role, nil
}

func structToJSONMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func credentialRequestFromMap(ctx context.Context, target string, payload map[string]any) (*http.Request, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// GetAuthSession returns the active session (staff or patient).
func (s *Server) GetAuthSession(ctx context.Context, _ GetAuthSessionRequestObject) (GetAuthSessionResponseObject, error) {
	sess, ok := auth.SessionFrom(ctx)
	if !ok {
		return GetAuthSession401ApplicationProblemPlusJSONResponse{unauthorized("unauthorized", "Sesión requerida", "Inicia sesión para continuar.")}, nil
	}
	dto, err := s.buildSessionDTO(ctx, sess)
	if err != nil {
		return GetAuthSession401ApplicationProblemPlusJSONResponse{unauthorized("unauthorized", "Sesión inválida", "No se pudo verificar la sesión actual.")}, nil
	}
	return GetAuthSession200JSONResponse(dto), nil
}

// PostAuthLogout revokes the current session and expires the cookie.
func (s *Server) PostAuthLogout(ctx context.Context, _ PostAuthLogoutRequestObject) (PostAuthLogoutResponseObject, error) {
	rawToken, ok := rawSessionTokenFrom(ctx)
	if !ok {
		return PostAuthLogout401ApplicationProblemPlusJSONResponse{unauthorized("unauthorized", "Sesión requerida", "")}, nil
	}
	_ = s.auth.Sessions.Revoke(ctx, rawToken)
	return PostAuthLogout204Response{
		Headers: PostAuthLogout204ResponseHeaders{
			SetCookie: s.clearSessionCookieValue(),
		},
	}, nil
}

// GetAuthSecurity returns the passkeys, TOTP status, and remaining recovery codes for the signed-in staff user.
func (s *Server) GetAuthSecurity(ctx context.Context, _ GetAuthSecurityRequestObject) (GetAuthSecurityResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return GetAuthSecurity403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "Solo el personal puede consultar su seguridad.")}, nil
	}
	status := SecurityStatus{
		TotpEnabled:            false,
		RecoveryCodesRemaining: 0,
		Passkeys:               []PasskeySummary{},
	}
	if s.auth.Creds != nil {
		creds, credErr := s.auth.Creds.ByUserID(ctx, sess.UserID)
		if credErr == nil {
			status.TotpEnabled = creds.TOTPEnabled
			status.RecoveryCodesRemaining = len(creds.RecoveryCodeHashes)
			for _, pk := range creds.WebAuthnCredentials {
				transports := make([]string, 0, len(pk.Transport))
				for _, tr := range pk.Transport {
					transports = append(transports, string(tr))
				}
				status.Passkeys = append(status.Passkeys, PasskeySummary{
					Id:         base64.RawURLEncoding.EncodeToString(pk.ID),
					Transports: transports,
				})
			}
		}
	}
	return GetAuthSecurity200JSONResponse(status), nil
}

// PostAuthLoginPassword signs in clinic staff using Argon2id password + TOTP or recovery code.
func (s *Server) PostAuthLoginPassword(ctx context.Context, req PostAuthLoginPasswordRequestObject) (PostAuthLoginPasswordResponseObject, error) {
	if req.Body == nil || strings.TrimSpace(req.Body.ClinicSlug) == "" || strings.TrimSpace(req.Body.Password) == "" || strings.TrimSpace(req.Body.Code) == "" {
		return PostAuthLoginPassword400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Datos incompletos", "Indica clínica, correo, contraseña y código.")}, nil
	}
	if s.auth.Creds == nil {
		return PostAuthLoginPassword401ApplicationProblemPlusJSONResponse{unauthorized("invalid_credentials", "Credenciales inválidas", "Verifica tus datos e intenta de nuevo.")}, nil
	}

	clinic, user, _, role, err := s.resolveStaffForClinicLogin(ctx, req.Body.ClinicSlug, string(req.Body.Email))
	if err != nil {
		return PostAuthLoginPassword401ApplicationProblemPlusJSONResponse{unauthorized("invalid_credentials", "Credenciales inválidas", "Verifica tu correo, contraseña y código.")}, nil
	}

	if err := s.auth.Creds.VerifyPasswordAndSecondFactor(ctx, user.ID, req.Body.Password, req.Body.Code); err != nil {
		return PostAuthLoginPassword401ApplicationProblemPlusJSONResponse{unauthorized("invalid_credentials", "Credenciales inválidas", "Verifica tu correo, contraseña y código.")}, nil
	}

	sess, rawToken, err := s.auth.Sessions.CreateStaffSession(ctx, user.ID, clinic.ID, role.ID, role.Grants, s.auth.SessionTTL)
	if err != nil {
		return PostAuthLoginPassword401ApplicationProblemPlusJSONResponse{unauthorized("session_failed", "No se pudo iniciar sesión", "")}, nil
	}
	dto, err := s.buildSessionDTO(ctx, sess)
	if err != nil {
		return PostAuthLoginPassword401ApplicationProblemPlusJSONResponse{unauthorized("session_failed", "No se pudo cargar la sesión", "")}, nil
	}
	return PostAuthLoginPassword200JSONResponse{
		Body:    dto,
		Headers: PostAuthLoginPassword200ResponseHeaders{SetCookie: s.sessionCookieValue(rawToken, s.auth.SessionTTL)},
	}, nil
}

// PostAuthLoginPasskeyBegin starts a WebAuthn assertion challenge for a staff user.
func (s *Server) PostAuthLoginPasskeyBegin(ctx context.Context, req PostAuthLoginPasskeyBeginRequestObject) (PostAuthLoginPasskeyBeginResponseObject, error) {
	if req.Body == nil || strings.TrimSpace(req.Body.ClinicSlug) == "" {
		return PostAuthLoginPasskeyBegin400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Datos incompletos", "Indica clínica y correo.")}, nil
	}
	if s.auth.WebAuthn == nil {
		return PostAuthLoginPasskeyBegin401ApplicationProblemPlusJSONResponse{unauthorized("passkey_unavailable", "Passkeys no disponibles", "")}, nil
	}

	clinic, user, _, role, err := s.resolveStaffForClinicLogin(ctx, req.Body.ClinicSlug, string(req.Body.Email))
	if err != nil {
		return PostAuthLoginPasskeyBegin401ApplicationProblemPlusJSONResponse{unauthorized("invalid_credentials", "No se encontró passkey activa", "Verifica la clínica, el correo o usa tu contraseña y TOTP.")}, nil
	}

	assertion, sessionData, err := s.auth.WebAuthn.BeginLogin(ctx, user.ID, user.Email, user.Name)
	if err != nil || sessionData == nil {
		return PostAuthLoginPasskeyBegin401ApplicationProblemPlusJSONResponse{unauthorized("invalid_credentials", "No se encontró passkey activa", "Registra una passkey o inicia con contraseña y TOTP.")}, nil
	}

	optionsMap, err := structToJSONMap(assertion)
	if err != nil {
		return PostAuthLoginPasskeyBegin400ApplicationProblemPlusJSONResponse{badRequest("encoding_failed", "Error preparando reto", "")}, nil
	}
	challengeID, err := s.challenges.put(challengeEntry{
		Kind:        challengeLogin,
		UserID:      user.ID,
		ClinicID:    clinic.ID,
		RoleID:      role.ID,
		Email:       user.Email,
		DisplayName: user.Name,
		Data:        *sessionData,
	})
	if err != nil {
		return PostAuthLoginPasskeyBegin400ApplicationProblemPlusJSONResponse{badRequest("challenge_failed", "No se pudo crear el reto", "")}, nil
	}
	return PostAuthLoginPasskeyBegin200JSONResponse{
		ChallengeId: challengeID,
		Options:     optionsMap,
	}, nil
}

// PostAuthLoginPasskeyFinish completes a WebAuthn assertion challenge and opens a staff session.
func (s *Server) PostAuthLoginPasskeyFinish(ctx context.Context, req PostAuthLoginPasskeyFinishRequestObject) (PostAuthLoginPasskeyFinishResponseObject, error) {
	if req.Body == nil || strings.TrimSpace(req.Body.ChallengeId) == "" || len(req.Body.Credential) == 0 {
		return PostAuthLoginPasskeyFinish400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Datos incompletos", "Falta la respuesta de la passkey.")}, nil
	}
	if s.auth.WebAuthn == nil {
		return PostAuthLoginPasskeyFinish401ApplicationProblemPlusJSONResponse{unauthorized("passkey_unavailable", "Passkeys no disponibles", "")}, nil
	}

	entry, ok := s.challenges.take(req.Body.ChallengeId, challengeLogin)
	if !ok {
		return PostAuthLoginPasskeyFinish401ApplicationProblemPlusJSONResponse{unauthorized("challenge_expired", "Reto vencido o inválido", "Inicia el acceso con passkey de nuevo.")}, nil
	}

	httpReq, err := credentialRequestFromMap(ctx, "/auth/login/passkey/finish", req.Body.Credential)
	if err != nil {
		return PostAuthLoginPasskeyFinish400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Credencial mal formada", "")}, nil
	}
	if _, err := s.auth.WebAuthn.FinishLogin(ctx, entry.UserID, entry.Email, entry.DisplayName, entry.Data, httpReq); err != nil {
		return PostAuthLoginPasskeyFinish401ApplicationProblemPlusJSONResponse{unauthorized("invalid_credentials", "Passkey inválida", "No se pudo verificar la passkey.")}, nil
	}

	user, err := s.auth.Users.UserByID(ctx, entry.UserID)
	if err != nil || user.Status != users.UserActive {
		return PostAuthLoginPasskeyFinish403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Usuario deshabilitado", "")}, nil
	}
	membership, err := s.auth.Users.MembershipByUserAndClinic(ctx, entry.UserID, entry.ClinicID)
	if err != nil || membership.Status != users.MembershipActive {
		return PostAuthLoginPasskeyFinish403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Membresía suspendida", "")}, nil
	}
	role, err := s.auth.Roles.ByID(ctx, entry.ClinicID, membership.RoleID)
	if err != nil {
		return PostAuthLoginPasskeyFinish403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Rol inválido", "")}, nil
	}

	sess, rawToken, err := s.auth.Sessions.CreateStaffSession(ctx, user.ID, entry.ClinicID, role.ID, role.Grants, s.auth.SessionTTL)
	if err != nil {
		return PostAuthLoginPasskeyFinish401ApplicationProblemPlusJSONResponse{unauthorized("session_failed", "No se pudo crear la sesión", "")}, nil
	}
	dto, err := s.buildSessionDTO(ctx, sess)
	if err != nil {
		return PostAuthLoginPasskeyFinish401ApplicationProblemPlusJSONResponse{unauthorized("session_failed", "No se pudo cargar la sesión", "")}, nil
	}
	return PostAuthLoginPasskeyFinish200JSONResponse{
		Body:    dto,
		Headers: PostAuthLoginPasskeyFinish200ResponseHeaders{SetCookie: s.sessionCookieValue(rawToken, s.auth.SessionTTL)},
	}, nil
}

// PostAuthInvitationPreview inspects an invitation token without consuming it.
func (s *Server) PostAuthInvitationPreview(ctx context.Context, req PostAuthInvitationPreviewRequestObject) (PostAuthInvitationPreviewResponseObject, error) {
	if req.Body == nil || strings.TrimSpace(req.Body.Token) == "" {
		return PostAuthInvitationPreview400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Token requerido", "")}, nil
	}
	inv, err := s.auth.Users.PreviewInvitation(ctx, req.Body.Token)
	switch {
	case errors.Is(err, users.ErrInvitationInvalid):
		return PostAuthInvitationPreview404ApplicationProblemPlusJSONResponse{notFound("invitation_not_found", "Invitación no encontrada", "")}, nil
	case errors.Is(err, users.ErrInvitationExpired), errors.Is(err, users.ErrInvitationUsed):
		return PostAuthInvitationPreview410ApplicationProblemPlusJSONResponse{gone("invitation_unavailable", "Invitación vencida o ya usada", "")}, nil
	case err != nil:
		return PostAuthInvitationPreview400ApplicationProblemPlusJSONResponse{badRequest("invitation_error", "No se pudo consultar la invitación", "")}, nil
	}

	clinic, err := s.auth.Clinics.ByID(ctx, inv.ClinicID)
	if err != nil {
		return PostAuthInvitationPreview404ApplicationProblemPlusJSONResponse{notFound("clinic_not_found", "Clínica no encontrada", "")}, nil
	}
	role, err := s.auth.Roles.ByID(ctx, inv.ClinicID, inv.RoleID)
	if err != nil {
		return PostAuthInvitationPreview404ApplicationProblemPlusJSONResponse{notFound("role_not_found", "Rol no encontrado", "")}, nil
	}
	return PostAuthInvitationPreview200JSONResponse{
		Email:      inv.Email,
		Name:       inv.Name,
		ClinicSlug: clinic.Slug,
		ClinicName: clinic.Name,
		RoleName:   role.Name,
		ExpiresAt:  inv.ExpiresAt,
	}, nil
}

// PostAuthInvitationAccept consumes an invitation token, links/creates the user, and opens a staff session.
func (s *Server) PostAuthInvitationAccept(ctx context.Context, req PostAuthInvitationAcceptRequestObject) (PostAuthInvitationAcceptResponseObject, error) {
	if req.Body == nil || strings.TrimSpace(req.Body.Token) == "" {
		return PostAuthInvitationAccept400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Token requerido", "")}, nil
	}
	user, membership, err := s.auth.Users.AcceptInvitation(ctx, req.Body.Token)
	switch {
	case errors.Is(err, users.ErrInvitationInvalid):
		return PostAuthInvitationAccept404ApplicationProblemPlusJSONResponse{notFound("invitation_not_found", "Invitación no encontrada", "")}, nil
	case errors.Is(err, users.ErrInvitationExpired), errors.Is(err, users.ErrInvitationUsed):
		return PostAuthInvitationAccept410ApplicationProblemPlusJSONResponse{gone("invitation_unavailable", "Invitación vencida o ya utilizada", "")}, nil
	case errors.Is(err, users.ErrMembershipExists):
		return PostAuthInvitationAccept409ApplicationProblemPlusJSONResponse{conflict("membership_exists", "El usuario ya pertenece a esta clínica", "")}, nil
	case err != nil:
		return PostAuthInvitationAccept400ApplicationProblemPlusJSONResponse{badRequest("invitation_error", "No se pudo aceptar la invitación", "")}, nil
	}

	role, err := s.auth.Roles.ByID(ctx, membership.ClinicID, membership.RoleID)
	if err != nil {
		return PostAuthInvitationAccept404ApplicationProblemPlusJSONResponse{notFound("role_not_found", "Rol no encontrado", "")}, nil
	}
	sess, rawToken, err := s.auth.Sessions.CreateStaffSession(ctx, user.ID, membership.ClinicID, role.ID, role.Grants, s.auth.SessionTTL)
	if err != nil {
		return PostAuthInvitationAccept400ApplicationProblemPlusJSONResponse{badRequest("session_failed", "No se pudo iniciar la sesión", "")}, nil
	}
	dto, err := s.buildSessionDTO(ctx, sess)
	if err != nil {
		return PostAuthInvitationAccept400ApplicationProblemPlusJSONResponse{badRequest("session_failed", "No se pudo cargar la sesión", "")}, nil
	}
	return PostAuthInvitationAccept200JSONResponse{
		Body:    dto,
		Headers: PostAuthInvitationAccept200ResponseHeaders{SetCookie: s.sessionCookieValue(rawToken, s.auth.SessionTTL)},
	}, nil
}

// PostAuthPasskeyRegisterBegin starts WebAuthn passkey registration for the authenticated staff member.
func (s *Server) PostAuthPasskeyRegisterBegin(ctx context.Context, _ PostAuthPasskeyRegisterBeginRequestObject) (PostAuthPasskeyRegisterBeginResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return PostAuthPasskeyRegisterBegin403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "Solo el personal puede registrar passkeys.")}, nil
	}
	if s.auth.WebAuthn == nil {
		return PostAuthPasskeyRegisterBegin401ApplicationProblemPlusJSONResponse{unauthorized("passkey_unavailable", "Passkeys no disponibles", "")}, nil
	}
	user, err := s.auth.Users.UserByID(ctx, sess.UserID)
	if err != nil {
		return PostAuthPasskeyRegisterBegin401ApplicationProblemPlusJSONResponse{unauthorized("unauthorized", "Usuario no encontrado", "")}, nil
	}

	creation, sessionData, err := s.auth.WebAuthn.BeginRegistration(ctx, user.ID, user.Email, user.Name)
	if err != nil || sessionData == nil {
		return PostAuthPasskeyRegisterBegin401ApplicationProblemPlusJSONResponse{unauthorized("webauthn_failed", "No se pudo iniciar el registro", "")}, nil
	}
	optionsMap, err := structToJSONMap(creation)
	if err != nil {
		return PostAuthPasskeyRegisterBegin401ApplicationProblemPlusJSONResponse{unauthorized("webauthn_failed", "Error serializando opciones WebAuthn", "")}, nil
	}
	challengeID, err := s.challenges.put(challengeEntry{
		Kind:        challengeRegister,
		UserID:      user.ID,
		Email:       user.Email,
		DisplayName: user.Name,
		Data:        *sessionData,
	})
	if err != nil {
		return PostAuthPasskeyRegisterBegin401ApplicationProblemPlusJSONResponse{unauthorized("webauthn_failed", "No se pudo guardar el reto", "")}, nil
	}
	return PostAuthPasskeyRegisterBegin200JSONResponse{
		ChallengeId: challengeID,
		Options:     optionsMap,
	}, nil
}

// PostAuthPasskeyRegisterFinish verifies the WebAuthn attestation and persists the new passkey.
func (s *Server) PostAuthPasskeyRegisterFinish(ctx context.Context, req PostAuthPasskeyRegisterFinishRequestObject) (PostAuthPasskeyRegisterFinishResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return PostAuthPasskeyRegisterFinish403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil || strings.TrimSpace(req.Body.ChallengeId) == "" || len(req.Body.Credential) == 0 {
		return PostAuthPasskeyRegisterFinish400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Datos incompletos", "Falta la credencial WebAuthn.")}, nil
	}
	if s.auth.WebAuthn == nil {
		return PostAuthPasskeyRegisterFinish400ApplicationProblemPlusJSONResponse{badRequest("passkey_unavailable", "Passkeys no disponibles", "")}, nil
	}

	entry, ok := s.challenges.take(req.Body.ChallengeId, challengeRegister)
	if !ok || entry.UserID != sess.UserID {
		return PostAuthPasskeyRegisterFinish400ApplicationProblemPlusJSONResponse{badRequest("challenge_expired", "Reto vencido o inválido", "Vuelve a iniciar el registro de passkey.")}, nil
	}

	httpReq, err := credentialRequestFromMap(ctx, "/auth/passkeys/register/finish", req.Body.Credential)
	if err != nil {
		return PostAuthPasskeyRegisterFinish400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Credencial mal formada", "")}, nil
	}
	cred, err := s.auth.WebAuthn.FinishRegistration(ctx, entry.UserID, entry.Email, entry.DisplayName, entry.Data, httpReq)
	if err != nil {
		var pErr *protocol.Error
		detail := "No se pudo verificar la passkey."
		if errors.As(err, &pErr) && pErr.DevInfo != "" {
			detail = pErr.DevInfo
		}
		return PostAuthPasskeyRegisterFinish400ApplicationProblemPlusJSONResponse{badRequest("webauthn_failed", "Passkey rechazada", detail)}, nil
	}

	transports := make([]string, 0, len(cred.Transport))
	for _, tr := range cred.Transport {
		transports = append(transports, string(tr))
	}
	return PostAuthPasskeyRegisterFinish200JSONResponse{
		Id:         base64.RawURLEncoding.EncodeToString(cred.ID),
		Transports: transports,
	}, nil
}

// PostAuthTotpSetup generates a TOTP secret and provisioning URI for the signed-in staff user.
func (s *Server) PostAuthTotpSetup(ctx context.Context, _ PostAuthTotpSetupRequestObject) (PostAuthTotpSetupResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return PostAuthTotpSetup403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	user, err := s.auth.Users.UserByID(ctx, sess.UserID)
	if err != nil {
		return PostAuthTotpSetup401ApplicationProblemPlusJSONResponse{unauthorized("unauthorized", "Usuario no encontrado", "")}, nil
	}
	setup, err := auth.GenerateTOTP(s.auth.TOTPIssuer, user.Email)
	if err != nil {
		return PostAuthTotpSetup401ApplicationProblemPlusJSONResponse{unauthorized("totp_failed", "No se pudo generar el secreto TOTP", "")}, nil
	}
	return PostAuthTotpSetup200JSONResponse{
		Secret: setup.Secret,
		Uri:    setup.URI,
	}, nil
}

// PostAuthTotpConfirm sets an Argon2id password, enables TOTP, and returns 10 single-use recovery codes.
func (s *Server) PostAuthTotpConfirm(ctx context.Context, req PostAuthTotpConfirmRequestObject) (PostAuthTotpConfirmResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return PostAuthTotpConfirm403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Secret) == "" || strings.TrimSpace(req.Body.Code) == "" || req.Body.Password == "" {
		return PostAuthTotpConfirm400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Datos incompletos", "Indica contraseña, secreto y código TOTP.")}, nil
	}
	if s.auth.Creds == nil {
		return PostAuthTotpConfirm400ApplicationProblemPlusJSONResponse{badRequest("credentials_unavailable", "Servicio de credenciales no disponible", "")}, nil
	}
	if !auth.VerifyTOTP(req.Body.Secret, req.Body.Code, time.Now().UTC()) {
		return PostAuthTotpConfirm400ApplicationProblemPlusJSONResponse{badRequest("invalid_totp", "Código TOTP incorrecto", "Verifica la hora de tu dispositivo e intenta de nuevo.")}, nil
	}

	plainCodes, hashes, err := auth.GenerateRecoveryCodes(auth.DefaultRecoveryCodeCount)
	if err != nil {
		return PostAuthTotpConfirm400ApplicationProblemPlusJSONResponse{badRequest("recovery_failed", "No se pudieron generar los códigos de respaldo", "")}, nil
	}
	err = s.auth.Creds.SetPasswordAndTOTP(ctx, sess.UserID, req.Body.Password, req.Body.Secret, hashes)
	if errors.Is(err, auth.ErrWeakPassword) {
		return PostAuthTotpConfirm400ApplicationProblemPlusJSONResponse{badRequest("weak_password", "Contraseña demasiado corta", "Usa al menos 12 caracteres.")}, nil
	}
	if err != nil {
		return PostAuthTotpConfirm400ApplicationProblemPlusJSONResponse{badRequest("save_failed", "No se pudieron guardar las credenciales", "")}, nil
	}
	return PostAuthTotpConfirm200JSONResponse{
		RecoveryCodes: plainCodes,
	}, nil
}

// PostAuthPatientOtpSend generates and dispatches a 6-digit OTP to a patient contact.
func (s *Server) PostAuthPatientOtpSend(ctx context.Context, req PostAuthPatientOtpSendRequestObject) (PostAuthPatientOtpSendResponseObject, error) {
	if req.Body == nil || strings.TrimSpace(req.Body.ClinicSlug) == "" || strings.TrimSpace(req.Body.Contact) == "" {
		return PostAuthPatientOtpSend400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Datos incompletos", "Indica clínica y teléfono o correo.")}, nil
	}
	if s.auth.OTPs == nil {
		return PostAuthPatientOtpSend400ApplicationProblemPlusJSONResponse{badRequest("otp_unavailable", "OTP no disponible", "")}, nil
	}
	clinic, err := s.auth.Clinics.BySlug(ctx, strings.TrimSpace(strings.ToLower(req.Body.ClinicSlug)))
	if err != nil || clinic.Status != platform.StatusActive {
		return PostAuthPatientOtpSend404ApplicationProblemPlusJSONResponse{notFound("clinic_not_found", "Clínica no disponible", "")}, nil
	}

	code, err := s.auth.OTPs.Issue(ctx, clinic.ID, req.Body.Contact)
	if errors.Is(err, auth.ErrOTPRateLimited) {
		return PostAuthPatientOtpSend429ApplicationProblemPlusJSONResponse{TooManyRequestsApplicationProblemPlusJSONResponse(newProblem(http.StatusTooManyRequests, "Límite de envíos alcanzado", "otp_rate_limited", "Máximo 3 códigos por hora por contacto."))}, nil
	}
	if errors.Is(err, auth.ErrOTPInvalid) {
		return PostAuthPatientOtpSend400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Contacto inválido", "")}, nil
	}
	if err != nil {
		return PostAuthPatientOtpSend400ApplicationProblemPlusJSONResponse{badRequest("otp_failed", "No se pudo emitir el código", "")}, nil
	}

	if err := s.auth.OTPSender.SendPatientOTP(ctx, clinic, req.Body.Contact, code); err != nil {
		return PostAuthPatientOtpSend400ApplicationProblemPlusJSONResponse{badRequest("otp_send_failed", "No se pudo enviar el código", "")}, nil
	}
	return PostAuthPatientOtpSend202JSONResponse{
		ExpiresInSeconds: int(auth.DefaultOTPTTL.Seconds()),
	}, nil
}

// PostAuthPatientOtpVerify validates a 6-digit OTP and creates a 30-minute patient session.
func (s *Server) PostAuthPatientOtpVerify(ctx context.Context, req PostAuthPatientOtpVerifyRequestObject) (PostAuthPatientOtpVerifyResponseObject, error) {
	if req.Body == nil || strings.TrimSpace(req.Body.ClinicSlug) == "" || strings.TrimSpace(req.Body.Contact) == "" || strings.TrimSpace(req.Body.Code) == "" {
		return PostAuthPatientOtpVerify400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Datos incompletos", "Indica clínica, contacto y código.")}, nil
	}
	if s.auth.OTPs == nil {
		return PostAuthPatientOtpVerify400ApplicationProblemPlusJSONResponse{badRequest("otp_unavailable", "OTP no disponible", "")}, nil
	}
	clinic, err := s.auth.Clinics.BySlug(ctx, strings.TrimSpace(strings.ToLower(req.Body.ClinicSlug)))
	if err != nil || clinic.Status != platform.StatusActive {
		return PostAuthPatientOtpVerify401ApplicationProblemPlusJSONResponse{unauthorized("invalid_otp", "Código inválido", "")}, nil
	}

	err = s.auth.OTPs.Verify(ctx, clinic.ID, req.Body.Contact, req.Body.Code)
	switch {
	case errors.Is(err, auth.ErrOTPExpired):
		return PostAuthPatientOtpVerify410ApplicationProblemPlusJSONResponse{gone("otp_expired", "Código vencido", "Solicita un código nuevo.")}, nil
	case errors.Is(err, auth.ErrOTPMaxAttempts):
		return PostAuthPatientOtpVerify429ApplicationProblemPlusJSONResponse{TooManyRequestsApplicationProblemPlusJSONResponse(newProblem(http.StatusTooManyRequests, "Demasiados intentos fallidos", "otp_max_attempts", "Solicita un código nuevo."))}, nil
	case err != nil:
		return PostAuthPatientOtpVerify401ApplicationProblemPlusJSONResponse{unauthorized("invalid_otp", "Código inválido", "")}, nil
	}

	sess, rawToken, err := s.auth.Sessions.CreatePatientSession(ctx, clinic.ID, strings.TrimSpace(strings.ToLower(req.Body.Contact)), s.auth.PatientSessionTTL)
	if err != nil {
		return PostAuthPatientOtpVerify401ApplicationProblemPlusJSONResponse{unauthorized("session_failed", "No se pudo crear la sesión", "")}, nil
	}
	dto, err := s.buildSessionDTO(ctx, sess)
	if err != nil {
		return PostAuthPatientOtpVerify401ApplicationProblemPlusJSONResponse{unauthorized("session_failed", "No se pudo cargar la sesión", "")}, nil
	}
	return PostAuthPatientOtpVerify200JSONResponse{
		Body:    dto,
		Headers: PostAuthPatientOtpVerify200ResponseHeaders{SetCookie: s.sessionCookieValue(rawToken, s.auth.PatientSessionTTL)},
	}, nil
}

func parseObjectID(hexStr string) (bson.ObjectID, error) {
	return bson.ObjectIDFromHex(strings.TrimSpace(hexStr))
}
