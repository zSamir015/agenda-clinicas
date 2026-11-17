"use client";

import * as Label from "@radix-ui/react-label";
import { useId, useState, type InputHTMLAttributes } from "react";
import { Button, Input } from "@/components/ui";

export interface PasswordFieldProps extends Omit<
  InputHTMLAttributes<HTMLInputElement>,
  "type"
> {
  label: string;
  hint?: string;
  error?: string;
}

export function PasswordField({
  label,
  hint,
  error,
  id,
  required,
  ...props
}: PasswordFieldProps) {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const hintId = hint ? `${inputId}-hint` : undefined;
  const errorId = error ? `${inputId}-error` : undefined;
  const describedBy = [hintId, errorId].filter(Boolean).join(" ") || undefined;
  const [visible, setVisible] = useState(false);

  return (
    <div className="flex flex-col gap-[var(--space-1)]">
      <Label.Root
        htmlFor={inputId}
        className="text-[length:var(--text-sm)] font-medium text-[var(--text-primary)]"
      >
        {label}
        {required ? (
          <span className="ml-1 text-[var(--danger-text)]" aria-hidden="true">
            *
          </span>
        ) : null}
      </Label.Root>

      <div className="flex items-stretch gap-2">
        <Input
          id={inputId}
          type={visible ? "text" : "password"}
          aria-describedby={describedBy}
          aria-invalid={error ? true : undefined}
          required={required}
          {...props}
        />
        <Button
          variant="secondary"
          aria-pressed={visible}
          aria-label={visible ? "Ocultar contraseña" : "Mostrar contraseña"}
          onClick={() => setVisible((prev) => !prev)}
        >
          {visible ? "Ocultar" : "Mostrar"}
        </Button>
      </div>

      {hint ? (
        <p
          id={hintId}
          className="text-[length:var(--text-sm)] text-[var(--text-muted)]"
        >
          {hint}
        </p>
      ) : null}
      {error ? (
        <p
          id={errorId}
          role="alert"
          className="text-[length:var(--text-sm)] font-medium text-[var(--danger-text)]"
        >
          {error}
        </p>
      ) : null}
    </div>
  );
}
