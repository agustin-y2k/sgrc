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
  // Al tope antes de disparar: la barra de navegación es `sticky top-0`, así que
  // en una captura de página completa se dibuja donde esté el scroll en ese
  // momento. Dos corridas del mismo commit daban la misma pantalla con la barra
  // arriba en una y por la mitad en la otra —10% de píxeles distintos, suficiente
  // para que el portón la marque como cambiada sin que nada haya cambiado—.
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.waitForTimeout(1000)
  await page.screenshot({ path: `${SALIDA}/${nombre}.png`, fullPage: true })
  console.log("  ✓", nombre)
}
/**
 * Un teléfono de 390 px de ancho, que es el que declara el viewport de la app.
 * DPR 3 porque es lo que tiene cualquier teléfono de los últimos diez años, y
 * porque la captura se sirve después a tamaño real: con DPR 1 el texto saldría
 * borroso apenas se lo mira en una pantalla densa.
 */
const TELEFONO = {
  viewport: { width: 390, height: 844 },
  deviceScaleFactor: 3,
  isMobile: true,
  hasTouch: true,
}

// ── Los recorridos ──────────────────────────────────────────────────────────
// Cada uno se hace dos veces: una en escritorio, que es lo que va al README, y
// otra en un teléfono, que termina en `-movil`. Las de teléfono existen para la
// galería del sitio: abajo de cierto ancho no puede mostrar una página de
// 1440 px encogida —el texto queda en tres píxeles— y necesita la interfaz tal
// como el sistema la arma en un teléfono, no un recorte de la de escritorio.

async function acceso(nav, opciones, sufijo) {
  const page = await (await contexto(nav, opciones)).newPage()
  await page.goto(`${BASE}/login`)
  await foto(page, `00-acceso${sufijo}`)
}

async function admin(nav, opciones, sufijo) {
  const page = await (await contexto(nav, opciones)).newPage()
  await login(page, "admin")
  await foto(page, `01-mostrador${sufijo}`)
  await page.goto(`${BASE}/admin/inventario`)
  await page.getByRole("button", { name: /gestionar equipos/i }).first().click().catch(() => {})
  await foto(page, `04-inventario-admin${sufijo}`)
  await page.goto(`${BASE}/admin/reportes`); await foto(page, `05-reportes${sufijo}`)
  await page.goto(`${BASE}/admin/entregas`); await foto(page, `06-entregas${sufijo}`)
  await page.goto(`${BASE}/admin/licencias`); await foto(page, `07-licencias${sufijo}`)
  await page.goto(`${BASE}/admin/academico`)
  await page.getByRole("button", { name: /^cursos$/i }).first().click().catch(() => {})
  await page.waitForTimeout(500)
  await page.getByRole("button", { name: /^materias$/i }).first().click().catch(() => {})
  await foto(page, `08-academico${sufijo}`)
}

async function oscuro(nav, opciones, sufijo) {
  const page = await (await contexto(nav, { ...opciones, tema: "oscuro", colorScheme: "dark" })).newPage()
  await login(page, "admin")
  await page.goto(`${BASE}/admin/reportes`); await foto(page, `09-reportes-oscuro${sufijo}`)
}

async function docente(nav, opciones, sufijo) {
  const page = await (await contexto(nav, opciones)).newPage()
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
  await foto(page, `02-nueva-reserva${sufijo}`)
  await page.goto(`${BASE}/reservas`); await foto(page, `03-mis-reservas${sufijo}`)
  await page.goto(`${BASE}/inventario`)
  await page.getByRole("button", { name: /ver equipos/i }).first().click().catch(() => {})
  await foto(page, `10-inventario-docente${sufijo}`)
}

const nav = await chromium.launch()

for (const [opciones, sufijo] of [
  [{}, ""],
  [TELEFONO, "-movil"],
]) {
  await acceso(nav, opciones, sufijo)
  await admin(nav, opciones, sufijo)
  await oscuro(nav, opciones, sufijo)
  await docente(nav, opciones, sufijo)
}

// El inicio del docente en el teléfono. Es la única pantalla que se captura
// solo en un tamaño: es la que muestra que la aplicación se usa desde el aula,
// y en escritorio no diría nada que las otras no digan ya.
{
  const page = await (await contexto(nav, TELEFONO)).newPage()
  await login(page, "docente")
  await foto(page, "11-movil")
}
await nav.close()
