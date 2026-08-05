import { existsSync, readFileSync } from "node:fs"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"

import { defineConfig, devices } from "@playwright/test"

/**
 * Las credenciales del Admin salen del `.env` del proyecto, que es el mismo
 * del que las toma el backend para sembrarlo. Sin esto había que copiarlas a
 * mano en variables E2E_*, y el test se salteaba en silencio si te
 * olvidabas — un test que no corre no avisa de nada.
 *
 * Se parsea a mano en vez de sumar dotenv: son cuatro líneas y evita una
 * dependencia para algo que solo usan los E2E.
 */
function cargarEnvDelProyecto(): Record<string, string> {
  // El proyecto es ESM, así que no hay __dirname.
  const aca = dirname(fileURLToPath(import.meta.url))
  const ruta = resolve(aca, "..", ".env")
  if (!existsSync(ruta)) return {}

  const valores: Record<string, string> = {}
  for (const linea of readFileSync(ruta, "utf8").split("\n")) {
    const limpia = linea.trim()
    if (limpia === "" || limpia.startsWith("#")) continue
    const i = limpia.indexOf("=")
    if (i > 0) valores[limpia.slice(0, i)] = limpia.slice(i + 1)
  }
  return valores
}

const env = cargarEnvDelProyecto()
process.env.E2E_ADMIN_EMAIL ??= env.SEED_ADMIN_EMAIL
process.env.E2E_ADMIN_PASSWORD ??= env.SEED_ADMIN_PASSWORD

// E2E — cubre el flujo crítico de docs/10-testing.md: login → reservar →
// cancelar, contra el sistema real.
//
// Con `make run` no hay nada que configurar: levanta el backend, publica la
// SPA compilada en :8081 (el mismo nginx que corre en el servidor) y siembra
// el docente de prueba que estos specs usan por defecto.
//
// Se corren a mano y no en cada `make test` porque necesitan todo el stack
// levantado. Contra otro entorno:
//
//   E2E_BASE_URL=... E2E_DOCENTE_EMAIL=... E2E_DOCENTE_PASSWORD=... npx playwright test
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  retries: 0,
  reporter: "list",
  use: {
    // :8081 es la SPA compilada servida por nginx — lo que realmente se
    // despliega. Vite (:5173) sirve para desarrollar, pero no usa el
    // nginx.conf ni el build de producción.
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:8081",
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
})
