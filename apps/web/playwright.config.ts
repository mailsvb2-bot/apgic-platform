import { defineConfig } from "@playwright/test";

const defaultWebPort = 43110;
const defaultApiPort = 43131;
const ciOffset = process.env.CI ? process.pid % 1000 : 0;
const webPort = Number(process.env.APGIC_WEB_PORT ?? (defaultWebPort + ciOffset));
const apiPort = Number(process.env.APGIC_API_PORT ?? (defaultApiPort + ciOffset));
const baseURL = `http://127.0.0.1:${webPort}`;

export default defineConfig({
  testDir: "./e2e",
  snapshotPathTemplate: "{testDir}/snapshots/{projectName}/{arg}{ext}",
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "line" : "list",
  use: {
    baseURL,
    trace: "retain-on-failure",
  },
  webServer: {
    command: `APGIC_WEB_PORT=${webPort} APGIC_HTTP_ADDR=:${apiPort} APGIC_API_ORIGIN=http://127.0.0.1:${apiPort} bash scripts/start-with-api.sh`,
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
  },
  projects: [
    {
      name: "phone",
      use: { viewport: { width: 390, height: 844 } },
    },
    {
      name: "tablet",
      use: { viewport: { width: 768, height: 1024 } },
    },
    {
      name: "desktop",
      use: { viewport: { width: 1440, height: 900 } },
    },
  ],
});
