import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import { createApiClient, fetchHealth } from "@/lib/api/client";

async function ApiStatus() {
  await connection();
  const status = await fetchHealth(
    createApiClient(process.env.API_URL ?? "http://localhost:8080"),
  );
  return (
    <p>
      Estado de la API:{" "}
      <strong>{status === "ok" ? "disponible" : "no disponible"}</strong>
    </p>
  );
}

export default function Home() {
  return (
    <main className="mx-auto flex min-h-screen max-w-xl flex-col justify-center gap-4 p-8">
      <h1 className="text-[var(--text-2xl)] font-semibold">agenda-clinicas</h1>
      <Suspense fallback={<p>Estado de la API: comprobando...</p>}>
        <ApiStatus />
      </Suspense>
      <nav
        aria-label="Accesos rápidos"
        className="flex flex-wrap gap-3 text-[var(--text-sm)] font-medium"
      >
        <Link
          href="/login"
          className="rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2 text-[var(--accent-text)]"
        >
          Iniciar sesión
        </Link>
        <Link
          href="/invitacion"
          className="rounded-[var(--radius-md)] border border-[var(--border-strong)] px-4 py-2"
        >
          Activar invitación
        </Link>
        <Link
          href="/panel/seguridad"
          className="rounded-[var(--radius-md)] border border-[var(--border-strong)] px-4 py-2"
        >
          Seguridad
        </Link>
        <Link
          href="/panel/roles"
          className="rounded-[var(--radius-md)] border border-[var(--border-strong)] px-4 py-2"
        >
          Roles y equipo
        </Link>
      </nav>
    </main>
  );
}
