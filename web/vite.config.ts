import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // the Go server from `make run` answers the API and the event stream
    proxy: { "/api": "http://localhost:8080" },
  },
});
