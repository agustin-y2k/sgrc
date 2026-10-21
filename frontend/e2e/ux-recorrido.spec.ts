import AxeBuilder from "@axe-core/playwright"
import { expect, test, type Page } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

async function accesibilidadMovil(page: Page) {
  await page.setViewportSize({ width: 320, height: 780 })
  await page.evaluate(() =>
    Promise.all(
      document
        .getAnimations()
        .map((animacion) => animacion.finished.catch(() => undefined))
    )
  )
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
  ).toBe(true)
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze())
      .violations
  ).toEqual([])
}

test("docente: Mi cuenta, pedido bajo demanda y búsqueda por curso", async ({ page }) => {
  await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
  let catalogoConsultado = 0
  await page.route("**/api/ciclos", (r) => {
    catalogoConsultado++
    return r.fulfill({
      json: { data: [{ id: "ciclo-ux", anio: 2026, activo: true, archivado: false }] },
    })
  })
  await page.route("**/api/ciclos/ciclo-ux/cursos", (r) =>
    r.fulfill({
      json: {
        data: [
          { id: "curso-ux", nombre: "3°1", modalidad: "Construcción", archivado: false },
        ],
      },
    })
  )
  await page.route("**/api/cursos/curso-ux/materias", (r) =>
    r.fulfill({
      json: {
        data: [
          {
            id: "materia-ux",
            cursoId: "curso-ux",
            nombre: "Dibujo Técnico",
            archivado: false,
          },
          { id: "otra-ux", cursoId: "curso-ux", nombre: "Matemática", archivado: false },
        ],
      },
    })
  )
  await page.setViewportSize({ width: 320, height: 780 })
  await page.getByRole("button", { name: /^Menú/ }).click()
  await expect(
    page.getByRole("link", { name: "Mi cuenta", exact: true })
  ).toHaveAttribute("href", "/perfil")
  await page.getByRole("link", { name: "Mi cuenta", exact: true }).click()
  await expect(
    page.getByRole("heading", { name: "Mi perfil", exact: true })
  ).toBeVisible()
  expect(catalogoConsultado).toBe(0)
  await expect(page.getByLabel("¿Cuál materia?", { exact: true })).toHaveCount(0)
  await page.getByRole("button", { name: "Pedir otra materia", exact: true }).click()
  await page.getByLabel("Filtrar por curso", { exact: true }).selectOption("curso-ux")
  await page.getByLabel("Buscar materia", { exact: true }).fill("dibujo tecnico")
  await page.getByLabel("¿Cuál materia?", { exact: true }).selectOption("materia-ux")
  await page
    .getByLabel("¿Por qué la pedís?", { exact: true })
    .fill("Me asignaron otro curso")
  await expect(
    page.getByRole("button", { name: "Mandar el pedido", exact: true })
  ).toBeEnabled()
  await page.getByRole("button", { name: "Ocultar formulario", exact: true }).click()
  await page.getByRole("button", { name: "Pedir otra materia", exact: true }).click()
  await expect(page.getByLabel("¿Por qué la pedís?", { exact: true })).toHaveValue(
    "Me asignaron otro curso"
  )
  await expect(
    page.getByRole("button", { name: "Mandar el pedido", exact: true })
  ).toBeEnabled()
  await accesibilidadMovil(page)
})

