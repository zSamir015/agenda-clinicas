"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type ReactNode } from "react";
import { Badge, Button, ThemeToggle } from "@/components/ui";
import { createBrowserApiClient, type SessionInfo } from "@/lib/api/client";

export interface AppShellProps {
  session?: SessionInfo | null;
  onSessionCleared?: () => void;
  children: ReactNode;
}

export function AppShell({
  session,
  onSessionCleared,
  children,
}: AppShellProps) {
  const router = useRouter();
  const [loggingOut, setLoggingOut] = useState(false);

  async function handleLogout() {
    if (!session || loggingOut) {
      return;
    }
    setLoggingOut(true);
    try {
      const api = createBrowserApiClient();
      await api.POST("/auth/logout", {
        headers: {
          "X-CSRF-Token": session.csrfToken,
        },
      });
      onSessionCleared?.();
      router.push("/login");
    } finally {
      setLoggingOut(false);
    }
  }

  return (
    <div className="min-h-dvh flex flex-col bg-[var(--bg-canvas)] text-[var(--text-primary)]">
      <header className="border-b border-[var(--border-default)] bg-[var(--bg-surface)]">
        <div className="mx-auto flex w-full max-w-5xl flex-wrap items-center justify-between gap-4 px-4 py-3">
          <div className="flex flex-wrap items-center gap-4">
            <Link
              href="/"
              className="text-[var(--text-lg)] font-bold tracking-tight text-[var(--text-primary)] hover:text-[var(--accent)]"
            >
              Agenda Clínicas
            </Link>
            <nav
              aria-label="Navegación principal"
              className="flex flex-wrap items-center gap-2 text-[var(--text-sm)]"
            >
              <Link
                href="/panel/seguridad"
                className="rounded-[var(--radius-md)] px-3 py-2 font-medium text-[var(--text-secondary)] hover:bg-[var(--bg-subtle)] hover:text-[var(--text-primary)]"
              >
                Seguridad
              </Link>
              <Link
                href="/panel/roles"
                className="rounded-[var(--radius-md)] px-3 py-2 font-medium text-[var(--text-secondary)] hover:bg-[var(--bg-subtle)] hover:text-[var(--text-primary)]"
              >
                Roles y equipo
              </Link>
              {!session ? (
                <Link
                  href="/login"
                  className="rounded-[var(--radius-md)] px-3 py-2 font-medium text-[var(--accent)] hover:bg-[var(--accent-subtle-bg)]"
                >
                  Iniciar sesión
                </Link>
              ) : null}
            </nav>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            {session ? (
              <>
                <Badge tone="info">
                  {session.clinicName}
                  {session.roleName ? ` · ${session.roleName}` : ""}
                </Badge>
                <Button
                  variant="secondary"
                  size="sm"
                  loading={loggingOut}
                  onClick={handleLogout}
                >
                  Cerrar sesión
                </Button>
              </>
            ) : null}
            <ThemeToggle />
          </div>
        </div>
      </header>

      <main className="mx-auto flex w-full max-w-5xl flex-1 flex-col px-4 py-8">
        {children}
      </main>
    </div>
  );
}
