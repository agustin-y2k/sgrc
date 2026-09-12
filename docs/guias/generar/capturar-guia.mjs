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
import { mkdirSync } from "node:fs"

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
mkdirSync(SALIDA, { recursive: true })

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
  await page.waitForLoadState("networkidle")
}

async function foto(page, nombre, { completa = true } = {}) {
  await page.waitForLoadState("networkidle").catch(() => {})
  await page.waitForTimeout(600)
  await page.screenshot({ path: `${SALIDA}/${nombre}.png`, fullPage: completa })
  console.log("  ✓", nombre)
}

const navegador = await chromium.launch()

// ── Pantallas públicas ──────────────────────────────────────────────
{
  const ctx = await navegador.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2 })
  const page = await ctx.newPage()
  console.log("público")
  await page.goto(`${BASE}/login`); await foto(page, "pub-01-login")
  await page.goto(`${BASE}/registro`); await foto(page, "pub-02-registro")
  await page.goto(`${BASE}/recuperar-password`); await foto(page, "pub-03-recuperar")
  await ctx.close()
}

// ── Docente ─────────────────────────────────────────────────────────
{
  const ctx = await navegador.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2 })
  const page = await ctx.newPage()
  console.log("docente")
  await login(page, "docente")
  await foto(page, "doc-01-inicio")
  await page.goto(`${BASE}/reservas`); await foto(page, "doc-02-mis-reservas")
  await page.goto(`${BASE}/reservas/nueva`); await foto(page, "doc-03-nueva-reserva")
  await page.goto(`${BASE}/inventario`); await foto(page, "doc-04-computadoras")
  await page.goto(`${BASE}/notificaciones`); await foto(page, "doc-05-notificaciones")
  await page.goto(`${BASE}/notificaciones?soporte=nuevo`); await foto(page, "doc-06-pedir-ayuda")
  await page.goto(`${BASE}/perfil`); await foto(page, "doc-07-perfil")
  await page.goto(`${BASE}/cambiar-password`); await foto(page, "doc-08-cambiar-password")
  await page.goto(`${BASE}/disponibilidad`); await foto(page, "doc-09-horario-admins")
  await ctx.close()
}

// ── Docente en teléfono ─────────────────────────────────────────────
{
  const ctx = await navegador.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true })
  const page = await ctx.newPage()
  console.log("docente (teléfono)")
  await login(page, "docente")
  await foto(page, "mov-01-inicio")
  await page.goto(`${BASE}/reservas`); await foto(page, "mov-02-reservas")
  await ctx.close()
}

// ── Admin ───────────────────────────────────────────────────────────
{
  const ctx = await navegador.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2 })
  const page = await ctx.newPage()
  console.log("admin")
  await login(page, "admin")
  await foto(page, "adm-01-inicio")
  for (const [ruta, nombre] of [
    ["/admin/aprobacion", "adm-02-aprobacion"],
    ["/admin/academico", "adm-03-academico"],
    ["/admin/usuarios", "adm-04-usuarios"],
    ["/admin/inventario", "adm-05-inventario"],
    ["/admin/entregas", "adm-06-entregas"],
    ["/admin/licencias", "adm-07-licencias"],
    ["/admin/reportes", "adm-08-reportes"],
    ["/admin/jornada", "adm-09-jornada"],
    ["/admin/pedidos-de-materia", "adm-10-pedidos-materia"],
    ["/admin/bloquear-equipos", "adm-11-bloquear"],
    ["/notificaciones", "adm-12-notificaciones"],
    ["/disponibilidad", "adm-13-disponibilidad"],
    ["/inventario", "adm-14-computadoras"],
    ["/reservas", "adm-15-reservas"],
  ]) {
    await page.goto(BASE + ruta)
    await foto(page, nombre)
  }
  await ctx.close()
}

await navegador.close()
console.log("listo")
