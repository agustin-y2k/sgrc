import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext } from "@playwright/test"
import { login } from "./helpers"

test.describe("levantamiento atómico de bloqueos", () => {
  test.skip(
    process.env.E2E_LEVANTAMIENTO_BLOQUEOS_AISLADO !== "1",
    "requiere base efímera"
  )
  test.describe.configure({ mode: "serial" })
  let token = "",
    docente = "",
    operacion = "",
    otra = ""
  let filas: { id: string; bloqueoOperacionId: string }[] = [],
    otras: { id: string; bloqueoOperacionId: string }[] = []
  const fecha = new Date()
  fecha.setDate(fecha.getDate() + 1)
  const dia = new Intl.DateTimeFormat("sv-SE", {
    timeZone: "America/Argentina/Buenos_Aires",
  }).format(fecha)
  const email = process.env.E2E_ADMIN_EMAIL!,
    password = process.env.E2E_ADMIN_PASSWORD!
  async function api(
    request: APIRequestContext,
    method: string,
    path: string,
    data?: unknown
  ) {
    const r = await request.fetch(path, {
      method,
      headers: { Authorization: `Bearer ${token}` },
      data,
    })
    expect(r.ok(), `${method} ${path}: ${r.status()}`).toBe(true)
    return r.headers()["content-type"]?.includes("application/json")
      ? r.json()
      : undefined
  }
  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    token = (
      await (await request.post("/api/auth/login", { data: { email, password } })).json()
    ).token
    docente = (
      await (
        await request.post("/api/auth/login", {
          data: {
            email: process.env.E2E_DOCENTE_EMAIL,
            password: process.env.E2E_DOCENTE_PASSWORD,
          },
        })
      ).json()
    ).token
    const pcs = (await api(request, "GET", "/api/equipos")).data
    filas = (
      await api(request, "POST", "/api/bloqueos", {
        equipoIds: pcs.slice(0, 2).map((p: { id: string }) => p.id),
        fecha: dia,
        horaInicio: "10:00",
        horaFin: "11:00",
        motivo: "Ensayo R14",
      })
    ).bloqueos
    otras = (
      await api(request, "POST", "/api/bloqueos", {
        equipoIds: pcs.slice(2, 4).map((p: { id: string }) => p.id),
        fecha: dia,
        horaInicio: "10:00",
        horaFin: "11:00",
        motivo: "Otra operación R14",
      })
    ).bloqueos
    operacion = filas[0].bloqueoOperacionId
    otra = otras[0].bloqueoOperacionId
    expect(operacion).not.toBe(otra)
  })
  test("un docente no puede levantar la operación de un administrador", async ({
    request,
  }) => {
    const r = await request.post(`/api/bloqueos/${operacion}/levantar`, {
      headers: { Authorization: `Bearer ${docente}` },
      data: { motivo: "Intento docente" },
    })
    expect(r.status()).toBe(403)
    for (const fila of filas)
      expect((await api(request, "GET", `/api/reservas/${fila.id}`)).estado).toBe(
        "CONFIRMADA"
      )
  })
  test("un fallo temporal permite reintentar la operación completa desde móvil", async ({
    page,
    request,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    let pedidos = 0
    await page.route(`**/api/bloqueos/${operacion}/levantar`, async (route) => {
      pedidos++
      if (pedidos === 1)
        return route.fulfill({ status: 503, json: { error: "Fallo temporal de prueba" } })
      await route.continue()
    })
    await login(page, email, password)
    await page.goto(`/reservas?desde=${dia}&hasta=${dia}`)
    const tarjeta = page
      .locator('[data-slot="card"]')
      .filter({ hasText: "Ensayo R14 · 2 equipos" })
    await tarjeta.getByRole("button", { name: "Levantar bloqueo", exact: true }).click()
    await tarjeta
      .getByRole("button", { name: "Confirmar cancelación", exact: true })
      .click()
    await expect(tarjeta.getByText(/Fallo temporal de prueba/)).toBeVisible()
    for (const fila of filas)
      expect((await api(request, "GET", `/api/reservas/${fila.id}`)).estado).toBe(
        "CONFIRMADA"
      )
    await expect(
      tarjeta.getByRole("button", { name: "Confirmar cancelación", exact: true })
    ).toBeEnabled()
    const respuesta = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === `/api/bloqueos/${operacion}/levantar` &&
        r.request().method() === "POST" &&
        r.status() === 200
    )
    await tarjeta
      .getByRole("button", { name: "Confirmar cancelación", exact: true })
      .click()
    expect((await (await respuesta).json()).reservasCanceladas).toBe(2)
    await expect(
      page.getByRole("button", { name: "Confirmar cancelación", exact: true })
    ).toHaveCount(0)
    await expect(
      page.getByRole("button", { name: "Levantar bloqueo", exact: true })
    ).toHaveCount(1)
    expect(pedidos).toBe(2)
    for (const fila of otras)
      expect((await api(request, "GET", `/api/reservas/${fila.id}`)).estado).toBe(
        "CONFIRMADA"
      )
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
    ).toBe(true)
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations
    ).toEqual([])
  })
  test("una respuesta perdida se reconcilia con la cancelación ya confirmada", async ({
    page,
    request,
  }) => {
    let pedidos = 0
    await page.route(`**/api/bloqueos/${otra}/levantar`, async (route) => {
      pedidos++
      const r = await route.fetch()
      expect(r.status()).toBe(200)
      await route.abort("failed")
    })
    await login(page, email, password)
    await page.goto(`/reservas?desde=${dia}&hasta=${dia}`)
    await page.getByRole("button", { name: "Levantar bloqueo", exact: true }).click()
    await page.getByRole("button", { name: "Confirmar cancelación", exact: true }).click()
    await expect(
      page.getByRole("button", { name: "Levantar bloqueo", exact: true })
    ).toHaveCount(0)
    await expect(
      page.getByRole("button", { name: "Confirmar cancelación", exact: true })
    ).toHaveCount(0)
    expect(pedidos).toBe(1)
    for (const fila of otras)
      expect((await api(request, "GET", `/api/reservas/${fila.id}`)).estado).toBe(
        "CANCELADA"
      )
  })
  test("repetir una orden aplicada conserva los motivos y una sola auditoría por operación", async ({
    request,
  }) => {
    const antes = await api(request, "GET", `/api/reservas/${otras[0].id}`)
    const r = await api(request, "POST", `/api/bloqueos/${otra}/levantar`, {
      motivo: "Reintento con otro texto",
    })
    expect(r).toMatchObject({ reservasCanceladas: 0, reservasYaCanceladas: 2 })
    expect(await api(request, "GET", `/api/reservas/${otras[0].id}`)).toEqual(antes)
    const auditorias = (
      await api(request, "GET", "/api/auditoria?accion=BLOQUEO_LEVANTADO")
    ).data
    expect(auditorias).toHaveLength(2)
    expect(new Set(auditorias.map((a: { entidadId: string }) => a.entidadId))).toEqual(
      new Set([operacion, otra])
    )
    expect(
      auditorias.every(
        (a: { detalle: { reservasCanceladas: number } }) =>
          a.detalle.reservasCanceladas === 2
      )
    ).toBe(true)
  })
})
