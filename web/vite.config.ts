/// <reference types="vitest/config" />
import { defineConfig } from "vite";
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
  // happy-dom, because the units under test read window.location, history
  // and localStorage — the browser surface is part of what they do.
  //
  // happy-dom rather than jsdom for its Node range: jsdom 30 declares
  // ^22.22.2 || ^24.15.0 || >=26.0.0, which excludes all of Node 20 and 23
  // and most of 24, so a contributor on Node 20 LTS cannot run the tests at
  // all. happy-dom needs >=20, matching the engines field. It is also about
  // 2.5x faster to set up.
  test: {
    environment: "happy-dom",
    include: ["src/**/*.test.ts"],
  },
});
