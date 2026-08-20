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
import { writeFileSync } from "node:fs"

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
  await page.goto(`${BASE}/login`)
  await page.getByLabel(/email/i).fill(CUENTAS[quien].email)
  await page.getByLabel(/contraseña/i).fill(CUENTAS[quien].pass)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()
  await page.waitForURL((u) => !u.pathname.endsWith("/login"), { timeout: 15000 })
}
// Devuelve la caja de un elemento en coordenadas del documento completo.
async function caja(page, texto) {
  return await page.evaluate((t) => {
    const nodos = [...document.querySelectorAll("button, a, input, select, label, h2, h3, p, div, section")]
    const candidatos = nodos
      .filter((n) => n.textContent?.trim().startsWith(t))
      .map((n) => ({ n, r: n.getBoundingClientRect() }))
      .filter((c) => c.r.width > 20 && c.r.height > 10)
    if (!candidatos.length) return null
    // el más chico: el contenedor que envuelve media pantalla también
    // "empieza con" ese texto, y marcarlo no señala nada.
    candidatos.sort((a, b) => a.r.width * a.r.height - b.r.width * b.r.height)
    const r = candidatos[0].r
    return { x: r.x + window.scrollX, y: r.y + window.scrollY, w: r.width, h: r.height }
  }, texto)
}

const PLAN = [
  { nombre: "marca-inicio-docente", quien: "docente", ruta: "/",
    marcas: ["Reservas", "Avisos", "Pedir ayuda", "Salir", "Reservar computadoras", "Tus próximas clases", "Otras cosas que podés hacer"] },
  { nombre: "marca-nueva-reserva", quien: "docente", ruta: "/reservas/nueva", llenar: true,
    marcas: ["Una sola fecha", "Materia", "Fecha", "Hora de inicio", "Qué computadoras necesitás", "Confirmar reserva"] },
  { nombre: "marca-mis-reservas", quien: "docente", ruta: "/reservas",
    marcas: ["Nueva reserva", "Mostrar también las canceladas", "Confirmada", "Cambiar computadora", "Cancelar"] },
  { nombre: "marca-inicio-admin", quien: "admin", ruta: "/",
    marcas: ["Para entregar ahora", "Afuera del laboratorio", "En el laboratorio ahora", "Entregar sin reserva", "Administración", "Lo que viene"] },
]
const nav = await chromium.launch()
const meta = {}
for (const p of PLAN) {
  const ctx = await nav.newContext({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 })
  const page = await ctx.newPage()
  await login(page, p.quien)
  await page.goto(BASE + p.ruta)
  if (p.llenar) {
    await page.getByLabel(/materia/i).selectOption({ index: 1 })
    await page.getByLabel(/fecha/i).fill("2026-08-25")
    const s = page.locator("select"); const n = await s.count()
    await s.nth(n - 4).selectOption("14"); await s.nth(n - 3).selectOption("00")
    await s.nth(n - 2).selectOption("15"); await s.nth(n - 1).selectOption("30")
    await page.waitForTimeout(1500)
    const c = page.getByRole("checkbox")
    for (let i = 0; i < Math.min(3, await c.count()); i++) await c.nth(i).check().catch(() => {})
  }
  await page.waitForTimeout(900)
  const cajas = []
  for (const m of p.marcas) cajas.push({ texto: m, caja: await caja(page, m) })
  await page.screenshot({ path: `${SALIDA}/${p.nombre}.png`, fullPage: true })
  meta[p.nombre] = cajas
  console.log("  ✓", p.nombre, cajas.filter((c) => c.caja).length + "/" + cajas.length, "marcas")
  await ctx.close()
}
writeFileSync(`${SALIDA}/marcas.json`, JSON.stringify(meta, null, 2))
await nav.close()
