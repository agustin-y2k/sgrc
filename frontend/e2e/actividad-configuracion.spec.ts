import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext } from "@playwright/test"
import { login } from "./helpers"

test.describe("actividad y configuración en base efímera", () => {
  test.skip(
    process.env.E2E_ACTIVIDAD_CONFIGURACION_AISLADA !== "1",
    "requiere base efímera"
  )
  test.describe.configure({ mode: "serial" })
  let token = "",
    docenteToken = "",
    ciclo = "",
    pc = "",
    usuario = "",
    prestamo = ""
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
    docenteToken = (
      await (
        await request.post("/api/auth/login", {
          data: {
            email: process.env.E2E_DOCENTE_EMAIL,
            password: process.env.E2E_DOCENTE_PASSWORD,
          },
        })
      ).json()
    ).token
    ciclo = (await api(request, "GET", "/api/ciclos")).data[0].id
    pc = (await api(request, "GET", "/api/equipos")).data[0].id
    usuario = (await api(request, "GET", "/api/auth/usuarios?pageSize=200")).data.find(
      (u: { email: string }) => u.email === process.env.E2E_DOCENTE_EMAIL
    ).id
  })
  test("la API impide al docente leer o modificar la configuración", async ({
    request,
  }) => {
    const cfg = await api(request, "GET", "/api/configuracion/operacion")
    for (const method of ["GET", "PUT"]) {
      const res = await request.fetch("/api/configuracion/operacion", {
        method,
        headers: { Authorization: `Bearer ${docenteToken}` },
        ...(method === "PUT" ? { data: cfg } : {}),
      })
      expect(res.status()).toBe(403)
    }
    expect(await api(request, "GET", "/api/configuracion/operacion")).toEqual(cfg)
  })
  test("el administrador guarda desde móvil y una edición antigua recibe conflicto", async ({
    page,
    request,
  }) => {
    const antes = await api(request, "GET", "/api/configuracion/operacion")
    await page.setViewportSize({ width: 390, height: 844 })
    await login(page, email, password)
    await page.goto("/admin/configuracion")
    const campo = page.getByLabel("Espera para retirar una reserva (minutos)")
    await expect(campo).toHaveValue(String(antes.retiroGraciaMinutos))
    await campo.fill("10")
    await page.getByRole("button", { name: "Guardar configuración", exact: true }).click()
    await expect(page.getByText(/Configuración guardada/)).toBeVisible()
    await page.reload()
    await expect(campo).toHaveValue("10")
    const viejo = await request.put("/api/configuracion/operacion", {
      headers: { Authorization: `Bearer ${token}` },
      data: { ...antes, retiroGraciaMinutos: 20 },
    })
    expect(viejo.status()).toBe(409)
    expect(
      (await api(request, "GET", "/api/configuracion/operacion")).retiroGraciaMinutos
    ).toBe(10)
    const avisos = (
      await api(request, "GET", "/api/auditoria?accion=CONFIGURACION_OPERATIVA_CAMBIADA")
    ).data
    expect(avisos).toHaveLength(1)
    expect(avisos[0].detalle.antes.retiroGraciaMinutos).toBe(antes.retiroGraciaMinutos)
    expect(avisos[0].detalle.despues.retiroGraciaMinutos).toBe(10)
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
  test("el reporte muestra un préstamo sin reserva al docente y su devolución", async ({
    page,
    request,
  }) => {
    const salida = await api(request, "POST", "/api/prestamos", {
      equipoIds: [pc],
      nombre: "Ada Lovelace",
      usuarioId: usuario,
    })
    expect(salida.entregadas).toHaveLength(1)
    prestamo = salida.entregadas[0].id
    const actividad = await api(request, "GET", `/api/ciclos/${ciclo}/actividad`)
    expect(actividad.totales).toMatchObject({
      reservasAtendidas: 0,
      prestamosConReserva: 0,
      prestamosSinReserva: 1,
      prestamosAbiertos: 1,
    })
    await login(page, email, password)
    await page.goto("/admin/reportes")
    const tabla = page.getByRole("table", { name: "Entregas por responsable" })
    await expect(
      tabla.getByRole("row", { name: "Ada Lovelace 0 0 1 0 1 0", exact: true })
    ).toBeVisible()
    await api(request, "POST", "/api/prestamos/recibir", { prestamoIds: [prestamo] })
    await page.reload()
    await expect(
      tabla.getByRole("row", { name: "Ada Lovelace 0 0 1 0 0 1", exact: true })
    ).toBeVisible()
    await page.setViewportSize({ width: 390, height: 844 })
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
})
