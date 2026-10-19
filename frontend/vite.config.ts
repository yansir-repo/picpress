import react from "@vitejs/plugin-react";
import { defineConfig } from "vite-plus";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": new URL("./src", import.meta.url).pathname,
    },
  },
  server: {
    host: "127.0.0.1",
    port: 9245,
    strictPort: true,
  },
  build: {
    outDir: "dist",
    // Keep .gitkeep so a fresh clone can still satisfy go:embed.
    emptyOutDir: false,
  },
  fmt: {
    ignorePatterns: ["dist/**", "src/bindings/**"],
  },
  lint: {
    ignorePatterns: ["dist/**", "src/bindings/**"],
    plugins: ["react", "typescript"],
  },
  test: {
    include: ["src/**/*.test.ts"],
    environment: "node",
  },
  run: {
    cache: { scripts: false },
    tasks: {
      ci: {
        command: ["vp check", "vp test run", "vp build"],
        cache: false,
      },
    },
  },
});
