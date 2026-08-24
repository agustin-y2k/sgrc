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

// Las cuentas de cada equipo (RF-03.22): con qué usuario se entra a cada
// máquina. Van en su propio archivo porque necesitan las dos sesiones y
// porque el panel se abre desde dos pantallas distintas —Computadoras para
// consultar, Gestión del inventario para cargar—, que es justamente la
// diferencia que la guía explica.
//
// Las cuentas las siembra datos-de-demostracion.sh sobre la primera PC, en
// los cuatro estados: sin contraseña, pública con contraseña, reservada a
// administración, y con una contraseña que nadie anotó.
const ADMIN_EMAIL = process.env.GUIA_ADMIN_EMAIL ?? "admin@escuela.edu.ar"
const ADMIN_PASSWORD = process.env.GUIA_ADMIN_PASSWORD ?? ""
const DOCENTE_EMAIL = process.env.GUIA_DOCENTE_EMAIL ?? "ana.gomez@escuela.edu.ar"
const DOCENTE_PASSWORD = process.env.GUIA_DOCENTE_PASSWORD ?? "guia.demo.2026"

const BASE = "http://localhost:8081"
const SALIDA = process.env.SALIDA

const nav = await chromium.launch()
const ctxOpts = { viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 }

async function entrar(page, email, password) {
  await page.goto(`${BASE}/login`)
  await page.getByLabel(/email/i).fill(email)
  await page.getByLabel(/contraseña/i).fill(password)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()
  await page.waitForURL((u) => !u.pathname.endsWith("/login"))
}

const foto = async (page, n) => {
  await page.waitForTimeout(800)
  await page.screenshot({ path: `${SALIDA}/${n}.png`, fullPage: true })
  console.log("  ✓", n)
}

// El panel solo, y no la pantalla entera. En Gestión del inventario la ficha
// de un equipo cuelga debajo de una lista larga: fotografiada completa, la
// página entra en la hoja como una tira donde no se lee ni un rótulo. Se
// recorta al panel más un respiro arriba, para que se vea de qué equipo
// cuelga.
const fotoDelPanel = async (page, n, margenArriba = 150) => {
  await page.waitForTimeout(800)
  const caja = await page.evaluate((arriba) => {
    const titulo = [...document.querySelectorAll("p")].find((p) =>
      /^Cómo entrar a /.test(p.textContent || "")
    )
    const panel = titulo.closest("div.rounded-md")
    const r = panel.getBoundingClientRect()
    return {
      x: Math.max(0, r.x + window.scrollX - 8),
      y: Math.max(0, r.y + window.scrollY - arriba),
      width: r.width + 16,
      height: r.height + arriba + 8,
    }
  }, margenArriba)
  await page.screenshot({ path: `${SALIDA}/${n}.png`, fullPage: true, clip: caja })
  console.log("  ✓", n)
}
const fichaDeUnaPC = (page) =>
  page.locator("div.rounded-md.border").filter({ hasText: "N° serie" }).first()

const paso = async (n, f) => {
  try {
    await f()
  } catch (e) {
    console.log("  ✗", n, String(e).split("\n")[0].slice(0, 100))
  }
}

{
  const ctx = await nav.newContext(ctxOpts)
  const page = await ctx.newPage()
  await entrar(page, ADMIN_EMAIL, ADMIN_PASSWORD)

  // El panel donde un Admin las carga: cuelga de la ficha del equipo, en la
  // misma pantalla donde están el alta, las licencias y las entregas.
  await paso("cuentas desde la gestión del inventario", async () => {
    await page.goto(`${BASE}/admin/inventario`)
    await page.getByRole("button", { name: /gestionar equipos/i }).first().click()
    await page.waitForTimeout(600)
    // La ficha de una COMPUTADORA del carro, no la del primer "Cómo entrar"
    // de la página: en esta pantalla "Otros equipos" va arriba de los carros,
    // así que el primero es el del cargador —que no tiene ninguna cuenta— y
    // la captura salía vacía. Las fichas de carro son las que rotulan el
    // número de serie.
    await fichaDeUnaPC(page).getByRole("button", { name: "Cómo entrar" }).click()
    await fotoDelPanel(page, "form-cuentas-admin")
  })

  await paso("formulario de una cuenta nueva", async () => {
    await fichaDeUnaPC(page).getByRole("button", { name: /agregar cuenta/i }).click()
    // Sin margen arriba: acá lo que importa es el formulario, y el panel ya
    // se mostró en la captura anterior.
    await fotoDelPanel(page, "form-cuenta-nueva", 0)
  })

  await ctx.close()
}

{
  const ctx = await nav.newContext(ctxOpts)
  const page = await ctx.newPage()
  await entrar(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)

  // Lo que ve un docente: las mismas cuentas, con la contraseña de las
  // públicas a la vista y un cartel donde no le corresponde.
  await paso("cuentas vistas por un docente", async () => {
    await page.goto(`${BASE}/inventario`)
    await page.getByRole("button", { name: /ver equipos/i }).first().click()
    await page.waitForTimeout(600)
    await page.getByRole("button", { name: "Cómo entrar" }).first().click()
    await page.waitForTimeout(800)
    await page.getByRole("button", { name: /ver contraseña/i }).first().click()
    await foto(page, "cue-03-cuentas-docente")
  })

  // Y desde el teléfono, que es desde donde se la consulta de verdad: parado
  // frente a la máquina, no sentado en la oficina.
  await paso("cuentas en el teléfono", async () => {
    // Mismo encuadre que la otra captura de teléfono de las guías: 390 de
    // ancho y escala 3, para que las dos se vean del mismo tamaño en la hoja.
    const movil = await nav.newContext({
      viewport: { width: 390, height: 844 },
      deviceScaleFactor: 3,
      isMobile: true,
      hasTouch: true,
    })
    const chico = await movil.newPage()
    await entrar(chico, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    await chico.goto(`${BASE}/inventario`)
    await chico.getByRole("button", { name: /ver equipos/i }).first().click()
    await chico.waitForTimeout(600)
    await chico.getByRole("button", { name: "Cómo entrar" }).first().click()
    await chico.waitForTimeout(800)
    await chico.getByRole("button", { name: /ver contraseña/i }).first().click()
    await chico.waitForTimeout(600)
    // Al tope de la pantalla, no "apenas visible": preparar-imagenes.py corta
    // las capturas muy largas por abajo, así que lo que importa tiene que
    // quedar arriba.
    await chico
      .getByText(/^Cómo entrar a /)
      .first()
      .evaluate((el) => el.scrollIntoView({ block: "start" }))
    await chico.waitForTimeout(400)
    // Recortada al viewport y no `fullPage`: lo que importa es que entra en
    // una pantalla de teléfono, y una captura larguísima no lo mostraría.
    await chico.screenshot({ path: `${SALIDA}/cue-04-cuentas-telefono.png` })
    console.log("  ✓", "cue-04-cuentas-telefono")
    await movil.close()
  })

  await ctx.close()
}

await nav.close()
