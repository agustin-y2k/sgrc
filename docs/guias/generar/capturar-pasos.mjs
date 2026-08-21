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
async function login(page, quien) {
  const { email, pass } = CUENTAS[quien]
  await page.goto(`${BASE}/login`)
  await page.getByLabel(/email/i).fill(email)
  await page.getByLabel(/contraseña/i).fill(pass)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()
  await page.waitForURL((u) => !u.pathname.endsWith("/login"), { timeout: 15000 })
}
async function foto(page, nombre, completa = true) {
  await page.waitForTimeout(700)
  await page.screenshot({ path: `${SALIDA}/${nombre}.png`, fullPage: completa })
  console.log("  ✓", nombre)
}
async function paso(nombre, fn) {
  try { await fn() } catch (e) { console.log("  ✗", nombre, "→", String(e).split("\n")[0].slice(0, 120)) }
}
const nav = await chromium.launch()

// ── Docente ────────────────────────────────────────────────────────
{
  const ctx = await nav.newContext({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 })
  const page = await ctx.newPage()
  await login(page, "docente")
  console.log("docente")

  await paso("form lleno", async () => {
    await page.goto(`${BASE}/reservas/nueva`)
    await page.getByLabel(/materia/i).selectOption({ label: "Programación · 1°A" }).catch(async () => {
      await page.getByLabel(/materia/i).selectOption({ index: 1 })
    })
    await page.getByLabel(/fecha/i).fill("2026-08-25")
    const selects = page.locator("select")
    const n = await selects.count()
    // hora inicio (hh, mm) y hora fin (hh, mm) son los selects que siguen al de materia
    await selects.nth(n - 4).selectOption("14")
    await selects.nth(n - 3).selectOption("00")
    await selects.nth(n - 2).selectOption("15")
    await selects.nth(n - 1).selectOption("30")
    await page.waitForTimeout(1500)
    await foto(page, "paso-01-reserva-llena")
    const casillas = page.getByRole("checkbox")
    const c = await casillas.count()
    for (let i = 0; i < Math.min(3, c); i++) await casillas.nth(i).check().catch(() => {})
    await foto(page, "paso-02-reserva-tildada")
  })

  await paso("recurrente", async () => {
    await page.goto(`${BASE}/reservas/nueva`)
    await page.getByRole("button", { name: /se repite todas las semanas/i }).click()
    await foto(page, "paso-03-reserva-semanal")
  })

  await paso("cancelar", async () => {
    await page.goto(`${BASE}/reservas`)
    await page.getByRole("button", { name: /^cancelar$/i }).first().click()
    await foto(page, "paso-04-cancelar-reserva")
    await page.keyboard.press("Escape")
  })

  await paso("cambiar computadora", async () => {
    await page.goto(`${BASE}/reservas`)
    await page.getByRole("button", { name: /cambiar computadora/i }).first().click()
    await foto(page, "paso-05-cambiar-computadora")
    await page.keyboard.press("Escape")
  })

  await paso("conversación soporte", async () => {
    await page.goto(`${BASE}/notificaciones`)
    // La lista de conversaciones viene plegada: sin este clic, el «Ver» de
    // cada hilo todavía no existe en la página.
    await page.getByRole("button", { name: /ver conversaciones/i }).click()
    await page.waitForTimeout(600)
    await page.getByRole("button", { name: /^ver$/i }).first().click()
    await foto(page, "paso-06-conversacion")
  })

  await paso("preferencias correo", async () => {
    await page.goto(`${BASE}/notificaciones`)
    await page.getByRole("button", { name: /elegir cuáles/i }).click()
    await foto(page, "paso-07-copias-por-correo")
  })

  await paso("avisar que no anda", async () => {
    await page.goto(`${BASE}/inventario`)
    await foto(page, "paso-08-computadoras-docente")
    await page.getByRole("button", { name: /ver equipos/i }).first().click()
    await page.waitForTimeout(600)
    await page.getByRole("button", { name: /reportar problema/i }).first().click()
    await foto(page, "paso-09-avisar-falla")
  })

  await paso("perfil", async () => {
    await page.goto(`${BASE}/perfil`)
    await foto(page, "paso-10-perfil")
  })

  await ctx.close()
}

// ── Admin ──────────────────────────────────────────────────────────
{
  const ctx = await nav.newContext({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 })
  const page = await ctx.newPage()
  await login(page, "admin")
  console.log("admin")

  await paso("entregar sin reserva", async () => {
    await page.goto(`${BASE}/`)
    await page.getByRole("button", { name: /entregar sin reserva/i }).first().click()
    await foto(page, "paso-20-entregar-sin-reserva")
    await page.keyboard.press("Escape")
  })

  await paso("menú administración", async () => {
    await page.goto(`${BASE}/`)
    await page.getByRole("button", { name: /administración/i }).click()
    await foto(page, "paso-21-menu-administracion", false)
  })

  await paso("panel soporte admin", async () => {
    await page.goto(`${BASE}/notificaciones`)
    await page.getByRole("button", { name: /ver conversaciones/i }).click()
    await page.waitForTimeout(600)
    await foto(page, "paso-22-soporte-admin")
    await page.getByRole("button", { name: /^ver$/i }).first().click()
    await foto(page, "paso-23-soporte-conversacion")
  })

  await paso("nueva licencia", async () => {
    await page.goto(`${BASE}/admin/licencias`)
    await page.getByRole("button", { name: /cargar|nueva|agregar/i }).first().click()
    await foto(page, "paso-24-cargar-licencia")
  })

  await paso("bloquear equipos", async () => {
    await page.goto(`${BASE}/admin/bloquear-equipos`)
    await foto(page, "paso-25-bloquear")
  })

  await paso("alta de equipo", async () => {
    await page.goto(`${BASE}/admin/inventario`)
    await foto(page, "paso-26-inventario-admin")
  })

  await paso("aprobación", async () => {
    await page.goto(`${BASE}/admin/aprobacion`)
    await foto(page, "paso-27-aprobacion")
  })

  await ctx.close()
}
await nav.close()
console.log("listo")
