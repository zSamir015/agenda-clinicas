"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
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
import {
  createBrowserApiClient,
  problemMessage,
  type InvitationPreviewDTO,
  type SessionInfo,
} from "@/lib/api/client";
import { registerPasskeyForSession } from "@/lib/webauthn";
import { AppShell } from "./app-shell";
import { TotpSetupCard } from "./totp-setup-card";

export function InvitationFlow() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const urlToken = searchParams.get("token") ?? "";

  const [token, setToken] = useState(urlToken);
  const [preview, setPreview] = useState<InvitationPreviewDTO | null>(null);
  const [existingSession, setExistingSession] = useState<SessionInfo | null>(
    null,
  );
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [passkeyAdded, setPasskeyAdded] = useState(false);
  const [totpConfigured, setTotpConfigured] = useState(false);
  const [loadingPreview, setLoadingPreview] = useState(Boolean(urlToken));
  const [accepting, setAccepting] = useState(false);
  const [registeringPasskey, setRegisteringPasskey] = useState(false);
  const [loginRequired, setLoginRequired] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  async function runPreview(rawToken: string) {
    const api = createBrowserApiClient();
    const { data, error: apiErr } = await api.POST(
      "/auth/invitations/preview",
      {
        body: { token: rawToken.trim() },
      },
    );
    return { data, apiErr };
  }

  useEffect(() => {
    let active = true;
    const api = createBrowserApiClient();
    void api
      .GET("/auth/session")
      .then(({ data }) => {
        if (active && data?.kind === "staff") {
          setExistingSession(data);
        }
      })
      .catch(() => {
        // Unauthenticated visitors simply accept without an existing session.
      });

    if (!urlToken.trim()) {
      return () => {
        active = false;
      };
    }

    void runPreview(urlToken)
      .then(({ data, apiErr }) => {
        if (!active) {
          return;
        }
        if (apiErr || !data) {
          setPreview(null);
          setError(
            problemMessage(
              apiErr,
              "La invitación no es válida, ya fue usada o expiró.",
            ),
          );
          return;
        }
        setPreview(data);
      })
      .catch(() => {
        if (active) {
          setError("Error de red al validar el enlace de invitación.");
        }
      })
      .finally(() => {
        if (active) {
          setLoadingPreview(false);
        }
      });
    return () => {
      active = false;
    };
  }, [urlToken]);

  async function handlePreviewSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token.trim()) {
      return;
    }
    setError("");
    setLoginRequired(false);
    setLoadingPreview(true);
    try {
      const { data, apiErr } = await runPreview(token);
      if (apiErr || !data) {
        setPreview(null);
        setError(
          problemMessage(
            apiErr,
            "La invitación no es válida, ya fue usada o expiró.",
          ),
        );
        return;
      }
      setPreview(data);
    } catch {
      setError("Error de red al validar el enlace de invitación.");
    } finally {
      setLoadingPreview(false);
    }
  }

  async function handleAccept() {
    if (!token.trim()) {
      setError("Ingresa el token de invitación.");
      return;
    }
    setError("");
    setLoginRequired(false);
    setAccepting(true);
    try {
      const api = createBrowserApiClient();
      const { data, error: apiErr } = await api.POST(
        "/auth/invitations/accept",
        {
          ...(existingSession
            ? { headers: { "X-CSRF-Token": existingSession.csrfToken } }
            : {}),
          body: { token: token.trim() },
        },
      );
      if (apiErr || !data) {
        const problemCode =
          apiErr && typeof apiErr === "object" && "code" in apiErr
            ? apiErr.code
            : undefined;
        if (problemCode === "login_required") {
          setLoginRequired(true);
        }
        setError(problemMessage(apiErr, "No se pudo aceptar la invitación."));
        return;
      }
      setSession(data);
      setNotice(
        "Cuenta activada. Registra ahora tu primera passkey o configura contraseña y TOTP como respaldo.",
      );
    } catch {
      setError("Error de red al aceptar la invitación.");
    } finally {
      setAccepting(false);
    }
  }

  async function handleRegisterPasskey() {
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
      setPasskeyAdded(true);
      setNotice("Passkey registrada correctamente en tu cuenta.");
    } finally {
      setRegisteringPasskey(false);
    }
  }

  return (
    <AppShell session={session} onSessionCleared={() => setSession(null)}>
      <div className="mx-auto w-full max-w-xl py-4">
        <Card>
          <CardHeader>
            <h1 className="text-[length:var(--text-2xl)] font-semibold leading-[var(--leading-tight)] text-[var(--text-primary)]">
              Activar cuenta por invitación
            </h1>
            <CardDescription>
              Enlace de un solo uso para incorporarte a una clínica y configurar
              tu acceso.
            </CardDescription>
          </CardHeader>

          <div className="flex flex-col gap-5">
            {error ? <Alert tone="danger">{error}</Alert> : null}
            {loginRequired && preview ? (
              <Alert
                tone="warning"
                title="Inicia sesión para vincular esta clínica"
              >
                <p>
                  El correo <strong>{preview.email}</strong> ya tiene una cuenta
                  activa. Inicia sesión con esa cuenta para aceptar esta
                  invitación.
                </p>
                <div className="mt-3">
                  <Link
                    href={`/login?email=${encodeURIComponent(preview.email)}&next=${encodeURIComponent(`/invitacion?token=${token.trim()}`)}`}
                    className="inline-flex min-h-10 items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent)] px-4 text-[length:var(--text-sm)] font-medium text-[var(--accent-text)] hover:bg-[var(--accent-hover)]"
                  >
                    Iniciar sesión y volver aquí
                  </Link>
                </div>
              </Alert>
            ) : null}
            {notice ? <Alert tone="success">{notice}</Alert> : null}

            {!session && !preview ? (
              <form
                onSubmit={handlePreviewSubmit}
                className="flex flex-col gap-4"
              >
                <Field
                  label="Token de invitación"
                  hint="Pega el token que te compartió el administrador de la clínica."
                  required
                >
                  <Input
                    name="token"
                    type="text"
                    spellCheck={false}
                    value={token}
                    onChange={(e) => setToken(e.target.value)}
                    className="min-h-12 font-mono"
                    required
                  />
                </Field>
                <Button type="submit" size="lg" loading={loadingPreview}>
                  Verificar invitación
                </Button>
              </form>
            ) : null}

            {!session && preview ? (
              <div className="flex flex-col gap-4">
                <div className="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-subtle)] p-4 text-[length:var(--text-sm)]">
                  <p className="font-semibold text-[var(--text-primary)]">
                    {preview.name} ({preview.email})
                  </p>
                  <p className="mt-1 text-[var(--text-secondary)]">
                    Clínica: <strong>{preview.clinicName}</strong> (
                    {preview.clinicSlug})
                  </p>
                  <div className="mt-2">
                    <Badge tone="info">Rol asignado: {preview.roleName}</Badge>
                  </div>
                </div>

                <Button
                  size="lg"
                  loading={accepting}
                  onClick={handleAccept}
                  className="w-full"
                >
                  Aceptar invitación y continuar
                </Button>
              </div>
            ) : null}

            {session ? (
              <div className="flex flex-col gap-6">
                <div className="flex flex-col gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] p-4">
                  <div className="flex items-center justify-between gap-2">
                    <CardTitle className="text-[length:var(--text-lg)]">
                      Opción principal: Passkey
                    </CardTitle>
                    {passkeyAdded ? (
                      <Badge tone="success">Registrada</Badge>
                    ) : null}
                  </div>
                  <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
                    Registra la biometría o llave de seguridad de este
                    dispositivo para entrar sin contraseñas.
                  </p>
                  <div>
                    <Button
                      size="lg"
                      loading={registeringPasskey}
                      onClick={handleRegisterPasskey}
                    >
                      Registrar passkey
                    </Button>
                  </div>
                </div>

                <div className="flex flex-col gap-3 rounded-[var(--radius-md)] border border-[var(--border-default)] p-4">
                  <div className="flex items-center justify-between gap-2">
                    <CardTitle className="text-[length:var(--text-lg)]">
                      Respaldo obligatorio: Contraseña + TOTP
                    </CardTitle>
                    {totpConfigured ? (
                      <Badge tone="success">Configurado</Badge>
                    ) : null}
                  </div>
                  <TotpSetupCard
                    csrfToken={session.csrfToken}
                    onConfigured={() => setTotpConfigured(true)}
                  />
                </div>

                <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-default)] pt-4">
                  <Link
                    href="/panel/seguridad"
                    className="text-[length:var(--text-sm)] font-medium text-[var(--accent)] underline underline-offset-4"
                  >
                    Ir a configuración de seguridad
                  </Link>
                  <Button
                    variant="secondary"
                    onClick={() => router.push("/panel/seguridad")}
                  >
                    Finalizar
                  </Button>
                </div>
              </div>
            ) : null}
          </div>
        </Card>
      </div>
    </AppShell>
  );
}
