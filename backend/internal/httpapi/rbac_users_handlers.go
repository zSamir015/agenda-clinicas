package httpapi

import (
	"context"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/users"
)

func toDTOGrants(in []rbac.Grant) []Grant {
	out := make([]Grant, 0, len(in))
	for _, g := range in {
		out = append(out, Grant{
			Permission: Permission(g.Permission),
			Scope:      Scope(g.Scope),
		})
	}
	return out
}

func fromDTOGrants(in []Grant) []rbac.Grant {
	out := make([]rbac.Grant, 0, len(in))
	for _, g := range in {
		out = append(out, rbac.Grant{
			Permission: rbac.Permission(g.Permission),
			Scope:      rbac.Scope(g.Scope),
		})
	}
	return out
}

func toDTORole(r rbac.Role) Role {
	return Role{
		Id:        r.ID.Hex(),
		Name:      r.Name,
		Template:  r.Template,
		Locked:    r.Locked,
		Grants:    toDTOGrants(r.Grants),
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

// GetRbacPermissions returns the fixed permission catalog and scopes.
func (s *Server) GetRbacPermissions(ctx context.Context, _ GetRbacPermissionsRequestObject) (GetRbacPermissionsResponseObject, error) {
	if _, err := s.requireStaff(ctx); err != nil {
		return GetRbacPermissions403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	perms := rbac.AllPermissions()
	dtoPerms := make([]Permission, 0, len(perms))
	for _, p := range perms {
		dtoPerms = append(dtoPerms, Permission(p))
	}
	return GetRbacPermissions200JSONResponse{
		Permissions: dtoPerms,
		Scopes:      []Scope{Scope(rbac.ScopeOwn), Scope(rbac.ScopeAll)},
	}, nil
}

// GetRbacRoles lists roles for the current clinic.
func (s *Server) GetRbacRoles(ctx context.Context, _ GetRbacRolesRequestObject) (GetRbacRolesResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return GetRbacRoles403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if err := rbac.Authorize(sess.Grants, rbac.PermRolesManage, rbac.ScopeAll); err != nil {
		if usersErr := rbac.Authorize(sess.Grants, rbac.PermUsersManage, rbac.ScopeAll); usersErr != nil {
			return GetRbacRoles403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", "Requiere roles.manage o users.manage.")}, nil
		}
	}

	roles, err := s.auth.Roles.ListByClinic(ctx, sess.ClinicID)
	if err != nil {
		return GetRbacRoles403ApplicationProblemPlusJSONResponse{forbidden("roles_load_failed", "No se pudieron listar los roles", "")}, nil
	}
	out := make([]Role, 0, len(roles))
	for _, r := range roles {
		out = append(out, toDTORole(r))
	}
	return GetRbacRoles200JSONResponse{Roles: out}, nil
}

// PostRbacRoles creates a custom role after checking roles.manage and anti-escalation.
func (s *Server) PostRbacRoles(ctx context.Context, req PostRbacRolesRequestObject) (PostRbacRolesResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return PostRbacRoles403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if err := rbac.Authorize(sess.Grants, rbac.PermRolesManage, rbac.ScopeAll); err != nil {
		return PostRbacRoles403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", "Requiere roles.manage.")}, nil
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Name) == "" {
		return PostRbacRoles400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Nombre requerido", "")}, nil
	}

	role, err := s.auth.Roles.Create(ctx, sess.Grants, sess.ClinicID, req.Body.Name, fromDTOGrants(req.Body.Grants))
	switch {
	case errors.Is(err, rbac.ErrPrivilegeEscalation):
		return PostRbacRoles403ApplicationProblemPlusJSONResponse{forbidden("privilege_escalation", "Escalada de permisos denegada", "No puedes otorgar permisos o alcances que no posees.")}, nil
	case errors.Is(err, rbac.ErrRoleTaken):
		return PostRbacRoles409ApplicationProblemPlusJSONResponse{conflict("role_taken", "Ya existe un rol con ese nombre", "")}, nil
	case errors.Is(err, rbac.ErrInvalidPermission), errors.Is(err, rbac.ErrInvalidScope), errors.Is(err, rbac.ErrInvalidRole):
		return PostRbacRoles400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Definición de rol inválida", "")}, nil
	case err != nil:
		return PostRbacRoles400ApplicationProblemPlusJSONResponse{badRequest("role_create_failed", "No se pudo crear el rol", "")}, nil
	}
	return PostRbacRoles201JSONResponse(toDTORole(role)), nil
}

// PutRbacRole updates an editable role and invalidates active session role caches.
func (s *Server) PutRbacRole(ctx context.Context, req PutRbacRoleRequestObject) (PutRbacRoleResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return PutRbacRole403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if err := rbac.Authorize(sess.Grants, rbac.PermRolesManage, rbac.ScopeAll); err != nil {
		return PutRbacRole403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", "Requiere roles.manage.")}, nil
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Name) == "" {
		return PutRbacRole400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Nombre requerido", "")}, nil
	}
	roleID, err := parseObjectID(req.RoleId)
	if err != nil {
		return PutRbacRole400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Identificador de rol inválido", "")}, nil
	}

	updated, err := s.auth.Roles.Update(ctx, sess.Grants, sess.ClinicID, roleID, req.Body.Name, fromDTOGrants(req.Body.Grants))
	switch {
	case errors.Is(err, rbac.ErrRoleNotFound):
		return PutRbacRole404ApplicationProblemPlusJSONResponse{notFound("role_not_found", "Rol no encontrado", "")}, nil
	case errors.Is(err, rbac.ErrLockedRole):
		return PutRbacRole403ApplicationProblemPlusJSONResponse{forbidden("locked_role", "El rol owner no se puede modificar", "")}, nil
	case errors.Is(err, rbac.ErrPrivilegeEscalation):
		return PutRbacRole403ApplicationProblemPlusJSONResponse{forbidden("privilege_escalation", "Escalada de permisos denegada", "No puedes otorgar permisos o alcances que no posees.")}, nil
	case errors.Is(err, rbac.ErrRoleTaken):
		return PutRbacRole409ApplicationProblemPlusJSONResponse{conflict("role_taken", "Ya existe un rol con ese nombre", "")}, nil
	case errors.Is(err, rbac.ErrInvalidPermission), errors.Is(err, rbac.ErrInvalidScope), errors.Is(err, rbac.ErrInvalidRole):
		return PutRbacRole400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Definición de rol inválida", "")}, nil
	case err != nil:
		return PutRbacRole400ApplicationProblemPlusJSONResponse{badRequest("role_update_failed", "No se pudo actualizar el rol", "")}, nil
	}

	if err := s.auth.Sessions.InvalidateRoleCache(ctx, sess.ClinicID, roleID, updated.Grants); err != nil {
		return PutRbacRole400ApplicationProblemPlusJSONResponse{badRequest("session_cache_failed", "No se pudo refrescar la caché de sesiones", "")}, nil
	}
	return PutRbacRole200JSONResponse(toDTORole(updated)), nil
}

// DeleteRbacRole deletes a non-locked, unused role in the current clinic.
func (s *Server) DeleteRbacRole(ctx context.Context, req DeleteRbacRoleRequestObject) (DeleteRbacRoleResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return DeleteRbacRole403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if err := rbac.Authorize(sess.Grants, rbac.PermRolesManage, rbac.ScopeAll); err != nil {
		return DeleteRbacRole403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", "Requiere roles.manage.")}, nil
	}
	roleID, err := parseObjectID(req.RoleId)
	if err != nil {
		return DeleteRbacRole400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Identificador de rol inválido", "")}, nil
	}

	members, err := s.auth.Users.ListMembershipsByClinic(ctx, sess.ClinicID)
	if err != nil {
		return DeleteRbacRole400ApplicationProblemPlusJSONResponse{badRequest("role_delete_failed", "No se pudo verificar los miembros del rol", "")}, nil
	}
	for _, m := range members {
		if m.RoleID == roleID && m.Status == users.MembershipActive {
			return DeleteRbacRole409ApplicationProblemPlusJSONResponse{conflict("role_in_use", "El rol tiene miembros activos asignados", "")}, nil
		}
	}

	err = s.auth.Roles.Delete(ctx, sess.ClinicID, roleID)
	switch {
	case errors.Is(err, rbac.ErrRoleNotFound):
		return DeleteRbacRole404ApplicationProblemPlusJSONResponse{notFound("role_not_found", "Rol no encontrado", "")}, nil
	case errors.Is(err, rbac.ErrLockedRole):
		return DeleteRbacRole403ApplicationProblemPlusJSONResponse{forbidden("locked_role", "El rol owner no se puede eliminar", "")}, nil
	case err != nil:
		return DeleteRbacRole400ApplicationProblemPlusJSONResponse{badRequest("role_delete_failed", "No se pudo eliminar el rol", "")}, nil
	}
	return DeleteRbacRole204Response{}, nil
}

// GetUsersMembers lists staff memberships in the current clinic.
func (s *Server) GetUsersMembers(ctx context.Context, _ GetUsersMembersRequestObject) (GetUsersMembersResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return GetUsersMembers403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if err := rbac.Authorize(sess.Grants, rbac.PermUsersManage, rbac.ScopeAll); err != nil {
		return GetUsersMembers403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", "Requiere users.manage.")}, nil
	}

	memberships, err := s.auth.Users.ListMembershipsByClinic(ctx, sess.ClinicID)
	if err != nil {
		return GetUsersMembers403ApplicationProblemPlusJSONResponse{forbidden("members_load_failed", "No se pudieron listar los miembros", "")}, nil
	}
	roles, err := s.auth.Roles.ListByClinic(ctx, sess.ClinicID)
	if err != nil {
		return GetUsersMembers403ApplicationProblemPlusJSONResponse{forbidden("roles_load_failed", "No se pudieron listar los roles", "")}, nil
	}
	roleNames := make(map[bson.ObjectID]string, len(roles))
	for _, r := range roles {
		roleNames[r.ID] = r.Name
	}

	out := make([]MemberSummary, 0, len(memberships))
	for _, m := range memberships {
		u, uErr := s.auth.Users.UserByID(ctx, m.UserID)
		if uErr != nil {
			continue
		}
		out = append(out, MemberSummary{
			UserId:           u.ID.Hex(),
			Email:            u.Email,
			Name:             u.Name,
			UserStatus:       MemberSummaryUserStatus(u.Status),
			MembershipStatus: MemberSummaryMembershipStatus(m.Status),
			RoleId:           m.RoleID.Hex(),
			RoleName:         roleNames[m.RoleID],
		})
	}
	return GetUsersMembers200JSONResponse{Members: out}, nil
}

// PostUsersInvitations creates a single-use staff invitation scoped to the current clinic.
func (s *Server) PostUsersInvitations(ctx context.Context, req PostUsersInvitationsRequestObject) (PostUsersInvitationsResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return PostUsersInvitations403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if err := rbac.Authorize(sess.Grants, rbac.PermUsersManage, rbac.ScopeAll); err != nil {
		return PostUsersInvitations403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", "Requiere users.manage.")}, nil
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Name) == "" || strings.TrimSpace(req.Body.RoleId) == "" {
		return PostUsersInvitations400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Datos incompletos", "")}, nil
	}
	roleID, err := parseObjectID(req.Body.RoleId)
	if err != nil {
		return PostUsersInvitations400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "Rol inválido", "")}, nil
	}
	role, err := s.auth.Roles.ByID(ctx, sess.ClinicID, roleID)
	if errors.Is(err, rbac.ErrRoleNotFound) {
		return PostUsersInvitations404ApplicationProblemPlusJSONResponse{notFound("role_not_found", "Rol no encontrado", "")}, nil
	}
	if err != nil {
		return PostUsersInvitations400ApplicationProblemPlusJSONResponse{badRequest("role_error", "No se pudo verificar el rol", "")}, nil
	}
	if err := rbac.ValidateEscalation(sess.Grants, role.Grants); err != nil {
		return PostUsersInvitations403ApplicationProblemPlusJSONResponse{forbidden("privilege_escalation", "Escalada de permisos denegada", "No puedes invitar con un rol que supera tus permisos.")}, nil
	}

	inv, rawToken, err := s.auth.Users.CreateInvitation(ctx, sess.ClinicID, string(req.Body.Email), req.Body.Name, role.ID, sess.UserID, s.auth.InvitationTTL)
	if err != nil {
		return PostUsersInvitations400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "No se pudo crear la invitación", "")}, nil
	}
	return PostUsersInvitations201JSONResponse{
		Id:        inv.ID.Hex(),
		Email:     inv.Email,
		Name:      inv.Name,
		RoleId:    inv.RoleID.Hex(),
		ExpiresAt: inv.ExpiresAt,
		Token:     rawToken,
	}, nil
}

