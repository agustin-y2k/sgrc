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
const BASE = "http://localhost:8081"
const SALIDA = process.env.SALIDA
const nav = await chromium.launch()
const ctx = await nav.newContext({ viewport: { width: 1280, height: 1000 }, deviceScaleFactor: 3 })
const page = await ctx.newPage()
for (const [ruta, nombre] of [["/login", "form-login"], ["/registro", "form-registro"], ["/recuperar-password", "form-recuperar"]]) {
  await page.goto(BASE + ruta)
  await page.waitForTimeout(900)

  // El registro se fotografía CON el cargo elegido: «¿Dónde vas a estar?» —los
  // desplegables de curso y materia que la guía explica paso a paso— sólo
  // aparece después de tocar "Docente", y antes de eso la captura mostraba un
  // formulario que no se parece al que la guía describe.
  //
  // Elegir además un curso hace aparecer Materia, que es el segundo paso: la
  // figura muestra los dos campos en vez de dejar la mitad a la imaginación.
  if (ruta === "/registro") {
    await page.getByRole("radio", { name: /^Docente/ }).first().click()
    await page.waitForTimeout(600)
    // La primera opción real del desplegable: la 0 es "Elegí una opción…".
    await page
      .getByLabel("Curso o lugar donde trabajás")
      .selectOption({ index: 1 })
      .catch(() => {})
    await page.waitForTimeout(700)
  }
  const caja = await page.evaluate(() => {
    const f = document.querySelector("form")
    const tarjeta = f?.closest("div.rounded-xl, div.rounded-lg, div.border") ?? f
    const r = tarjeta.getBoundingClientRect()
    return { x: r.x, y: r.y, width: r.width, height: r.height }
  })
  const m = 26
  await page.screenshot({
    path: `${SALIDA}/${nombre}.png`,
    fullPage: true,
    clip: { x: Math.max(0, caja.x - m), y: Math.max(0, caja.y - m), width: caja.width + m * 2, height: caja.height + m * 2 },
  })
  console.log("  ✓", nombre, Math.round(caja.width) + "x" + Math.round(caja.height))

  // El bloque de curso y materia, aparte.
  //
  // No alcanza con que esté en la captura del formulario entero: el recorte
  // topa el alto en 1,35 veces el ancho (ver preparar-imagenes.py) porque una
  // imagen más larga que eso queda ilegible en el PDF, y con los dos
  // desplegables abiertos este formulario mide el triple. Justo la parte que
  // la guía explica paso a paso quedaba del lado cortado.
  if (ruta === "/registro") {
    const bloque = await page.evaluate(() => {
      const t = [...document.querySelectorAll("div")].find((n) =>
        n.textContent?.trim().startsWith("¿Dónde vas a estar?")
      )
      if (!t) return null
      const r = t.getBoundingClientRect()
      return { x: r.x, y: r.y, width: r.width, height: r.height }
    })
    if (bloque) {
      await page.screenshot({
        path: `${SALIDA}/form-registro-donde.png`,
        fullPage: true,
        clip: {
          x: Math.max(0, bloque.x - m),
          y: Math.max(0, bloque.y - m),
          width: bloque.width + m * 2,
          height: bloque.height + m * 2,
        },
      })
      console.log("  ✓ form-registro-donde", Math.round(bloque.width) + "x" + Math.round(bloque.height))
    }
  }
}
await nav.close()
