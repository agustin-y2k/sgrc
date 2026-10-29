import { expect, test, type APIRequestContext } from "@playwright/test"
import { login } from "./helpers"

test.describe("reservas dentro del año lectivo", () => {
  test.skip(process.env.E2E_ANIO_LECTIVO_AISLADO !== "1", "requiere una base efímera")
  test.describe.configure({ mode: "serial" })
  let admin = "",
    docente = "",
    materia = "",
    equipo = "",
    anio = 0
  async function api(
    request: APIRequestContext,
    method: string,
    ruta: string,
    data?: unknown,
    token = admin
  ) {
    const r = await request.fetch(ruta, {
      method,
      data,
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(r.ok(), `${method} ${ruta}: ${r.status()}`).toBe(true)
    return r.headers()["content-type"]?.includes("application/json")
      ? r.json()
      : undefined
  }
  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    admin = (
      await api(
        request,
        "POST",
        "/api/auth/login",
        { email: process.env.E2E_ADMIN_EMAIL, password: process.env.E2E_ADMIN_PASSWORD },
        ""
      )
    ).token
    docente = (
      await api(
        request,
        "POST",
        "/api/auth/login",
        {
          email: process.env.E2E_DOCENTE_EMAIL,
          password: process.env.E2E_DOCENTE_PASSWORD,
        },
        ""
      )
    ).token
    await api(request, "PUT", "/api/jornada", {
      tramos: [
        "LUNES",
        "MARTES",
        "MIERCOLES",
        "JUEVES",
        "VIERNES",
        "SABADO",
        "DOMINGO",
      ].map((diaSemana) => ({ diaSemana, horaInicio: "06:00", horaFin: "02:00" })),
    })
    const m = (await api(request, "GET", "/api/mis-materias", undefined, docente)).data[0]
    materia = m.materiaId
    anio = m.cicloAnio
    equipo = (await api(request, "GET", "/api/equipos")).data[0].id
  })
  for (const rol of ["ADMIN", "DOCENTE"] as const) {
    test(`${rol}: API rechaza el siguiente año y las series completas sin crear reservas`, async ({
      request,
    }) => {
      const token = rol === "ADMIN" ? admin : docente
      const antes = await api(
        request,
        "GET",
        "/api/reservas?incluirCanceladas=true",
        undefined,
        token
      )
      const base = {
        materiaId: materia,
        horaInicio: "08:00",
        horaFin: "09:00",
        equipoIds: [equipo],
      }
      const serie = {
        ...base,
        fechaInicio: `${anio}-12-28`,
        fechaFin: `${anio + 1}-01-01`,
        diaSemana: "LUNES",
      }
      for (const [ruta, data] of [
        ["/api/reservas", { ...base, fecha: `${anio + 1}-01-01` }],
        ["/api/reservas/recurrentes", serie],
        ["/api/reservas/recurrentes/previsualizar", serie],
        [
          "/api/reservas",
          { ...base, fecha: `${anio}-12-31`, horaInicio: "23:00", horaFin: "01:00" },
        ],
      ] as const) {
        const r = await request.post(ruta, {
          data,
          headers: { Authorization: `Bearer ${token}` },
        })
        expect(r.status()).toBe(400)
        expect(await r.text()).toContain("año del ciclo lectivo")
      }
      const disponibles = await request.get(
        `/api/equipos-disponibles?materiaId=${materia}&fecha=${anio + 1}-01-01&horaInicio=08:00&horaFin=09:00`,
        { headers: { Authorization: `Bearer ${token}` } }
      )
      expect(disponibles.status()).toBe(400)
      const despues = await api(
        request,
        "GET",
        "/api/reservas?incluirCanceladas=true",
        undefined,
        token
      )
      expect(despues.meta.total).toBe(antes.meta.total)
    })
    test(`${rol}: formulario limita las fechas al ciclo e informa la franja nocturna inválida`, async ({
      page,
    }) => {
      await login(
        page,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_EMAIL" : "E2E_DOCENTE_EMAIL"]!,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_PASSWORD" : "E2E_DOCENTE_PASSWORD"]!
      )
      await page.goto("/reservas/nueva")
      await expect(page.getByLabel("Materia", { exact: true })).toContainText(
        "Programación"
      )
      await page.getByLabel("Materia", { exact: true }).selectOption(materia)
      await expect(page.getByLabel("Fecha", { exact: true })).toHaveAttribute(
        "max",
        `${anio}-12-31`
      )
      await page.getByLabel("Fecha", { exact: true }).fill(`${anio + 1}-01-01`)
      await expect(page.getByRole("alert")).toContainText(
        `Las reservas del ciclo ${anio}`
      )
      await expect(page.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
      await page.getByLabel("Fecha", { exact: true }).fill(`${anio}-12-31`)
      await page.getByLabel("Hora de inicio: hora", { exact: true }).selectOption("23")
      await page.getByLabel("Hora de inicio: minutos", { exact: true }).selectOption("00")
      await page.getByLabel("Hora de fin: hora", { exact: true }).selectOption("01")
      await page.getByLabel("Hora de fin: minutos", { exact: true }).selectOption("00")
      await expect(page.getByRole("alert")).toContainText(
        `Las reservas del ciclo ${anio}`
      )
      await page.getByRole("button", { name: "Se repite todas las semanas" }).click()
      await expect(page.getByLabel("Desde", { exact: true })).toHaveAttribute(
        "max",
        `${anio}-12-31`
      )
      await expect(page.getByLabel("Hasta", { exact: true })).toHaveAttribute(
        "max",
        `${anio}-12-31`
      )
      await page.getByLabel("Desde", { exact: true }).fill(`${anio}-12-28`)
      await page.getByLabel("Hasta", { exact: true }).fill(`${anio + 1}-01-01`)
      await expect(page.getByRole("alert")).toContainText(
        `Las reservas del ciclo ${anio}`
      )
      await expect(page.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
    })
  }
  test("el 31 de diciembre puede terminar exactamente a medianoche", async ({
    request,
  }) => {
    const creada = await api(
      request,
      "POST",
      "/api/reservas",
      {
        materiaId: materia,
        fecha: `${anio}-12-31`,
        horaInicio: "23:00",
        horaFin: "00:00",
        equipoIds: [equipo],
      },
      docente
    )
    expect(creada.reservas).toHaveLength(1)
    expect(creada.reservas[0].fecha).toBe(`${anio}-12-31`)
  })
})
