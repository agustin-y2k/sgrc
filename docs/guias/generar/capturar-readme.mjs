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
const CUENTAS = {
  admin: { email: ADMIN_EMAIL, pass: ADMIN_PASSWORD },
  docente: { email: DOCENTE_EMAIL, pass: DOCENTE_PASSWORD },
}
async function contexto(nav, opciones = {}) {
  const ctx = await nav.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2, ...opciones })
  if (opciones.tema) {
    await ctx.addInitScript((t) => localStorage.setItem("sgrc-tema", t), opciones.tema)
  }
  return ctx
}
async function login(page, quien) {
  await page.goto(`${BASE}/login`)
  await page.getByLabel(/email/i).fill(CUENTAS[quien].email)
  await page.getByLabel(/contraseña/i).fill(CUENTAS[quien].pass)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()
  await page.waitForURL((u) => !u.pathname.endsWith("/login"), { timeout: 15000 })
}
async function foto(page, nombre) {
  await page.waitForTimeout(1000)
  await page.screenshot({ path: `${SALIDA}/${nombre}.png`, fullPage: true })
  console.log("  ✓", nombre)
}
const nav = await chromium.launch()

// 00 acceso (sin sesión)
{
  const page = await (await contexto(nav)).newPage()
  await page.goto(`${BASE}/login`)
  await foto(page, "00-acceso")
}
// admin
{
  const page = await (await contexto(nav)).newPage()
  await login(page, "admin")
  await foto(page, "01-mostrador")
  await page.goto(`${BASE}/admin/inventario`)
  await page.getByRole("button", { name: /gestionar equipos/i }).first().click().catch(() => {})
  await foto(page, "04-inventario-admin")
  await page.goto(`${BASE}/admin/reportes`); await foto(page, "05-reportes")
  await page.goto(`${BASE}/admin/entregas`); await foto(page, "06-entregas")
  await page.goto(`${BASE}/admin/licencias`); await foto(page, "07-licencias")
  await page.goto(`${BASE}/admin/academico`)
  await page.getByRole("button", { name: /^cursos$/i }).first().click().catch(() => {})
  await page.waitForTimeout(500)
  await page.getByRole("button", { name: /^materias$/i }).first().click().catch(() => {})
  await foto(page, "08-academico")
}
// reportes en oscuro
{
  const page = await (await contexto(nav, { tema: "oscuro", colorScheme: "dark" })).newPage()
  await login(page, "admin")
  await page.goto(`${BASE}/admin/reportes`); await foto(page, "09-reportes-oscuro")
}
// docente
{
  const page = await (await contexto(nav)).newPage()
  await login(page, "docente")
  await page.goto(`${BASE}/reservas/nueva`)
  await page.getByLabel(/materia/i).selectOption({ index: 1 })
  await page.getByLabel(/fecha/i).fill("2026-08-25")
  const s = page.locator("select"); const n = await s.count()
  await s.nth(n - 4).selectOption("14"); await s.nth(n - 3).selectOption("00")
  await s.nth(n - 2).selectOption("15"); await s.nth(n - 1).selectOption("30")
  await page.waitForTimeout(1500)
  const c = page.getByRole("checkbox")
  for (let i = 0; i < Math.min(3, await c.count()); i++) await c.nth(i).check().catch(() => {})
  await foto(page, "02-nueva-reserva")
  await page.goto(`${BASE}/reservas`); await foto(page, "03-mis-reservas")
  await page.goto(`${BASE}/inventario`)
  await page.getByRole("button", { name: /ver equipos/i }).first().click().catch(() => {})
  await foto(page, "10-inventario-docente")
}
// móvil
{
  const ctx = await nav.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true })
  const page = await ctx.newPage()
  await login(page, "docente")
  await foto(page, "11-movil")
}
await nav.close()
