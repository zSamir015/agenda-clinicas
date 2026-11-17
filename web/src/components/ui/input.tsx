import type { InputHTMLAttributes } from "react";
import { cn } from "./cn";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  invalid?: boolean;
}

export function Input({ invalid = false, className, ...props }: InputProps) {
  return (
    <input
      aria-invalid={invalid || undefined}
      className={cn(
        "min-h-12 w-full rounded-[var(--radius-md)] border bg-[var(--bg-surface)] px-3 py-2 text-[length:var(--text-base)] text-[var(--text-primary)] placeholder:text-[var(--text-muted)] transition-colors duration-[var(--duration-fast)] ease-[var(--ease-standard)] disabled:cursor-not-allowed disabled:bg-[var(--bg-subtle)] disabled:opacity-70",
        invalid
          ? "border-[var(--danger-border)]"
          : "border-[var(--border-strong)]",
        className,
      )}
      {...props}
    />
  );
}
