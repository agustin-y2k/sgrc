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
import { pathToFileURL } from "node:url"

const GUIAS = [
  { html: process.argv[2], pdf: process.argv[3], titulo: "SGRC · Guía para docentes" },
  { html: process.argv[4], pdf: process.argv[5], titulo: "SGRC · Guía para administradores" },
]
const nav = await chromium.launch()
for (const g of GUIAS) {
  const page = await (await nav.newContext()).newPage()
  await page.goto(pathToFileURL(g.html).href, { waitUntil: "networkidle" })
  await page.emulateMedia({ media: "print" })
  await page.pdf({
    path: g.pdf,
    format: "A4",
    printBackground: true,
    displayHeaderFooter: true,
    headerTemplate: "<div></div>",
    footerTemplate: `<div style="width:100%;font-size:8pt;color:#7a828d;
        font-family:'Segoe UI',Arial,sans-serif;padding:0 15mm;
        display:flex;justify-content:space-between;">
        <span>${g.titulo}</span>
        <span class="pageNumber"></span>
      </div>`,
    margin: { top: "17mm", bottom: "20mm", left: "15mm", right: "15mm" },
  })
  console.log("  ✓", g.pdf)
}
await nav.close()
