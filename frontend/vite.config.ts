import path from "node:path"
import { createRequire } from "node:module"
import { defineConfig } from "vitest/config"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

const { version } = createRequire(import.meta.url)("./package.json")

// https://vite.dev/config/
export default defineConfig({
  // La versión que muestra el pie de la aplicación sale de package.json y no
  // de una constante escrita a mano: una constante se olvida de actualizar
  // justo cuando importa, que es cuando alguien reporta un problema y hay
  // que saber qué versión está corriendo.
  define: {
    __VERSION__: JSON.stringify(version),
  },
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    exclude: ["**/node_modules/**", "**/e2e/**"],
    // Tiene que ser mayor que el asyncUtilTimeout de Testing Library (ver
    // src/test/setup.ts): si fueran iguales, un findBy* que agota su espera
    // mata el test por timeout antes de poder fallar con su propio mensaje,
    // que es el que dice qué elemento no apareció.
    testTimeout: 20000,
  },
  server: {
    // En dev, /api se resuelve contra el backend local sin pelear con CORS
    // (ver docs/09-seguridad-rbac.md §4 — FRONTEND_ORIGIN es para cuando no
    // se usa este proxy, ej. build de producción same-origin detrás de
    // Cloudflare Tunnel).
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
})
