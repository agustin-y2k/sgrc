import AxeBuilder from "@axe-core/playwright"
import { readFile } from "node:fs/promises"
import { expect, test, type APIRequestContext, type Page } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"
import { leerCSV } from "../src/lib/csv"

test.describe("reportes y auditoría en una base efímera vacía", () => {
  test.skip(
    process.env.E2E_REPORTES_AISLADO !== "1",
    "requiere E2E_REPORTES_AISLADO=1 y base efímera vacía"
  )
  test.describe.configure({ mode: "serial" })
  test.setTimeout(90_000)
  const email = process.env.E2E_ADMIN_EMAIL!,
    password = process.env.E2E_ADMIN_PASSWORD!
  const hoy = new Date(),
    iso = (d: Date) =>
      `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
  const manana = new Date(hoy)
  manana.setDate(manana.getDate() + 1)
  const fecha = iso(manana),
    anio = hoy.getFullYear(),
    nombreCarro = '=SUM(1;2) "Carro"'
  let admin = "",
    docente = "",
    usuario = "",
    ciclo = "",
    carro = ""
  const pcs: string[] = []
  async function api(
    r: APIRequestContext,
    method: string,
    path: string,
    data?: unknown,
    token = admin
  ) {
    const res = await r.fetch(path, {
      method,
      headers: { Authorization: `Bearer ${token}` },
      data,
    })
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBe(true)
    return (res.headers()["content-type"] ?? "").includes("application/json")
      ? res.json()
      : undefined
  }
  async function abrir(p: Page, path = "/admin/reportes") {
    await login(p, email, password)
    await p.goto(path)
  }
  async function movil(p: Page) {
    await p.setViewportSize({ width: 320, height: 780 })
    expect(
      await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)
    ).toBe(true)
    expect(
      (
        await new AxeBuilder({ page: p })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations
    ).toEqual([])
  }
  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    admin = (await api(request, "POST", "/api/auth/login", { email, password }, "")).token
    for (const path of ["/api/ciclos", "/api/equipos", "/api/prestamos"])
      expect((await api(request, "GET", path)).data).toHaveLength(0)
    expect(
      (await api(request, "GET", "/api/auth/usuarios?pageSize=200")).data
    ).toHaveLength(1)
    await api(request, "PUT", "/api/jornada", {
      tramos: [
        "LUNES",
        "MARTES",
        "MIERCOLES",
        "JUEVES",
        "VIERNES",
        "SABADO",
        "DOMINGO",
      ].flatMap((diaSemana) => [
        { diaSemana, horaInicio: "00:00", horaFin: "12:00" },
        { diaSemana, horaInicio: "12:00", horaFin: "00:00" },
      ]),
    })
    ciclo = (await api(request, "POST", "/api/ciclos", { anio })).id
    const curso = (
      await api(request, "POST", `/api/ciclos/${ciclo}/cursos`, {
        anio: 4,
        division: "A",
      })
    ).id
    const materia = (
      await api(request, "POST", `/api/cursos/${curso}/materias`, {
        nombre: "Tecnología",
      })
    ).id
    await api(
      request,
      "POST",
      "/api/auth/registro",
      {
        nombre: "Docente",
        apellido: "Reportes",
        email: DOCENTE_EMAIL,
        password: DOCENTE_PASSWORD,
        cargoSolicitado: "DOCENTE",
        rolSolicitado: "TITULAR",
      },
      ""
    )
    usuario = (await api(request, "GET", "/api/auth/usuarios?pageSize=200")).data.find(
      (u: { email: string }) => u.email === DOCENTE_EMAIL
    ).id
    await api(request, "PATCH", `/api/auth/usuarios/${usuario}/estado`, {
      estado: "APROBADA",
    })
    await api(request, "POST", `/api/materias/${materia}/docentes`, {
      usuarioId: usuario,
      rol: "TITULAR",
    })
    docente = (
      await api(
        request,
        "POST",
        "/api/auth/login",
        { email: DOCENTE_EMAIL, password: DOCENTE_PASSWORD },
        ""
      )
    ).token
    carro = (await api(request, "POST", "/api/carros", { nombre: nombreCarro })).id
    for (let i = 1; i <= 4; i++)
      pcs.push(
        (
          await api(request, "POST", `/api/carros/${carro}/equipos`, {
            identificador: i,
            numeroSerie: `REPORTES-PRUEBA-${i}`,
          })
        ).id
      )
    await api(
      request,
      "POST",
      "/api/reservas",
      {
        materiaId: materia,
        equipoIds: pcs.slice(0, 2),
        fecha,
        horaInicio: "10:00",
        horaFin: "11:00",
      },
      docente
    )
    const cancelada = await api(
      request,
      "POST",
      "/api/reservas",
      {
        materiaId: materia,
        equipoIds: [pcs[2]],
        fecha,
        horaInicio: "11:00",
        horaFin: "12:00",
      },
      docente
    )
    await api(
      request,
      "POST",
      `/api/reservas/${cancelada.reservas[0].id}/cancelar`,
      { motivo: "No se dará esa clase" },
      docente
    )
    await api(request, "POST", "/api/bloqueos", {
      equipoIds: [pcs[2]],
      fecha,
      horaInicio: "10:00",
      horaFin: "11:00",
      motivo: "Evaluación institucional",
    })
    for (let i = 0; i < 3; i++) {
      const incidencia = await api(
        request,
        "POST",
        "/api/incidencias",
        {
          equipoId: pcs[i === 2 ? 3 : i],
          descripcion: `Diagnóstico de prueba ${i + 1}`,
          categoria: i < 2 ? "Batería" : "Cable",
          gravedad: i === 2 ? "GRAVE" : "LEVE",
        },
        docente
      )
      if (i < 2)
        await api(request, "PATCH", `/api/incidencias/${incidencia.id}`, {
          estado: i === 0 ? "EN_REPARACION" : "RESUELTA",
        })
    }
    await api(request, "PATCH", `/api/equipos/${pcs[3]}/estado`, {
      estado: "EN_MANTENIMIENTO",
      motivo: "Diagnóstico de cable",
    })
    // Historial real suficiente para revisar la segunda página, sin insertar SQL.
    for (let i = 0; i < 54; i++)
      await api(request, "PATCH", `/api/carros/${carro}`, {
        nombre: `Carro en revisión ${i + 1}`,
      })
    await api(request, "PATCH", `/api/carros/${carro}`, { nombre: nombreCarro })
  })

  test("las cantidades coinciden con la API, excluyen canceladas y distinguen equipos de clases", async ({
    page,
    request,
  }) => {
    await abrir(page)
    const equipos = (await api(request, "GET", `/api/ciclos/${ciclo}/uso-equipos`)).data
    expect(equipos).toHaveLength(3)
    expect(
      equipos.reduce(
        (s: number, u: { minutosReservados: number }) => s + u.minutosReservados,
        0
      )
    ).toBe(180)
    const docentes = (await api(request, "GET", `/api/ciclos/${ciclo}/uso-docentes`)).data
    expect(docentes).toMatchObject([{ cantidadReservas: 2, minutosReservados: 120 }])
    await expect(
      page.getByRole("table", { name: "Uso por equipo", exact: true }).getByRole("row")
    ).toHaveCount(4)
    await expect(
      page
        .getByRole("table", { name: "Horas reservadas por docente" })
        .getByRole("row")
        .last()
    ).toContainText("2h")
    await expect(
      page.getByText(
        /una clase de 10 equipos durante una hora suma 10 reservas y 10 horas de equipo/
      )
    ).toBeVisible()
    await expect(page.getByText(/2 sin resolver/)).toBeVisible()
    await movil(page)
  })

  test("las fechas filtran uso e incidencias y el CSV refleja el alcance y protege las fórmulas", async ({
    page,
  }) => {
    await abrir(page)
    await page.getByLabel("Desde", { exact: true }).fill(fecha)
    await page.getByLabel("Hasta", { exact: true }).fill(fecha)
    await expect(
      page.getByRole("table", { name: "Incidencias por equipo", exact: true })
    ).toHaveCount(0)
    await expect(
      page.getByRole("table", { name: "Uso por equipo", exact: true })
    ).toBeVisible()
    await expect(
      page
        .getByRole("table", { name: "Equipos fuera de circulación", exact: true })
        .getByText("PC 4")
    ).toBeVisible()
    const descarga = page.waitForEvent("download")
    await page
      .getByRole("button", { name: "Descargar CSV: Uso por equipo", exact: true })
      .click()
    const archivo = await descarga
    expect(archivo.suggestedFilename()).toBe(
      `uso-por-equipo_${anio}_${fecha}_a_${fecha}.csv`
    )
    const texto = await readFile((await archivo.path())!, "utf8")
    expect(texto.charCodeAt(0)).toBe(0xfeff)
    const filas = leerCSV(texto)
    expect(filas).toHaveLength(4)
    expect(filas[1][1]).toBe("'" + nombreCarro)
    expect(filas.slice(1).every((f) => f[2] === "1" && f[3] === "60")).toBe(true)
    const actual = page.waitForEvent("download")
    await page
      .getByRole("button", { name: "Descargar CSV: Estado del parque de equipos" })
      .click()
    expect((await actual).suggestedFilename()).toBe("estado-inventario_actual.csv")
  })

  test("un fallo de actualización oculta datos y descarga antiguos, y el reintento conserva filtros", async ({
    page,
  }) => {
    await abrir(page)
    await page.getByLabel("Desde", { exact: true }).fill(fecha)
    await expect(
      page.getByRole("table", { name: "Uso por equipo", exact: true })
    ).toBeVisible()
    let fallar = true
    await page.route(`**/api/ciclos/${ciclo}/uso-equipos*`, (r) =>
      fallar
        ? r.fulfill({ status: 503, json: { error: "Caída temporal" } })
        : r.continue()
    )
    await page.evaluate(() => window.dispatchEvent(new Event("visibilitychange")))
    await expect(page.getByText(/No pudimos consultar uso por equipo/)).toBeVisible({
      timeout: 20_000,
    })
    await expect(
      page.getByRole("table", { name: "Uso por equipo", exact: true })
    ).toHaveCount(0)
    await expect(
      page.getByRole("button", { name: "Descargar CSV: Uso por equipo", exact: true })
    ).toHaveCount(0)
    await expect(
      page.getByRole("table", { name: "Horas reservadas por docente" })
    ).toBeVisible()
    fallar = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await expect(
      page.getByRole("table", { name: "Uso por equipo", exact: true })
    ).toBeVisible()
    await expect(page.getByLabel("Desde", { exact: true })).toHaveValue(fecha)
  })

  test("auditoría permite paginar y leer cambios legibles sin modificar el historial", async ({
    page,
    request,
  }) => {
    await abrir(page, "/admin/auditoria")
    const inicial = (await api(request, "GET", "/api/auditoria")).meta.total
    expect(inicial).toBeGreaterThan(50)
    await page.getByRole("button", { name: "Siguiente", exact: true }).click()
    await expect(page.getByText(/Página 2 de/)).toBeVisible()
    await page.getByLabel("Acción", { exact: true }).selectOption("CAMBIO_TRANSACCIONAL")
    await page.getByLabel("Sobre qué").selectOption("equipo")
    await expect(page.getByRole("article").first()).toHaveAccessibleName(
      /Se actualizó el equipo/
    )
    const fila = page.getByRole("article").first()
    await fila.getByRole("button", { name: "Ver el detalle" }).click()
    await expect(
      fila.getByRole("columnheader", { name: "Antes", exact: true })
    ).toBeVisible()
    await expect(
      fila.getByRole("columnheader", { name: "Después", exact: true })
    ).toBeVisible()
    await expect(page.locator("main pre")).toHaveCount(0)
    expect(await page.locator("main").innerText()).not.toMatch(
      /password_hash|CAMBIO_TRANSACCIONAL|"operacion":/
    )
    await expect(
      fila.getByRole("region", { name: "Cambios registrados" })
    ).toHaveAttribute("tabindex", "0")
    expect((await api(request, "GET", "/api/auditoria")).meta.total).toBe(inicial)
    await movil(page)
  })

  test("cambiar el tipo de ficha quita el identificador anterior y los fallos de catálogos se recuperan", async ({
    page,
  }) => {
    await abrir(page, "/admin/auditoria")
    await page.getByLabel("Sobre qué").selectOption("carro")
    await page
      .getByRole("article")
      .first()
      .getByRole("button", { name: "Ver todo lo de este carro" })
      .click()
    await expect(
      page.getByRole("button", { name: "Ver todas", exact: true })
    ).toBeVisible()
    const cambio = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname.replace(/\/$/, "") === "/api/auditoria" &&
        new URL(r.url()).searchParams.get("entidad") === "equipo"
    )
    await page.getByLabel("Sobre qué").selectOption("equipo")
    expect(new URL((await cambio).url()).searchParams.has("entidadId")).toBe(false)
    await expect(
      page.getByRole("button", { name: "Ver todas", exact: true })
    ).toHaveCount(0)
    let fallar = true
    await page.route("**/api/auditoria/opciones", (r) =>
      fallar
        ? r.fulfill({ status: 503, json: { error: "Caída temporal" } })
        : r.continue()
    )
    await page.evaluate(() => window.dispatchEvent(new Event("visibilitychange")))
    await expect(page.getByText(/No pudimos consultar las acciones/)).toBeVisible({
      timeout: 20_000,
    })
    await expect(page.getByLabel("Sobre qué")).toBeDisabled()
    fallar = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await expect(page.getByLabel("Sobre qué")).toBeEnabled()
    await expect(page.getByLabel("Sobre qué")).toHaveValue("equipo")
  })

  test("los rangos inválidos no consultan y la API los rechaza con 400", async ({
    page,
    request,
  }) => {
    const consultas: string[] = []
    await abrir(page)
    await page.getByLabel("Desde", { exact: true }).fill(fecha)
    page.on("request", (r) => {
      if (
        /\/uso-equipos|\/incidencias\/resumen/.test(r.url()) &&
        new URL(r.url()).searchParams.get("hasta") === iso(hoy)
      )
        consultas.push(r.url())
    })
    await page.getByLabel("Hasta", { exact: true }).fill(iso(hoy))
    await expect(
      page
        .getByRole("alert")
        .filter({ hasText: "La fecha Desde no puede ser posterior a Hasta." })
    ).toBeVisible()
    await expect(
      page.getByRole("table", { name: "Uso por equipo", exact: true })
    ).toHaveCount(0)
    expect(consultas).toHaveLength(0)
    for (const ruta of [
      `/api/ciclos/${ciclo}/uso-equipos`,
      `/api/ciclos/${ciclo}/uso-docentes`,
      "/api/incidencias/resumen/por-equipo",
      "/api/incidencias/resumen/por-carro",
      "/api/incidencias/resumen/por-categoria",
    ]) {
      const r = await request.get(`${ruta}?desde=${fecha}&hasta=${iso(hoy)}`, {
        headers: { Authorization: `Bearer ${admin}` },
      })
      expect(r.status()).toBe(400)
    }
    await page.goto("/admin/auditoria")
    await page.getByLabel("Desde", { exact: true }).fill("2024-01-01")
    await page.getByLabel("Hasta", { exact: true }).fill("2026-01-03")
    await expect(
      page.getByText(/El intervalo entre las fechas no puede superar 366 días/)
    ).toBeVisible()
    await expect(page.getByRole("article")).toHaveCount(0)
    await page.getByRole("button", { name: "Quitar los filtros" }).click()
    await expect(page.getByRole("article").first()).toBeVisible()
  })

  test("el docente no accede a reportes ni auditoría por rutas, menú o API", async ({
    page,
    request,
  }) => {
    await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    await expect(page.getByRole("link", { name: "Reportes", exact: true })).toHaveCount(0)
    for (const path of ["/admin/reportes", "/admin/auditoria"]) {
      await page.goto(path)
      await expect(page).not.toHaveURL(new RegExp(path + "$"))
    }
    for (const path of [
      `/api/ciclos/${ciclo}/uso-equipos`,
      `/api/ciclos/${ciclo}/uso-docentes`,
      `/api/historico/${anio}/uso-equipos`,
      `/api/historico/${anio}/uso-docentes`,
      "/api/incidencias/resumen/por-equipo",
      "/api/incidencias/resumen/por-carro",
      "/api/incidencias/resumen/por-categoria",
      "/api/inventario/estado",
      "/api/inventario/fuera-de-circulacion",
      "/api/auditoria",
      "/api/auditoria/opciones",
    ]) {
      expect(
        (
          await request.get(path, { headers: { Authorization: `Bearer ${docente}` } })
        ).status()
      ).toBe(403)
      expect((await request.get(path)).status()).toBe(401)
    }
  })

  test("el histórico conserva los nombres y valores después del cierre y de eliminar al docente", async ({
    page,
    request,
  }) => {
    await api(request, "POST", `/api/ciclos/${ciclo}/archivar`, {})
    await api(request, "PATCH", `/api/carros/${carro}`, {
      nombre: "Nombre actual distinto",
    })
    await api(request, "PATCH", `/api/auth/usuarios/${usuario}/estado`, {
      estado: "BAJA",
    })
    await api(request, "DELETE", `/api/auth/usuarios/${usuario}`)
    await abrir(page)
    const tabla = page.getByRole("table", {
      name: `Histórico de uso por docente en ${anio}`,
    })
    await expect(tabla.getByText("Docente Reportes")).toBeVisible()
    await expect(tabla.getByText("Cuenta eliminada")).toBeVisible()
    await expect(tabla.getByRole("row").last()).toContainText("2h")
    await expect(
      page
        .getByRole("table", { name: `Histórico de uso por equipo en ${anio}` })
        .getByText(nombreCarro)
    ).toHaveCount(3)
    const descarga = page.waitForEvent("download")
    await page
      .getByRole("button", { name: `Descargar CSV: Uso por docente en ${anio}` })
      .click()
    const archivo = await descarga
    expect(archivo.suggestedFilename()).toBe(`historico-uso-por-docente_${anio}.csv`)
    expect(
      leerCSV(await readFile((await archivo.path())!, "utf8"))[1].slice(0, 3)
    ).toEqual(["Docente Reportes", "2", "120"])
    await movil(page)
  })

  test("la actividad de una cuenta eliminada sigue localizable desde auditoría", async ({
    page,
    request,
  }) => {
    await abrir(page, "/admin/auditoria")
    await page.getByLabel("Sobre qué").selectOption("incidencia")
    const fila = page
      .getByRole("article")
      .filter({ hasText: "Una cuenta que después se eliminó" })
      .first()
    await fila.getByRole("button", { name: "Ver acciones de esta cuenta" }).click()
    await expect(page.getByLabel("Quién")).toHaveValue(usuario)
    await expect(
      page.getByRole("option", { name: "Una cuenta que después se eliminó", exact: true })
    ).toBeAttached()
    expect(
      (
        await api(
          request,
          "GET",
          `/api/auditoria?usuarioId=${usuario}&entidad=incidencia`
        )
      ).meta.total
    ).toBeGreaterThan(0)
    await movil(page)
  })
})
