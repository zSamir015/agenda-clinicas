"use client";

import * as Label from "@radix-ui/react-label";
import { cloneElement, isValidElement, useId, type ReactElement } from "react";
import { cn } from "./cn";

export interface FieldProps {
  label: string;
  htmlFor?: string;
  hint?: string;
  error?: string;
  required?: boolean;
  className?: string;
  children: ReactElement<{
    id?: string;
    "aria-describedby"?: string;
    "aria-invalid"?: boolean;
    required?: boolean;
  }>;
}

export function Field({
  label,
  htmlFor,
  hint,
  error,
  required,
  className,
  children,
}: FieldProps) {
  const generatedId = useId();
  const inputId = htmlFor ?? generatedId;
  const hintId = hint ? `${inputId}-hint` : undefined;
  const errorId = error ? `${inputId}-error` : undefined;
  const describedBy = [hintId, errorId].filter(Boolean).join(" ") || undefined;

  const control = isValidElement(children)
    ? cloneElement(children, {
        id: children.props.id ?? inputId,
        "aria-describedby": describedBy,
        "aria-invalid": error ? true : children.props["aria-invalid"],
        required: required ?? children.props.required,
      })
    : children;

  return (
    <div className={cn("flex flex-col gap-[var(--space-1)]", className)}>
      <Label.Root
        htmlFor={inputId}
        className="text-[var(--text-sm)] font-medium text-[var(--text-primary)]"
      >
        {label}
        {required ? (
          <span className="ml-1 text-[var(--danger-text)]" aria-hidden="true">
            *
          </span>
        ) : null}
      </Label.Root>
      {control}
      {hint ? (
        <p
          id={hintId}
          className="text-[var(--text-sm)] text-[var(--text-muted)]"
        >
          {hint}
        </p>
      ) : null}
      {error ? (
        <p
          id={errorId}
          role="alert"
          className="text-[var(--text-sm)] font-medium text-[var(--danger-text)]"
        >
          {error}
        </p>
      ) : null}
    </div>
  );
}
