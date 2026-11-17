import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LoginForm } from "./login-form";
import { InvitationFlow } from "./invitation-flow";
import { RolesPanel } from "./roles-panel";

const pushMock = vi.fn();
let searchParamValues: Record<string, string> = {};

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
  useSearchParams: () => ({
    get: (key: string) => searchParamValues[key] ?? null,
  }),
}));

describe("Auth, invitación y roles UI", () => {
  beforeEach(() => {
    pushMock.mockReset();
    searchParamValues = {};
    vi.restoreAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("permite login con contraseña y TOTP y toggling de contraseña accesible", async () => {
    searchParamValues = { clinica: "clinica-panama" };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          kind: "staff",
          clinicId: "c1",
          clinicSlug: "clinica-panama",
          clinicName: "Clínica Panamá",
          csrfToken: "csrf-123",
          expiresAt: "2026-11-17T23:00:00Z",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    render(<LoginForm />);

    const emailInput = screen.getByLabelText(/Correo del personal/i);
    expect(emailInput).toHaveAttribute("autocomplete", "username webauthn");

    fireEvent.change(emailInput, { target: { value: "dra.ana@demo.example" } });
    fireEvent.click(
      screen.getByRole("button", {
        name: /Usar contraseña y TOTP como respaldo/i,
      }),
    );

    const passwordInput = screen.getByLabelText(/^Contraseña/i);
    expect(passwordInput).toHaveAttribute("type", "password");

    const toggleBtn = screen.getByRole("button", {
      name: /Mostrar contraseña/i,
    });
    expect(toggleBtn).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(toggleBtn);
    expect(passwordInput).toHaveAttribute("type", "text");
    expect(toggleBtn).toHaveAttribute("aria-pressed", "true");

    fireEvent.change(passwordInput, {
      target: { value: "ClaveSuperSegura2026" },
    });
    fireEvent.change(screen.getByLabelText(/Código TOTP o de recuperación/i), {
      target: { value: "123456" },
    });

    fireEvent.click(
      screen.getByRole("button", { name: /Entrar con contraseña y TOTP/i }),
    );

    await waitFor(() => {
      expect(pushMock).toHaveBeenCalledWith("/panel/seguridad");
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("previsualiza y acepta una invitación mostrando opciones de passkey y TOTP", async () => {
    searchParamValues = { token: "invite-tok-1" };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            email: "nuevo@demo.example",
            name: "Dr. Luis Vega",
            clinicSlug: "clinica-panama",
            clinicName: "Clínica Panamá",
            roleName: "practitioner",
            expiresAt: "2026-11-24T00:00:00Z",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            kind: "staff",
            clinicId: "c1",
            clinicSlug: "clinica-panama",
            clinicName: "Clínica Panamá",
            csrfToken: "csrf-new",
            expiresAt: "2026-11-17T23:00:00Z",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);

    render(<InvitationFlow />);

    expect(
      await screen.findByText(/Dr\. Luis Vega \(nuevo@demo\.example\)/i),
    ).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: /Aceptar invitación y continuar/i }),
    );

    expect(
      await screen.findByRole("button", { name: /Registrar passkey/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Generar clave TOTP/i }),
    ).toBeInTheDocument();
  });

  it("muestra roles, bloquea owner y envía creación de rol con X-CSRF-Token", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url =
        typeof input === "string"
          ? input
          : input instanceof Request
            ? input.url
            : input.toString();
      if (url.endsWith("/auth/session")) {
        return new Response(
          JSON.stringify({
            kind: "staff",
            clinicId: "c1",
            clinicSlug: "clinica-panama",
            clinicName: "Clínica Panamá",
            csrfToken: "csrf-owner",
            expiresAt: "2026-11-17T23:00:00Z",
            userId: "u1",
            roleName: "owner",
            grants: [
              { permission: "roles.manage", scope: "all" },
              { permission: "appointments.read", scope: "all" },
            ],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.endsWith("/rbac/permissions")) {
        return new Response(
          JSON.stringify({
            permissions: ["appointments.read", "roles.manage"],
            scopes: ["own", "all"],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.endsWith("/rbac/roles")) {
        return new Response(
          JSON.stringify({
            roles: [
              {
                id: "r-owner",
                name: "owner",
                template: true,
                locked: true,
                grants: [{ permission: "roles.manage", scope: "all" }],
                createdAt: "2026-11-17T00:00:00Z",
                updatedAt: "2026-11-17T00:00:00Z",
              },
            ],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      return new Response(JSON.stringify({}), { status: 201 });
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<RolesPanel />);

    expect(await screen.findByText("Bloqueado")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/Nombre del rol/i), {
      target: { value: "Auditoría" },
    });
    fireEvent.change(screen.getByLabelText("Alcance para appointments.read"), {
      target: { value: "all" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Crear rol/i }));

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find((call) => {
        const req = call[0] as Request;
        return req.url?.endsWith("/rbac/roles") && req.method === "POST";
      });
      expect(postCall).toBeTruthy();
    });
  });
});
