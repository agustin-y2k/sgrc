import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext, type Page } from "@playwright/test"
import { login } from "./helpers"

test.describe("cambios de curso y prioridades con datos aislados", () => {
  test.skip(
    process.env.E2E_CURSO_PRIORIDADES_AISLADOS !== "1",
    "requiere PostgreSQL efímero"
  )
  test.describe.configure({ mode: "serial" })
  let token = "",
    ciclo = "",
    equipo = "",
    contador = 80
  const email = process.env.E2E_ADMIN_EMAIL!
  const password = process.env.E2E_ADMIN_PASSWORD!
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
    ciclo = (await api(request, "GET", "/api/ciclos")).data[0].id
    equipo = (await api(request, "GET", "/api/equipos")).data.find(
      (e: { identificador: number }) => e.identificador === 4
    ).id
  })
  async function preparar(request: APIRequestContext, page: Page) {
    const division = String(++contador)
    const curso = await api(request, "POST", `/api/ciclos/${ciclo}/cursos`, {
      anio: 3,
      division,
      modalidad: "Prueba de prioridades",
    })
    const materia = await api(request, "POST", `/api/cursos/${curso.id}/materias`, {
      nombre: `Dibujo de prueba ${division}`,
    })
    await api(request, "POST", "/api/preferencias", {
      equipoIds: [equipo],
      materiaNombre: materia.nombre,
      anio: 3,
      division,
      modalidad: "Prueba de prioridades",
      prioridad: 1,
      cursoExacto: true,
    })
    await login(page, email, password)
    await page.goto("/admin/academico")
    await page.getByRole("button", { name: "Cursos", exact: true }).click()
    const fila = page
      .getByText(`3°${division} · Prueba de prioridades`, { exact: true })
      .locator("../..")
    await fila.getByRole("button", { name: "Renombrar", exact: true }).click()
    const formulario = page
      .locator("form")
      .filter({ has: page.locator(`[id="editar-curso-${curso.id}-anio"]`) })
    await formulario.getByLabel("División (opcional)").fill(`1${division}`)
    await formulario.getByRole("button", { name: "Guardar", exact: true }).click()
    await expect(
      formulario.getByText("Revisá las prioridades antes de guardar")
    ).toBeVisible()
    return { curso, materia, division, formulario }
  }
  test("conserva las prioridades al cambiar de división y el motor de reservas las reconoce", async ({
    request,
    page,
  }) => {
    const { curso, materia, division, formulario } = await preparar(request, page)
    expect((await api(request, "GET", `/api/cursos/${curso.id}`)).division).toBe(division)
    await expect(formulario.getByText(/PC 4/)).toBeVisible()
    await page.setViewportSize({ width: 390, height: 844 })
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)
      )
      .toBe(true)
    const axe = await new AxeBuilder({ page })
      .include("form")
      .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
      .analyze()
    expect(axe.violations.map((v) => v.id)).toEqual([])
    await formulario.getByRole("button", { name: "Confirmar cambio de curso" }).click()
    await expect(formulario).toHaveCount(0)
    expect((await api(request, "GET", `/api/cursos/${curso.id}`)).division).toBe(
      `1${division}`
    )
    const marcas = (
      await api(request, "GET", `/api/equipos/${equipo}/preferencias`)
    ).data.filter((p: { materiaNombre: string }) => p.materiaNombre === materia.nombre)
    expect(marcas).toHaveLength(1)
    expect(marcas[0]).toMatchObject({
      division: `1${division}`,
      prioridad: 1,
      cursoExacto: true,
    })
    const dia = new Intl.DateTimeFormat("sv-SE", {
      timeZone: "America/Argentina/Buenos_Aires",
    }).format(new Date(Date.now() + 86400000))
    const disponibles = await api(
      request,
      "GET",
      `/api/equipos-disponibles?fecha=${dia}&horaInicio=08:00&horaFin=09:00&materiaId=${materia.id}`
    )
    const pc = disponibles.data.find((e: { equipoId: string }) => e.equipoId === equipo)
    expect(pc).toMatchObject({ tramo: "PREFERENTE" })
  })
  test("cancelar no modifica el curso ni sus marcas", async ({ request, page }) => {
    const { curso, materia, division, formulario } = await preparar(request, page)
    await formulario.getByRole("button", { name: "Cancelar", exact: true }).click()
    expect((await api(request, "GET", `/api/cursos/${curso.id}`)).division).toBe(division)
    const marca = (
      await api(request, "GET", `/api/equipos/${equipo}/preferencias`)
    ).data.find((m: { materiaNombre: string }) => m.materiaNombre === materia.nombre)
    expect(marca.division).toBe(division)
  })
  test("permite aceptar expresamente que las marcas mantengan su alcance anterior", async ({
    request,
    page,
  }) => {
    const { curso, materia, division, formulario } = await preparar(request, page)
    await formulario.getByRole("radio", { name: /Mantener todas las marcas/ }).check()
    await formulario.getByRole("button", { name: "Confirmar cambio de curso" }).click()
    await expect(formulario).toHaveCount(0)
    expect((await api(request, "GET", `/api/cursos/${curso.id}`)).division).toBe(
      `1${division}`
    )
    const marca = (
      await api(request, "GET", `/api/equipos/${equipo}/preferencias`)
    ).data.find((m: { materiaNombre: string }) => m.materiaNombre === materia.nombre)
    expect(marca.division).toBe(division)
  })
  test("rechaza confirmar un impacto que cambió y permite revisarlo nuevamente", async ({
    request,
    page,
  }) => {
    const { curso, materia, division, formulario } = await preparar(request, page)
    const marca = (
      await api(request, "GET", `/api/equipos/${equipo}/preferencias`)
    ).data.find((m: { materiaNombre: string }) => m.materiaNombre === materia.nombre)
    await api(request, "PATCH", `/api/preferencias/${marca.id}`, {
      anio: 3,
      division,
      modalidad: "Prueba de prioridades",
      prioridad: 2,
      cursoExacto: true,
    })
    await formulario.getByRole("button", { name: "Confirmar cambio de curso" }).click()
    await expect(formulario.getByRole("alert")).toContainText(
      "volvé a revisar el impacto"
    )
    expect((await api(request, "GET", `/api/cursos/${curso.id}`)).division).toBe(division)
    await formulario.getByRole("button", { name: "Guardar", exact: true }).click()
    await expect(formulario.getByText(/prioridad 2/)).toBeVisible()
    await formulario.getByRole("button", { name: "Confirmar cambio de curso" }).click()
    await expect(formulario).toHaveCount(0)
  })
})