// PutUsersMemberRole changes a clinic member's role while enforcing anti-escalation and last-owner protection.
func (s *Server) PutUsersMemberRole(ctx context.Context, req PutUsersMemberRoleRequestObject) (PutUsersMemberRoleResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return PutUsersMemberRole403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if err := rbac.Authorize(sess.Grants, rbac.PermUsersManage, rbac.ScopeAll); err != nil {
		return PutUsersMemberRole403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", "Requiere users.manage.")}, nil
	}
	if req.Body == nil || strings.TrimSpace(req.Body.RoleId) == "" {
		return PutUsersMemberRole400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "roleId requerido", "")}, nil
	}
	targetUserID, err := parseObjectID(req.UserId)
	if err != nil {
		return PutUsersMemberRole400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "userId inválido", "")}, nil
	}
	targetRoleID, err := parseObjectID(req.Body.RoleId)
	if err != nil {
		return PutUsersMemberRole400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "roleId inválido", "")}, nil
	}

	targetRole, err := s.auth.Roles.ByID(ctx, sess.ClinicID, targetRoleID)
	if errors.Is(err, rbac.ErrRoleNotFound) {
		return PutUsersMemberRole404ApplicationProblemPlusJSONResponse{notFound("role_not_found", "Rol no encontrado", "")}, nil
	}
	if err != nil {
		return PutUsersMemberRole400ApplicationProblemPlusJSONResponse{badRequest("role_error", "No se pudo verificar el rol", "")}, nil
	}
	if err := rbac.ValidateEscalation(sess.Grants, targetRole.Grants); err != nil {
		return PutUsersMemberRole403ApplicationProblemPlusJSONResponse{forbidden("privilege_escalation", "Escalada de permisos denegada", "No puedes asignar un rol con permisos que no posees.")}, nil
	}

	currentMembership, err := s.auth.Users.MembershipByUserAndClinic(ctx, targetUserID, sess.ClinicID)
	if errors.Is(err, users.ErrMembershipNotFound) {
		return PutUsersMemberRole404ApplicationProblemPlusJSONResponse{notFound("membership_not_found", "Membresía no encontrada", "")}, nil
	}
	if err != nil {
		return PutUsersMemberRole400ApplicationProblemPlusJSONResponse{badRequest("membership_error", "No se pudo consultar la membresía", "")}, nil
	}
	if currentRole, rErr := s.auth.Roles.ByID(ctx, sess.ClinicID, currentMembership.RoleID); rErr == nil {
		if escErr := rbac.ValidateEscalation(sess.Grants, currentRole.Grants); escErr != nil {
			return PutUsersMemberRole403ApplicationProblemPlusJSONResponse{forbidden("privilege_escalation", "No puedes modificar el rol de alguien con más permisos", "")}, nil
		}
	}

	ownerRole, err := s.auth.Roles.ByName(ctx, sess.ClinicID, rbac.RoleOwner)
	if err != nil {
		return PutUsersMemberRole400ApplicationProblemPlusJSONResponse{badRequest("owner_role_missing", "No se encontró el rol owner", "")}, nil
	}

	updated, err := s.auth.Users.UpdateMembershipRole(ctx, sess.ClinicID, targetUserID, targetRole.ID, ownerRole.ID)
	if errors.Is(err, users.ErrLastOwner) {
		return PutUsersMemberRole409ApplicationProblemPlusJSONResponse{conflict("last_owner", "No se puede degradar al último owner de la clínica", "")}, nil
	}
	if err != nil {
		return PutUsersMemberRole400ApplicationProblemPlusJSONResponse{badRequest("membership_update_failed", "No se pudo actualizar el rol del miembro", "")}, nil
	}

	_ = s.auth.Sessions.RevokeByUser(ctx, targetUserID)

	u, err := s.auth.Users.UserByID(ctx, targetUserID)
	if err != nil {
		return PutUsersMemberRole404ApplicationProblemPlusJSONResponse{notFound("user_not_found", "Usuario no encontrado", "")}, nil
	}
	return PutUsersMemberRole200JSONResponse{
		UserId:           u.ID.Hex(),
		Email:            u.Email,
		Name:             u.Name,
		UserStatus:       MemberSummaryUserStatus(u.Status),
		MembershipStatus: MemberSummaryMembershipStatus(updated.Status),
		RoleId:           updated.RoleID.Hex(),
		RoleName:         targetRole.Name,
	}, nil
}

