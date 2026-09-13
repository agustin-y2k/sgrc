/**
 * ¿Alguna pantalla arranca mostrando un cartel de error?
 *
 *   node scripts/barrer-pantallas.mjs
 *
 * Entra con los dos roles y visita todas las rutas de App.tsx, una por una,
 * preguntándole al DOM si quedó algún `[data-slot="alert"]` en rojo. Sale con
 * código distinto de cero si encuentra alguno, y dice cuál y qué decía.
 *
 * **Por qué existe.** Los reportes de incidencias estuvieron rotos sin que
 * nadie lo notara: sus rutas chocaban con `/api/incidencias/{id}` y las tres
 * respondían «el ID indicado no tiene un formato válido», así que la pestaña de
 * Reportes se abría con un cartel rojo. Ni los tests de pantalla lo veían —cada
 * uno monta su componente con la API mockeada, que es lo correcto para probar
 * lógica pero no cruza el router de verdad— ni los del backend, que registran
 * un módulo por vez y nunca los dos que competían. Estuvo a la vista en una
 * captura de la guía durante semanas.
 *
 * Esto no reemplaza a ninguno de los dos: es la pregunta más barata que ninguno
 * de los dos podía contestar — con el sistema entero levantado, ¿alguna pantalla
 * se abre rota?
 *
 * Necesita el sistema corriendo y datos cargados. En el pipeline de capturas
 * corre solo, después de las capturas, aprovechando la pila que ya está en pie.
 */
import { createRequire } from "node:module"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"

// Playwright se resuelve desde frontend/node_modules y no desde esta carpeta:
// Node busca los paquetes al lado del archivo que los importa, así que un
// import pelado falla apenas el script se corre desde otro lugar.
const aca = dirname(fileURLToPath(import.meta.url))
const requerir = createRequire(resolve(aca, "..", "frontend", "package.json"))
const { chromium } = requerir("@playwright/test")

const BASE = process.env.BASE_URL ?? "http://localhost:8081"

const CUENTAS = {
  Admin: {
    email: process.env.GUIA_ADMIN_EMAIL ?? "admin@escuela.edu.ar",
    pass: process.env.GUIA_ADMIN_PASSWORD ?? "",
    rutas: [
      "/",
      "/reservas",
      "/reservas/nueva",
      "/inventario",
      "/disponibilidad",
      "/notificaciones",
      "/perfil",
      "/admin/aprobacion",
      "/admin/academico",
      "/admin/usuarios",
      "/admin/inventario",
      "/admin/entregas",
      "/admin/licencias",
      "/admin/bloquear-equipos",
      "/admin/jornada",
      "/admin/pedidos-de-materia",
      "/admin/reportes",
      "/admin/auditoria",
    ],
  },
  Docente: {
    email: process.env.GUIA_DOCENTE_EMAIL ?? "ana.gomez@escuela.edu.ar",
    pass: process.env.GUIA_DOCENTE_PASSWORD ?? "guia.demo.2026",
    // Un docente no ve /admin/*: el menú no se las ofrece y ProtectedRoute las
    // rechaza. Se listan las suyas, que son las que puede abrir.
    rutas: [
      "/",
      "/reservas",
      "/reservas/nueva",
      "/inventario",
      "/disponibilidad",
      "/notificaciones",
      "/perfil",
    ],
  },
}

async function entrar(page, cuenta) {
  await page.goto(`${BASE}/login`)
  await page.getByLabel(/email/i).fill(cuenta.email)
  await page.getByLabel(/contraseña/i).fill(cuenta.pass)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()
  await page.waitForURL((u) => !u.pathname.endsWith("/login"), { timeout: 20000 })
}

/** Los carteles rojos que quedaron en pantalla, con su texto. */
function erroresEnPantalla(page) {
  return page.evaluate(() =>
    [...document.querySelectorAll('[data-slot="alert"]')]
      .filter((e) => (e.className || "").includes("destructive"))
      .map((e) => (e.textContent || "").trim().replace(/\s+/g, " ").slice(0, 160))
      .filter((t) => t !== "")
  )
}

const nav = await chromium.launch()
let problemas = 0

for (const [rol, cuenta] of Object.entries(CUENTAS)) {
  const ctx = await nav.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  console.log(`── ${rol}`)
  await entrar(page, cuenta)

  for (const ruta of cuenta.rutas) {
    await page.goto(BASE + ruta)
    // Las pantallas piden sus datos al montarse: sin esperar, se fotografía el
    // "Cargando…" y cualquier error llegaría después de mirar.
    await page.waitForTimeout(1800)

    const errores = await erroresEnPantalla(page)
    if (errores.length === 0) {
      console.log(`   ✓ ${ruta}`)
      continue
    }
    problemas++
    console.log(`   ✗ ${ruta}`)
    errores.forEach((e) => console.log(`       «${e}»`))
  }
  await ctx.close()
}

await nav.close()

if (problemas > 0) {
  console.log(`\n${problemas} pantalla(s) se abren con un cartel de error.`)
  process.exit(1)
}
console.log("\nNinguna pantalla se abre con un cartel de error.")
