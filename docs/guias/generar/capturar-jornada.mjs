// Las pantallas de la jornada de la escuela.
//
// CORRE PRIMERO, antes que el resto de los capturadores, y por dos motivos:
//
//   1. La primera captura necesita que la escuela NO tenga jornada declarada,
//      que es el estado de una base recién creada. Cualquier otro script que
//      corra antes y declare una la deja imposible de tomar.
//   2. Al terminar, este script deja la jornada declarada. Hace falta: sin
//      ningún tramo cargado, el sistema le pide a cada Admin que la declare y
//      no lo deja navegar, así que todas las capturas de Admin que vengan
//      después mostrarían el asistente en vez de su pantalla.
//
// La confirmación del impacto se toma sin aplicarla: se propone un horario que
// deja clases afuera, se fotografía la pregunta y se sale con «Volver sin
// cambiar nada». Así los datos de demostración quedan intactos para el resto
// del pipeline.
import { createRequire } from "node:module"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import { mkdirSync } from "node:fs"

const aca = dirname(fileURLToPath(import.meta.url))
const requerir = createRequire(resolve(aca, "..", "..", "..", "frontend", "package.json"))
const { chromium } = requerir("@playwright/test")

const ADMIN_EMAIL = process.env.GUIA_ADMIN_EMAIL ?? "admin@escuela.edu.ar"
const ADMIN_PASSWORD = process.env.GUIA_ADMIN_PASSWORD ?? ""

const BASE = "http://localhost:8081"
const SALIDA = process.env.SALIDA
mkdirSync(SALIDA, { recursive: true })

async function foto(page, nombre) {
  await page.waitForLoadState("networkidle").catch(() => {})
  await page.waitForTimeout(600)
  await page.screenshot({ path: `${SALIDA}/${nombre}.png`, fullPage: true })
  console.log("  " + nombre)
}

async function login(page) {
  await page.goto(`${BASE}/login`)
  await page.getByLabel(/email/i).fill(ADMIN_EMAIL)
  await page.getByLabel(/contraseña/i).fill(ADMIN_PASSWORD)
  await page.waitForTimeout(400)
  await page.getByRole("button", { name: /^iniciar sesión$/i }).click()
  await page.waitForURL((u) => !u.pathname.endsWith("/login"), { timeout: 20000 })
  await page.waitForLoadState("networkidle")
}

const navegador = await chromium.launch()
const ctx = await navegador.newContext({ viewport: { width: 1280, height: 900 } })
const page = await ctx.newPage()

console.log("jornada")
await login(page)

// El envío al asistente no pasa en el login: lo decide ProtectedRoute recién
// cuando responde la consulta de la jornada, un tick DESPUÉS de que la página
// terminó de cargar. Sin esta espera el script leía la URL antes de tiempo,
// daba por hecho que la escuela ya tenía jornada declarada y se salteaba las
// dos capturas del asistente —y como además no la declaraba, se caía en el
// paso siguiente, que edita el tramo que tendría que haber creado—.
await page.waitForURL(/primera-jornada/, { timeout: 8000 }).catch(() => {})

// 1. El asistente del primer arranque, tal como lo ve un Admin que entra a un
//    sistema recién instalado.
if (!page.url().includes("primera-jornada")) {
  console.log("  AVISO: ya hay jornada declarada, no se puede tomar el asistente")
} else {
  await foto(page, "adm-16-primera-jornada")

  // 2. Declararla desde el asistente: lunes a viernes, de 7 a 18.
  await page.getByRole("button", { name: "Lunes a viernes" }).click()
  await page.getByLabel("Abre: hora").selectOption("07")
  await page.getByLabel("Cierra: hora").selectOption("18")
  await page.getByRole("button", { name: "Agregar tramo" }).click()
  await foto(page, "adm-17-primera-jornada-cargada")
  await page.getByRole("button", { name: /guardar la jornada/i }).click()
  await page.waitForURL((u) => !u.pathname.includes("primera-jornada"), {
    timeout: 20000,
  })
}

// 3. La pantalla de siempre, ya con la jornada cargada.
await page.goto(`${BASE}/admin/jornada`)
await page.waitForLoadState("networkidle")
await foto(page, "adm-09-jornada")

// 4. La confirmación al achicar el horario con clases ya reservadas. Se
//    propone y se descarta: los datos de demostración no se tocan.
await page.getByRole("button", { name: /^Editar Lunes a viernes/ }).click()
const enEdicion = page
  .locator("li")
  .filter({ has: page.getByRole("button", { name: "Guardar" }) })
await enEdicion.getByLabel("Abre: hora").selectOption("13")
await enEdicion.getByLabel("Cierra: hora").selectOption("18")
await page.getByRole("button", { name: "Guardar" }).click()
await page.waitForTimeout(2500)

if (await page.getByRole("button", { name: /Guardar y cancelar/ }).count()) {
  await foto(page, "adm-18-jornada-impacto")
  await page.getByRole("button", { name: /Volver sin cambiar nada/ }).click()
  await page.waitForTimeout(800)
} else {
  console.log("  AVISO: no apareció la confirmación (¿no hay reservas afuera?)")
}

await ctx.close()
await navegador.close()
console.log("listo")
