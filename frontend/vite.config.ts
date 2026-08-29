// Zona fija para TODA la suite de tests, puesta ACÁ y no en el setup porque
// este archivo es el único que corre antes de que existan los workers —y los
// workers heredan el entorno— y porque tsconfig.app.json excluye los tipos de
// Node a propósito: el código de la aplicación no usa `process`.
//
// Sin esto la suite se comporta distinto en la máquina de quien programa
// (Argentina) que en CI, que corre en UTC, y el caso que más importa —un
// instante de las 23:40 cuyo día en UTC ya es el siguiente— deja de fallar
// justo donde tendría que hacerlo.
process.env.TZ = "America/Argentina/Buenos_Aires"

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
