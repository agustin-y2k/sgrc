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
  await page.getByLabel(/email/i).fill(CUENTAS[quien].email)
  await page.getByLabel(/contraseña/i).fill(CUENTAS[quien].pass)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()
  await page.waitForURL((u) => !u.pathname.endsWith("/login"), { timeout: 15000 })
}
async function foto(page, nombre, completa = true) {
  await page.waitForTimeout(800)
  await page.screenshot({ path: `${SALIDA}/${nombre}.png`, fullPage: completa })
  console.log("  ✓", nombre)
}
async function paso(n, fn) { try { await fn() } catch (e) { console.log("  ✗", n, String(e).split("\n")[0].slice(0,110)) } }
const nav = await chromium.launch()
{
  const ctx = await nav.newContext({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 })
  const page = await ctx.newPage()
  await login(page, "docente")
  console.log("docente")
  await paso("avisar falla", async () => {
    await page.goto(`${BASE}/`)
    await page.getByText("Avisar que una no anda").click()
    await foto(page, "paso-09-avisar-falla")
    await page.keyboard.press("Escape")
  })
  await paso("conversaciones", async () => {
    await page.goto(`${BASE}/notificaciones`)
    await page.getByRole("button", { name: /ver conversaciones/i }).click()
    await foto(page, "paso-06a-lista-conversaciones")
    await page.getByRole("button", { name: /^ver$/i }).first().click()
    await foto(page, "paso-06-conversacion")
  })
  await paso("equipos desplegados", async () => {
    await page.goto(`${BASE}/inventario`)
    await page.getByRole("button", { name: /ver equipos/i }).first().click()
    await foto(page, "paso-08b-computadoras-desplegado")
  })
  await ctx.close()
}
{
  const ctx = await nav.newContext({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 })
  const page = await ctx.newPage()
  await login(page, "admin")
  console.log("admin")
  await paso("soporte admin", async () => {
    await page.goto(`${BASE}/notificaciones`)
    await page.getByRole("button", { name: /ver conversaciones|pedidos de ayuda/i }).first().click()
    await foto(page, "paso-22-soporte-admin")
    await page.getByRole("button", { name: /^ver$/i }).first().click()
    await foto(page, "paso-23-soporte-conversacion")
  })
  await ctx.close()
}
await nav.close()
console.log("listo")
