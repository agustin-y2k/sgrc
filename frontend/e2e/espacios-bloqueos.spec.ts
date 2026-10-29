import { expect, test, type APIRequestContext } from "@playwright/test"
import { login } from "./helpers"

test.describe("integridad de espacios, bloqueos y navegación de canceladas", () => {
  test.skip(
    process.env.E2E_ESPACIOS_BLOQUEOS_AISLADOS !== "1",
    "requiere una base efímera"
  )
  test.describe.configure({ mode: "serial" })
  let admin = "",
    docente = "",
    materiaNombre = ""
  const fecha = new Date()
  fecha.setDate(fecha.getDate() + 1)
  const dia = new Intl.DateTimeFormat("sv-SE", {
    timeZone: "America/Argentina/Buenos_Aires",
  }).format(fecha)
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
    const pcs = (await api(request, "GET", "/api/equipos")).data
    for (const [indices, motivo] of [
      [[0, 1], "Mantenimiento independiente"],
      [[2, 3], "Evaluación independiente"],
    ] as const) {
      const resultado = await api(request, "POST", "/api/bloqueos", {
        equipoIds: indices.map((i) => pcs[i].id),
        fecha: dia,
        horaInicio: "08:00",
        horaFin: "09:00",
        motivo,
      })
      const ids = resultado.bloqueos.map(
        (b: { bloqueoOperacionId: string }) => b.bloqueoOperacionId
      )
      expect(new Set(ids).size).toBe(1)
      expect(ids[0]).toBeTruthy()
    }
    const materia = (await api(request, "GET", "/api/mis-materias", undefined, docente))
      .data[0]
    materiaNombre = materia.materiaNombre
    const creada = await api(
      request,
      "POST",
      "/api/reservas",
      {
        materiaId: materia.materiaId,
        fecha: dia,
        horaInicio: "11:00",
        horaFin: "12:00",
        equipoIds: [pcs[4].id],
      },
      docente
    )
    await api(
      request,
      "POST",
      `/api/reservas/${creada.reservas[0].id}/cancelar`,
      { motivo: "Prueba de navegación de canceladas" },
      docente
    )
  })
  test("la materia interna se protege y se renombra desde el espacio", async ({
    request,
  }) => {
    const ciclo = (await api(request, "GET", "/api/ciclos")).data[0]
    const espacio = await api(request, "POST", `/api/ciclos/${ciclo.id}/espacios`, {
      nombre: "Biblioteca de prueba",
    })
    const interna = (await api(request, "GET", "/api/mis-materias")).data.find(
      (m: { materiaNombre: string }) => m.materiaNombre === "Biblioteca de prueba"
    )
    expect(interna).toBeTruthy()
    for (const method of ["PATCH", "DELETE"]) {
      const r = await request.fetch(`/api/materias/${interna.materiaId}`, {
        method,
        headers: { Authorization: `Bearer ${admin}` },
        data: method === "PATCH" ? { nombre: "Nombre incorrecto" } : undefined,
      })
      expect(r.status()).toBe(409)
      expect(await r.text()).toMatch(/Espacios/)
    }
    await api(request, "PATCH", `/api/espacios/${espacio.id}`, {
      nombre: "Biblioteca central de prueba",
    })
    const actual = await api(request, "GET", `/api/materias/${interna.materiaId}`)
    expect(actual.nombre).toBe("Biblioteca central de prueba")
    expect(actual.internaDeEspacio).toBe(true)
    await api(request, "DELETE", `/api/espacios/${espacio.id}`)
  })
  test("los bloqueos coincidentes tienen tarjetas, motivos y cancelaciones independientes", async ({
    page,
    request,
  }) => {
    const paginas = []
    for (let page = 1; page <= 2; page++) {
      const r = await api(
        request,
        "GET",
        `/api/reservas?desde=${dia}&hasta=${dia}&porClases=true&page=${page}&pageSize=1`
      )
      expect(r.meta.total).toBe(2)
      expect(r.data).toHaveLength(2)
      expect(
        new Set(r.data.map((b: { bloqueoOperacionId: string }) => b.bloqueoOperacionId))
          .size
      ).toBe(1)
      paginas.push(r.data[0].bloqueoOperacionId)
    }
    expect(new Set(paginas).size).toBe(2)
    await login(page, process.env.E2E_ADMIN_EMAIL!, process.env.E2E_ADMIN_PASSWORD!)
    await page.goto(`/reservas?desde=${dia}&hasta=${dia}`)
    await expect(page.getByRole("button", { name: "Levantar bloqueo" })).toHaveCount(2)
    await expect(page.getByText(/Mantenimiento independiente · 2 equipos/)).toBeVisible()
    await expect(page.getByText(/Evaluación independiente · 2 equipos/)).toBeVisible()
    await page.getByRole("button", { name: "Levantar bloqueo" }).first().click()
    const levantamiento = page.waitForResponse(
      (r) =>
        r.request().method() === "POST" &&
        /\/api\/bloqueos\/[^/]+\/levantar$/.test(new URL(r.url()).pathname)
    )
    await page.getByRole("button", { name: "Confirmar cancelación", exact: true }).click()
    expect((await levantamiento).ok()).toBe(true)
    await expect(
      page.getByRole("button", { name: "Confirmar cancelación", exact: true })
    ).toHaveCount(0)
    await expect(page.getByRole("button", { name: "Levantar bloqueo" })).toHaveCount(1)
    const vigentes = await api(
      request,
      "GET",
      `/api/reservas?desde=${dia}&hasta=${dia}&porClases=true`
    )
    expect(vigentes.meta.total).toBe(1)
    expect(vigentes.data).toHaveLength(2)
    await page.goto("/reservas/calendario")
    await page.getByLabel("Ir a fecha").fill(dia)
    await expect(
      page.getByRole("button", { name: /Bloqueo administrativo,/ })
    ).toHaveCount(1)
  })
  for (const rol of ["ADMIN", "DOCENTE"] as const) {
    test(`${rol} llega desde una reserva cancelada al listado con su filtro activado`, async ({
      page,
    }) => {
      await login(
        page,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_EMAIL" : "E2E_DOCENTE_EMAIL"]!,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_PASSWORD" : "E2E_DOCENTE_PASSWORD"]!
      )
      await page.goto("/reservas/calendario")
      await page.getByLabel("Ir a fecha").fill(dia)
      await page.getByRole("checkbox", { name: "Mostrar canceladas" }).check()
      await page
        .getByRole("button", { name: new RegExp(`${materiaNombre},.*11:00`) })
        .click()
      await page
        .getByRole("dialog")
        .getByRole("link", { name: "Gestionar reserva" })
        .click()
      await expect(page).toHaveURL(/canceladas=true/)
      await expect(
        page.getByRole("checkbox", { name: "Mostrar también las canceladas" })
      ).toBeChecked()
      await expect(page.getByText(new RegExp(`^${materiaNombre} —`))).toBeVisible()
    })
  }
})
