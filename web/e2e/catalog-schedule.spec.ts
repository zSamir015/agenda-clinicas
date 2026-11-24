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

async function loginAsSeededOwner(page: Page, seed: E2ESeed) {
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

  await page.goto(`/invitacion?token=${encodeURIComponent(seed.inviteToken)}`);
  await page
    .getByRole("button", { name: /Aceptar invitación y continuar/i })
    .click();
  await page.getByRole("button", { name: /Registrar passkey/i }).click();
  await expect(page.getByText("Registrada", { exact: true })).toBeVisible();
}

test.describe("Catálogo, horarios y disponibilidad con WCAG 2.2 AA", () => {
  test("configura catálogo, reglas y bloqueos y verifica disponibilidad y accesibilidad", async ({
    page,
  }) => {
    const seed = seedE2EClinic();
    await loginAsSeededOwner(page, seed);

    // 1. Navegar a /panel/catalogo y auditar WCAG 2.2 AA.
    await page.getByRole("link", { name: "Catálogo" }).click();
    await expect(
      page.getByRole("heading", { name: "Catálogo de la clínica" }),
    ).toBeVisible();
    await expectAccessible(page);

    // Crear servicio.
    await page.getByLabel("Nombre del servicio").fill("Consulta general");
    await page.getByLabel("Duración (min)").fill("30");
    await page.getByLabel("Margen (min)").fill("0");
    await page.getByLabel("Tipo de consultorio requerido").fill("consultorio");
    await page.getByRole("button", { name: "Crear servicio" }).click();
    await expect(page.getByText("Servicio creado.")).toBeVisible();
    await expect(
      page.getByText("Consulta general", { exact: true }),
    ).toBeVisible();

    // Crear consultorio.
    await page.getByRole("tab", { name: /Consultorios/i }).click();
    await page.getByLabel("Nombre").fill("Consultorio 1");
    await page.getByLabel("Tipo de consultorio").fill("consultorio");
    await page.getByRole("button", { name: "Crear consultorio" }).click();
    await expect(page.getByText("Consultorio creado.")).toBeVisible();
    await expect(
      page.getByText("Consultorio 1", { exact: true }),
    ).toBeVisible();

    // Crear profesional asignado al servicio.
    await page.getByRole("tab", { name: /Profesionales/i }).click();
    await page.getByLabel("Nombre completo").fill("Dra. Sofía Mendoza");
    await page.getByLabel("Especialidad").fill("Medicina General");
    await page.getByLabel("Consulta general").check();
    await page.getByRole("button", { name: "Crear profesional" }).click();
    await expect(page.getByText("Profesional creado.")).toBeVisible();
    await expect(
      page.getByText("Dra. Sofía Mendoza", { exact: true }),
    ).toBeVisible();

    // 2. Navegar a /panel/horarios y auditar WCAG 2.2 AA (claro y oscuro).
    await page.getByRole("link", { name: "Horarios" }).click();
    await expect(
      page.getByRole("heading", {
        name: "Horarios, bloqueos y disponibilidad",
      }),
    ).toBeVisible();
    await expectAccessible(page);
    await page.emulateMedia({ colorScheme: "dark" });
    await expectAccessible(page);
    await page.emulateMedia({ colorScheme: "light" });

    // Agregar regla los lunes 09:00–11:00.
    await page.getByLabel("Día de la semana").selectOption("1");
    await page.getByLabel("Inicio (HH:MM)").fill("09:00");
    await page.getByLabel("Fin (HH:MM)").fill("11:00");
    await page.getByRole("button", { name: "Agregar regla" }).click();
    await expect(page.getByText("Regla semanal agregada.")).toBeVisible();
    await expect(page.getByText(/Lunes · 09:00–11:00/)).toBeVisible();

    // Consultar disponibilidad para el lunes 2026-12-14.
    await page.getByLabel("Fecha (Panamá)").fill("2026-12-14");
    await page.getByRole("button", { name: "Consultar horarios" }).click();
    await expect(
      page.locator("li", { hasText: /^09:00 · .*Consultorio 1$/ }),
    ).toBeVisible();
    await expect(
      page.locator("li", { hasText: /^09:30 · .*Consultorio 1$/ }),
    ).toBeVisible();
    await expect(
      page.locator("li", { hasText: /^10:00 · .*Consultorio 1$/ }),
    ).toBeVisible();
    await expect(
      page.locator("li", { hasText: /^10:30 · .*Consultorio 1$/ }),
    ).toBeVisible();

    // Agregar bloqueo 09:30–10:00 y verificar que el espacio 09:30 desaparece.
    await page.getByLabel("Motivo").fill("Reunión clínica");
    await page.getByLabel("Desde").fill("2026-12-14T09:30");
    await page.getByLabel("Hasta").fill("2026-12-14T10:00");
    await page.getByRole("button", { name: "Crear bloqueo" }).click();
    await expect(page.getByText("Bloqueo registrado.")).toBeVisible();

    await page.getByRole("button", { name: "Consultar horarios" }).click();
    await expect(
      page.locator("li", { hasText: /^09:00 · .*Consultorio 1$/ }),
    ).toBeVisible();
    await expect(
      page.locator("li", { hasText: /^09:30 · .*Consultorio 1$/ }),
    ).toHaveCount(0);
    await expect(
      page.locator("li", { hasText: /^10:00 · .*Consultorio 1$/ }),
    ).toBeVisible();
  });
});
