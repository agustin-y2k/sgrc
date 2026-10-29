import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext, type Page } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

test.describe("cierre anual real en una base efímera vacía", () => {
  test.skip(
    process.env.E2E_CIERRE_AISLADO !== "1",
    "requiere E2E_CIERRE_AISLADO=1 y base vacía"
  )
  test.describe.configure({ mode: "serial" })
  test.setTimeout(90_000)
  const email = process.env.E2E_ADMIN_EMAIL!,
    password = process.env.E2E_ADMIN_PASSWORD!
  const hoy = new Date(),
    anio = hoy.getFullYear()
  const iso = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
  // La última franja termina a medianoche, sin generar una hora de fin anterior.
  const minuto = Math.min(
    23 * 60,
    hoy.getHours() * 60 + Math.floor(hoy.getMinutes() / 5) * 5
  )
  const hora = (m: number) =>
    `${String(Math.floor((m % 1440) / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`
  const horario = {
    fecha: iso(hoy),
    horaInicio: hora(minuto),
    horaFin: hora(minuto + 60),
  }
  let admin = "",
    docente = "",
    docenteId = "",
    ciclo = "",
    nuevo = "",
    materia = "",
    equipo = "",
    prestamo = "",
    incidencia = ""
  let cierre: unknown,
    marcas: unknown,
    usoEquipos: { cantidadReservas: number; minutosReservados: number }[],
    usoDocentes: { cantidadReservas: number; minutosReservados: number }[]
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
  async function abrir(p: Page) {
    await login(p, email, password)
    await p.goto("/admin/academico")
  }
  async function confirmar(p: Page, label: string) {
    const respuesta = p.waitForResponse(
      (r) =>
        /\/api\/ciclos\/[^/]+\/archivar$/.test(new URL(r.url()).pathname) &&
        r.request().method() === "POST"
    )
    await p.getByRole("button", { name: label, exact: true }).click()
    return respuesta
  }
  async function movil(p: Page) {
    await p.setViewportSize({ width: 320, height: 780 })
    expect(
      await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)
    ).toBe(true)
    for (const el of await p
      .locator('main button:visible, main input:visible, main [role="alert"]:visible')
      .all()) {
      const b = await el.boundingBox()
      if (b) {
        expect(b.x).toBeGreaterThanOrEqual(0)
        expect(b.x + b.width).toBeLessThanOrEqual(320)
      }
    }
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
    expect((await api(request, "GET", "/api/ciclos")).data).toHaveLength(0)
    expect((await api(request, "GET", "/api/equipos")).data).toHaveLength(0)
    expect((await api(request, "GET", "/api/prestamos")).data).toHaveLength(0)
    await api(request, "PUT", "/api/jornada", {
      tramos: [
        "DOMINGO",
        "LUNES",
        "MARTES",
        "MIERCOLES",
        "JUEVES",
        "VIERNES",
        "SABADO",
      ].flatMap((diaSemana) => [
        { diaSemana, horaInicio: "00:00", horaFin: "12:00" },
        { diaSemana, horaInicio: "12:00", horaFin: "00:00" },
      ]),
    })
    ciclo = (await api(request, "POST", "/api/ciclos", { anio })).id
    const curso = await api(request, "POST", `/api/ciclos/${ciclo}/cursos`, {
      anio: 4,
      division: "A",
      modalidad: "Construcción",
    })
    materia = (
      await api(request, "POST", `/api/cursos/${curso.id}/materias`, {
        nombre: "Dibujo Técnico",
      })
    ).id
    await api(request, "POST", `/api/ciclos/${ciclo}/espacios`, { nombre: "Biblioteca" })
    await api(
      request,
      "POST",
      "/api/auth/registro",
      {
        nombre: "Docente",
        apellido: "Cierre",
        email: DOCENTE_EMAIL,
        password: DOCENTE_PASSWORD,
        cargoSolicitado: "DOCENTE",
        rolSolicitado: "TITULAR",
      },
      ""
    )
    docenteId = (await api(request, "GET", "/api/auth/usuarios?pageSize=200")).data.find(
      (u: { email: string }) => u.email === DOCENTE_EMAIL
    ).id
    await api(request, "PATCH", `/api/auth/usuarios/${docenteId}/estado`, {
      estado: "APROBADA",
    })
    await api(request, "POST", `/api/materias/${materia}/docentes`, {
      usuarioId: docenteId,
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
    const carro = await api(request, "POST", "/api/carros", {
      nombre: "Carro del cierre",
    })
    equipo = (
      await api(request, "POST", `/api/carros/${carro.id}/equipos`, {
        identificador: 1,
        numeroSerie: "CIERRE-FICTICIO-1",
      })
    ).id
    await api(request, "POST", "/api/preferencias", {
      equipoIds: [equipo],
      materiaNombre: "Dibujo Técnico",
      modalidad: "Construcción",
      anio: 4,
      division: "A",
      prioridad: 1,
    })
    await api(request, "POST", "/api/preferencias", {
      equipoIds: [equipo],
      materiaNombre: "Dibujo Técnico",
      modalidad: "Construcción",
      anio: 4,
      division: "A",
      prioridad: 1,
      cursoExacto: true,
    })
    marcas = (await api(request, "GET", `/api/equipos/${equipo}/preferencias`)).data
    const reserva = await api(
      request,
      "POST",
      "/api/reservas",
      { materiaId: materia, equipoIds: [equipo], ...horario },
      docente
    )
    prestamo = (
      await api(request, "POST", "/api/prestamos/por-reserva", {
        reservaIds: [reserva.reservas[0].id],
        equiposEsperados: { [reserva.reservas[0].id]: reserva.reservas[0].equipoId },
        retiradoPor: "Estudiante del cierre",
      })
    ).entregadas[0].id
    incidencia = (
      await api(request, "POST", "/api/incidencias", {
        equipoId: equipo,
        descripcion: "Falta una tecla; equipo de prueba",
        gravedad: "LEVE",
      })
    ).id
    usoEquipos = (await api(request, "GET", `/api/ciclos/${ciclo}/uso-equipos`)).data
    usoDocentes = (await api(request, "GET", `/api/ciclos/${ciclo}/uso-docentes`)).data
    expect(usoEquipos).toHaveLength(1)
    expect(usoDocentes).toHaveLength(1)
  })

  test("un fallo de lectura bloquea la creación y se recupera sin inventar una lista vacía", async ({
    page,
  }) => {
    await login(page, email, password)
    let falla = true
    await page.route("**/api/ciclos", (r) =>
      falla && r.request().method() === "GET"
        ? r.fulfill({ status: 503, json: { error: "Lectura de prueba inaccesible" } })
        : r.continue()
    )
    await page.goto("/admin/academico")
    await expect(page.getByText(/No pudimos consultar los ciclos lectivos/)).toBeVisible({
      timeout: 15_000,
    })
    await expect(page.getByText(/Todavía no hay ningún ciclo/)).toHaveCount(0)
    await expect(
      page.getByRole("button", { name: "Crear ciclo", exact: true })
    ).toBeDisabled()
    falla = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await expect(
      page.getByRole("button", {
        name: "Cerrar el año y abrir el siguiente",
        exact: true,
      })
    ).toBeEnabled()
  })

  test("el docente no puede cerrar desde la interfaz ni directamente por API", async ({
    page,
    request,
  }) => {
    await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    await page.goto("/admin/academico")
    await expect(page).not.toHaveURL(/\/admin\/academico$/)
    const res = await request.post(`/api/ciclos/${ciclo}/archivar`, {
      headers: { Authorization: `Bearer ${docente}` },
      data: { clonarA: anio + 1 },
    })
    expect(res.status()).toBe(403)
    expect((await api(request, "GET", `/api/ciclos/${ciclo}`)).activo).toBe(true)
  })

  test("cierra, copia cursos y espacios, y dos solicitudes conservan un único resultado", async ({
    page,
    request,
  }) => {
    await abrir(page)
    await page
      .getByRole("button", { name: "Cerrar el año y abrir el siguiente", exact: true })
      .click()
    await expect(page.getByText(/Cerrar el año no registra su devolución/)).toBeVisible()
    await movil(page)
    let liberar!: () => void, recibido!: () => void
    const puerta = new Promise<void>((r) => {
        liberar = r
      }),
      llego = new Promise<void>((r) => {
        recibido = r
      })
    await page.route(`**/api/ciclos/${ciclo}/archivar`, async (r) => {
      const res = await r.fetch()
      expect(res.ok()).toBe(true)
      cierre = await res.json()
      recibido()
      await puerta
      await r.fulfill({ response: res })
    })
    const pendiente = confirmar(page, `Cerrar ${anio} y crear ${anio + 1}`)
    try {
      await llego
      await expect(page.getByLabel(/Año del ciclo siguiente/)).toBeDisabled()
      await expect(
        page.getByLabel("Cerrar sin crear otro ciclo", { exact: true })
      ).toBeDisabled()
      expect(
        await api(request, "POST", `/api/ciclos/${ciclo}/archivar`, { clonarA: anio + 1 })
      ).toEqual(cierre)
    } finally {
      liberar()
    }
    expect((await pendiente).ok()).toBe(true)
    await expect(page.getByText(/Ciclo cerrado/)).toBeVisible()
    const ciclos = (await api(request, "GET", "/api/ciclos")).data
    expect(ciclos).toHaveLength(2)
    nuevo = ciclos.find((c: { activo: boolean }) => c.activo).id
    expect(ciclos.find((c: { id: string }) => c.id === ciclo)).toMatchObject({
      activo: false,
      archivado: true,
    })
    const cursos = (await api(request, "GET", `/api/ciclos/${nuevo}/cursos`)).data
    expect(cursos).toHaveLength(1)
    expect(cursos[0]).toMatchObject({
      anio: 4,
      division: "A",
      modalidad: "Construcción",
      archivado: false,
    })
    expect((await api(request, "GET", `/api/ciclos/${nuevo}/espacios`)).data).toEqual([
      expect.objectContaining({ nombre: "Biblioteca", archivado: false }),
    ])
    expect(
      (await api(request, "GET", `/api/ciclos/${nuevo}/asignaciones`)).data
    ).toHaveLength(0)
    expect(
      (await api(request, "GET", `/api/ciclos/${ciclo}/asignaciones`)).data
    ).toHaveLength(1)
    expect((await api(request, "GET", "/api/reservas?pageSize=200")).data).toHaveLength(0)
    expect(
      (await api(request, "GET", `/api/equipos/${equipo}/preferencias`)).data
    ).toEqual(marcas)
    expect(marcas).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ cursoExacto: false }),
        expect.objectContaining({ cursoExacto: true }),
      ])
    )
    const auditoria = (
      await api(request, "GET", "/api/auditoria/?entidad=ciclo_lectivo&pageSize=200")
    ).data
    expect(auditoria).toContainEqual(
      expect.objectContaining({
        accion: "CICLO_ARCHIVADO_RESERVAS_ELIMINADAS",
        entidadId: ciclo,
      })
    )
    expect(auditoria).toContainEqual(
      expect.objectContaining({ accion: "CICLO_CLONADO", entidadId: nuevo })
    )
    await expect(page.getByText("Ocultar cursos", { exact: true })).toBeVisible()
  })

  test("conserva el histórico, las incidencias y el préstamo abierto, que puede devolverse después del cierre", async ({
    page,
    request,
  }) => {
    const eq = (await api(request, "GET", `/api/historico/${anio}/uso-equipos`)).data
    const doc = (await api(request, "GET", `/api/historico/${anio}/uso-docentes`)).data
    expect(eq).toHaveLength(1)
    expect(doc).toHaveLength(1)
    expect(eq[0]).toMatchObject({
      cantidadReservas: usoEquipos[0].cantidadReservas,
      minutosReservados: usoEquipos[0].minutosReservados,
    })
    expect(doc[0]).toMatchObject({
      cantidadReservas: usoDocentes[0].cantidadReservas,
      minutosTotales: usoDocentes[0].minutosReservados,
    })
    expect(
      (await api(request, "GET", `/api/incidencias/${incidencia}`)).descripcion
    ).toBe("Falta una tecla; equipo de prueba")
    const afuera = (await api(request, "GET", "/api/prestamos")).data
    expect(afuera).toHaveLength(1)
    expect(afuera[0]).toMatchObject({
      id: prestamo,
      equipoId: equipo,
      abierto: true,
      entregadoAUsuarioId: docenteId,
      retiradoPor: "Estudiante del cierre",
      materiaNombre: "Dibujo Técnico",
      destino: "4°A · Construcción",
      contextoAcademico: {
        materiaId: materia,
        materiaNombre: "Dibujo Técnico",
        contenedorTipo: "CURSO",
        contenedorNombre: "4°A",
        modalidad: "Construcción",
        cicloId: ciclo,
        cicloAnio: anio,
        reconstruido: false,
      },
    })
    expect(afuera[0].reservaId).toBeUndefined()
    await login(page, email, password)
    await page.goto("/admin/entregas")
    await expect(
      page.getByText(`Dibujo Técnico · 4°A · Construcción · Ciclo ${anio} · 1 equipo`, {
        exact: true,
      })
    ).toBeVisible()
    await page
      .getByRole("button", { name: "Recibir 1 equipo de Docente Cierre", exact: true })
      .click()
    await expect
      .poll(async () => (await api(request, "GET", "/api/prestamos")).data.length)
      .toBe(0)
    expect(
      (await api(request, "GET", `/api/equipos/${equipo}/prestamos`)).data[0]
    ).toMatchObject({
      id: prestamo,
      abierto: false,
      retiradoPor: "Estudiante del cierre",
      materiaNombre: "Dibujo Técnico",
      contextoAcademico: {
        materiaNombre: "Dibujo Técnico",
        contenedorNombre: "4°A",
        cicloAnio: anio,
      },
    })
    expect(
      (await api(request, "GET", `/api/historico/${anio}/uso-equipos`)).data
    ).toEqual(eq)
  })

  test("el ciclo nuevo exige reasignar docentes y conserva la prioridad por nombre y alcance", async ({
    request,
  }) => {
    expect(
      (await api(request, "GET", "/api/mis-materias", undefined, docente)).data
    ).toHaveLength(0)
    const curso = (await api(request, "GET", `/api/ciclos/${nuevo}/cursos`)).data[0]
    const nuevaMateria = (await api(request, "GET", `/api/cursos/${curso.id}/materias`))
      .data[0]
    const datos = {
      materiaId: nuevaMateria.id,
      equipoIds: [equipo],
      fecha: `${anio + 1}-05-04`,
      horaInicio: "10:00",
      horaFin: "11:00",
    }
    const denegada = await request.post("/api/reservas", {
      headers: { Authorization: `Bearer ${docente}` },
      data: datos,
    })
    expect(denegada.status()).toBe(409)
    expect(await denegada.text()).toMatch(/asignado/i)
    await api(request, "POST", `/api/materias/${nuevaMateria.id}/docentes`, {
      usuarioId: docenteId,
      rol: "TITULAR",
    })
    expect(
      (await api(request, "GET", "/api/mis-materias", undefined, docente)).data
    ).toHaveLength(1)
    const disponibilidad = await api(
      request,
      "GET",
      `/api/equipos-disponibles?fecha=${datos.fecha}&horaInicio=10:00&horaFin=11:00&materiaId=${nuevaMateria.id}`,
      undefined,
      docente
    )
    expect(
      disponibilidad.data.find((e: { equipoId: string }) => e.equipoId === equipo)
    ).toMatchObject({ tramo: "PREFERENTE" })
    await api(request, "POST", "/api/reservas", datos, docente)
  })

  test("un cierre sin ciclo nuevo recupera una respuesta perdida y rechaza cambiar su destino", async ({
    page,
    request,
  }) => {
    await abrir(page)
    await page
      .getByRole("button", { name: "Cerrar el año y abrir el siguiente", exact: true })
      .click()
    await page.getByLabel("Cerrar sin crear otro ciclo", { exact: true }).check()
    const originales = (await api(request, "GET", "/api/ciclos")).data
    let primera = true,
      confirmado: unknown
    await page.route(`**/api/ciclos/${nuevo}/archivar`, async (r) => {
      if (!primera) return r.continue()
      primera = false
      const res = await r.fetch()
      expect(res.ok()).toBe(true)
      confirmado = await res.json()
      await r.fulfill({
        status: 503,
        json: { error: "No se pudo recibir la respuesta del cierre de prueba." },
      })
    })
    const nombre = `Cerrar ${anio + 1} sin abrir otro ciclo`
    expect((await confirmar(page, nombre)).status()).toBe(503)
    await expect(
      page.getByText("No se pudo recibir la respuesta del cierre de prueba.")
    ).toBeVisible()
    await expect(
      page.getByLabel("Cerrar sin crear otro ciclo", { exact: true })
    ).toBeChecked()
    expect(await (await confirmar(page, nombre)).json()).toEqual(confirmado)
    await expect(page.getByText(/No se creó un ciclo nuevo/)).toBeVisible()
    const ciclos = (await api(request, "GET", "/api/ciclos")).data
    expect(ciclos).toHaveLength(originales.length)
    expect(
      ciclos.every(
        (c: { archivado: boolean; activo: boolean }) => c.archivado && !c.activo
      )
    ).toBe(true)
    expect((await api(request, "GET", "/api/reservas?pageSize=200")).data).toHaveLength(0)
    const otro = await request.post(`/api/ciclos/${nuevo}/archivar`, {
      headers: { Authorization: `Bearer ${admin}` },
      data: { clonarA: anio + 2 },
    })
    expect(otro.status()).toBe(409)
    expect(
      (await api(request, "GET", `/api/historico/${anio + 1}/uso-equipos`)).data[0]
    ).toMatchObject({ cantidadReservas: 1, minutosReservados: 60 })
  })

  test("el histórico con una lectura fallida no anuncia un año sin uso y permite reintentar", async ({
    page,
  }) => {
    await login(page, email, password)
    let falla = true
    await page.route(`**/api/historico/${anio + 1}/uso-equipos`, (r) =>
      falla
        ? r.fulfill({ status: 503, json: { error: "Histórico de prueba inaccesible" } })
        : r.continue()
    )
    await page.goto("/admin/reportes")
    const panel = page
      .locator('[data-slot="card"]')
      .filter({ hasText: `Uso por equipo en ${anio + 1}` })
    await expect(
      panel.getByText(/No pudimos consultar el histórico de equipos/)
    ).toBeVisible({ timeout: 15_000 })
    await expect(panel.getByText(/sin ninguna reserva registrada/)).toHaveCount(0)
    await expect(panel.getByRole("button", { name: /Descargar CSV/ })).toHaveCount(0)
    falla = false
    await panel.getByRole("button", { name: "Reintentar", exact: true }).click()
    await expect(panel.getByText("PC 1", { exact: true })).toBeVisible()
    await movil(page)
    const tabla = page.getByRole("region", {
      name: `Histórico de uso por equipo en ${anio + 1}`,
      exact: true,
    })
    await tabla.focus()
    await expect(tabla).toBeFocused()
    await tabla.press("ArrowRight")
    await expect.poll(() => tabla.evaluate((el) => el.scrollLeft)).toBeGreaterThan(0)
  })

  test("después de cerrar sin abrir otro ciclo se puede crear uno manualmente con un año libre", async ({
    page,
    request,
  }) => {
    await abrir(page)
    const campo = page.getByLabel("Año", { exact: true })
    const crear = page.getByRole("button", { name: "Crear ciclo", exact: true })
    for (const valor of [String(anio), "1999", "2101"]) {
      await campo.fill(valor)
      await expect(crear).toBeDisabled()
    }
    await campo.fill(String(anio + 2))
    const respuesta = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === "/api/ciclos" && r.request().method() === "POST"
    )
    await crear.click()
    expect((await respuesta).status()).toBe(201)
    await expect(page.getByText("Activo", { exact: true })).toBeVisible()
    const ciclos = (await api(request, "GET", "/api/ciclos")).data
    expect(ciclos).toHaveLength(3)
    expect(ciclos.filter((c: { activo: boolean }) => c.activo)).toEqual([
      expect.objectContaining({ anio: anio + 2, archivado: false }),
    ])
    expect((await api(request, "GET", "/api/reservas?pageSize=200")).data).toHaveLength(0)
  })
})
