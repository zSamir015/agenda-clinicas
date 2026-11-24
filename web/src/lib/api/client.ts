import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

export type ApiClient = ReturnType<typeof createClient<paths>>;
export type Problem = components["schemas"]["Problem"];
export type SessionInfo = components["schemas"]["Session"];
export type RoleDTO = components["schemas"]["Role"];
export type GrantDTO = components["schemas"]["Grant"];
export type PermissionDTO = components["schemas"]["Permission"];
export type ScopeDTO = components["schemas"]["Scope"];
export type MemberSummaryDTO = components["schemas"]["MemberSummary"];
export type SecurityStatusDTO = components["schemas"]["SecurityStatus"];
export type InvitationPreviewDTO = components["schemas"]["InvitationPreview"];
export type PractitionerDTO = components["schemas"]["Practitioner"];
export type RoomDTO = components["schemas"]["Room"];
export type ServiceDTO = components["schemas"]["Service"];
export type PaymentModeDTO = components["schemas"]["PaymentMode"];
export type AvailabilityRuleDTO = components["schemas"]["AvailabilityRule"];
export type BlockDTO = components["schemas"]["Block"];
export type BlockScopeDTO = components["schemas"]["BlockScope"];
export type AvailableSlotDTO = components["schemas"]["AvailableSlot"];

export function getBrowserApiUrl(): string {
  return process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
}

export function createApiClient(
  baseUrl: string,
  fetchImpl?: typeof fetch,
): ApiClient {
  return createClient<paths>({
    baseUrl,
    ...(fetchImpl ? { fetch: fetchImpl } : {}),
  });
}

export function createBrowserApiClient(
  baseUrl = getBrowserApiUrl(),
  fetchImpl?: typeof fetch,
): ApiClient {
  const boundFetch: typeof fetch = (input, init) => {
    const caller = fetchImpl ?? globalThis.fetch;
    return caller(input, { credentials: "include", ...init });
  };
  return createClient<paths>({
    baseUrl,
    fetch: boundFetch,
  });
}

export function problemMessage(error: unknown, fallback: string): string {
  if (error && typeof error === "object") {
    const maybe = error as Partial<Problem>;
    if (typeof maybe.detail === "string" && maybe.detail.trim() !== "") {
      return maybe.detail;
    }
    if (typeof maybe.title === "string" && maybe.title.trim() !== "") {
      return maybe.title;
    }
  }
  return fallback;
}

/**
 * Consulta GET /readyz usando el cliente tipado y devuelve "ok" si la API
 * responde 200 con `{ status: "ok" }`, o "unavailable" ante cualquier error
 * HTTP (por ejemplo 503) o fallo de red.
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
