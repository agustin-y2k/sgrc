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

// Las credenciales salen del entorno: este archivo vive en un repositorio
// público y la instalación local de cada uno tiene las suyas. Los valores por
// defecto son los que deja `datos-de-demostracion.sh`; la contraseña del Admin
// es la que haya en el .env del proyecto (SEED_ADMIN_PASSWORD).
const ADMIN_EMAIL = process.env.GUIA_ADMIN_EMAIL ?? "admin@escuela.edu.ar"
const ADMIN_PASSWORD = process.env.GUIA_ADMIN_PASSWORD ?? ""
const DOCENTE_EMAIL = process.env.GUIA_DOCENTE_EMAIL ?? "ana.gomez@escuela.edu.ar"
const DOCENTE_PASSWORD = process.env.GUIA_DOCENTE_PASSWORD ?? "guia.demo.2026"

const BASE = "http://localhost:8081"
const SALIDA = process.env.SALIDA
const nav = await chromium.launch()
const ctx = await nav.newContext({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 })
const page = await ctx.newPage()
await page.goto(`${BASE}/login`)
await page.getByLabel(/email/i).fill(ADMIN_EMAIL)
await page.getByLabel(/contraseña/i).fill(ADMIN_PASSWORD)
await page.getByRole("button", { name: /iniciar sesión/i }).click()
await page.waitForURL((u) => !u.pathname.endsWith("/login"))
const foto = async (n) => { await page.waitForTimeout(800); await page.screenshot({ path: `${SALIDA}/${n}.png`, fullPage: true }); console.log("  ✓", n) }
const paso = async (n, f) => { try { await f() } catch (e) { console.log("  ✗", n, String(e).split("\n")[0].slice(0,100)) } }

await paso("aprobacion", async () => { await page.goto(`${BASE}/admin/aprobacion`); await foto("adm2-aprobacion") })
await paso("academico", async () => {
  await page.goto(`${BASE}/admin/academico`)
  await page.getByRole("button", { name: /^cursos$/i }).first().click()
  await page.waitForTimeout(600)
  await page.getByRole("button", { name: /^materias$/i }).first().click()
  await foto("adm2-academico")
})
await paso("equipos", async () => {
  await page.goto(`${BASE}/admin/inventario`)
  await page.getByRole("button", { name: /gestionar equipos/i }).first().click()
  await foto("adm2-equipos")
})
await paso("licencias", async () => { await page.goto(`${BASE}/admin/licencias`); await foto("adm2-licencias") })
await paso("cargar licencia", async () => {
  await page.goto(`${BASE}/admin/licencias`)
  await page.getByRole("button", { name: /cargar una licencia/i }).first().click()
  await foto("adm2-cargar-licencia")
})
await paso("entregar", async () => {
  await page.goto(`${BASE}/admin/entregas`)
  await page.getByRole("button", { name: /entregar sin reserva/i }).first().click()
  await foto("adm2-entregar-sin-reserva")
})
await paso("bloquear lleno", async () => {
  await page.goto(`${BASE}/admin/bloquear-equipos`)
  await page.getByLabel(/fecha/i).fill("2026-08-24")
  const s = page.locator("select"); const n = await s.count()
  await s.nth(n - 4).selectOption("10"); await s.nth(n - 3).selectOption("00")
  await s.nth(n - 2).selectOption("11"); await s.nth(n - 1).selectOption("30")
  await page.waitForTimeout(1500)
  await foto("adm2-bloquear")
})
await paso("pedidos", async () => { await page.goto(`${BASE}/admin/pedidos-de-materia`); await foto("adm2-pedidos") })
await paso("reportes", async () => { await page.goto(`${BASE}/admin/reportes`); await foto("adm2-reportes") })
await nav.close()
