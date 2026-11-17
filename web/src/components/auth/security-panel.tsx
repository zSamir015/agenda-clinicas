"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui";
import {
  createBrowserApiClient,
  problemMessage,
  type SecurityStatusDTO,
  type SessionInfo,
} from "@/lib/api/client";
import { registerPasskeyForSession } from "@/lib/webauthn";
import { AppShell } from "./app-shell";
import { TotpSetupCard } from "./totp-setup-card";

export function SecurityPanel() {
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [security, setSecurity] = useState<SecurityStatusDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [registeringPasskey, setRegisteringPasskey] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const fetchSecurityState = useCallback(async () => {
    const api = createBrowserApiClient();
    const { data: sessData } = await api.GET("/auth/session");
    if (!sessData || sessData.kind !== "staff") {
      return {
        session: null,
        security: null,
        error: "",
      };
    }

    const { data: secData, error: secErr } = await api.GET("/auth/security");
    if (secErr || !secData) {
      return {
        session: sessData,
        security: null,
        error: problemMessage(
          secErr,
          "No se pudo cargar el estado de seguridad.",
        ),
      };
    }
    return {
      session: sessData,
      security: secData,
      error: "",
    };
  }, []);

  const refreshData = useCallback(async () => {
    setError("");
    try {
      const res = await fetchSecurityState();
      setSession(res.session);
      setSecurity(res.security);
      if (res.error) {
        setError(res.error);
      }
    } catch {
      setError("No se pudo conectar con la API para cargar tu seguridad.");
    } finally {
      setLoading(false);
    }
  }, [fetchSecurityState]);

  useEffect(() => {
    let active = true;
    void fetchSecurityState()
      .then((res) => {
        if (!active) {
          return;
        }
        setSession(res.session);
        setSecurity(res.security);
        if (res.error) {
          setError(res.error);
        }
      })
      .catch(() => {
        if (active) {
          setError("No se pudo conectar con la API para cargar tu seguridad.");
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
  }, [fetchSecurityState]);

  async function handleAddPasskey() {
    if (!session) {
      return;
    }
    setError("");
    setNotice("");
    setRegisteringPasskey(true);
    try {
      const result = await registerPasskeyForSession(session.csrfToken);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setNotice("Nueva passkey registrada correctamente.");
      await refreshData();
    } finally {
      setRegisteringPasskey(false);
    }
  }

  return (
    <AppShell session={session} onSessionCleared={() => setSession(null)}>
      <div className="flex flex-col gap-6">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <div>
            <h1 className="text-[var(--text-2xl)] font-semibold leading-[var(--leading-tight)]">
              Seguridad de la cuenta
            </h1>
            <p className="text-[var(--text-sm)] text-[var(--text-secondary)]">
              Administra tus passkeys, contraseña Argon2id y códigos TOTP.
            </p>
          </div>
          {session?.email ? (
            <Badge tone="neutral">{session.email}</Badge>
          ) : null}
        </div>

        {loading ? (
          <Card>
            <p className="text-[var(--text-sm)] text-[var(--text-secondary)]">
              Cargando configuración de seguridad...
            </p>
          </Card>
        ) : !session ? (
          <Card>
            <CardHeader>
              <CardTitle>Sesión requerida</CardTitle>
              <CardDescription>
                Inicia sesión como personal de una clínica para administrar tus
                credenciales.
              </CardDescription>
            </CardHeader>
            <Link
              href="/login"
              className="inline-flex min-h-11 items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent)] px-4 font-medium text-[var(--accent-text)] hover:bg-[var(--accent-hover)]"
            >
              Ir a iniciar sesión
            </Link>
          </Card>
        ) : (
          <>
            {error ? <Alert tone="danger">{error}</Alert> : null}
            {notice ? <Alert tone="success">{notice}</Alert> : null}

            <Card>
              <CardHeader>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <CardTitle>Passkeys registradas</CardTitle>
                  <Badge
                    tone={
                      security && security.passkeys.length > 0
                        ? "success"
                        : "warning"
                    }
                  >
                    {security ? `${security.passkeys.length} activas` : "0"}
                  </Badge>
                </div>
                <CardDescription>
                  Método principal sin contraseña mediante WebAuthn.
                </CardDescription>
              </CardHeader>

              <div className="flex flex-col gap-4">
                {security && security.passkeys.length > 0 ? (
                  <ul className="flex flex-col gap-2">
                    {security.passkeys.map((pk, idx) => (
                      <li
                        key={pk.id}
                        className="flex flex-wrap items-center justify-between gap-2 rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-subtle)] px-4 py-3 text-[var(--text-sm)]"
                      >
                        <div>
                          <p className="font-medium text-[var(--text-primary)]">
                            Passkey #{idx + 1}
                          </p>
                          <p className="font-mono text-[var(--text-xs)] text-[var(--text-muted)]">
                            {pk.id}
                          </p>
                        </div>
                        <div className="flex flex-wrap gap-1">
                          {pk.transports.length > 0 ? (
                            pk.transports.map((tr) => (
                              <Badge key={tr} tone="neutral">
                                {tr}
                              </Badge>
                            ))
                          ) : (
                            <Badge tone="neutral">webauthn</Badge>
                          )}
                        </div>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-[var(--text-sm)] text-[var(--text-secondary)]">
                    Aún no has registrado una passkey en esta cuenta.
                  </p>
                )}

                <div>
                  <Button
                    size="lg"
                    loading={registeringPasskey}
                    onClick={handleAddPasskey}
                  >
                    Agregar passkey
                  </Button>
                </div>
              </div>
            </Card>

            <Card>
              <CardHeader>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <CardTitle>Contraseña y TOTP de respaldo</CardTitle>
                  <Badge tone={security?.totpEnabled ? "success" : "warning"}>
                    {security?.totpEnabled
                      ? `TOTP activo (${security.recoveryCodesRemaining} códigos restantes)`
                      : "Sin configurar"}
                  </Badge>
                </div>
                <CardDescription>
                  Respaldo con Argon2id, código temporal TOTP y 10 códigos de
                  recuperación de un solo uso.
                </CardDescription>
              </CardHeader>

              <TotpSetupCard
                csrfToken={session.csrfToken}
                onConfigured={() => {
                  setNotice(
                    "Contraseña y TOTP guardados. Conserva tus nuevos códigos de respaldo.",
                  );
                  void refreshData();
                }}
              />
            </Card>
          </>
        )}
      </div>
    </AppShell>
  );
}