test("administrador: encuentra un equipo y abre su ficha sin desplegar altas", async ({
  page,
}) => {
  await login(
    page,
    process.env.E2E_ADMIN_EMAIL ?? "",
    process.env.E2E_ADMIN_PASSWORD ?? ""
  )
  const equipo = {
    id: "equipo-ux",
    carroId: "carro-ux",
    identificador: 12,
    etiqueta: "PC 12",
    numeroSerie: "SERIE-UX-12",
    softwareInstalado: "AutoCAD 2026",
    estado: "DISPONIBLE",
    esComputadora: true,
    tipo: "PC",
    reservable: true,
    dadoDeBaja: false,
  }
  await page.route("**/api/carros", (r) =>
    r.fulfill({
      json: { data: [{ id: "carro-ux", nombre: "Carro EDUTEC", dadoDeBaja: false }] },
    })
  )
  await page.route("**/api/equipos", (r) => r.fulfill({ json: { data: [equipo] } }))
  await page.route("**/api/equipos?enCarro=false", (r) =>
    r.fulfill({ json: { data: [] } })
  )
  await page.route("**/api/carros/carro-ux/equipos", (r) =>
    r.fulfill({ json: { data: [equipo] } })
  )
  await page.goto("/admin/inventario")
  await expect(
    page.getByRole("button", { name: "Crear carro", exact: true })
  ).toBeHidden()
  await page
    .getByLabel("Equipo, carro, serie o software", { exact: true })
    .fill("autocad")
  await page.getByRole("button", { name: "Abrir ficha de PC 12", exact: true }).click()
  const ficha = page.locator("#equipo-inventario-equipo-ux")
  await expect(ficha).toBeFocused()
  await expect(ficha.locator("details")).toHaveAttribute("open", "")
  await expect(ficha.getByRole("button", { name: "Editar", exact: true })).toBeVisible()
  await expect(
    page.getByRole("button", { name: "Agregar al carro", exact: true })
  ).toBeHidden()
  await accesibilidadMovil(page)
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.screenshot({
    path: test.info().outputPath("inventario-busqueda.png"),
    fullPage: true,
  })
})

test("reservas: filtros, paginación y regreso del formulario conservan el contexto", async ({
  page,
}) => {
  await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
  const llamadas: URL[] = []
  await page.route("**/api/reservas?*", (r) => {
    const url = new URL(r.request().url())
    llamadas.push(url)
    return r.fulfill({
      json: {
        data: [
          {
            id: "reserva-ux",
            reservaGrupoId: "grupo-ux",
            equipoId: "equipo-ux",
            materiaId: "materia-ux",
            materiaNombre: "TIC",
            cursoNombre: "3°1",
            fecha: "2026-11-02",
            horaInicio: "08:00",
            horaFin: "09:00",
            estado: "CONFIRMADA",
            tipo: "NORMAL",
            etiqueta: "PC 1",
          },
        ],
        meta: {
          page: Number(url.searchParams.get("page") ?? 1),
          pageSize: 20,
          total: 41,
        },
      },
    })
  })
  await page.goto("/reservas")
  await expect(
    page.getByRole("button", { name: "Próximas", exact: true })
  ).toHaveAttribute("aria-pressed", "true")
  expect(llamadas.at(-1)?.searchParams.get("desde")).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  await page.getByLabel("Desde", { exact: true }).fill("2026-11-02")
  await page.getByLabel("Hasta", { exact: true }).fill("2026-11-16")
  await page.getByLabel("Buscar materia, curso o modalidad", { exact: true }).fill("TIC")
  await page
    .getByLabel("Estado de los equipos", { exact: true })
    .selectOption("CONFIRMADA")
  await page.getByRole("button", { name: "Aplicar filtros", exact: true }).click()
  await expect(page).toHaveURL(/busqueda=TIC/)
  await page.getByRole("button", { name: "Siguiente", exact: true }).click()
  await expect.poll(() => llamadas.at(-1)?.searchParams.get("page")).toBe("2")
  expect(llamadas.at(-1)?.searchParams.get("estado")).toBe("CONFIRMADA")
  expect(llamadas.at(-1)?.searchParams.get("desde")).toBe("2026-11-02")
  const destino = page.url()
  await page.getByRole("link", { name: "Nueva reserva", exact: true }).click()
  await page.getByRole("button", { name: "Cancelar", exact: true }).click()
  await expect(page).toHaveURL(destino)
  await expect(
    page.getByLabel("Buscar materia, curso o modalidad", { exact: true })
  ).toHaveValue("TIC")
  await accesibilidadMovil(page)
})