// DeleteUsersMember suspends a clinic member and revokes their active sessions.
func (s *Server) DeleteUsersMember(ctx context.Context, req DeleteUsersMemberRequestObject) (DeleteUsersMemberResponseObject, error) {
	sess, err := s.requireStaff(ctx)
	if err != nil {
		return DeleteUsersMember403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Acceso restringido", "")}, nil
	}
	if err := rbac.Authorize(sess.Grants, rbac.PermUsersManage, rbac.ScopeAll); err != nil {
		return DeleteUsersMember403ApplicationProblemPlusJSONResponse{forbidden("forbidden", "Permiso insuficiente", "Requiere users.manage.")}, nil
	}
	targetUserID, err := parseObjectID(req.UserId)
	if err != nil {
		return DeleteUsersMember400ApplicationProblemPlusJSONResponse{badRequest("validation_failed", "userId inválido", "")}, nil
	}

	currentMembership, err := s.auth.Users.MembershipByUserAndClinic(ctx, targetUserID, sess.ClinicID)
	if errors.Is(err, users.ErrMembershipNotFound) {
		return DeleteUsersMember404ApplicationProblemPlusJSONResponse{notFound("membership_not_found", "Membresía no encontrada", "")}, nil
	}
	if err != nil {
		return DeleteUsersMember400ApplicationProblemPlusJSONResponse{badRequest("membership_error", "No se pudo consultar la membresía", "")}, nil
	}
	if currentRole, rErr := s.auth.Roles.ByID(ctx, sess.ClinicID, currentMembership.RoleID); rErr == nil {
		if escErr := rbac.ValidateEscalation(sess.Grants, currentRole.Grants); escErr != nil {
			return DeleteUsersMember403ApplicationProblemPlusJSONResponse{forbidden("privilege_escalation", "No puedes suspender a alguien con más permisos", "")}, nil
		}
	}

	ownerRole, err := s.auth.Roles.ByName(ctx, sess.ClinicID, rbac.RoleOwner)
	if err != nil {
		return DeleteUsersMember400ApplicationProblemPlusJSONResponse{badRequest("owner_role_missing", "No se encontró el rol owner", "")}, nil
	}

	err = s.auth.Users.RevokeMembership(ctx, sess.ClinicID, targetUserID, ownerRole.ID)
	if errors.Is(err, users.ErrLastOwner) {
		return DeleteUsersMember409ApplicationProblemPlusJSONResponse{conflict("last_owner", "No se puede suspender al último owner de la clínica", "")}, nil
	}
	if err != nil {
		return DeleteUsersMember400ApplicationProblemPlusJSONResponse{badRequest("membership_revoke_failed", "No se pudo suspender al miembro", "")}, nil
	}
	_ = s.auth.Sessions.RevokeByUser(ctx, targetUserID)
	return DeleteUsersMember204Response{}, nil
}
