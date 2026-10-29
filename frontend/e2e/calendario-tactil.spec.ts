import {
  devices,
  expect,
  test,
  type APIRequestContext,
  type Locator,
} from "@playwright/test"
import AxeBuilder from "@axe-core/playwright"
import { login } from "./helpers"

test.use({ ...devices["Pixel 7"] })

async function comprobarControles(raiz: Locator) {
  const pequenos = await raiz.evaluate((region) => {
    const errores: string[] = []
    for (const el of region.querySelectorAll<HTMLElement>("button,a,input,select")) {
      if (!el.getClientRects().length || getComputedStyle(el).visibility === "hidden")
        continue
      const objetivo = el.matches('input[type="checkbox"]')
        ? (el.closest("label") ?? el)
        : el
      const rect = objetivo.getBoundingClientRect()
      if (rect.width < 43.9 || rect.height < 43.9)
        errores.push(
          `${el.getAttribute("aria-label") ?? el.textContent?.trim() ?? el.id}: ${rect.width}×${rect.height}`
        )
    }
    return errores
  })
  expect(pequenos, "controles menores a 44×44 px").toEqual([])
}

async function sinDesbordamiento(page: import("@playwright/test").Page) {
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)
  ).toBe(true)
}

test.describe("calendario táctil con datos aislados", () => {
  test.describe.configure({ mode: "serial" })
  test.skip(
    process.env.E2E_CALENDARIO_TACTIL_AISLADO !== "1",
    "requiere una base efímera"
  )
  test.setTimeout(90_000)
  let token = "",
    materia = "",
    dia = ""
  async function api(
    request: APIRequestContext,
    method: string,
    ruta: string,
    data?: unknown,
    autorizacion = token
  ) {
    const r = await request.fetch(ruta, {
      method,
      data,
      headers: { Authorization: `Bearer ${autorizacion}` },
    })
    expect(r.ok(), `${method} ${ruta}: ${r.status()}`).toBe(true)
    return r.headers()["content-type"]?.includes("application/json")
      ? r.json()
      : undefined
  }
  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    token = (
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
    materia = (await api(request, "GET", "/api/mis-materias")).data[0].materiaId
    const fecha = new Date()
    fecha.setDate(fecha.getDate() + 2)
    dia = new Intl.DateTimeFormat("sv-SE", {
      timeZone: "America/Argentina/Buenos_Aires",
    }).format(fecha)
    const equipos = (await api(request, "GET", "/api/equipos")).data
    expect(equipos.length).toBeGreaterThanOrEqual(8)
    for (const equipo of equipos.slice(0, 8)) {
      await api(request, "POST", "/api/reservas", {
        materiaId: materia,
        fecha: dia,
        horaInicio: "09:00",
        horaFin: "09:05",
        equipoIds: [equipo.id],
      })
    }
    for (const [horaInicio, horaFin] of [
      ["10:00", "10:05"],
      ["10:05", "10:10"],
    ]) {
      await api(request, "POST", "/api/reservas", {
        materiaId: materia,
        fecha: dia,
        horaInicio,
        horaFin,
        equipoIds: [equipos[0].id],
      })
    }
  })
  for (const rol of ["ADMIN", "DOCENTE"] as const) {
    for (const ancho of [320, 390]) {
      test(`${rol} a ${ancho}px: controles, mes, grilla y detalle alcanzables`, async ({
        page,
      }) => {
        await page.setViewportSize({ width: ancho, height: 844 })
        await login(
          page,
          process.env[rol === "ADMIN" ? "E2E_ADMIN_EMAIL" : "E2E_DOCENTE_EMAIL"]!,
          process.env[rol === "ADMIN" ? "E2E_ADMIN_PASSWORD" : "E2E_DOCENTE_PASSWORD"]!
        )
        await page.goto("/reservas/calendario")
        const calendario = page.getByRole("region", {
          name: "Calendario de reservas",
          exact: true,
        })
        await page.getByLabel("Ir a fecha").fill(dia)
        await expect(
          calendario.getByRole("button", { name: /Programación,.*09:00/ })
        ).toHaveCount(8)
        await comprobarControles(calendario)
        await sinDesbordamiento(page)
        await calendario.getByRole("button", { name: "Mes", exact: true }).tap()
        await expect(calendario.getByRole("button", { name: /Ver \d+ más/ })).toHaveCount(
          1
        )
        await comprobarControles(calendario)
        await sinDesbordamiento(page)
        await calendario.getByRole("button", { name: /Ver \d+ más/ }).tap()
        await expect(
          calendario.getByRole("button", { name: "Agenda", exact: true })
        ).toHaveAttribute("aria-pressed", "true")
        await calendario.getByRole("button", { name: "Semana", exact: true }).tap()
        const grilla = page.getByLabel("Grilla semanal de reservas, desplazable")
        await expect(grilla).toBeVisible()
        await comprobarControles(calendario)
        const tarjetas = grilla.getByRole("button", { name: /Programación,/ })
        await expect(tarjetas).toHaveCount(10)
        const solapamientos = await tarjetas.evaluateAll((elements) => {
          const cajas = elements.map((el) => el.getBoundingClientRect())
          const errores: string[] = []
          for (let i = 0; i < cajas.length; i++)
            for (let j = i + 1; j < cajas.length; j++) {
              const a = cajas[i],
                b = cajas[j]
              if (
                Math.min(a.right, b.right) - Math.max(a.left, b.left) > 0.5 &&
                Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > 0.5
              )
                errores.push(`${i} con ${j}`)
            }
          return errores
        })
        expect(solapamientos, "las tarjetas táctiles no deben taparse entre sí").toEqual(
          []
        )
        const equiposAbiertos = new Set<string>()
        for (let i = 0; i < 10; i++) {
          const tarjeta = tarjetas.nth(i)
          await tarjeta.tap()
          const detalle = page.getByRole("dialog")
          await expect(detalle).toBeVisible()
          await comprobarControles(detalle)
          equiposAbiertos.add(
            (await detalle.getByRole("link", { name: /PC / }).getAttribute("href"))!
          )
          await detalle.getByRole("button", { name: "Cerrar detalles" }).tap()
          await expect(detalle).toHaveCount(0)
          await expect(tarjeta).toBeFocused()
        }
        expect(equiposAbiertos.size).toBe(8)
        await sinDesbordamiento(page)
        const axe = await new AxeBuilder({ page })
          .include('section[aria-label="Calendario de reservas"]')
          .analyze()
        expect(axe.violations).toEqual([])
      })
    }
    test(`${rol}: reintento y estado vacío mantienen controles táctiles`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 320, height: 844 })
      await login(
        page,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_EMAIL" : "E2E_DOCENTE_EMAIL"]!,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_PASSWORD" : "E2E_DOCENTE_PASSWORD"]!
      )
      let falla = true
      await page.route("**/api/calendario/reservas?**", (r) =>
        falla
          ? r.fulfill({ status: 503, body: "Consulta no disponible" })
          : r.fulfill({ json: { data: [], meta: { total: 0, page: 1, pageSize: 100 } } })
      )
      await page.goto("/reservas/calendario")
      const calendario = page.getByRole("region", {
        name: "Calendario de reservas",
        exact: true,
      })
      await expect(
        calendario.getByRole("button", { name: "Reintentar calendario" })
      ).toBeVisible({ timeout: 15000 })
      await comprobarControles(calendario)
      falla = false
      await calendario.getByRole("button", { name: "Reintentar calendario" }).tap()
      await expect(calendario.getByText(/No hay reservas que coincidan/)).toBeVisible()
      await comprobarControles(calendario)
      await sinDesbordamiento(page)
    })
  }
})
