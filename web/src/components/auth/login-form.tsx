"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";
import {
  Alert,
  Button,
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
  Field,
  Input,
} from "@/components/ui";
import { createBrowserApiClient, problemMessage } from "@/lib/api/client";
import {
  describePasskeyError,
  parseRequestOptions,
  serializeCredential,
  signalUnknownCredentialIfSupported,
  supportsPasskeys,
  unwrapPublicKeyOptions,
} from "@/lib/webauthn";
import { AppShell } from "./app-shell";
import { PasswordField } from "./password-field";

export function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const initialClinic = searchParams.get("clinica") ?? "clinica-demo";

  const [clinicSlug, setClinicSlug] = useState(initialClinic);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [showFallback, setShowFallback] = useState(false);
  const [passkeyLoading, setPasskeyLoading] = useState(false);
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [error, setError] = useState("");
  const [statusMessage, setStatusMessage] = useState("");
  const passkeyAbortRef = useRef<AbortController | null>(null);

  async function handlePasskeyLogin(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault();
    setError("");
    setStatusMessage("");

    if (!clinicSlug.trim() || !email.trim()) {
      setError("Indica el identificador de la clínica y tu correo.");
      return;
    }
    if (!supportsPasskeys()) {
      setError(
        "Este navegador no admite passkeys. Usa el acceso con contraseña y TOTP.",
      );
      setShowFallback(true);
      return;
    }

    passkeyAbortRef.current?.abort();
    const ac = new AbortController();
    passkeyAbortRef.current = ac;

    setPasskeyLoading(true);
    try {
      const api = createBrowserApiClient();
      const { data: beginData, error: beginErr } = await api.POST(
        "/auth/login/passkey/begin",
        {
          body: {
            clinicSlug: clinicSlug.trim().toLowerCase(),
            email: email.trim().toLowerCase(),
          },
        },
      );
      if (beginErr || !beginData) {
        setError(
          problemMessage(
            beginErr,
            "No se encontró una passkey activa para esa cuenta.",
          ),
        );
        return;
      }

      const pkJson = unwrapPublicKeyOptions(beginData.options);
      const rpId = typeof pkJson.rpId === "string" ? pkJson.rpId : undefined;
      const publicKey = parseRequestOptions(beginData.options);

      let credential: Credential | null = null;
      try {
        credential = await navigator.credentials.get({
          publicKey,
          signal: ac.signal,
        });
      } catch (err) {
        const info = describePasskeyError(err);
        setError(info.message);
        return;
      }

      if (!credential || credential.type !== "public-key") {
        setError("No se recibió una credencial WebAuthn válida.");
        return;
      }

      const encoded = serializeCredential(credential as PublicKeyCredential);
      try {
        const {
          data: finishData,
          error: finishErr,
          response,
        } = await api.POST("/auth/login/passkey/finish", {
          body: {
            challengeId: beginData.challengeId,
            credential: encoded,
          },
        });
        if (finishErr || !finishData) {
          if (response.status === 404) {
            await signalUnknownCredentialIfSupported(
              rpId,
              typeof encoded.id === "string" ? encoded.id : undefined,
            );
          }
          setError(
            problemMessage(finishErr, "No se pudo verificar la passkey."),
          );
          return;
        }
      } catch {
        setError("Error de red al verificar la passkey.");
        return;
      }

      setStatusMessage("Sesión iniciada con passkey. Redirigiendo al panel...");
      router.push("/panel/seguridad");
    } finally {
      setPasskeyLoading(false);
    }
  }

  async function handlePasswordLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setStatusMessage("");

    if (!clinicSlug.trim() || !email.trim() || !password || !code.trim()) {
      setError(
        "Completa clínica, correo, contraseña y tu código TOTP o de respaldo.",
      );
      return;
    }

    setPasswordLoading(true);
    try {
      const api = createBrowserApiClient();
      const { data, error: apiErr } = await api.POST("/auth/login/password", {
        body: {
          clinicSlug: clinicSlug.trim().toLowerCase(),
          email: email.trim().toLowerCase(),
          password,
          code: code.trim(),
        },
      });
      if (apiErr || !data) {
        setError(
          problemMessage(
            apiErr,
            "Credenciales inválidas. Verifica correo, contraseña y código.",
          ),
        );
        return;
      }
      setStatusMessage("Sesión iniciada. Redirigiendo al panel...");
      router.push("/panel/seguridad");
    } catch {
      setError("Error de red al iniciar sesión.");
    } finally {
      setPasswordLoading(false);
    }
  }

  return (
    <AppShell>
      <div className="mx-auto my-auto w-full max-w-md py-4">
        <Card>
          <CardHeader>
            <h1
              id="login-heading"
              className="text-[var(--text-2xl)] font-semibold leading-[var(--leading-tight)] text-[var(--text-primary)]"
            >
              Iniciar sesión
            </h1>
            <CardDescription>
              Acceso para personal de clínica con passkey o contraseña con TOTP.
            </CardDescription>
          </CardHeader>

          <div className="flex flex-col gap-5">
            {error ? <Alert tone="danger">{error}</Alert> : null}
            {statusMessage ? (
              <Alert tone="success">{statusMessage}</Alert>
            ) : null}

            <form
              onSubmit={handlePasskeyLogin}
              className="flex flex-col gap-4"
              aria-labelledby="login-heading"
            >
              <Field
                label="Clínica (subdominio)"
                hint="Por ejemplo: clinica-demo"
                required
              >
                <Input
                  name="clinicSlug"
                  type="text"
                  autoComplete="organization"
                  spellCheck={false}
                  value={clinicSlug}
                  onChange={(e) => setClinicSlug(e.target.value)}
                  className="min-h-12"
                  required
                />
              </Field>

              <Field
                label="Correo del personal"
                hint="Usa el correo con el que aceptaste tu invitación."
                required
              >
                <Input
                  name="email"
                  type="email"
                  autoComplete="username webauthn"
                  spellCheck={false}
                  autoFocus
                  aria-describedby="login-heading"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="min-h-12"
                  required
                />
              </Field>

              <Button
                type="submit"
                size="lg"
                loading={passkeyLoading}
                className="w-full"
              >
                Entrar con passkey
              </Button>
            </form>

            <div className="border-t border-[var(--border-default)] pt-4">
              <Button
                variant="secondary"
                size="md"
                aria-expanded={showFallback}
                onClick={() => setShowFallback((prev) => !prev)}
                className="w-full"
              >
                {showFallback
                  ? "Ocultar acceso con contraseña y TOTP"
                  : "Usar contraseña y TOTP como respaldo"}
              </Button>

              {showFallback ? (
                <form
                  onSubmit={handlePasswordLogin}
                  className="mt-4 flex flex-col gap-4"
                >
                  <CardTitle className="text-[var(--text-base)]">
                    Acceso de respaldo
                  </CardTitle>
                  <PasswordField
                    label="Contraseña"
                    name="password"
                    autoComplete="current-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="min-h-12"
                    required
                  />
                  <Field
                    label="Código TOTP o de recuperación"
                    hint="6 dígitos del autenticador o un código de respaldo."
                    required
                  >
                    <Input
                      name="code"
                      type="text"
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      maxLength={20}
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                      className="min-h-12 font-mono tracking-[0.15em]"
                      required
                    />
                  </Field>
                  <Button
                    type="submit"
                    size="lg"
                    loading={passwordLoading}
                    className="w-full"
                  >
                    Entrar con contraseña y TOTP
                  </Button>
                </form>
              ) : null}
            </div>

            <p className="text-center text-[var(--text-sm)] text-[var(--text-secondary)]">
              ¿Recibiste una invitación?{" "}
              <Link
                href="/invitacion"
                className="font-medium text-[var(--accent)] underline underline-offset-4"
              >
                Activa tu cuenta aquí
              </Link>
            </p>
          </div>
        </Card>
      </div>
    </AppShell>
  );
}
