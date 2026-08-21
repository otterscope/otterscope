import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Dev server proxies API + health to a locally running `otterscope serve`
// so `npm run dev` gives hot reload against real data.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://localhost:8317",
      "/healthz": "http://localhost:8317",
    },
  },
  test: {
    // happy-dom gives the tests a `window` — the units under test read
    // location/history/localStorage directly.
    environment: "happy-dom",
    include: ["src/**/*.test.ts"],
  },
});
