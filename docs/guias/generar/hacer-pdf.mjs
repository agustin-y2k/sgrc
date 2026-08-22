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

// El título del pie sale del NOMBRE del archivo, no de la posición del
// argumento. Antes el primer par era siempre "docentes" y el segundo siempre
// "administradores": regenerar una sola guía —lo normal cuando se corrige un
// capítulo— le estampaba el título de la otra en todas las páginas.
const TITULOS = [
  [/docentes/, "SGRC · Guía para docentes"],
  [/admin/, "SGRC · Guía para administradores"],
]

function tituloDe(html) {
  for (const [patron, titulo] of TITULOS) {
    if (patron.test(html)) return titulo
  }
  throw new Error(
    `no sé qué título poner en el pie de ${html}: el nombre no dice si es la ` +
      `guía de docentes o la de administradores (ver TITULOS en este script)`
  )
}

// De a pares html→pdf, los que vengan. Con uno solo alcanza.
const args = process.argv.slice(2)
if (args.length === 0 || args.length % 2 !== 0) {
  throw new Error(
    "uso: hacer-pdf.mjs <guia.html> <salida.pdf> [<guia.html> <salida.pdf> ...]"
  )
}
const GUIAS = []
for (let i = 0; i < args.length; i += 2) {
  GUIAS.push({ html: args[i], pdf: args[i + 1], titulo: tituloDe(args[i]) })
}
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
