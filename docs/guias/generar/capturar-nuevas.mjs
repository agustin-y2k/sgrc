import { createRequire } from "node:module"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"

// Playwright se resuelve desde frontend/node_modules y no desde esta carpeta:
// Node busca los paquetes al lado del archivo que los importa, así que un
// `import` pelado falla apenas el script se corre desde otro lugar. Con esto
// anda desde cualquier directorio.
const aca = dirname(fileURLToPath(import.meta.url))
const requerir = createRequire(resolve(aca, "..", "..", "..", "frontend", "package.json"))
const { chromium } = requerir("@playwright/test")

// Las capturas de este archivo son las de los capítulos que se escribieron
// después de la primera versión de las guías: cerrar el año, el calendario de
// un equipo y el perfil. Van aparte y no dentro de capturar-admin.mjs porque
// dos de ellas necesitan la sesión del docente.
const ADMIN_EMAIL = process.env.GUIA_ADMIN_EMAIL ?? "admin@escuela.edu.ar"
const ADMIN_PASSWORD = process.env.GUIA_ADMIN_PASSWORD ?? ""
const DOCENTE_EMAIL = process.env.GUIA_DOCENTE_EMAIL ?? "ana.gomez@escuela.edu.ar"
const DOCENTE_PASSWORD = process.env.GUIA_DOCENTE_PASSWORD ?? "guia.demo.2026"

const BASE = "http://localhost:8081"
const SALIDA = process.env.SALIDA

const nav = await chromium.launch()
const ctxOpts = { viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 }

async function entrar(page, email, password) {
  await page.goto(`${BASE}/login`)
  await page.getByLabel(/email/i).fill(email)
  await page.getByLabel(/contraseña/i).fill(password)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()
  await page.waitForURL((u) => !u.pathname.endsWith("/login"))
}

const foto = async (page, n) => {
  await page.waitForTimeout(800)
  await page.screenshot({ path: `${SALIDA}/${n}.png`, fullPage: true })
  console.log("  ✓", n)
}
const paso = async (n, f) => {
  try { await f() } catch (e) { console.log("  ✗", n, String(e).split("\n")[0].slice(0, 100)) }
}

// El calendario se captura en la semana que tiene las reservas de
// demostración —la que viene, ver datos-de-demostracion.sh—, porque un
// calendario vacío no muestra ni los colores ni la referencia.
async function calendarioDelPrimerEquipo(page, nombre) {
  await page.goto(`${BASE}/inventario`)
  await page.getByRole("button", { name: /ver equipos/i }).first().click()
  await page.waitForTimeout(600)
  await page.getByRole("link", { name: /calendario/i }).first().click()
  await page.waitForTimeout(600)
  await page.getByRole("button", { name: /semana siguiente/i }).click()
  await foto(page, nombre)
}

{
  const ctx = await nav.newContext(ctxOpts)
  const page = await ctx.newPage()
  await entrar(page, ADMIN_EMAIL, ADMIN_PASSWORD)

  await paso("cerrar el año", async () => {
    await page.goto(`${BASE}/admin/academico`)
    await page.getByRole("button", { name: /cerrar el año/i }).first().click()
    await foto(page, "nue-cerrar-anio")
  })

  await paso("calendario del equipo (admin)", async () => {
    await calendarioDelPrimerEquipo(page, "nue-calendario-equipo-admin")
  })

  await paso("perfil del admin", async () => {
    await page.goto(`${BASE}/perfil`)
    await foto(page, "nue-perfil-admin")
  })

  await ctx.close()
}

{
  const ctx = await nav.newContext(ctxOpts)
  const page = await ctx.newPage()
  await entrar(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)

  await paso("calendario del equipo (docente)", async () => {
    await calendarioDelPrimerEquipo(page, "nue-calendario-equipo-docente")
  })

  await ctx.close()
}

await nav.close()
