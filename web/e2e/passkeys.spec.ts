import AxeBuilder from "@axe-core/playwright";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";

const wcagTags = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];

interface E2ESeed {
  clinicSlug: string;
  clinicName: string;
  email: string;
  inviteToken: string;
}

function seedE2EClinic(): E2ESeed {
  const backendDir = path.resolve(process.cwd(), "../backend");
  const mongoUri =
    process.env.MONGO_URI ?? "mongodb://localhost:27017/?replicaSet=rs0";
  const stdout = execFileSync("go", ["run", "./cmd/e2eseed"], {
    cwd: backendDir,
    env: { ...process.env, MONGO_URI: mongoUri },
    encoding: "utf8",
  });
  return JSON.parse(stdout) as E2ESeed;
}

async function expectAccessible(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(wcagTags).analyze();
  expect(results.violations).toEqual([]);
}

test.describe("Flujo de passkeys WebAuthn y accesibilidad WCAG 2.2 AA", () => {
  test("login y activación cumplen WCAG 2.2 AA en tema claro y oscuro", async ({
    page,
  }) => {
    const seed = seedE2EClinic();

    for (const colorScheme of ["light", "dark"] as const) {
      await page.emulateMedia({ colorScheme });

      await page.goto("/login");
      await expect(
        page.getByRole("heading", { name: "Iniciar sesión" }),
      ).toBeVisible();
      await expectAccessible(page);

      await page.goto(
        `/invitacion?token=${encodeURIComponent(seed.inviteToken)}`,
      );
      await expect(
        page.getByText(new RegExp(seed.clinicName, "i")),
      ).toBeVisible();
      await expectAccessible(page);
    }
  });

  test("registra passkey al aceptar invitación, cierra sesión y reingresa con autenticador virtual", async ({
    page,
  }) => {
    const seed = seedE2EClinic();

    const cdp = await page.context().newCDPSession(page);
    await cdp.send("WebAuthn.enable");
    await cdp.send("WebAuthn.addVirtualAuthenticator", {
      options: {
        protocol: "ctap2",
        transport: "internal",
        hasResidentKey: true,
        hasUserVerification: true,
        isUserVerified: true,
        automaticPresenceSimulation: true,
      },
    });

    // 1. Previsualizar y aceptar invitación.
    await page.goto(
      `/invitacion?token=${encodeURIComponent(seed.inviteToken)}`,
    );
    await expect(page.getByText(/Rol asignado: owner/i)).toBeVisible();

    await page
      .getByRole("button", { name: /Aceptar invitación y continuar/i })
      .click();

    await expect(
      page.getByRole("button", { name: /Registrar passkey/i }),
    ).toBeVisible();

    // 2. Registrar primera passkey contra go-webauthn + MongoDB.
    await page.getByRole("button", { name: /Registrar passkey/i }).click();
    await expect(page.getByText("Registrada", { exact: true })).toBeVisible();

    // 3. Verificar en /panel/seguridad y probar WCAG 2.2 AA autenticada.
    await page
      .getByRole("link", { name: /Ir a configuración de seguridad/i })
      .click();
    await expect(
      page.getByRole("heading", { name: "Seguridad de la cuenta" }),
    ).toBeVisible();
    await expect(page.getByText("1 activas")).toBeVisible();
    await expectAccessible(page);

    // 4. Verificar /panel/roles y crear un rol personalizado.
    await page.getByRole("link", { name: "Roles y equipo" }).click();
    await expect(
      page.getByRole("heading", { name: "Roles y equipo de la clínica" }),
    ).toBeVisible();
    await expect(page.getByText("Bloqueado", { exact: true })).toBeVisible();
    await expectAccessible(page);

    await page.getByLabel(/Nombre del rol/i).fill("Asistente");
    await page.getByLabel("Alcance para appointments.read").selectOption("all");
    await page.getByRole("button", { name: /Crear rol/i }).click();
    await expect(
      page.locator("span.font-semibold", { hasText: "Asistente" }),
    ).toBeVisible();

    // 5. Cerrar sesión y volver a entrar únicamente con passkey.
    await page.getByRole("button", { name: /Cerrar sesión/i }).click();
    await expect(
      page.getByRole("heading", { name: "Iniciar sesión" }),
    ).toBeVisible();

    await page.getByLabel(/Clínica \(subdominio\)/i).fill(seed.clinicSlug);
    await page.getByLabel(/Correo del personal/i).fill(seed.email);
    await page.getByRole("button", { name: /Entrar con passkey/i }).click();

    await expect(
      page.getByRole("heading", { name: "Seguridad de la cuenta" }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText("1 activas")).toBeVisible();
  });
});
