import type { HTMLAttributes } from "react";
import { cn } from "./cn";

export type BadgeTone = "neutral" | "info" | "success" | "warning" | "danger";

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone;
}

const toneClasses: Record<BadgeTone, string> = {
  neutral:
    "bg-[var(--bg-subtle)] text-[var(--text-secondary)] border-[var(--border-default)]",
  info: "bg-[var(--info-bg)] text-[var(--info-text)] border-[var(--info-border)]",
  success:
    "bg-[var(--success-bg)] text-[var(--success-text)] border-[var(--success-border)]",
  warning:
    "bg-[var(--warning-bg)] text-[var(--warning-text)] border-[var(--warning-border)]",
  danger:
    "bg-[var(--danger-bg)] text-[var(--danger-text)] border-[var(--danger-border)]",
};

export function Badge({ tone = "neutral", className, ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-[var(--radius-full)] border px-2.5 py-0.5 text-[var(--text-xs)] font-medium",
        toneClasses[tone],
        className,
      )}
      {...props}
    />
  );
}
