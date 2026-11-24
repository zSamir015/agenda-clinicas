"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
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
  type PaymentModeDTO,
  type PractitionerDTO,
  type RoomDTO,
  type ServiceDTO,
  type SessionInfo,
} from "@/lib/api/client";
import { formatCentsUSD } from "./schedule-utils";

type CatalogTab = "services" | "rooms" | "practitioners";

const PAYMENT_MODE_LABELS: Record<PaymentModeDTO, string> = {
  none: "Sin cobro en línea",
  deposit: "Depósito",
  full: "Pago completo",
};

export function CatalogPanel() {
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [services, setServices] = useState<ServiceDTO[]>([]);
  const [rooms, setRooms] = useState<RoomDTO[]>([]);
  const [practitioners, setPractitioners] = useState<PractitionerDTO[]>([]);
  const [tab, setTab] = useState<CatalogTab>("services");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  // Service form state
  const [editingServiceId, setEditingServiceId] = useState<string | null>(null);
  const [svcName, setSvcName] = useState("");
  const [svcDuration, setSvcDuration] = useState("30");
  const [svcBuffer, setSvcBuffer] = useState("5");
  const [svcRoomType, setSvcRoomType] = useState("consultorio");
  const [svcPaymentMode, setSvcPaymentMode] = useState<PaymentModeDTO>("none");
  const [svcPriceDollars, setSvcPriceDollars] = useState("0");
  const [svcDepositDollars, setSvcDepositDollars] = useState("0");
  const [svcActive, setSvcActive] = useState(true);

  // Room form state
  const [editingRoomId, setEditingRoomId] = useState<string | null>(null);
  const [roomName, setRoomName] = useState("");
  const [roomType, setRoomType] = useState("consultorio");
  const [roomActive, setRoomActive] = useState(true);

  // Practitioner form state
  const [editingPracId, setEditingPracId] = useState<string | null>(null);
  const [pracName, setPracName] = useState("");
  const [pracSpecialty, setPracSpecialty] = useState("");
  const [pracServiceIds, setPracServiceIds] = useState<string[]>([]);
  const [pracActive, setPracActive] = useState(true);

  const loadAll = useCallback(async () => {
    const api = createBrowserApiClient();
    try {
      const sessRes = await api.GET("/auth/session");
      if (!sessRes.data) {
        setSession(null);
        return;
      }
      setSession(sessRes.data);

      const [svcRes, roomRes, pracRes] = await Promise.all([
        api.GET("/schedule/services"),
        api.GET("/schedule/rooms"),
        api.GET("/schedule/practitioners"),
      ]);
      if (svcRes.error) {
        setError(
          problemMessage(svcRes.error, "No se pudieron cargar los servicios."),
        );
        return;
      }
      if (roomRes.error) {
        setError(
          problemMessage(
            roomRes.error,
            "No se pudieron cargar los consultorios.",
          ),
        );
        return;
      }
      if (pracRes.error) {
        setError(
          problemMessage(
            pracRes.error,
            "No se pudieron cargar los profesionales.",
          ),
        );
        return;
      }
      setError("");
      setServices(svcRes.data?.services ?? []);
      setRooms(roomRes.data?.rooms ?? []);
      setPractitioners(pracRes.data?.practitioners ?? []);
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

  function resetServiceForm() {
    setEditingServiceId(null);
    setSvcName("");
    setSvcDuration("30");
    setSvcBuffer("5");
    setSvcRoomType("consultorio");
    setSvcPaymentMode("none");
    setSvcPriceDollars("0");
    setSvcDepositDollars("0");
    setSvcActive(true);
  }

  function startEditService(svc: ServiceDTO) {
    setEditingServiceId(svc.id);
    setSvcName(svc.name);
    setSvcDuration(String(svc.durationMinutes));
    setSvcBuffer(String(svc.bufferMinutes));
    setSvcRoomType(svc.roomType);
    setSvcPaymentMode(svc.paymentMode);
    setSvcPriceDollars((svc.priceCents / 100).toFixed(2));
    setSvcDepositDollars((svc.depositCents / 100).toFixed(2));
    setSvcActive(svc.active);
    setNotice("");
    setError("");
  }

  async function handleSaveService(e: FormEvent) {
    e.preventDefault();
    if (!session) return;
    setSaving(true);
    setError("");
    setNotice("");

    const priceCents = Math.round(Number(svcPriceDollars) * 100);
    const depositCents =
      svcPaymentMode === "deposit"
        ? Math.round(Number(svcDepositDollars) * 100)
        : 0;

    const body = {
      name: svcName,
      durationMinutes: Number(svcDuration),
      bufferMinutes: Number(svcBuffer),
      roomType: svcRoomType,
      paymentMode: svcPaymentMode,
      priceCents: Number.isFinite(priceCents) ? priceCents : 0,
      depositCents: Number.isFinite(depositCents) ? depositCents : 0,
      depositPercent: 0,
      active: svcActive,
    };

    const api = createBrowserApiClient();
    const headers = { "X-CSRF-Token": session.csrfToken };
    try {
      const res = editingServiceId
        ? await api.PUT("/schedule/services/{serviceId}", {
            params: { path: { serviceId: editingServiceId } },
            headers,
            body,
          })
        : await api.POST("/schedule/services", { headers, body });

      if (res.error) {
        setError(problemMessage(res.error, "No se pudo guardar el servicio."));
        return;
      }
      resetServiceForm();
      setNotice(
        editingServiceId ? "Servicio actualizado." : "Servicio creado.",
      );
      await loadAll();
    } finally {
      setSaving(false);
    }
  }

  function resetRoomForm() {
    setEditingRoomId(null);
    setRoomName("");
    setRoomType("consultorio");
    setRoomActive(true);
  }

  function startEditRoom(rm: RoomDTO) {
    setEditingRoomId(rm.id);
    setRoomName(rm.name);
    setRoomType(rm.roomType);
    setRoomActive(rm.active);
    setNotice("");
    setError("");
  }

  async function handleSaveRoom(e: FormEvent) {
    e.preventDefault();
    if (!session) return;
    setSaving(true);
    setError("");
    setNotice("");

    const body = { name: roomName, roomType, active: roomActive };
    const api = createBrowserApiClient();
    const headers = { "X-CSRF-Token": session.csrfToken };
    try {
      const res = editingRoomId
        ? await api.PUT("/schedule/rooms/{roomId}", {
            params: { path: { roomId: editingRoomId } },
            headers,
            body,
          })
        : await api.POST("/schedule/rooms", { headers, body });

      if (res.error) {
        setError(
          problemMessage(res.error, "No se pudo guardar el consultorio."),
        );
        return;
      }
      resetRoomForm();
      setNotice(
        editingRoomId ? "Consultorio actualizado." : "Consultorio creado.",
      );
      await loadAll();
    } finally {
      setSaving(false);
    }
  }

  function resetPracForm() {
    setEditingPracId(null);
    setPracName("");
    setPracSpecialty("");
    setPracServiceIds([]);
    setPracActive(true);
  }

  function startEditPrac(p: PractitionerDTO) {
    setEditingPracId(p.id);
    setPracName(p.name);
    setPracSpecialty(p.specialty);
    setPracServiceIds(p.serviceIds);
    setPracActive(p.active);
    setNotice("");
    setError("");
  }

  function togglePracService(serviceId: string) {
    setPracServiceIds((prev) =>
      prev.includes(serviceId)
        ? prev.filter((id) => id !== serviceId)
        : [...prev, serviceId],
    );
  }

  async function handleSavePrac(e: FormEvent) {
    e.preventDefault();
    if (!session) return;
    setSaving(true);
    setError("");
    setNotice("");

    const body = {
      name: pracName,
      specialty: pracSpecialty,
      serviceIds: pracServiceIds,
      active: pracActive,
    };
    const api = createBrowserApiClient();
    const headers = { "X-CSRF-Token": session.csrfToken };
    try {
      const res = editingPracId
        ? await api.PUT("/schedule/practitioners/{practitionerId}", {
            params: { path: { practitionerId: editingPracId } },
            headers,
            body,
          })
        : await api.POST("/schedule/practitioners", { headers, body });

      if (res.error) {
        setError(
          problemMessage(res.error, "No se pudo guardar el profesional."),
        );
        return;
      }
      resetPracForm();
      setNotice(
        editingPracId ? "Profesional actualizado." : "Profesional creado.",
      );
      await loadAll();
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <AppShell session={session} onSessionCleared={() => setSession(null)}>
        <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
          Cargando catálogo de la clínica...
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
              Inicia sesión con tu cuenta del personal para configurar el
              catálogo.
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
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h1 className="text-[length:var(--text-2xl)] font-bold tracking-tight">
              Catálogo de la clínica
            </h1>
            <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
              Configura servicios, consultorios y profesionales antes de definir
              horarios.
            </p>
          </div>
          <div
            role="tablist"
            aria-label="Secciones del catálogo"
            className="flex flex-wrap gap-2"
          >
            {(
              [
                ["services", `Servicios (${services.length})`],
                ["rooms", `Consultorios (${rooms.length})`],
                ["practitioners", `Profesionales (${practitioners.length})`],
              ] as const
            ).map(([key, label]) => (
              <Button
                key={key}
                type="button"
                role="tab"
                aria-selected={tab === key}
                variant={tab === key ? "primary" : "secondary"}
                size="sm"
                onClick={() => setTab(key)}
              >
                {label}
              </Button>
            ))}
          </div>
        </div>

        <div aria-live="polite" className="flex flex-col gap-2">
          {error ? <Alert tone="danger">{error}</Alert> : null}
          {notice ? <Alert tone="success">{notice}</Alert> : null}
        </div>

        {tab === "services" ? (
          <div className="grid gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>
                  {editingServiceId ? "Editar servicio" : "Nuevo servicio"}
                </CardTitle>
                <CardDescription>
                  Duración y margen deben ser múltiplos de 5 minutos.
                </CardDescription>
              </CardHeader>
              <form
                onSubmit={handleSaveService}
                className="flex flex-col gap-4"
              >
                <Field label="Nombre del servicio" htmlFor="svc-name">
                  <Input
                    id="svc-name"
                    required
                    value={svcName}
                    onChange={(e) => setSvcName(e.target.value)}
                    placeholder="Consulta de medicina general"
                  />
                </Field>
                <div className="grid grid-cols-2 gap-3">
                  <Field label="Duración (min)" htmlFor="svc-duration">
                    <Input
                      id="svc-duration"
                      type="number"
                      min={5}
                      max={480}
                      step={5}
                      required
                      value={svcDuration}
                      onChange={(e) => setSvcDuration(e.target.value)}
                    />
                  </Field>
                  <Field label="Margen (min)" htmlFor="svc-buffer">
                    <Input
                      id="svc-buffer"
                      type="number"
                      min={0}
                      max={120}
                      step={5}
                      required
                      value={svcBuffer}
                      onChange={(e) => setSvcBuffer(e.target.value)}
                    />
                  </Field>
                </div>
                <Field
                  label="Tipo de consultorio requerido"
                  htmlFor="svc-room-type"
                >
                  <Input
                    id="svc-room-type"
                    required
                    value={svcRoomType}
                    onChange={(e) => setSvcRoomType(e.target.value)}
                    placeholder="consultorio"
                  />
                </Field>
                <Field label="Modo de pago" htmlFor="svc-payment-mode">
                  <select
                    id="svc-payment-mode"
                    value={svcPaymentMode}
                    onChange={(e) =>
                      setSvcPaymentMode(e.target.value as PaymentModeDTO)
                    }
                    className="h-10 w-full rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-surface)] px-3 text-[length:var(--text-sm)]"
                  >
                    <option value="none">Sin cobro en línea</option>
                    <option value="deposit">Depósito</option>
                    <option value="full">Pago completo</option>
                  </select>
                </Field>
                <div className="grid grid-cols-2 gap-3">
                  <Field label="Precio (USD)" htmlFor="svc-price">
                    <Input
                      id="svc-price"
                      type="number"
                      min={0}
                      step="0.01"
                      value={svcPriceDollars}
                      onChange={(e) => setSvcPriceDollars(e.target.value)}
                    />
                  </Field>
                  {svcPaymentMode === "deposit" ? (
                    <Field label="Depósito (USD)" htmlFor="svc-deposit">
                      <Input
                        id="svc-deposit"
                        type="number"
                        min={0}
                        step="0.01"
                        value={svcDepositDollars}
                        onChange={(e) => setSvcDepositDollars(e.target.value)}
                      />
                    </Field>
                  ) : null}
                </div>
                <label className="flex items-center gap-2 text-[length:var(--text-sm)]">
                  <input
                    type="checkbox"
                    checked={svcActive}
                    onChange={(e) => setSvcActive(e.target.checked)}
                  />
                  Servicio activo
                </label>
                <div className="flex flex-wrap gap-2">
                  <Button type="submit" loading={saving}>
                    {editingServiceId ? "Guardar cambios" : "Crear servicio"}
                  </Button>
                  {editingServiceId ? (
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={resetServiceForm}
                    >
                      Cancelar
                    </Button>
                  ) : null}
                </div>
              </form>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Servicios registrados</CardTitle>
                <CardDescription>
                  Haz clic en editar para ajustar duración, tarifa o estado.
                </CardDescription>
              </CardHeader>
              {services.length === 0 ? (
                <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
                  Todavía no hay servicios configurados.
                </p>
              ) : (
                <ul className="flex flex-col gap-3">
                  {services.map((svc) => (
                    <li
                      key={svc.id}
                      className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] p-3"
                    >
                      <div className="flex flex-col gap-1">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold">{svc.name}</span>
                          <Badge tone={svc.active ? "success" : "neutral"}>
                            {svc.active ? "Activo" : "Inactivo"}
                          </Badge>
                        </div>
                        <span className="text-[length:var(--text-xs)] text-[var(--text-secondary)]">
                          {svc.durationMinutes} min (+{svc.bufferMinutes} min
                          margen) · {svc.roomType} ·{" "}
                          {PAYMENT_MODE_LABELS[svc.paymentMode]} (
                          {formatCentsUSD(svc.priceCents)})
                        </span>
                      </div>
                      <Button
                        type="button"
                        size="sm"
                        variant="secondary"
                        onClick={() => startEditService(svc)}
                      >
                        Editar
                      </Button>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          </div>
        ) : null}

        {tab === "rooms" ? (
          <div className="grid gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>
                  {editingRoomId ? "Editar consultorio" : "Nuevo consultorio"}
                </CardTitle>
                <CardDescription>
                  El tipo de consultorio debe coincidir con el requerido por
                  cada servicio.
                </CardDescription>
              </CardHeader>
              <form onSubmit={handleSaveRoom} className="flex flex-col gap-4">
                <Field label="Nombre" htmlFor="room-name">
                  <Input
                    id="room-name"
                    required
                    value={roomName}
                    onChange={(e) => setRoomName(e.target.value)}
                    placeholder="Consultorio 1"
                  />
                </Field>
                <Field label="Tipo de consultorio" htmlFor="room-type">
                  <Input
                    id="room-type"
                    required
                    value={roomType}
                    onChange={(e) => setRoomType(e.target.value)}
                    placeholder="consultorio"
                  />
                </Field>
                <label className="flex items-center gap-2 text-[length:var(--text-sm)]">
                  <input
                    type="checkbox"
                    checked={roomActive}
                    onChange={(e) => setRoomActive(e.target.checked)}
                  />
                  Consultorio activo
                </label>
                <div className="flex flex-wrap gap-2">
                  <Button type="submit" loading={saving}>
                    {editingRoomId ? "Guardar cambios" : "Crear consultorio"}
                  </Button>
                  {editingRoomId ? (
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={resetRoomForm}
                    >
                      Cancelar
                    </Button>
                  ) : null}
                </div>
              </form>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Consultorios registrados</CardTitle>
              </CardHeader>
              {rooms.length === 0 ? (
                <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
                  Todavía no hay consultorios configurados.
                </p>
              ) : (
                <ul className="flex flex-col gap-3">
                  {rooms.map((rm) => (
                    <li
                      key={rm.id}
                      className="flex items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] p-3"
                    >
                      <div className="flex flex-col gap-1">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold">{rm.name}</span>
                          <Badge tone={rm.active ? "success" : "neutral"}>
                            {rm.active ? "Activo" : "Inactivo"}
                          </Badge>
                        </div>
                        <span className="text-[length:var(--text-xs)] text-[var(--text-secondary)]">
                          Tipo: {rm.roomType}
                        </span>
                      </div>
                      <Button
                        type="button"
                        size="sm"
                        variant="secondary"
                        onClick={() => startEditRoom(rm)}
                      >
                        Editar
                      </Button>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          </div>
        ) : null}

        {tab === "practitioners" ? (
          <div className="grid gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>
                  {editingPracId ? "Editar profesional" : "Nuevo profesional"}
                </CardTitle>
                <CardDescription>
                  Asigna los servicios que atiende cada médico o especialista.
                </CardDescription>
              </CardHeader>
              <form onSubmit={handleSavePrac} className="flex flex-col gap-4">
                <Field label="Nombre completo" htmlFor="prac-name">
                  <Input
                    id="prac-name"
                    required
                    value={pracName}
                    onChange={(e) => setPracName(e.target.value)}
                    placeholder="Dra. Sofía Mendoza"
                  />
                </Field>
                <Field label="Especialidad" htmlFor="prac-specialty">
                  <Input
                    id="prac-specialty"
                    required
                    value={pracSpecialty}
                    onChange={(e) => setPracSpecialty(e.target.value)}
                    placeholder="Medicina General"
                  />
                </Field>
                <fieldset className="flex flex-col gap-2">
                  <legend className="text-[length:var(--text-sm)] font-medium">
                    Servicios habilitados
                  </legend>
                  {services.length === 0 ? (
                    <p className="text-[length:var(--text-xs)] text-[var(--text-secondary)]">
                      Crea al menos un servicio primero.
                    </p>
                  ) : (
                    <div className="flex flex-col gap-2">
                      {services.map((svc) => (
                        <label
                          key={svc.id}
                          className="flex items-center gap-2 text-[length:var(--text-sm)]"
                        >
                          <input
                            type="checkbox"
                            checked={pracServiceIds.includes(svc.id)}
                            onChange={() => togglePracService(svc.id)}
                          />
                          {svc.name}
                        </label>
                      ))}
                    </div>
                  )}
                </fieldset>
                <label className="flex items-center gap-2 text-[length:var(--text-sm)]">
                  <input
                    type="checkbox"
                    checked={pracActive}
                    onChange={(e) => setPracActive(e.target.checked)}
                  />
                  Profesional activo
                </label>
                <div className="flex flex-wrap gap-2">
                  <Button type="submit" loading={saving}>
                    {editingPracId ? "Guardar cambios" : "Crear profesional"}
                  </Button>
                  {editingPracId ? (
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={resetPracForm}
                    >
                      Cancelar
                    </Button>
                  ) : null}
                </div>
              </form>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Profesionales registrados</CardTitle>
              </CardHeader>
              {practitioners.length === 0 ? (
                <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
                  Todavía no hay profesionales configurados.
                </p>
              ) : (
                <ul className="flex flex-col gap-3">
                  {practitioners.map((p) => (
                    <li
                      key={p.id}
                      className="flex items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] p-3"
                    >
                      <div className="flex flex-col gap-1">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold">{p.name}</span>
                          <Badge tone={p.active ? "success" : "neutral"}>
                            {p.active ? "Activo" : "Inactivo"}
                          </Badge>
                        </div>
                        <span className="text-[length:var(--text-xs)] text-[var(--text-secondary)]">
                          {p.specialty} · {p.serviceIds.length} servicio(s)
                        </span>
                      </div>
                      <Button
                        type="button"
                        size="sm"
                        variant="secondary"
                        onClick={() => startEditPrac(p)}
                      >
                        Editar
                      </Button>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          </div>
        ) : null}
      </div>
    </AppShell>
  );
}
