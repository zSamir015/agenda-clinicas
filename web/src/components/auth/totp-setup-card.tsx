"use client";

import { useState, type FormEvent } from "react";
import { Alert, Button, Field, Input } from "@/components/ui";
import { PasswordField } from "./password-field";
import { createBrowserApiClient, problemMessage } from "@/lib/api/client";

export interface TotpSetupCardProps {
  csrfToken: string;
  onConfigured?: (recoveryCodes: string[]) => void;
}

export function TotpSetupCard({ csrfToken, onConfigured }: TotpSetupCardProps) {
  const [secret, setSecret] = useState("");
  const [uri, setUri] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [loadingSetup, setLoadingSetup] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function handleStartSetup() {
    setError("");
    setLoadingSetup(true);
    try {
      const api = createBrowserApiClient();
      const { data, error: apiErr } = await api.POST("/auth/totp/setup", {
        headers: { "X-CSRF-Token": csrfToken },
      });
      if (apiErr || !data) {
        setError(problemMessage(apiErr, "No se pudo generar el secreto TOTP."));
        return;
      }
      setSecret(data.secret);
      setUri(data.uri);
    } catch {
      setError("Error de red al preparar la configuración TOTP.");
    } finally {
      setLoadingSetup(false);
    }
  }

  async function handleConfirm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    if (password.length < 12) {
      setError("La contraseña debe tener al menos 12 caracteres.");
      return;
    }
    if (!/^\d{6}$/.test(code.trim())) {
      setError(
        "Ingresa el código de 6 dígitos de tu aplicación autenticadora.",
      );
      return;
    }
    setSubmitting(true);
    try {
      const api = createBrowserApiClient();
      const { data, error: apiErr } = await api.POST("/auth/totp/confirm", {
        headers: { "X-CSRF-Token": csrfToken },
        body: {
          password,
          secret,
          code: code.trim(),
        },
      });
      if (apiErr || !data) {
        setError(
          problemMessage(
            apiErr,
            "No se pudo activar la contraseña y TOTP. Verifica el código.",
          ),
        );
        return;
      }
      setRecoveryCodes(data.recoveryCodes);
      setPassword("");
      setCode("");
      onConfigured?.(data.recoveryCodes);
    } catch {
      setError("Error de red al confirmar la configuración TOTP.");
    } finally {
      setSubmitting(false);
    }
  }

  if (recoveryCodes) {
    return (
      <div className="flex flex-col gap-4">
        <Alert tone="success" title="Respaldo TOTP activado">
          Guarda estos 10 códigos de recuperación en un lugar seguro. Cada
          código funciona una sola vez y no volverán a mostrarse.
        </Alert>
        <ul
          aria-label="Códigos de recuperación de un solo uso"
          className="grid grid-cols-2 gap-2 rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-subtle)] p-4 font-mono text-[var(--text-sm)] sm:grid-cols-5"
        >
          {recoveryCodes.map((rc) => (
            <li key={rc} className="select-all">
              {rc}
            </li>
          ))}
        </ul>
      </div>
    );
  }

  if (!secret) {
    return (
      <div className="flex flex-col gap-3">
        {error ? <Alert tone="danger">{error}</Alert> : null}
        <p className="text-[var(--text-sm)] text-[var(--text-secondary)]">
          Como respaldo de tu passkey, define una contraseña de al menos 12
          caracteres y vincula un código temporal TOTP en tu autenticador.
        </p>
        <div>
          <Button
            variant="secondary"
            loading={loadingSetup}
            onClick={handleStartSetup}
          >
            Generar clave TOTP
          </Button>
        </div>
      </div>
    );
  }

  return (
    <form onSubmit={handleConfirm} className="flex flex-col gap-4" noValidate>
      {error ? <Alert tone="danger">{error}</Alert> : null}

      <div className="rounded-[var(--radius-md)] border border-[var(--border-default)] bg-[var(--bg-subtle)] p-4 text-[var(--text-sm)]">
        <p className="font-medium text-[var(--text-primary)]">
          1. Agrega esta clave en tu aplicación autenticadora:
        </p>
        <p className="mt-2 break-all font-mono text-[var(--text-sm)] select-all text-[var(--text-primary)]">
          {secret}
        </p>
        <details className="mt-2">
          <summary className="cursor-pointer text-[var(--text-xs)] text-[var(--text-secondary)]">
            Ver enlace otpauth:// completo
          </summary>
          <p className="mt-1 break-all font-mono text-[var(--text-xs)] select-all text-[var(--text-muted)]">
            {uri}
          </p>
        </details>
      </div>

      <PasswordField
        label="Nueva contraseña (mínimo 12 caracteres)"
        hint="Se cifra con Argon2id. No bloqueamos el pegado desde tu gestor."
        name="new-password"
        autoComplete="new-password"
        minLength={12}
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        required
      />

      <Field
        label="Código TOTP de 6 dígitos"
        hint="Ingresa el código actual que muestra tu aplicación autenticadora."
        required
      >
        <Input
          name="totp-confirm-code"
          type="text"
          inputMode="numeric"
          autoComplete="one-time-code"
          pattern="\d{6}"
          maxLength={6}
          value={code}
          onChange={(e) => setCode(e.target.value)}
          className="font-mono tracking-[0.25em]"
          required
        />
      </Field>

      <div>
        <Button type="submit" loading={submitting}>
          Guardar contraseña y activar TOTP
        </Button>
      </div>
    </form>
  );
}
