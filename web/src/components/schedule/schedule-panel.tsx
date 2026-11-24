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
  Field,
  Input,
} from "@/components/ui";
import { AppShell } from "@/components/auth/app-shell";
import {
  createBrowserApiClient,
  problemMessage,
  type AvailabilityRuleDTO,
  type AvailableSlotDTO,
  type BlockDTO,
  type BlockScopeDTO,
  type PractitionerDTO,
  type RoomDTO,
  type ServiceDTO,
  type SessionInfo,
} from "@/lib/api/client";
import {
  WEEKDAY_LABELS,
  formatMinutesToTime,
  formatPanamaDateTime,
  formatPanamaTime,
  panamaDayRangeISO,
  parseTimeToMinutes,
} from "./schedule-utils";

export function SchedulePanel() {
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [practitioners, setPractitioners] = useState<PractitionerDTO[]>([]);
  const [rooms, setRooms] = useState<RoomDTO[]>([]);
  const [services, setServices] = useState<ServiceDTO[]>([]);
  const [rules, setRules] = useState<AvailabilityRuleDTO[]>([]);
  const [blocks, setBlocks] = useState<BlockDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  // Rule form state
  const [rulePracId, setRulePracId] = useState("");
  const [ruleRoomId, setRuleRoomId] = useState("");
  const [ruleWeekday, setRuleWeekday] = useState("1");
  const [ruleStart, setRuleStart] = useState("08:00");
  const [ruleEnd, setRuleEnd] = useState("17:00");

  // Block form state
  const [blockScope, setBlockScope] = useState<BlockScopeDTO>("clinic");
  const [blockPracId, setBlockPracId] = useState("");
  const [blockRoomId, setBlockRoomId] = useState("");
  const [blockReason, setBlockReason] = useState("");
  const [blockStartLocal, setBlockStartLocal] = useState("2026-11-25T12:00");
  const [blockEndLocal, setBlockEndLocal] = useState("2026-11-25T13:00");

  // Availability preview state
  const [availServiceId, setAvailServiceId] = useState("");
  const [availPracId, setAvailPracId] = useState("");
  const [availDate, setAvailDate] = useState("2026-11-25");
  const [slots, setSlots] = useState<AvailableSlotDTO[]>([]);
  const [checkingAvail, setCheckingAvail] = useState(false);

  const practitionerNameById = useMemo(
    () => new Map(practitioners.map((p) => [p.id, p.name])),
    [practitioners],
  );
  const roomNameById = useMemo(
    () => new Map(rooms.map((r) => [r.id, r.name])),
    [rooms],
  );

  const loadAll = useCallback(async () => {
    const api = createBrowserApiClient();
    try {
      const sessRes = await api.GET("/auth/session");
      if (!sessRes.data) {
        setSession(null);
        return;
      }
      setSession(sessRes.data);

      const now = new Date();
      const in30Days = new Date(now.getTime() + 30 * 24 * 60 * 60 * 1000);

      const [pracRes, roomRes, svcRes, rulesRes, blocksRes] = await Promise.all(
        [
          api.GET("/schedule/practitioners"),
          api.GET("/schedule/rooms"),
          api.GET("/schedule/services"),
          api.GET("/schedule/rules"),
          api.GET("/schedule/blocks", {
            params: {
              query: { from: now.toISOString(), to: in30Days.toISOString() },
            },
          }),
        ],
      );

      if (
        pracRes.error ||
        roomRes.error ||
        svcRes.error ||
        rulesRes.error ||
        blocksRes.error
      ) {
        setError(
          problemMessage(
            pracRes.error ??
              roomRes.error ??
              svcRes.error ??
              rulesRes.error ??
              blocksRes.error,
            "No se pudieron cargar los horarios.",
          ),
        );
        return;
      }

      const pracList = pracRes.data?.practitioners ?? [];
      const roomList = roomRes.data?.rooms ?? [];
      const svcList = svcRes.data?.services ?? [];

      setError("");
      setPractitioners(pracList);
      setRooms(roomList);
      setServices(svcList);
      setRules(rulesRes.data?.rules ?? []);
      setBlocks(blocksRes.data?.blocks ?? []);

      if (pracList.length > 0) {
        setRulePracId((cur) => cur || pracList[0].id);
        setBlockPracId((cur) => cur || pracList[0].id);
      }
      if (roomList.length > 0) {
        setRuleRoomId((cur) => cur || roomList[0].id);
        setBlockRoomId((cur) => cur || roomList[0].id);
      }
      if (svcList.length > 0) {
        setAvailServiceId((cur) => cur || svcList[0].id);
      }
    } catch {
      setError("Error de conexión con el servidor.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => {
      void loadAll();
    }, 0);
    return () => clearTimeout(timer);
  }, [loadAll]);

  async function handleCreateRule(e: FormEvent) {
    e.preventDefault();
    if (!session) return;
    setError("");
    setNotice("");

    const startMinute = parseTimeToMinutes(ruleStart);
    const endMinute = parseTimeToMinutes(ruleEnd);
    if (
      startMinute === null ||
      endMinute === null ||
      endMinute <= startMinute
    ) {
      setError(
        "Las horas deben ser múltiplos de 5 minutos y la hora fin debe ser mayor.",
      );
      return;
    }

    setSaving(true);
    const api = createBrowserApiClient();
    try {
      const res = await api.POST("/schedule/rules", {
        headers: { "X-CSRF-Token": session.csrfToken },
        body: {
          practitionerId: rulePracId,
          roomId: ruleRoomId,
          weekday: Number(ruleWeekday),
          startMinute,
          endMinute,
          active: true,
        },
      });
      if (res.error) {
        setError(problemMessage(res.error, "No se pudo crear la regla."));
        return;
      }
      setNotice("Regla semanal agregada.");
      await loadAll();
    } finally {
      setSaving(false);
    }
  }

  async function handleDeleteRule(ruleId: string) {
    if (!session) return;
    setError("");
    setNotice("");
    const api = createBrowserApiClient();
    const res = await api.DELETE("/schedule/rules/{ruleId}", {
      params: { path: { ruleId } },
      headers: { "X-CSRF-Token": session.csrfToken },
    });
    if (res.error) {
      setError(problemMessage(res.error, "No se pudo eliminar la regla."));
      return;
    }
    setNotice("Regla eliminada.");
    await loadAll();
  }

  async function handleCreateBlock(e: FormEvent) {
    e.preventDefault();
    if (!session) return;
    setError("");
    setNotice("");

    const startAt = new Date(`${blockStartLocal}:00-05:00`);
    const endAt = new Date(`${blockEndLocal}:00-05:00`);
    if (
      Number.isNaN(startAt.getTime()) ||
      Number.isNaN(endAt.getTime()) ||
      endAt <= startAt
    ) {
      setError("El rango de bloqueo es inválido.");
      return;
    }

    setSaving(true);
    const api = createBrowserApiClient();
    try {
      const res = await api.POST("/schedule/blocks", {
        headers: { "X-CSRF-Token": session.csrfToken },
        body: {
          scope: blockScope,
          ...(blockScope === "practitioner"
            ? { practitionerId: blockPracId }
            : {}),
          ...(blockScope === "room" ? { roomId: blockRoomId } : {}),
          reason: blockReason,
          startAt: startAt.toISOString(),
          endAt: endAt.toISOString(),
        },
      });
      if (res.error) {
        setError(problemMessage(res.error, "No se pudo crear el bloqueo."));
        return;
      }
      setBlockReason("");
      setNotice("Bloqueo registrado.");
      await loadAll();
    } finally {
      setSaving(false);
    }
  }

  async function handleDeleteBlock(blockId: string) {
    if (!session) return;
    setError("");
    setNotice("");
    const api = createBrowserApiClient();
    const res = await api.DELETE("/schedule/blocks/{blockId}", {
      params: { path: { blockId } },
      headers: { "X-CSRF-Token": session.csrfToken },
    });
    if (res.error) {
      setError(problemMessage(res.error, "No se pudo eliminar el bloqueo."));
      return;
    }
    setNotice("Bloqueo eliminado.");
    await loadAll();
  }

  async function handleCheckAvailability(e: FormEvent) {
    e.preventDefault();
    if (!availServiceId) return;
    const range = panamaDayRangeISO(availDate);
    if (!range) {
      setError("Fecha inválida para consultar disponibilidad.");
      return;
    }
    setCheckingAvail(true);
    setError("");
    const api = createBrowserApiClient();
    try {
      const res = await api.GET("/schedule/availability", {
        params: {
          query: {
            serviceId: availServiceId,
            from: range.from,
            to: range.to,
            ...(availPracId ? { practitionerId: availPracId } : {}),
            stepMinutes: 30,
          },
        },
      });
      if (res.error) {
        setError(
          problemMessage(res.error, "No se pudo calcular la disponibilidad."),
        );
        return;
      }
      setSlots(res.data?.slots ?? []);
    } finally {
      setCheckingAvail(false);
    }
  }

  if (loading) {
    return (
      <AppShell session={session} onSessionCleared={() => setSession(null)}>
        <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
          Cargando horarios y bloqueos...
        </p>
      </AppShell>
    );
  }

  if (!session) {
    return (
      <AppShell session={null}>
        <Card className="max-w-lg">
          <CardHeader>
            <CardTitle>Sesión requerida</CardTitle>
            <CardDescription>
              Inicia sesión con tu cuenta del personal para gestionar horarios.
            </CardDescription>
          </CardHeader>
          <Link
            href="/login"
            className="inline-flex rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2 text-[length:var(--text-sm)] font-medium text-[var(--text-on-accent)]"
          >
            Ir a iniciar sesión
          </Link>
        </Card>
      </AppShell>
    );
  }

  return (
    <AppShell session={session} onSessionCleared={() => setSession(null)}>
      <div className="flex flex-col gap-6">
        <div>
          <h1 className="text-[length:var(--text-2xl)] font-bold tracking-tight">
            Horarios, bloqueos y disponibilidad
          </h1>
          <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
            Todas las horas se muestran en zona horaria de Panamá
            (America/Panama).
          </p>
        </div>

        <div aria-live="polite" className="flex flex-col gap-2">
          {error ? <Alert tone="danger">{error}</Alert> : null}
          {notice ? <Alert tone="success">{notice}</Alert> : null}
        </div>

        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Nueva regla semanal</CardTitle>
              <CardDescription>
                Define un turno recurrente por profesional y consultorio.
              </CardDescription>
            </CardHeader>
            <form onSubmit={handleCreateRule} className="flex flex-col gap-4">
              <Field label="Profesional" htmlFor="rule-prac">
                <select
                  id="rule-prac"
                  value={rulePracId}
                  onChange={(e) => setRulePracId(e.target.value)}
                  className="h-10 w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-[length:var(--text-sm)]"
                >
                  {practitioners.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Consultorio" htmlFor="rule-room">
                <select
                  id="rule-room"
                  value={ruleRoomId}
                  onChange={(e) => setRuleRoomId(e.target.value)}
                  className="h-10 w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-[length:var(--text-sm)]"
                >
                  {rooms.map((r) => (
                    <option key={r.id} value={r.id}>
                      {r.name}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Día de la semana" htmlFor="rule-weekday">
                <select
                  id="rule-weekday"
                  value={ruleWeekday}
                  onChange={(e) => setRuleWeekday(e.target.value)}
                  className="h-10 w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-[length:var(--text-sm)]"
                >
                  {WEEKDAY_LABELS.map((label, idx) => (
                    <option key={label} value={String(idx)}>
                      {label}
                    </option>
                  ))}
                </select>
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Inicio (HH:MM)" htmlFor="rule-start">
                  <Input
                    id="rule-start"
                    type="time"
                    step={300}
                    required
                    value={ruleStart}
                    onChange={(e) => setRuleStart(e.target.value)}
                  />
                </Field>
                <Field label="Fin (HH:MM)" htmlFor="rule-end">
                  <Input
                    id="rule-end"
                    type="time"
                    step={300}
                    required
                    value={ruleEnd}
                    onChange={(e) => setRuleEnd(e.target.value)}
                  />
                </Field>
              </div>
              <Button
                type="submit"
                loading={saving}
                disabled={practitioners.length === 0 || rooms.length === 0}
              >
                Agregar regla
              </Button>
            </form>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Reglas configuradas ({rules.length})</CardTitle>
            </CardHeader>
            {rules.length === 0 ? (
              <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
                No hay reglas de horario registradas.
              </p>
            ) : (
              <ul className="flex flex-col gap-3">
                {rules.map((r) => (
                  <li
                    key={r.id}
                    className="flex items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] p-3"
                  >
                    <div className="flex flex-col gap-1">
                      <span className="font-semibold">
                        {WEEKDAY_LABELS[r.weekday] ?? `Día ${r.weekday}`} ·{" "}
                        {formatMinutesToTime(r.startMinute)}–
                        {formatMinutesToTime(r.endMinute)}
                      </span>
                      <span className="text-[length:var(--text-xs)] text-[var(--text-secondary)]">
                        {practitionerNameById.get(r.practitionerId) ??
                          r.practitionerId}{" "}
                        · {roomNameById.get(r.roomId) ?? r.roomId}
                      </span>
                    </div>
                    <Button
                      type="button"
                      size="sm"
                      variant="danger"
                      onClick={() => handleDeleteRule(r.id)}
                    >
                      Eliminar
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>

        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Nuevo bloqueo puntual</CardTitle>
              <CardDescription>
                Cierra la clínica, un profesional o un consultorio en un rango
                horario.
              </CardDescription>
            </CardHeader>
            <form onSubmit={handleCreateBlock} className="flex flex-col gap-4">
              <Field label="Alcance" htmlFor="block-scope">
                <select
                  id="block-scope"
                  value={blockScope}
                  onChange={(e) =>
                    setBlockScope(e.target.value as BlockScopeDTO)
                  }
                  className="h-10 w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-[length:var(--text-sm)]"
                >
                  <option value="clinic">Toda la clínica</option>
                  <option value="practitioner">Profesional</option>
                  <option value="room">Consultorio</option>
                </select>
              </Field>
              {blockScope === "practitioner" ? (
                <Field label="Profesional" htmlFor="block-prac">
                  <select
                    id="block-prac"
                    value={blockPracId}
                    onChange={(e) => setBlockPracId(e.target.value)}
                    className="h-10 w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-[length:var(--text-sm)]"
                  >
                    {practitioners.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                </Field>
              ) : null}
              {blockScope === "room" ? (
                <Field label="Consultorio" htmlFor="block-room">
                  <select
                    id="block-room"
                    value={blockRoomId}
                    onChange={(e) => setBlockRoomId(e.target.value)}
                    className="h-10 w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-[length:var(--text-sm)]"
                  >
                    {rooms.map((r) => (
                      <option key={r.id} value={r.id}>
                        {r.name}
                      </option>
                    ))}
                  </select>
                </Field>
              ) : null}
              <Field label="Motivo" htmlFor="block-reason">
                <Input
                  id="block-reason"
                  required
                  value={blockReason}
                  onChange={(e) => setBlockReason(e.target.value)}
                  placeholder="Mantenimiento o capacitación"
                />
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Desde" htmlFor="block-start">
                  <Input
                    id="block-start"
                    type="datetime-local"
                    required
                    value={blockStartLocal}
                    onChange={(e) => setBlockStartLocal(e.target.value)}
                  />
                </Field>
                <Field label="Hasta" htmlFor="block-end">
                  <Input
                    id="block-end"
                    type="datetime-local"
                    required
                    value={blockEndLocal}
                    onChange={(e) => setBlockEndLocal(e.target.value)}
                  />
                </Field>
              </div>
              <Button type="submit" loading={saving}>
                Crear bloqueo
              </Button>
            </form>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Bloqueos próximos 30 días ({blocks.length})</CardTitle>
            </CardHeader>
            {blocks.length === 0 ? (
              <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
                Sin bloqueos en los próximos 30 días.
              </p>
            ) : (
              <ul className="flex flex-col gap-3">
                {blocks.map((b) => (
                  <li
                    key={b.id}
                    className="flex items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] p-3"
                  >
                    <div className="flex flex-col gap-1">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold">{b.reason}</span>
                        <Badge
                          tone={b.source === "holiday" ? "info" : "neutral"}
                        >
                          {b.source === "holiday" ? "Feriado" : b.scope}
                        </Badge>
                      </div>
                      <span className="text-[length:var(--text-xs)] text-[var(--text-secondary)]">
                        {formatPanamaDateTime(b.startAt)} →{" "}
                        {formatPanamaDateTime(b.endAt)}
                      </span>
                    </div>
                    <Button
                      type="button"
                      size="sm"
                      variant="danger"
                      onClick={() => handleDeleteBlock(b.id)}
                    >
                      Eliminar
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Vista previa de disponibilidad</CardTitle>
            <CardDescription>
              Consulta los espacios calculados por el motor para un servicio y
              fecha.
            </CardDescription>
          </CardHeader>
          <form
            onSubmit={handleCheckAvailability}
            className="grid gap-4 md:grid-cols-4"
          >
            <Field label="Servicio" htmlFor="avail-service">
              <select
                id="avail-service"
                value={availServiceId}
                onChange={(e) => setAvailServiceId(e.target.value)}
                className="h-10 w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-[length:var(--text-sm)]"
              >
                {services.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Profesional (opcional)" htmlFor="avail-prac">
              <select
                id="avail-prac"
                value={availPracId}
                onChange={(e) => setAvailPracId(e.target.value)}
                className="h-10 w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-[length:var(--text-sm)]"
              >
                <option value="">Todos</option>
                {practitioners.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Fecha (Panamá)" htmlFor="avail-date">
              <Input
                id="avail-date"
                type="date"
                required
                value={availDate}
                onChange={(e) => setAvailDate(e.target.value)}
              />
            </Field>
            <div className="flex items-end">
              <Button
                type="submit"
                loading={checkingAvail}
                disabled={!availServiceId}
              >
                Consultar horarios
              </Button>
            </div>
          </form>

          {slots.length > 0 ? (
            <ul className="mt-4 flex flex-wrap gap-2">
              {slots.map((sl) => (
                <li
                  key={`${sl.startAt}-${sl.practitionerId}-${sl.roomId}`}
                  className="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-subtle)] px-3 py-2 text-[length:var(--text-xs)]"
                >
                  <span className="font-semibold">
                    {formatPanamaTime(sl.startAt)}
                  </span>{" "}
                  ·{" "}
                  {practitionerNameById.get(sl.practitionerId) ??
                    sl.practitionerId}{" "}
                  · {sl.roomName}
                </li>
              ))}
            </ul>
          ) : (
            <p className="mt-4 text-[length:var(--text-sm)] text-[var(--text-secondary)]">
              Selecciona un servicio y fecha y pulsa &ldquo;Consultar
              horarios&rdquo;.
            </p>
          )}
        </Card>
      </div>
    </AppShell>
  );
}
