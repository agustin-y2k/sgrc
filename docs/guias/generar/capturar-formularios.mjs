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
  const caja = await page.evaluate(() => {
    const f = document.querySelector("form")
    const tarjeta = f?.closest("div.rounded-xl, div.rounded-lg, div.border") ?? f
    const r = tarjeta.getBoundingClientRect()
    return { x: r.x, y: r.y, width: r.width, height: r.height }
  })
  const m = 26
  await page.screenshot({
    path: `${SALIDA}/${nombre}.png`,
    clip: { x: Math.max(0, caja.x - m), y: Math.max(0, caja.y - m), width: caja.width + m * 2, height: caja.height + m * 2 },
  })
  console.log("  ✓", nombre, Math.round(caja.width) + "x" + Math.round(caja.height))
}
await nav.close()
