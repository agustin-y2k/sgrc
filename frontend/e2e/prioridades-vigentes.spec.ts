import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext } from "@playwright/test"
import { login } from "./helpers"

test.describe("prioridades vigentes e históricas con base aislada", () => {
  test.skip(
    process.env.E2E_PRIORIDADES_VIGENTES_AISLADAS !== "1",
    "requiere una base efímera"
  )
  test.describe.configure({ mode: "serial" })
  let token = "",
    equipo = "",
    materiaArchivada = "",
    materiaNueva = "",
    anio = 0
  const nombre = "Diseño de prueba de vigencia"
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
    const ingreso = await request.post("/api/auth/login", { data: { email, password } })
    expect(ingreso.ok()).toBe(true)
    token = (await ingreso.json()).token
    const ciclo = (await api(request, "GET", "/api/ciclos")).data[0]
    anio = ciclo.anio
    equipo = (await api(request, "GET", "/api/equipos")).data.find(
      (e: { identificador: number }) => e.identificador === 4
    ).id
    const curso = await api(request, "POST", `/api/ciclos/${ciclo.id}/cursos`, {
      anio: 3,
      division: "91",
      modalidad: "Orientación de prueba",
    })
    materiaArchivada = (
      await api(request, "POST", `/api/cursos/${curso.id}/materias`, { nombre })
    ).id
    await api(request, "POST", "/api/preferencias", {
      equipoIds: [equipo],
      materiaNombre: nombre,
      prioridad: 2,
    })
    await api(request, "POST", "/api/preferencias", {
      equipoIds: [equipo],
      materiaNombre: nombre,
      prioridad: 1,
      anio: 3,
      division: "91",
      modalidad: "Orientación de prueba",
      cursoExacto: true,
    })
    const cierre = await api(request, "POST", `/api/ciclos/${ciclo.id}/archivar`, {
      clonarA: anio + 1,
    })
    const nuevoCurso = (
      await api(request, "GET", `/api/ciclos/${cierre.nuevoCicloId}/cursos`)
    ).data.find((c: { division: string }) => c.division === "91")
    materiaNueva = (
      await api(request, "GET", `/api/cursos/${nuevoCurso.id}/materias`)
    ).data.find((m: { nombre: string }) => m.nombre === nombre).id
  })
  test("clonar el ciclo mantiene las marcas aplicables al curso nuevo", async ({
    request,
  }) => {
    expect(
      (await api(request, "GET", `/api/materias/${materiaArchivada}`)).archivado
    ).toBe(true)
    expect((await api(request, "GET", `/api/materias/${materiaNueva}`)).archivado).toBe(
      false
    )
    const huerfanas = (
      await api(request, "GET", "/api/preferencias/huerfanas")
    ).data.filter((m: { materiaNombre: string }) => m.materiaNombre === nombre)
    expect(huerfanas).toHaveLength(0)
    expect(
      (await api(request, "GET", `/api/equipos/${equipo}/preferencias`)).data
    ).toHaveLength(2)
  })
  test("renombrar la materia vigente informa las dos marcas aunque sobreviva la copia archivada", async ({
    request,
    page,
  }) => {
    await login(page, email, password)
    await page.goto("/admin/academico")
    const panel = page
      .locator('[data-slot="card"]')
      .filter({
        has: page
          .locator('[data-slot="card-title"]')
          .filter({ hasText: String(anio + 1) }),
      })
    await panel.getByRole("button", { name: "Cursos", exact: true }).click()
    const curso = page
      .getByText("3°91 · Orientación de prueba", { exact: true })
      .locator("../..")
    await curso.getByRole("button", { name: "Materias", exact: true }).click()
    await curso
      .getByText(nombre, { exact: true })
      .locator("../..")
      .getByRole("button", { name: "Renombrar", exact: true })
      .click()
    const campo = page.locator(`[id="editar-materia-${materiaNueva}"]`)
    await campo.fill("Diseño actualizado de prueba")
    const respuesta = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === `/api/materias/${materiaNueva}` &&
        r.request().method() === "PATCH"
    )
    await campo
      .locator("xpath=ancestor::form")
      .getByRole("button", { name: "Guardar", exact: true })
      .click()
    expect((await (await respuesta).json()).marcasDeEquipoAfectadas).toBe(2)
    await expect(page.getByText(/2 marcas de equipo quedaron/)).toBeVisible()
    const huerfanas = (await api(request, "GET", "/api/preferencias/huerfanas")).data
    expect(huerfanas).toHaveLength(2)
    expect(
      huerfanas.every((m: { coincideConArchivadas: boolean }) => m.coincideConArchivadas)
    ).toBe(true)
    await page.goto("/admin/inventario")
    await expect(
      page.getByText("Prioridades sin materias vigentes", { exact: true })
    ).toBeVisible()
    await expect(page.getByText(/Solo coincide con materias archivadas/)).toHaveCount(2)
    await page.setViewportSize({ width: 390, height: 844 })
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)
      )
      .toBe(true)
    const tarjeta = page
      .locator('[data-slot="card"]')
      .filter({
        has: page.getByText("Prioridades sin materias vigentes", { exact: true }),
      })
    const axe = await new AxeBuilder({ page })
      .include('[data-slot="card"]')
      .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
      .analyze()
    expect(axe.violations.map((v) => v.id)).toEqual([])
    await expect(
      tarjeta.getByRole("button", { name: "Quitar", exact: true })
    ).toHaveCount(2)
  })
  test("las marcas se conservan hasta que el administrador elige quitarlas", async ({
    request,
    page,
  }) => {
    expect(
      (await api(request, "GET", `/api/equipos/${equipo}/preferencias`)).data
    ).toHaveLength(2)
    await login(page, email, password)
    await page.goto("/admin/inventario")
    const tarjeta = page
      .locator('[data-slot="card"]')
      .filter({
        has: page.getByText("Prioridades sin materias vigentes", { exact: true }),
      })
    await tarjeta.getByRole("button", { name: "Quitar", exact: true }).first().click()
    await expect(
      tarjeta.getByRole("button", { name: "Quitar", exact: true })
    ).toHaveCount(1)
    expect(
      (await api(request, "GET", `/api/equipos/${equipo}/preferencias`)).data
    ).toHaveLength(1)
    const historica = await api(request, "GET", `/api/materias/${materiaArchivada}`)
    expect(historica).toMatchObject({ nombre, archivado: true })
  })
  test("una coincidencia vigente vuelve a retirar la advertencia sin recrear la marca", async ({
    request,
    page,
  }) => {
    const antes = (await api(request, "GET", `/api/equipos/${equipo}/preferencias`))
      .data[0]
    await api(request, "PATCH", `/api/materias/${materiaNueva}`, { nombre })
    expect((await api(request, "GET", "/api/preferencias/huerfanas")).data).toHaveLength(
      0
    )
    expect(
      (await api(request, "GET", `/api/equipos/${equipo}/preferencias`)).data[0].id
    ).toBe(antes.id)
    await login(page, email, password)
    await page.goto("/admin/inventario")
    await expect(
      page.getByText("Prioridades sin materias vigentes", { exact: true })
    ).toHaveCount(0)
  })
})
