"use client";

import Link from "next/link";
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import {
  Alert,
  Badge,
  Button,
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
  Field,
  Input,
} from "@/components/ui";
import {
  createBrowserApiClient,
  problemMessage,
  type GrantDTO,
  type MemberSummaryDTO,
  type PermissionDTO,
  type RoleDTO,
  type ScopeDTO,
  type SessionInfo,
} from "@/lib/api/client";
import { AppShell } from "./app-shell";

const permissionLabels: Record<PermissionDTO, string> = {
  "appointments.read": "Ver citas",
  "appointments.write": "Crear y mover citas",
  "schedule.manage": "Horarios y bloqueos",
  "patients.read": "Ver fichas de pacientes",
  "patients.write": "Editar pacientes",
  "patients.export": "Exportar datos de pacientes",
  "patients.delete": "Suprimir datos de pacientes",
  "payments.read": "Ver pagos",
  "payments.refund": "Emitir reembolsos",
  "users.manage": "Invitar y gestionar personal",
  "roles.manage": "Configurar roles y permisos",
  "clinic.settings": "Ajustes de la clínica",
};

type ScopeSelection = "none" | ScopeDTO;

function hasPermission(
  grants: GrantDTO[] | undefined,
  perm: PermissionDTO,
): boolean {
  return Boolean(grants?.some((g) => g.permission === perm));
}

function grantsToRecord(
  grants: GrantDTO[],
): Partial<Record<PermissionDTO, ScopeSelection>> {
  const map: Partial<Record<PermissionDTO, ScopeSelection>> = {};
  for (const g of grants) {
    map[g.permission] = g.scope;
  }
  return map;
}

function recordToGrants(
  map: Partial<Record<PermissionDTO, ScopeSelection>>,
  catalog: PermissionDTO[],
): GrantDTO[] {
  const out: GrantDTO[] = [];
  for (const perm of catalog) {
    const scope = map[perm];
    if (scope === "own" || scope === "all") {
      out.push({ permission: perm, scope });
    }
  }
  return out;
}

