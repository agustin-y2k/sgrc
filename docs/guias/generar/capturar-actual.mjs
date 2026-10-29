import { createRequire } from "node:module"
import { dirname, resolve, join } from "node:path"
import { fileURLToPath } from "node:url"
import { readFile, writeFile } from "node:fs/promises"
import { prepararDatos } from "./datos-actuales.mjs"
import { medirControles } from "./medir-controles.mjs"

const raiz = resolve(dirname(fileURLToPath(import.meta.url)), "../../..")
const { chromium, expect } = createRequire(join(raiz, "frontend/package.json"))(
  "@playwright/test"
)
const base = process.env.GUIA_BASE_URL
if (!base || new URL(base).hostname !== "127.0.0.1")
  throw new Error("Se requiere el origen efímero de generar-guias.mjs")
const imagenes = process.env.GUIA_IMAGENES
const informe = []
async function api(method, path, body, token = "") {
  const r = await fetch(base + path, {
    method,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  })
  if (!r.ok) throw new Error(`${method} ${path}: HTTP ${r.status}`)
  return r.headers.get("content-type")?.includes("application/json")
    ? r.json()
    : undefined
}
const navegador = await chromium.launch({ args: ["--lang=es-AR"] })
const opciones = {
  viewport: { width: 1280, height: 1000 },
  deviceScaleFactor: 2,
  locale: "es-AR",
  timezoneId: "America/Argentina/Buenos_Aires",
  colorScheme: "light",
}
const errores = []
const paginas = []
async function contexto(movil = false) {
  const ctx = await navegador.newContext({
    ...opciones,
    ...(movil
      ? {
          viewport: { width: 390, height: 844 },
          isMobile: true,
          hasTouch: true,
        }
      : {}),
  })
  const p = await ctx.newPage()
  paginas.push(p)
  p.setDefaultTimeout(15_000)
  p.on("pageerror", (e) => errores.push(e.message))
  return p
}
async function entrar(p, rol) {
  await p.goto(base + "/login")
  await p.getByLabel("Email", { exact: true }).fill(process.env[`GUIA_${rol}_EMAIL`])
  const password = p.getByLabel("Contraseña", { exact: true })
  await password.fill(process.env[`GUIA_${rol}_PASSWORD`])
  const respuesta = p.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === "/api/auth/login" && r.request().method() === "POST"
  )
  await p.getByRole("button", { name: "Iniciar sesión", exact: true }).click()
  const r = await respuesta
  if (r.status() !== 200) {
    await password.fill("")
    throw new Error(`Ingreso ${rol}: HTTP ${r.status()}`)
  }
  await expect(p).not.toHaveURL(/\/login$/)
  await p.waitForLoadState("networkidle")
}
async function ir(p, ruta) {
  await p.goto(base + ruta)
  await expect(p.getByRole("main")).toBeVisible()
  await p.waitForLoadState("networkidle")
  await expect(p.locator('main [data-slot="alert"][class*="destructive"]')).toHaveCount(0)
}
async function foto(
  nombre,
  p,
  target,
  { viewport = false, hasta, desde, maxHeight = 950, escenario } = {}
) {
  await p.waitForLoadState("networkidle")
  await p.evaluate(() => document.fonts.ready)
  await expect(
    p.getByText(/^Cargando(?:…|\.\.\.)$/).filter({ visible: true })
  ).toHaveCount(0)
  const path = join(imagenes, nombre + ".png")
  let encuadre
  if (viewport) {
    await p.evaluate(() => scrollTo({ top: 0, left: 0, behavior: "instant" }))
    encuadre = { x: 0, y: 0, ...p.viewportSize() }
    await p.screenshot({ path, animations: "disabled" })
  } else {
    const area = target ?? p.getByRole("main")
    await expect(area).toBeVisible()
    // El recorte se obtiene del DOM antes de capturar. Mantener el scroll al
    // inicio impide que la barra sticky se fotografíe atravesando el panel.
    await p.evaluate(() => scrollTo({ top: 0, left: 0, behavior: "instant" }))
    const caja = await area.evaluate((el) => {
      const r = el.getBoundingClientRect()
      return {
        x: r.x + scrollX,
        y: r.y + scrollY,
        width: r.width,
        height: r.height,
      }
    })
    if (!target) {
      // El main crece hasta el pie en pantallas cortas. El encuadre termina
      // en su contenido para no imprimir ese espacio vacío como parte del panel.
      const fondo = await area
        .locator(":scope > *")
        .evaluateAll((elementos) =>
          Math.max(
            ...elementos
              .filter((el) => el.getClientRects().length)
              .map((el) => el.getBoundingClientRect().bottom + scrollY)
          )
        )
      if (Number.isFinite(fondo)) caja.height = Math.min(caja.height, fondo - caja.y + 12)
    }
    if (desde) {
      const y = await desde.evaluate((el) => el.getBoundingClientRect().top + scrollY)
      caja.height -= y - caja.y
      caja.y = y
    }
    if (hasta)
      caja.height =
        (await hasta.evaluate((el) => el.getBoundingClientRect().bottom + scrollY)) -
        caja.y +
        12
    caja.height = Math.min(caja.height, maxHeight)
    encuadre = caja
    await p.screenshot({
      path,
      animations: "disabled",
      fullPage: true,
      clip: caja,
    })
  }
  informe.push({
    archivo: nombre + ".png",
    ruta: new URL(p.url()).pathname,
    fecha: new Date().toISOString(),
    encuadre: { ancho: encuadre.width, alto: encuadre.height },
    controles: await medirControles(p, encuadre),
    ...(escenario ? { escenario } : {}),
  })
  console.log("  Capturada:", nombre)
}
const boton = (p, name) => p.getByRole("button", { name, exact: true })
async function hora(p, campo, valor) {
  const [h, m] = valor.split(":")
  await p.getByLabel(`${campo}: hora`, { exact: true }).selectOption(h)
  await p.getByLabel(`${campo}: minutos`, { exact: true }).selectOption(m)
}
let datos
async function ficha(p, rol = "admin") {
  await ir(p, rol === "admin" ? "/admin/inventario" : "/inventario")
  await boton(p, rol === "admin" ? "Gestionar equipos" : "Ver equipos")
    .first()
    .click()
  const area =
    rol === "admin"
      ? p.locator(`#equipo-inventario-${datos.pcs[0].id}`)
      : p.locator('[data-slot="card"]').filter({ hasText: "Carro 1" }).last()
  if (rol === "admin") await area.locator("summary").click()
  return area
}
try {
  const adminToken = (
    await api("POST", "/api/auth/login", {
      email: process.env.GUIA_ADMIN_EMAIL,
      password: process.env.GUIA_ADMIN_PASSWORD,
    })
  ).token
  for (const ruta of ["/api/ciclos", "/api/equipos", "/api/prestamos"])
    if ((await api("GET", ruta, undefined, adminToken)).data.length)
      throw new Error("La base de las guías debe estar vacía")
  const admin = await contexto()
  await entrar(admin, "ADMIN")
  await expect(admin).toHaveURL(/primera-jornada/)
  await foto("admin-12c-primera-jornada", admin, admin.locator("body"))
  datos = await prepararDatos(api, adminToken)
  const docente = await contexto()
  await entrar(docente, "DOCENTE")

  // Formularios públicos: se fotografían los controles reales, sin enviarlos.
  const publico = await contexto()
  await publico.goto(base + "/login")
  await foto("docente-01-entrar", publico, publico.locator('[data-slot="card"]'))
  await publico.goto(base + "/registro")
  await publico.getByRole("radio", { name: /^Docente/ }).click()
  await publico.getByLabel("Curso o lugar donde trabajás").selectOption({ label: "1°1" })
  await foto("docente-02-crear-cuenta", publico, publico.locator('[data-slot="card"]'))
  const donde = publico
    .locator("div")
    .filter({ has: publico.getByText("¿Dónde vas a estar?", { exact: true }) })
    .last()
  await foto("docente-02b-donde-vas-a-estar", publico, donde)
  await publico.goto(base + "/recuperar-password")
  await foto(
    "docente-03-olvide-contrasena",
    publico,
    publico.locator('[data-slot="card"]')
  )

  await ir(docente, "/")
  await foto("docente-04-pantalla-de-inicio", docente, undefined, {
    viewport: true,
  })
  await ir(docente, "/reservas/nueva")
  await docente.getByLabel("Materia", { exact: true }).selectOption(datos.programacion.id)
  await docente.getByLabel("Fecha", { exact: true }).fill(datos.lunes)
  await hora(docente, "Hora de inicio", "16:00")
  await hora(docente, "Hora de fin", "17:30")
  for (const pc of ["PC 1", "PC 2", "PC 3"])
    await docente
      .getByRole("checkbox", { name: new RegExp(`\\b${pc}\\b`) })
      .first()
      .check()
  await expect(boton(docente, "Confirmar reserva")).toBeEnabled()
  const inicioEquipos = docente.getByText("Qué computadoras necesitás", {
    exact: true,
  })
  await foto("docente-05-nueva-reserva", docente, undefined, {
    hasta: inicioEquipos,
  })
  await foto("docente-23-seleccion-equipos", docente, docente.locator("form"), {
    desde: inicioEquipos,
    hasta: boton(docente, "Confirmar reserva"),
  })
  await boton(docente, "Se repite todas las semanas").click()
  await docente.getByLabel("Desde", { exact: true }).fill(datos.lunes)
  await docente.getByLabel("Hasta", { exact: true }).fill(datos.finSerie)
  await hora(docente, "Hora de inicio", "14:00")
  await hora(docente, "Hora de fin", "15:30")
  await docente.getByLabel("Día de la semana", { exact: true }).selectOption("LUNES")
  for (const pc of ["PC 1", "PC 2", "PC 3"])
    await docente
      .getByRole("checkbox", { name: new RegExp(`\\b${pc}\\b`) })
      .first()
      .check()
  await boton(docente, "Comprobar serie").click()
  const consentimiento = docente.getByRole("checkbox", {
    name: /Autorizo tomar los equipos por prioridad/,
  })
  await expect(consentimiento).toBeVisible()
  await consentimiento.check()
  await foto("docente-06-reserva-semanal", docente, undefined, {
    hasta: docente.getByText("Qué computadoras necesitás", { exact: true }),
  })
  await foto(
    "docente-22-consentimiento-serie",
    docente,
    docente.locator("section").filter({ has: consentimiento })
  )

  await ir(docente, "/reservas")
  await expect(
    docente.getByText("Confirmada", { exact: true }).filter({ visible: true }).first()
  ).toBeVisible()
  await foto("docente-07-mis-reservas", docente)
  await boton(docente, "Cancelar").first().click()
  await foto(
    "docente-08-cancelar",
    docente,
    docente
      .locator('[data-slot="card"]')
      .filter({ has: boton(docente, "Confirmar cancelación") })
  )
  await ir(docente, "/reservas")
  await boton(docente, "Cambiar computadora").first().click()
  await foto(
    "docente-09-cambiar-computadora",
    docente,
    docente.locator('[data-slot="card"]').filter({ hasText: "¿Cuál cambiás?" })
  )
  await ir(docente, "/inventario")
  await boton(docente, "Ver equipos").first().click()
  await foto("docente-10-computadoras", docente)
  await boton(docente, "Reportar problema").first().click()
  await foto(
    "docente-11-avisar-falla",
    docente,
    docente.locator("form").filter({
      has: docente.getByRole("button", { name: /Reportar|Avisar|Enviar/ }),
    })
  )
  await ir(docente, "/notificaciones")
  await foto("docente-12-avisos", docente)
  await boton(docente, "Ver conversaciones").click()
  const hilo = docente.getByRole("group", {
    name: `Conversación: Cambiar una computadora de mi clase`,
    exact: true,
  })
  if (!(await hilo.getByRole("textbox").isVisible())) await boton(hilo, "Ver").click()
  await foto("docente-13-conversacion", docente, hilo)
  await ir(docente, "/notificaciones")
  await docente.getByRole("button", { name: /Elegir cuáles/ }).click()
  await foto("docente-14-copias-por-correo", docente)
  for (const [ruta, nombre] of [
    ["/perfil", "docente-15-perfil"],
    ["/cambiar-password", "docente-16-cambiar-contrasena"],
    ["/disponibilidad", "docente-17-horario-admins"],
  ]) {
    await ir(docente, ruta)
    await foto(nombre, docente)
  }
  const movil = await contexto(true)
  await entrar(movil, "DOCENTE")
  await foto("docente-18-telefono", movil, undefined, { viewport: true })
  await ir(docente, "/reservas/calendario")
  await docente.getByLabel("Ir a fecha").fill(datos.lunes)
  await docente.getByLabel("Mostrar reservas").selectOption("todas")
  await boton(docente, "Semana").click()
  await expect(
    docente.getByText(
      "Los horarios indican la duración real; las clases cortas se muestran ampliadas."
    )
  ).toBeVisible()
  await foto("docente-25-calendario-semana", docente)
  await docente.setViewportSize({ width: 390, height: 844 })
  await boton(docente, "Agenda").click()
  await foto("docente-24-calendario-agenda", docente, undefined, {
    viewport: true,
  })
  const agendaDocente = docente
    .getByRole("button", { name: /^Programación,/ })
    .first()
    .locator("xpath=ancestor::section[1]")
  await foto("docente-26-agenda-clases", docente, agendaDocente)

  await docente.setViewportSize(opciones.viewport)

  await ir(admin, "/reservas/calendario")
  await admin.getByLabel("Ir a fecha").fill(datos.lunes)
  await boton(admin, "Semana").click()
  await foto("admin-30-calendario", admin)
  await admin.setViewportSize({ width: 390, height: 844 })
  await boton(admin, "Agenda").click()
  const agendaAdmin = admin
    .getByRole("button", { name: /^Programación,/ })
    .first()
    .locator("xpath=ancestor::section[1]")
  await foto("admin-33-calendario-movil", admin, agendaAdmin)
  await admin.setViewportSize(opciones.viewport)

  for (const [p, nombre] of [
    [docente, "docente-19-calendario-equipo"],
    [admin, "admin-20-calendario-equipo"],
  ]) {
    await ir(p, `/inventario/equipos/${datos.pcs[0].id}/calendario`)
    await p.getByRole("button", { name: /Semana siguiente/i }).click()
    await foto(nombre, p)
  }
  for (const [p, nombre, esMovil] of [
    [docente, "docente-20-como-entrar", false],
    [movil, "docente-21-como-entrar-telefono", true],
  ]) {
    await ir(p, "/inventario")
    await boton(p, "Ver equipos").first().click()
    await boton(p, "Cómo entrar").first().click()
    await p
      .getByRole("button", { name: /Ver contraseña/ })
      .first()
      .click()
    const titulo = p.getByText(/^Cómo entrar a PC 1/).first()
    await expect(titulo).toBeVisible()
    const panel = titulo.locator("xpath=../..")
    for (const texto of [
      "Ejemplo.2026",
      "Entra sin contraseña",
      "Solo la ven los administradores",
      "Contraseña no anotada",
    ])
      await expect(panel.getByText(texto, { exact: true })).toBeVisible()
    await foto(nombre, p, panel, { maxHeight: 1600 })
  }

  // Administración: se abren formularios y confirmaciones sin aplicar cambios.
  await ir(admin, "/")
  await foto("admin-01-mostrador", admin, undefined, { viewport: true })
  await boton(admin, "Administración").click()
  await foto("admin-02-menu", admin, undefined, { viewport: true })
  for (const [ruta, nombre] of [
    ["/admin/aprobacion", "admin-03-aprobacion"],
    ["/admin/usuarios", "admin-05-usuarios"],
    ["/admin/entregas", "admin-07-entregas"],
    ["/admin/licencias", "admin-09-licencias"],
    ["/admin/pedidos-de-materia", "admin-13-pedidos-de-materia"],
    ["/admin/reportes", "admin-16-reportes"],
    ["/admin/configuracion", "admin-34-configuracion"],
    ["/disponibilidad", "admin-17-horarios-admin"],
    ["/notificaciones", "admin-18-avisos"],
    ["/perfil", "admin-21-perfil"],
  ]) {
    await ir(admin, ruta)
    await foto(nombre, admin)
  }
  await ir(admin, "/admin/academico")
  await boton(admin, "Cursos").first().click()
  await boton(admin, "Materias").first().click()
  await admin.setViewportSize({ width: 800, height: 1000 })
  const curso = boton(admin, "Ocultar materias").locator(
    "xpath=ancestor::div[contains(@class, 'rounded-md')][1]"
  )
  await expect(curso.getByText("Programación", { exact: true })).toBeVisible()
  await expect(curso.getByText("Matemática", { exact: true })).toBeVisible()
  await foto("admin-04-academico", admin, curso)
  for (const [nombreMateria, nombreFoto, efecto] of [
    ["Matemática", "admin-27-quitar-docente-compartida", "Las reservas se mantienen."],
    [
      "Programación",
      "admin-28-quitar-ultimo-docente",
      "se cancelarán sus reservas futuras",
    ],
  ]) {
    const materia = admin
      .getByText(nombreMateria, { exact: true })
      .locator("xpath=ancestor::div[contains(@class, 'rounded-md')][1]")
    await expect(materia).toHaveCount(1)
    await boton(materia, "Docentes").click()
    const quitar = boton(materia, "Quitar").first()
    await expect(quitar).toBeEnabled()
    await quitar.click()
    const confirmacion = materia.getByRole("group", {
      name: "Confirmar quitar docente",
      exact: true,
    })
    await expect(confirmacion.getByText(efecto, { exact: false })).toBeVisible()
    await foto(nombreFoto, admin, confirmacion)
    await boton(confirmacion, "Volver").click()
    await expect(confirmacion).toHaveCount(0)
    await boton(materia, "Ocultar docentes").click()
  }
  await ir(admin, "/admin/academico")
  await boton(admin, "Cursos").first().click()
  const filaCurso = admin
    .getByText("1°1", { exact: true })
    .locator("xpath=ancestor::div[contains(@class, 'rounded-md')][1]")
  await boton(filaCurso, "Renombrar").click()
  const division = admin.locator(`[id="editar-curso-${datos.curso.id}-division"]`)
  await division.fill("2")
  const formularioCurso = division.locator("xpath=ancestor::form[1]")
  await boton(formularioCurso, "Guardar").click()
  await expect(
    admin.locator('[aria-label="Impacto sobre las prioridades"]')
  ).toBeVisible()
  await foto("admin-31-editar-curso-prioridades", admin, formularioCurso)
  await boton(formularioCurso, "Cancelar").click()

  await admin.setViewportSize(opciones.viewport)
  await ir(admin, "/admin/academico")
  await admin.getByRole("button", { name: /Cerrar el año y abrir el siguiente/ }).click()
  await foto("admin-19-cerrar-anio", admin)
  const fichaRotulo = await ficha(admin)
  await boton(fichaRotulo, "Editar").click()
  await boton(fichaRotulo, "Cambiar número de PC").click()
  await fichaRotulo.getByLabel("Nuevo número de PC").fill("44")
  await fichaRotulo
    .getByLabel("Motivo del cambio de rótulo")
    .fill("Reponer un rótulo deteriorado")
  await fichaRotulo
    .getByRole("checkbox", { name: /Confirmo que es la misma computadora/ })
    .check()
  const formularioRotulo = boton(fichaRotulo, "Confirmar nuevo rótulo").locator(
    "xpath=ancestor::form[1]"
  )
  await foto("admin-32-cambiar-rotulo", admin, formularioRotulo)

  const equipo = await ficha(admin)
  await admin.setViewportSize({ width: 900, height: 1000 })
  await expect(boton(equipo, "Cómo entrar")).toBeVisible()
  await foto("admin-06-inventario", admin, equipo)
  await admin.setViewportSize(opciones.viewport)
  await boton(equipo, "Preferencias").click()
  await foto("admin-24-preferencias", admin, equipo)
  await boton(equipo, "Preferencias").click()
  await boton(equipo, "Cómo entrar").click()
  const cuentas = equipo.getByText(/^Cómo entrar a PC 1/).locator("xpath=../..")
  await foto("admin-22-cuentas-de-equipo", admin, cuentas)
  await equipo.getByRole("button", { name: /Agregar cuenta/ }).click()
  await foto("admin-23-cuenta-nueva", admin, cuentas.locator("form"))
  await ir(admin, "/admin/entregas")
  await boton(admin, "Entregar sin reserva").click()
  await admin.getByLabel("¿A quién?", { exact: true }).fill("Personal de Biblioteca")
  await foto("admin-08-entregar-sin-reserva", admin)
  await ir(admin, "/admin/licencias")
  await boton(admin, "Cargar una licencia").click()
  await admin.getByLabel("Software", { exact: true }).fill("Editor de imágenes")
  await foto("admin-10-cargar-licencia", admin)
  await ir(admin, "/admin/bloquear-equipos")
  await admin.getByLabel("Fecha", { exact: true }).fill(datos.lunes)
  await hora(admin, "Hora de inicio", "16:00")
  await hora(admin, "Hora de fin", "17:30")
  await foto("admin-11-bloquear", admin)
  await ir(admin, "/admin/jornada")
  await foto("admin-12-jornada", admin)
  await admin
    .getByRole("button", { name: /^Editar / })
    .first()
    .click()
  const tramo = admin.locator("li").filter({ has: boton(admin, "Guardar") })
  await hora(tramo, "Abre", "13:00")
  await boton(admin, "Guardar").click()
  await expect(admin.getByRole("button", { name: /Guardar y cancelar/ })).toBeVisible()
  await foto("admin-12b-jornada-impacto", admin)
  await admin.getByRole("button", { name: /Volver sin cambiar nada/ }).click()
  await ir(admin, "/notificaciones")
  await boton(admin, "Ver conversaciones").click()
  await foto("admin-14-soporte", admin)
  const hiloAdmin = admin.getByRole("group", {
    name: "Conversación: Cambiar una computadora de mi clase",
    exact: true,
  })
  if (!(await hiloAdmin.getByRole("textbox").isVisible()))
    await boton(hiloAdmin, "Ver").click()
  await foto("admin-15-soporte-conversacion", admin, hiloAdmin)
  await ir(admin, "/admin/auditoria")
  await foto("admin-25-auditoria", admin, undefined, {
    hasta: admin.getByRole("article").first(),
  })
  await admin.getByLabel("Acción", { exact: true }).selectOption("CAMBIO_TRANSACCIONAL")
  await admin.getByLabel("Sobre qué", { exact: true }).selectOption("equipo")
  await expect(admin.getByRole("article").first()).toHaveAccessibleName(
    /Se actualizó el equipo/
  )
  const entrada = admin.getByRole("article").first()
  await boton(entrada, "Ver el detalle").click()
  await expect(
    entrada.getByRole("columnheader", { name: "Antes", exact: true })
  ).toBeVisible()
  await expect(
    entrada.getByRole("columnheader", { name: "Después", exact: true })
  ).toBeVisible()
  await foto("admin-26-detalle-auditoria", admin, entrada)

  const adminMovil = await contexto(true)
  await entrar(adminMovil, "ADMIN")
  await adminMovil.getByRole("button", { name: /^Menú/ }).click()
  await foto("admin-29-menu-movil", adminMovil, undefined, { viewport: true })

  // Se provoca un fallo real de descarga, sin sustituir datos de la API ni
  // fabricar el mensaje. Después se comprueba que la recarga lo resuelva.
  const recuperacion = await contexto()
  let descargas = 0
  await recuperacion.route("**/assets/MisReservasPage-*.js", (route) => {
    descargas++
    return descargas === 1 ? route.abort("failed") : route.continue()
  })
  await entrar(recuperacion, "DOCENTE")
  await recuperacion.goto(base + "/reservas")
  const errorDePantalla = recuperacion.getByRole("heading", {
    name: "No pudimos abrir esta pantalla",
    exact: true,
  })
  await expect(errorDePantalla).toBeVisible()
  await foto(
    "general-01-recuperar-pantalla",
    recuperacion,
    errorDePantalla.locator(".."),
    {
      escenario:
        "Descarga de un módulo interrumpida deliberadamente; API y sesión reales. Se verifica la recuperación al recargar.",
    }
  )
  await boton(recuperacion, "Recargar la página").click()
  await expect(recuperacion.locator("main h1")).toHaveText("Mis reservas")
  if (descargas !== 2) throw new Error("La recarga no volvió a descargar la vista")

  const esperadas = new Set()
  for (const nombre of ["guia-docentes.html", "guia-admins.html"]) {
    const html = await readFile(join(raiz, "docs/guias", nombre), "utf8")
    for (const m of html.matchAll(/src="imagenes\/([^"]+)"/g)) esperadas.add(m[1])
  }
  const tomadas = new Set(informe.map((i) => i.archivo))
  for (const nombre of esperadas)
    if (!tomadas.has(nombre))
      throw new Error(`La guía usa una imagen que no se renovó: ${nombre}`)
  if (errores.length) throw new Error(`Errores de navegador: ${errores.join("; ")}`)
  await writeFile(
    join(process.env.SALIDA, "capturas.json"),
    JSON.stringify(
      {
        version: JSON.parse(await readFile(join(raiz, "frontend/package.json"), "utf8"))
          .version,
        origen: base,
        capturas: informe,
      },
      null,
      2
    )
  )
  console.log(`Completadas ${informe.length} capturas reales, sin errores de JavaScript.`)
} catch (error) {
  await writeFile(join(process.env.SALIDA, "error.txt"), error.stack ?? String(error))
  for (const [i, p] of paginas.entries())
    if (!p.isClosed())
      await p
        .screenshot({
          path: join(process.env.SALIDA, `diagnostico-${i}.png`),
          fullPage: true,
        })
        .catch(() => {})
  throw new Error(error.message)
} finally {
  await navegador.close()
}
