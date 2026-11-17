import { defineConfig, devices } from "@playwright/test";

const mongoUri =
  process.env.MONGO_URI ?? "mongodb://localhost:27017/?replicaSet=rs0";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: "list",
  use: {
    baseURL: "http://localhost:3000",
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command: "go run ./cmd/api",
      cwd: "../backend",
      url: "http://localhost:8080/healthz",
      reuseExistingServer: !process.env.CI,
      timeout: 60_000,
      env: {
        APP_ENV: "development",
        HTTP_ADDR: ":8080",
        MONGO_URI: mongoUri,
        WEBAUTHN_RP_ID: "localhost",
        WEBAUTHN_RP_ORIGINS: "http://localhost:3000",
        CORS_ALLOWED_ORIGINS: "http://localhost:3000",
      },
    },
    {
      command: "npm run dev -- --port 3000",
      cwd: ".",
      url: "http://localhost:3000",
      reuseExistingServer: !process.env.CI,
      timeout: 60_000,
      env: {
        API_URL: "http://localhost:8080",
        NEXT_PUBLIC_API_URL: "http://localhost:8080",
      },
    },
  ],
});
