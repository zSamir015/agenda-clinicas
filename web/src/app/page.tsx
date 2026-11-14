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
    <main className="mx-auto flex min-h-screen max-w-xl flex-col justify-center gap-2 p-8">
      <h1 className="text-2xl font-semibold">agenda-clinicas</h1>
      <Suspense fallback={<p>Estado de la API: comprobando...</p>}>
        <ApiStatus />
      </Suspense>
    </main>
  );
}
