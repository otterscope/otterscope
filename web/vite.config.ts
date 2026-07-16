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
});
