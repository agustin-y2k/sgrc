import { expect, test, type APIRequestContext, type Page } from "@playwright/test"
import { DOCENTE_PASSWORD, etiquetaDeReserva, login } from "./helpers"

test.describe("reservas reales sobre una base vacía y aislada", () => {
  test.skip(
    process.env.E2E_RESERVAS_AISLADAS !== "1",
    "requiere una base temporal vacía y E2E_RESERVAS_AISLADAS=1"
  )
  test.describe.configure({ mode: "serial" })
  test.setTimeout(90_000)
  let admin = ""
  const materias: string[] = [],
    equipos: string[] = [],
    tokens: string[] = [],
    usuarios: string[] = []
  const nombres = ["Dibujo Técnico", "TIC", "Matemática"]
  const correos = [
    "dibujo@revision.test",
    "tic@revision.test",
    "matematica@revision.test",
  ]
  const base = new Date()
  base.setDate(base.getDate() + 14)
  while (base.getDay() !== 1) base.setDate(base.getDate() + 1)
  const fecha = (semanas: number) => {
    const d = new Date(base)
    d.setDate(d.getDate() + semanas * 7)
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
  }

  async function api(
    request: APIRequestContext,
    token: string,
    method: string,
    ruta: string,
    data?: unknown
  ) {
    const res = await request.fetch(ruta, {
      method,
      headers: { Authorization: `Bearer ${token}` },
      data,
    })
    expect(res.ok(), `${method} ${ruta}: ${res.status()}`).toBe(true)
    return (res.headers()["content-type"] ?? "").includes("application/json")
      ? res.json()
      : undefined
  }
  async function loginApi(request: APIRequestContext, email: string, password: string) {
    return api(request, "", "POST", "/api/auth/login", { email, password })
  }
  const cuerpo = (
    materia: number,
    semana: number,
    equipo: number,
    inicio = "11:00",
    fin = "12:00"
  ) => ({
    materiaId: materias[materia],
    fecha: fecha(semana),
    horaInicio: inicio,
    horaFin: fin,
    equipoIds: [equipos[equipo]],
  })
  const serie = (materia: number, desde: number, hasta: number) => ({
    materiaId: materias[materia],
    diaSemana: "LUNES",
    fechaInicio: fecha(desde),
    fechaFin: fecha(hasta),
    horaInicio: "11:00",
    horaFin: "12:00",
    equipoIds: [equipos[0]],
  })
  async function formulario(page: Page, desde: number, hasta?: number) {
    await login(page, correos[0], DOCENTE_PASSWORD)
    await page.goto("/reservas/nueva")
    await page.getByLabel("Materia", { exact: true }).selectOption(materias[0])
    if (hasta !== undefined) {
      await page.getByRole("button", { name: "Se repite todas las semanas" }).click()
      await page.getByLabel("Desde", { exact: true }).fill(fecha(desde))
      await page.getByLabel("Hasta", { exact: true }).fill(fecha(hasta))
    } else await page.getByLabel("Fecha", { exact: true }).fill(fecha(desde))
    await page.getByLabel("Hora de inicio: hora").selectOption("11")
    await page.getByLabel("Hora de fin: hora").selectOption("12")
    await page.getByRole("checkbox", { name: /PC 1\b/ }).check()
  }
  async function confirmar(page: Page, recurrente = false) {
    const respuesta = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname ===
          (recurrente ? "/api/reservas/recurrentes" : "/api/reservas") &&
        r.request().method() === "POST"
    )
    await page.getByRole("button", { name: "Confirmar reserva", exact: true }).click()
    const res = await respuesta
    expect(res.status()).toBe(201)
    await expect(page).toHaveURL(/\/reservas$/)
    return res.json()
  }
  async function reserva(request: APIRequestContext, id: string) {
    return api(request, admin, "GET", `/api/reservas/${id}`)
  }

  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    const ingreso = await loginApi(
      request,
      process.env.E2E_ADMIN_EMAIL!,
      process.env.E2E_ADMIN_PASSWORD!
    )
    admin = ingreso.token
    // La guarda se comprueba antes de crear cualquier dato de la revisión.
    expect((await api(request, admin, "GET", "/api/ciclos")).data).toHaveLength(0)
    expect((await api(request, admin, "GET", "/api/equipos")).data).toHaveLength(0)
    await api(request, admin, "PUT", "/api/jornada", {
      tramos: [{ diaSemana: "LUNES", horaInicio: "08:00", horaFin: "16:00" }],
    })
    const ciclo = await api(request, admin, "POST", "/api/ciclos", {
      anio: base.getFullYear(),
    })
    const curso = await api(request, admin, "POST", `/api/ciclos/${ciclo.id}/cursos`, {
      anio: 3,
      division: "A",
    })
    for (let i = 0; i < 3; i++) {
      materias.push(
        (
          await api(request, admin, "POST", `/api/cursos/${curso.id}/materias`, {
            nombre: nombres[i],
          })
        ).id
      )
      await api(request, "", "POST", "/api/auth/registro", {
        nombre: "Docente",
        apellido: nombres[i],
        email: correos[i],
        password: DOCENTE_PASSWORD,
        cargoSolicitado: "DOCENTE",
        rolSolicitado: "TITULAR",
      })
      const personas = await api(request, admin, "GET", "/api/auth/usuarios?pageSize=200")
      const id = personas.data.find((u: { email: string }) => u.email === correos[i]).id
      usuarios.push(id)
      await api(request, admin, "PATCH", `/api/auth/usuarios/${id}/estado`, {
        estado: "APROBADA",
      })
      await api(request, admin, "POST", `/api/materias/${materias[i]}/docentes`, {
        usuarioId: id,
        rol: "TITULAR",
      })
      tokens.push((await loginApi(request, correos[i], DOCENTE_PASSWORD)).token)
    }
    const carro = await api(request, admin, "POST", "/api/carros", {
      nombre: "Carro de revisión",
      descripcion: "Datos ficticios",
    })
    for (let i = 1; i <= 3; i++)
      equipos.push(
        (
          await api(request, admin, "POST", `/api/carros/${carro.id}/equipos`, {
            identificador: i,
            numeroSerie: `REVISION-${i}`,
            softwareInstalado: i === 1 ? "AutoCAD 2026" : "AutoCAD 2007",
          })
        ).id
      )
    for (const [equipo, materia, prioridad] of [
      [0, 0, 1],
      [0, 1, 2],
      [1, 1, 1],
      [1, 2, 3],
      [2, 2, 1],
    ])
      await api(request, admin, "POST", "/api/preferencias", {
        equipoIds: [equipos[equipo]],
        materiaNombre: nombres[materia],
        prioridad,
      })
  })

  test("toma puntual repone TIC y Matemática en cascada y mantiene el horario completo", async ({
    page,
    request,
  }) => {
    const tic = await api(
      request,
      tokens[1],
      "POST",
      "/api/reservas",
      cuerpo(1, 0, 0, "10:00", "13:00")
    )
    const math = await api(
      request,
      tokens[2],
      "POST",
      "/api/reservas",
      cuerpo(2, 0, 1, "10:00", "13:00")
    )
    await formulario(page, 0)
    const resultado = await confirmar(page)
    expect(resultado.reservasReubicadas).toBe(2)
    expect(resultado.reservasCanceladas).toBe(0)
    for (const [original, sustituto, docente] of [
      [tic, equipos[1], usuarios[1]],
      [math, equipos[2], usuarios[2]],
    ]) {
      const actual = await reserva(request, original.reservas[0].id)
      expect(actual).toMatchObject({
        equipoId: sustituto,
        estado: "CONFIRMADA",
        horaInicio: "10:00",
        horaFin: "13:00",
        creadoPor: docente,
        reservaGrupoId: original.grupo.id,
      })
    }
    for (const token of [tokens[1], tokens[2]])
      await expect
        .poll(async () => {
          const avisos = await api(request, token, "GET", "/api/notifications/")
          return avisos.data.some((n: { tipo: string }) => n.tipo === "RESERVA_REUBICADA")
        })
        .toBe(true)
  })

  test("la toma recurrente desplaza sólo las fechas solicitadas de la serie ajena", async ({
    page,
    request,
  }) => {
    const tic = await api(
      request,
      tokens[1],
      "POST",
      "/api/reservas/recurrentes",
      serie(1, 1, 3)
    )
    await formulario(page, 1, 2)
    await page.getByRole("button", { name: "Comprobar serie" }).click()
    await page.getByRole("checkbox", { name: /Autorizo tomar/ }).check()
    const resultado = await confirmar(page, true)
    expect(resultado.reservasReubicadas).toBe(2)
    const listado = await api(
      request,
      admin,
      "GET",
      `/api/reservas?desde=${fecha(1)}&hasta=${fecha(3)}&incluirCanceladas=true&pageSize=200`
    )
    const filas = listado.data.filter(
      (r: { creadoPor: string }) => r.creadoPor === usuarios[1]
    )
    expect(filas).toHaveLength(3)
    for (let i = 0; i < 3; i++)
      expect(
        filas.find((r: { fecha: string }) => r.fecha === fecha(i + 1))
      ).toMatchObject({
        equipoId: equipos[i === 2 ? 0 : 1],
        estado: "CONFIRMADA",
        reglaRecurrenciaId: tic.reglaId,
      })
  })

  let grupos: { id: string }[] = []
  test("un conflicto que aparece después de la primera fecha requiere autorización y se confirma en la API real", async ({
    page,
    request,
  }) => {
    await api(request, tokens[2], "POST", "/api/reservas", cuerpo(2, 6, 0))
    await formulario(page, 4, 6)
    await expect(
      page.getByRole("button", { name: "Confirmar reserva", exact: true })
    ).toBeDisabled()
    await page.getByRole("button", { name: "Comprobar serie" }).click()
    const permiso = page.getByRole("checkbox", { name: /Autorizo tomar/ })
    await expect(permiso).toBeVisible()
    await expect(
      page.getByRole("button", { name: "Confirmar reserva", exact: true })
    ).toBeDisabled()
    await permiso.check()
    const resultado = await confirmar(page, true)
    grupos = resultado.grupos
    expect(grupos).toHaveLength(3)
    expect(resultado.reservasReubicadas).toBe(1)
  })

  test("cancelación docente de esta fecha y siguientes conserva la primera clase", async ({
    page,
    request,
  }) => {
    await login(page, correos[0], DOCENTE_PASSWORD)
    await page.goto(`/reservas?desde=${fecha(4)}&hasta=${fecha(6)}`)
    const tarjeta = page
      .locator('[data-slot="card"]')
      .filter({ hasText: etiquetaDeReserva(fecha(5), "11:00", "12:00") })
    await tarjeta.getByRole("button", { name: /^Cancelar$/ }).click()
    await tarjeta.getByLabel("Esta fecha y todas las siguientes").check()
    await tarjeta.getByRole("button", { name: /Confirmar cancelación/ }).click()
    await expect(tarjeta).toHaveCount(0)
    for (let i = 0; i < 3; i++)
      expect(
        await api(request, admin, "GET", `/api/grupos/${grupos[i].id}`)
      ).toMatchObject({ estado: i === 0 ? "CONFIRMADA" : "CANCELADA" })
  })

  test("administración exige motivo al cancelar una clase ajena", async ({
    page,
    request,
  }) => {
    await login(page, process.env.E2E_ADMIN_EMAIL!, process.env.E2E_ADMIN_PASSWORD!)
    await page.goto(
      `/reservas?desde=${fecha(0)}&hasta=${fecha(0)}&creadoPor=${usuarios[0]}`
    )
    const tarjeta = page
      .locator('[data-slot="card"]')
      .filter({ hasText: etiquetaDeReserva(fecha(0), "11:00", "12:00") })
    await tarjeta.getByRole("button", { name: /^Cancelar$/ }).click()
    const confirmar = tarjeta.getByRole("button", { name: /Confirmar cancelación/ })
    await expect(confirmar).toBeDisabled()
    await tarjeta.getByLabel(/Motivo/).fill("Actividad institucional de revisión")
    await confirmar.click()
    await expect(tarjeta).toHaveCount(0)
    const datos = await api(
      request,
      admin,
      "GET",
      `/api/reservas?desde=${fecha(0)}&hasta=${fecha(0)}&creadoPor=${usuarios[0]}&incluirCanceladas=true`
    )
    expect(datos.data[0]).toMatchObject({
      estado: "CANCELADA",
      motivoCancelacion: "Actividad institucional de revisión",
    })
  })

  test("jornada inaccesible bloquea el formulario y reintentar recupera los datos", async ({
    page,
  }) => {
    await login(page, correos[0], DOCENTE_PASSWORD)
    let falla = true
    await page.route("**/api/jornada", (r) =>
      falla ? r.fulfill({ status: 503, body: "Jornada inaccesible" }) : r.continue()
    )
    await page.goto("/reservas/nueva")
    await page.getByLabel("Materia", { exact: true }).selectOption(materias[0])
    await page.getByLabel("Fecha", { exact: true }).fill(fecha(8))
    await page.getByLabel("Hora de inicio: hora").selectOption("11")
    await page.getByLabel("Hora de fin: hora").selectOption("12")
    await expect(page.getByText(/No pudimos consultar la jornada/)).toBeVisible({
      timeout: 15000,
    })
    await expect(
      page.getByRole("button", { name: "Confirmar reserva", exact: true })
    ).toBeDisabled()
    falla = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await expect(page.getByRole("checkbox", { name: /PC 1\b/ })).toBeVisible()
    await expect(page.getByLabel("Fecha", { exact: true })).toHaveValue(fecha(8))
  })
})
