import createClient from "openapi-fetch";
import type { paths } from "./schema";

export type ApiClient = ReturnType<typeof createClient<paths>>;

export function createApiClient(
  baseUrl: string,
  fetchImpl?: typeof fetch,
): ApiClient {
  return createClient<paths>({
    baseUrl,
    ...(fetchImpl ? { fetch: fetchImpl } : {}),
  });
}

/**
 * Consulta GET /readyz usando el cliente tipado y devuelve "ok" si la API
 * responde 200 con `{ status: "ok" }`, o "unavailable" ante cualquier error
 * HTTP (por ejemplo 503) o fallo de red.
 *
 * Pistas:
 * - Usa `client.GET("/readyz", { cache: "no-store" })` para no cachear el estado en Next.js.
 * - `openapi-fetch` devuelve `{ data, error }` sin lanzar excepción en respuestas 4xx/5xx,
 *   pero sí puede lanzar si falla la red (`fetch` rechaza la promesa).
 * - Envuelve la llamada en `try / catch` y verifica `data?.status === "ok"`.
 */
export async function fetchHealth(
  client: ApiClient,
): Promise<"ok" | "unavailable"> {
  try {
    const { data, error } = await client.GET("/readyz", {
      cache: "no-store",
    });
    if (error || data?.status !== "ok") {
      return "unavailable";
    }
    return "ok";
  } catch {
    return "unavailable";
  }
}
