import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import tsconfigPaths from "vite-tsconfig-paths";

export default defineConfig(({ mode }) => {
  const apiPort = mode === "test" ? 3001 : 3000;
  return {
    plugins: [react(), tailwindcss(), tsconfigPaths()],
    build: {
      chunkSizeWarningLimit: 600,
      rollupOptions: {
        output: {
          // Pin only the framework runtime into a long-lived cacheable
          // chunk. Everything else is left to Rollup's automatic
          // splitting so lazy routes/tabs keep their own async chunks —
          // forcing a generic `vendor` chunk here would pull lazy-only
          // deps (recharts, react-day-picker) back into the entry.
          manualChunks(id) {
            if (
              id.includes("node_modules/react/") ||
              id.includes("node_modules/react-dom/") ||
              id.includes("node_modules/scheduler/")
            ) {
              return "react-vendor";
            }
          },
        },
      },
    },
    server: {
      port: 5173,
      proxy: {
        // During local dev, proxy API requests to the Go backend
        "/api": {
          target: `http://localhost:${apiPort}`,
          changeOrigin: true,
        },
      },
    },
  };
});
