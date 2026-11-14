import type { HTMLAttributes } from "react";
import { cn } from "./cn";

export type AlertTone = "info" | "success" | "warning" | "danger";

export interface AlertProps extends HTMLAttributes<HTMLDivElement> {
  tone?: AlertTone;
  title?: string;
}

const toneClasses: Record<AlertTone, string> = {
  info: "bg-[var(--info-bg)] text-[var(--info-text)] border-[var(--info-border)]",
  success:
    "bg-[var(--success-bg)] text-[var(--success-text)] border-[var(--success-border)]",
  warning:
    "bg-[var(--warning-bg)] text-[var(--warning-text)] border-[var(--warning-border)]",
  danger:
    "bg-[var(--danger-bg)] text-[var(--danger-text)] border-[var(--danger-border)]",
};

export function Alert({
  tone = "info",
  title,
  className,
  children,
  ...props
}: AlertProps) {
  const role = tone === "danger" || tone === "warning" ? "alert" : "status";

  return (
    <div
      role={role}
      className={cn(
        "rounded-[var(--radius-md)] border px-4 py-3 text-[var(--text-sm)]",
        toneClasses[tone],
        className,
      )}
      {...props}
    >
      {title ? <p className="font-semibold">{title}</p> : null}
      <div>{children}</div>
    </div>
  );
}
