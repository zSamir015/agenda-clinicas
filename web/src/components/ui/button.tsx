import type { ButtonHTMLAttributes } from "react";
import { cn } from "./cn";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md" | "lg";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
}

const variantClasses: Record<ButtonVariant, string> = {
  primary:
    "bg-[var(--accent)] text-[var(--accent-text)] border-transparent hover:bg-[var(--accent-hover)]",
  secondary:
    "bg-[var(--bg-surface)] text-[var(--text-primary)] border-[var(--border-strong)] hover:bg-[var(--bg-subtle)]",
  ghost:
    "bg-transparent text-[var(--text-primary)] border-transparent hover:bg-[var(--bg-subtle)]",
  danger:
    "bg-[var(--danger-bg)] text-[var(--danger-text)] border-[var(--danger-border)] hover:opacity-90",
};

const sizeClasses: Record<ButtonSize, string> = {
  sm: "min-h-10 px-3 text-[var(--text-sm)]",
  md: "min-h-11 px-4 text-[var(--text-base)]",
  lg: "min-h-12 px-5 text-[var(--text-base)]",
};

export function Button({
  variant = "primary",
  size = "md",
  loading = false,
  disabled,
  className,
  type = "button",
  children,
  ...props
}: ButtonProps) {
  const isDisabled = Boolean(disabled || loading);

  return (
    <button
      type={type}
      disabled={isDisabled}
      aria-busy={loading || undefined}
      className={cn(
        "inline-flex items-center justify-center gap-2 rounded-[var(--radius-md)] border font-medium transition-colors duration-[var(--duration-fast)] ease-[var(--ease-standard)] disabled:cursor-not-allowed disabled:opacity-60",
        variantClasses[variant],
        sizeClasses[size],
        className,
      )}
      {...props}
    >
      {loading ? <span aria-hidden="true">...</span> : null}
      {children}
    </button>
  );
}