export function RolesPanel() {
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [catalog, setCatalog] = useState<PermissionDTO[]>([]);
  const [roles, setRoles] = useState<RoleDTO[]>([]);
  const [members, setMembers] = useState<MemberSummaryDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  // Role editor state
  const [editingRoleId, setEditingRoleId] = useState<string | null>(null);
  const [roleName, setRoleName] = useState("");
  const [grantMap, setGrantMap] = useState<
    Partial<Record<PermissionDTO, ScopeSelection>>
  >({});
  const [savingRole, setSavingRole] = useState(false);

  // Invitation form state
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteName, setInviteName] = useState("");
  const [inviteRoleId, setInviteRoleId] = useState("");
  const [inviting, setInviting] = useState(false);
  const [createdToken, setCreatedToken] = useState("");

  const canManageRoles = hasPermission(session?.grants, "roles.manage");
  const canManageUsers = hasPermission(session?.grants, "users.manage");

  const actorHeldMap = useMemo(
    () => grantsToRecord(session?.grants ?? []),
    [session?.grants],
  );

  const fetchRolesState = useCallback(async () => {
    const api = createBrowserApiClient();
    const { data: sessData } = await api.GET("/auth/session");
    if (!sessData || sessData.kind !== "staff") {
      return {
        session: null,
        catalog: [] as PermissionDTO[],
        roles: [] as RoleDTO[],
        members: [] as MemberSummaryDTO[],
        error: "",
      };
    }

    let nextCatalog: PermissionDTO[] = [];
    const { data: permData } = await api.GET("/rbac/permissions");
    if (permData) {
      nextCatalog = permData.permissions;
    }

    let nextRoles: RoleDTO[] = [];
    let rolesErrMsg = "";
    const mayReadRoles =
      hasPermission(sessData.grants, "roles.manage") ||
      hasPermission(sessData.grants, "users.manage");
    if (mayReadRoles) {
      const { data: rolesData, error: rolesErr } = await api.GET("/rbac/roles");
      if (rolesErr) {
        rolesErrMsg = problemMessage(
          rolesErr,
          "No se pudieron cargar los roles.",
        );
      } else if (rolesData) {
        nextRoles = rolesData.roles;
      }
    }

    let nextMembers: MemberSummaryDTO[] = [];
    if (hasPermission(sessData.grants, "users.manage")) {
      const { data: membersData } = await api.GET("/users/members");
      if (membersData) {
        nextMembers = membersData.members;
      }
    }

    return {
      session: sessData,
      catalog: nextCatalog,
      roles: nextRoles,
      members: nextMembers,
      error: rolesErrMsg,
    };
  }, []);

  const applyRolesState = useCallback(
    (res: Awaited<ReturnType<typeof fetchRolesState>>) => {
      setSession(res.session);
      setCatalog(res.catalog);
      setRoles(res.roles);
      setMembers(res.members);
      if (res.error) {
        setError(res.error);
      }
      if (res.roles.length > 0) {
        setInviteRoleId((prev) => {
          if (prev) {
            return prev;
          }
          const nonOwner = res.roles.find((r) => !r.locked);
          return nonOwner?.id ?? res.roles[0].id;
        });
      }
    },
    [],
  );

  const loadData = useCallback(async () => {
    setError("");
    try {
      const res = await fetchRolesState();
      applyRolesState(res);
    } catch {
      setError("Error de red al cargar roles y personal.");
    } finally {
      setLoading(false);
    }
  }, [fetchRolesState, applyRolesState]);

  useEffect(() => {
    let active = true;
    void fetchRolesState()
      .then((res) => {
        if (active) {
          applyRolesState(res);
        }
      })
      .catch(() => {
        if (active) {
          setError("Error de red al cargar roles y personal.");
        }
      })
      .finally(() => {
        if (active) {
          setLoading(false);
        }
      });
    return () => {
      active = false;
    };
  }, [fetchRolesState, applyRolesState]);

  function startNewRole() {
    setEditingRoleId(null);
    setRoleName("");
    setGrantMap({});
    setError("");
  }

  function startEditRole(role: RoleDTO) {
    setEditingRoleId(role.id);
    setRoleName(role.name);
    setGrantMap(grantsToRecord(role.grants));
    setError("");
    setNotice("");
  }

  async function handleSaveRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session) {
      return;
    }
    setError("");
    setNotice("");
    const grants = recordToGrants(grantMap, catalog);
    if (!roleName.trim()) {
      setError("Indica un nombre para el rol.");
      return;
    }

    setSavingRole(true);
    try {
      const api = createBrowserApiClient();
      if (editingRoleId) {
        const { error: updErr } = await api.PUT("/rbac/roles/{roleId}", {
          params: { path: { roleId: editingRoleId } },
          headers: { "X-CSRF-Token": session.csrfToken },
          body: { name: roleName.trim(), grants },
        });
        if (updErr) {
          setError(problemMessage(updErr, "No se pudo actualizar el rol."));
          return;
        }
        setNotice("Rol actualizado y sesiones activas sincronizadas.");
      } else {
        const { error: createErr } = await api.POST("/rbac/roles", {
          headers: { "X-CSRF-Token": session.csrfToken },
          body: { name: roleName.trim(), grants },
        });
        if (createErr) {
          setError(problemMessage(createErr, "No se pudo crear el rol."));
          return;
        }
        setNotice("Rol personalizado creado.");
      }
      startNewRole();
      await loadData();
    } finally {
      setSavingRole(false);
    }
  }

  async function handleDeleteRole(roleId: string) {
    if (!session) {
      return;
    }
    setError("");
    setNotice("");
    const api = createBrowserApiClient();
    const { error: delErr } = await api.DELETE("/rbac/roles/{roleId}", {
      params: { path: { roleId } },
      headers: { "X-CSRF-Token": session.csrfToken },
    });
    if (delErr) {
      setError(problemMessage(delErr, "No se pudo eliminar el rol."));
      return;
    }
    if (editingRoleId === roleId) {
      startNewRole();
    }
    setNotice("Rol eliminado.");
    await loadData();
  }

  async function handleInviteMember(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session || !inviteRoleId) {
      return;
    }
    setError("");
    setNotice("");
    setCreatedToken("");
    setInviting(true);
    try {
      const api = createBrowserApiClient();
      const { data, error: invErr } = await api.POST("/users/invitations", {
        headers: { "X-CSRF-Token": session.csrfToken },
        body: {
          email: inviteEmail.trim().toLowerCase(),
          name: inviteName.trim(),
          roleId: inviteRoleId,
        },
      });
      if (invErr || !data) {
        setError(problemMessage(invErr, "No se pudo crear la invitación."));
        return;
      }
      setCreatedToken(data.token);
      setInviteEmail("");
      setInviteName("");
      setNotice(`Invitación generada para ${data.email}.`);
    } finally {
      setInviting(false);
    }
  }

  async function handleChangeMemberRole(userId: string, newRoleId: string) {
    if (!session) {
      return;
    }
    setError("");
    setNotice("");
    const api = createBrowserApiClient();
    const { error: updErr } = await api.PUT("/users/members/{userId}/role", {
      params: { path: { userId } },
      headers: { "X-CSRF-Token": session.csrfToken },
      body: { roleId: newRoleId },
    });
    if (updErr) {
      setError(
        problemMessage(updErr, "No se pudo cambiar el rol del miembro."),
      );
      return;
    }
    setNotice("Rol del miembro actualizado.");
    await loadData();
  }

  async function handleRevokeMember(userId: string) {
    if (!session) {
      return;
    }
    setError("");
    setNotice("");
    const api = createBrowserApiClient();
    const { error: delErr } = await api.DELETE("/users/members/{userId}", {
      params: { path: { userId } },
      headers: { "X-CSRF-Token": session.csrfToken },
    });
    if (delErr) {
      setError(problemMessage(delErr, "No se pudo suspender al miembro."));
      return;
    }
    setNotice("Membresía suspendida y sesiones revocadas.");
    await loadData();
  }

  return (
    <AppShell session={session} onSessionCleared={() => setSession(null)}>
      <div className="flex flex-col gap-6">
        <div>
          <h1 className="text-[var(--text-2xl)] font-semibold leading-[var(--leading-tight)]">
            Roles y equipo de la clínica
          </h1>
          <p className="text-[var(--text-sm)] text-[var(--text-secondary)]">
            Permisos con alcance propio o de toda la clínica, protección de
            escalada y gestión de miembros.
          </p>
        </div>

        {loading ? (
          <Card>
            <p className="text-[var(--text-sm)] text-[var(--text-secondary)]">
              Cargando roles y membresías...
            </p>
          </Card>
        ) : !session ? (
          <Card>
            <CardHeader>
              <CardTitle>Sesión requerida</CardTitle>
              <CardDescription>
                Inicia sesión para gestionar roles y personal.
              </CardDescription>
            </CardHeader>
            <Link
              href="/login"
              className="inline-flex min-h-11 items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent)] px-4 font-medium text-[var(--accent-text)] hover:bg-[var(--accent-hover)]"
            >
              Ir a iniciar sesión
            </Link>
          </Card>
        ) : !canManageRoles && !canManageUsers ? (
          <Alert tone="warning" title="Permisos insuficientes">
            Tu rol actual ({session.roleName}) no incluye{" "}
            <code>roles.manage</code> ni <code>users.manage</code>.
          </Alert>
        ) : (
          <>
            {error ? <Alert tone="danger">{error}</Alert> : null}
            {notice ? <Alert tone="success">{notice}</Alert> : null}

            <Card>
              <CardHeader>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <CardTitle>Roles de la clínica</CardTitle>
                  {canManageRoles && editingRoleId ? (
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={startNewRole}
                    >
                      Nuevo rol
                    </Button>
                  ) : null}
                </div>
                <CardDescription>
                  <code>owner</code> conserva todos los permisos bloqueados; los
                  demás roles pueden adaptarse sin superar tus propios permisos.
                </CardDescription>
              </CardHeader>

              <ul className="flex flex-col gap-3">
                {roles.map((r) => (
                  <li
                    key={r.id}
                    className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-subtle)] px-4 py-3"
                  >
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-semibold text-[var(--text-primary)]">
                          {r.name}
                        </span>
                        {r.template ? (
                          <Badge tone="neutral">Plantilla</Badge>
                        ) : null}
                        {r.locked ? (
                          <Badge tone="warning">Bloqueado</Badge>
                        ) : null}
                      </div>
                      <p className="mt-1 text-[var(--text-xs)] text-[var(--text-secondary)]">
                        {r.grants.length} permisos asignados
                      </p>
                    </div>

                    {canManageRoles && !r.locked ? (
                      <div className="flex flex-wrap items-center gap-2">
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() => startEditRole(r)}
                        >
                          Editar
                        </Button>
                        <Dialog>
                          <DialogTrigger asChild>
                            <Button variant="danger" size="sm">
                              Eliminar
                            </Button>
                          </DialogTrigger>
                          <DialogContent>
                            <DialogTitle>Eliminar rol {r.name}</DialogTitle>
                            <DialogDescription>
                              Esta acción elimina el rol personalizado si no
                              tiene miembros activos asignados.
                            </DialogDescription>
                            <div className="mt-6 flex justify-end gap-2">
                              <DialogClose asChild>
                                <Button variant="secondary">Cancelar</Button>
                              </DialogClose>
                              <DialogClose asChild>
                                <Button
                                  variant="danger"
                                  onClick={() => handleDeleteRole(r.id)}
                                >
                                  Confirmar eliminación
                                </Button>
                              </DialogClose>
                            </div>
                          </DialogContent>
                        </Dialog>
                      </div>
                    ) : null}
                  </li>
                ))}
              </ul>
            </Card>

            {canManageRoles ? (
              <Card>
                <CardHeader>
                  <CardTitle>
                    {editingRoleId
                      ? `Editar rol: ${roleName}`
                      : "Crear rol personalizado"}
                  </CardTitle>
                  <CardDescription>
                    Solo puedes otorgar permisos y alcances (<code>own</code> o{" "}
                    <code>all</code>) que tu propio rol posee.
                  </CardDescription>
                </CardHeader>

                <form onSubmit={handleSaveRole} className="flex flex-col gap-5">
                  <Field label="Nombre del rol" required>
                    <Input
                      name="roleName"
                      type="text"
                      value={roleName}
                      onChange={(e) => setRoleName(e.target.value)}
                      required
                    />
                  </Field>

                  <div className="overflow-x-auto">
                    <table className="w-full border-collapse text-left text-[var(--text-sm)]">
                      <thead>
                        <tr className="border-b border-[var(--border-default)] text-[var(--text-secondary)]">
                          <th className="py-2 pr-4 font-semibold">Permiso</th>
                          <th className="py-2 pr-4 font-semibold">Alcance</th>
                        </tr>
                      </thead>
                      <tbody>
                        {catalog.map((perm) => {
                          const current = grantMap[perm] ?? "none";
                          const held = actorHeldMap[perm];
                          return (
                            <tr
                              key={perm}
                              className="border-b border-[var(--border-default)]"
                            >
                              <td className="py-3 pr-4">
                                <div className="font-medium text-[var(--text-primary)]">
                                  {permissionLabels[perm]}
                                </div>
                                <div className="font-mono text-[var(--text-xs)] text-[var(--text-muted)]">
                                  {perm}
                                </div>
                              </td>
                              <td className="py-3">
                                <select
                                  aria-label={`Alcance para ${perm}`}
                                  value={current}
                                  disabled={!held}
                                  onChange={(e) =>
                                    setGrantMap((prev) => ({
                                      ...prev,
                                      [perm]: e.target.value as ScopeSelection,
                                    }))
                                  }
                                  className="min-h-10 rounded-[var(--radius-md)] border border-[var(--border-strong)] bg-[var(--bg-surface)] px-3 py-1.5 text-[var(--text-sm)] text-[var(--text-primary)] disabled:opacity-50"
                                >
                                  <option value="none">Sin acceso</option>
                                  <option value="own">Solo propio (own)</option>
                                  {held === "all" ? (
                                    <option value="all">
                                      Toda la clínica (all)
                                    </option>
                                  ) : null}
                                </select>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>

                  <div className="flex flex-wrap gap-3">
                    <Button type="submit" loading={savingRole}>
                      {editingRoleId ? "Guardar cambios" : "Crear rol"}
                    </Button>
                    {editingRoleId ? (
                      <Button variant="secondary" onClick={startNewRole}>
                        Cancelar edición
                      </Button>
                    ) : null}
                  </div>
                </form>
              </Card>
            ) : null}

            {canManageUsers ? (
              <Card>
                <CardHeader>
                  <CardTitle>Personal e invitaciones</CardTitle>
                  <CardDescription>
                    Invita nuevas personas con enlace de un solo uso o ajusta
                    sus roles actuales.
                  </CardDescription>
                </CardHeader>

                <div className="flex flex-col gap-6">
                  <form
                    onSubmit={handleInviteMember}
                    className="grid gap-4 rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-subtle)] p-4 sm:grid-cols-3"
                  >
                    <Field label="Nombre completo" required>
                      <Input
                        name="inviteName"
                        type="text"
                        value={inviteName}
                        onChange={(e) => setInviteName(e.target.value)}
                        required
                      />
                    </Field>
                    <Field label="Correo electrónico" required>
                      <Input
                        name="inviteEmail"
                        type="email"
                        spellCheck={false}
                        value={inviteEmail}
                        onChange={(e) => setInviteEmail(e.target.value)}
                        required
                      />
                    </Field>
                    <div className="flex flex-col gap-[var(--space-1)]">
                      <label
                        htmlFor="invite-role-select"
                        className="text-[var(--text-sm)] font-medium text-[var(--text-primary)]"
                      >
                        Rol asignado
                      </label>
                      <select
                        id="invite-role-select"
                        value={inviteRoleId}
                        onChange={(e) => setInviteRoleId(e.target.value)}
                        className="min-h-11 rounded-[var(--radius-md)] border border-[var(--border-strong)] bg-[var(--bg-surface)] px-3 py-2 text-[var(--text-base)] text-[var(--text-primary)]"
                      >
                        {roles.map((r) => (
                          <option key={r.id} value={r.id}>
                            {r.name}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="sm:col-span-3">
                      <Button type="submit" loading={inviting}>
                        Generar invitación
                      </Button>
                    </div>
                  </form>

                  {createdToken ? (
                    <Alert
                      tone="info"
                      title="Enlace de activación de un solo uso"
                    >
                      Comparte este enlace con la persona invitada:
                      <p className="mt-1 break-all font-mono text-[var(--text-xs)] select-all">
                        /invitacion?token={createdToken}
                      </p>
                    </Alert>
                  ) : null}

                  <ul className="flex flex-col gap-3">
                    {members.map((m) => (
                      <li
                        key={m.userId}
                        className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] px-4 py-3"
                      >
                        <div>
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-semibold text-[var(--text-primary)]">
                              {m.name}
                            </span>
                            <Badge
                              tone={
                                m.membershipStatus === "active"
                                  ? "success"
                                  : "danger"
                              }
                            >
                              {m.membershipStatus === "active"
                                ? "Activo"
                                : "Suspendido"}
                            </Badge>
                          </div>
                          <p className="text-[var(--text-xs)] text-[var(--text-secondary)]">
                            {m.email}
                          </p>
                        </div>

                        <div className="flex flex-wrap items-center gap-2">
                          <select
                            aria-label={`Rol de ${m.name}`}
                            value={m.roleId}
                            disabled={m.membershipStatus !== "active"}
                            onChange={(e) =>
                              void handleChangeMemberRole(
                                m.userId,
                                e.target.value,
                              )
                            }
                            className="min-h-10 rounded-[var(--radius-md)] border border-[var(--border-strong)] bg-[var(--bg-surface)] px-3 py-1.5 text-[var(--text-sm)]"
                          >
                            {roles.map((r) => (
                              <option key={r.id} value={r.id}>
                                {r.name}
                              </option>
                            ))}
                          </select>

                          {m.membershipStatus === "active" ? (
                            <Dialog>
                              <DialogTrigger asChild>
                                <Button variant="danger" size="sm">
                                  Suspender
                                </Button>
                              </DialogTrigger>
                              <DialogContent>
                                <DialogTitle>
                                  Suspender acceso de {m.name}
                                </DialogTitle>
                                <DialogDescription>
                                  Se revocarán inmediatamente sus sesiones en
                                  esta clínica. No se permite suspender al
                                  último owner.
                                </DialogDescription>
                                <div className="mt-6 flex justify-end gap-2">
                                  <DialogClose asChild>
                                    <Button variant="secondary">
                                      Cancelar
                                    </Button>
                                  </DialogClose>
                                  <DialogClose asChild>
                                    <Button
                                      variant="danger"
                                      onClick={() =>
                                        handleRevokeMember(m.userId)
                                      }
                                    >
                                      Confirmar suspensión
                                    </Button>
                                  </DialogClose>
                                </div>
                              </DialogContent>
                            </Dialog>
                          ) : null}
                        </div>
                      </li>
                    ))}
                  </ul>
                </div>
              </Card>
            ) : null}
          </>
        )}
      </div>
    </AppShell>
  );
}
