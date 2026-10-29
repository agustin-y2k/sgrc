import { expect, test, type APIRequestContext } from "@playwright/test"
import { login } from "./helpers"

test.describe("vigencia de entregas con base aislada", () => {
  test.skip(process.env.E2E_ENTREGA_VIGENTE_AISLADA !== "1", "requiere base efímera")
  test.describe.configure({ mode: "serial" })
  let token = "",
    futura = "",
    pendiente = "",
    primera = "",
    equipoFuturo = "",
    equipoPrimero = "",
    equipoPendiente = ""
  const email = process.env.E2E_ADMIN_EMAIL!,
    password = process.env.E2E_ADMIN_PASSWORD!
  async function api(
    request: APIRequestContext,
    metodo: string,
    ruta: string,
    data?: unknown
  ) {
    const r = await request.fetch(ruta, {
      method: metodo,
      headers: { Authorization: `Bearer ${token}` },
      data,
    })
    expect(r.ok(), `${metodo} ${ruta}: ${r.status()}`).toBe(true)
    return r.headers()["content-type"]?.includes("application/json")
      ? r.json()
      : undefined
  }
  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    const r = await request.post("/api/auth/login", { data: { email, password } })
    expect(r.ok()).toBe(true)
    token = (await r.json()).token
    const materia = (await api(request, "GET", "/api/mis-materias")).data.find(
      (m: { materiaNombre: string }) => m.materiaNombre === "Programación"
    )
    const pcs = (await api(request, "GET", "/api/equipos")).data
    equipoPrimero = pcs.find((p: { identificador: number }) => p.identificador === 4).id
    equipoPendiente = pcs.find((p: { identificador: number }) => p.identificador === 5).id
    equipoFuturo = pcs.find((p: { identificador: number }) => p.identificador === 6).id
    const hoy = new Date(),
      manana = new Date(hoy)
    manana.setDate(manana.getDate() + 1)
    const iso = (dia: Date) =>
      new Intl.DateTimeFormat("sv-SE", {
        timeZone: "America/Argentina/Buenos_Aires",
      }).format(dia)
    const turno = { materiaId: materia.materiaId, horaInicio: "23:00", horaFin: "00:00" }
    const deHoy = await api(request, "POST", "/api/reservas", {
      ...turno,
      fecha: iso(hoy),
      equipoIds: [equipoPrimero, equipoPendiente],
    })
    primera = deHoy.reservas.find(
      (r: { equipoId: string }) => r.equipoId === equipoPrimero
    ).id
    pendiente = deHoy.reservas.find(
      (r: { equipoId: string }) => r.equipoId === equipoPendiente
    ).id
    futura = (
      await api(request, "POST", "/api/reservas", {
        ...turno,
        fecha: iso(manana),
        equipoIds: [equipoFuturo],
      })
    ).reservas[0].id
  })
  test("un ID de reserva futura no crea un préstamo ni libera otras reservas", async ({
    request,
  }) => {
    const resultado = await api(request, "POST", "/api/prestamos/por-reserva", {
      reservaIds: [futura],
      equiposEsperados: { [futura]: equipoFuturo },
      liberarNoEntregadas: true,
    })
    expect(resultado.entregadas).toHaveLength(0)
    expect(resultado.noEntregadas).toEqual([
      expect.objectContaining({ equipoId: equipoFuturo, razon: "RESERVA_NO_VIGENTE" }),
    ])
    expect(resultado.liberadas ?? []).toHaveLength(0)
    expect((await api(request, "GET", "/api/prestamos")).data).toHaveLength(0)
  })
  test("un lote mixto entrega solo la válida y libera únicamente sus pendientes", async ({
    request,
  }) => {
    const resultado = await api(request, "POST", "/api/prestamos/por-reserva", {
      reservaIds: [futura, primera],
      equiposEsperados: { [futura]: equipoFuturo, [primera]: equipoPrimero },
      liberarNoEntregadas: true,
    })
    expect(resultado.entregadas).toHaveLength(1)
    expect(resultado.entregadas[0].equipoId).toBe(equipoPrimero)
    expect(
      new Date(resultado.entregadas[0].devolucionEstimada).getTime()
    ).toBeGreaterThan(new Date(resultado.entregadas[0].entregadoEn).getTime())
    expect(resultado.liberadas).toEqual([pendiente])
    expect(resultado.noEntregadas).toEqual([
      expect.objectContaining({ equipoId: equipoFuturo, razon: "RESERVA_NO_VIGENTE" }),
    ])
    expect((await api(request, "GET", `/api/reservas/${futura}`)).estado).toBe(
      "CONFIRMADA"
    )
    expect((await api(request, "GET", `/api/reservas/${pendiente}`)).estado).toBe(
      "NO_RETIRADA"
    )
  })
  test("la pantalla permite recoger la NO_RETIRADA mientras su franja siga vigente", async ({
    page,
    request,
  }) => {
    await login(page, email, password)
    await page.goto("/admin/entregas")
    await page.getByRole("checkbox", { name: /^PC 5\b/ }).check()
    const respuesta = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === "/api/prestamos/por-reserva" &&
        r.request().method() === "POST"
    )
    await page.getByRole("button", { name: "Entregar 1 equipo", exact: true }).click()
    const resultado = await (await respuesta).json()
    expect(resultado.entregadas).toHaveLength(1)
    expect(resultado.entregadas[0]).toMatchObject({
      equipoId: equipoPendiente,
      reservaId: pendiente,
    })
    expect(
      new Date(resultado.entregadas[0].devolucionEstimada).getTime()
    ).toBeGreaterThan(new Date(resultado.entregadas[0].entregadoEn).getTime())
    expect((await api(request, "GET", "/api/prestamos")).data).toHaveLength(2)
  })
})
