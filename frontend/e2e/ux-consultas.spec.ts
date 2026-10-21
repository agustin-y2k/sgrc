import AxeBuilder from "@axe-core/playwright"
import { expect, test, type Page } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

const adminEmail = process.env.E2E_ADMIN_EMAIL ?? ""
const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? ""
const carro = { id: "carro-ux", nombre: "Carro de prueba", dadoDeBaja: false }

async function comprobarMovil(page: Page, nombre: string) {
  await page.setViewportSize({ width: 320, height: 780 })
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
  ).toBe(true)
  for (const elemento of await page
    .locator('main select, main [role="alert"], main button:visible')
    .all()) {
    const limites = await elemento.boundingBox()
    if (limites) {
      expect(limites.x).toBeGreaterThanOrEqual(0)
      expect(limites.x + limites.width).toBeLessThanOrEqual(320)
    }
  }
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze())
      .violations
  ).toEqual([])
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.screenshot({ path: test.info().outputPath(`${nombre}.png`), fullPage: true })
}

test.describe("consultas fallidas y recuperación", () => {
  test.setTimeout(45_000)

  test("inventario: un carro inaccesible no parece vacío y se recupera con reintento", async ({
    page,
  }) => {
    test.skip(!adminEmail || !adminPassword, "requiere un administrador de prueba")
    await login(page, adminEmail, adminPassword)
    await page.route("**/api/carros", (ruta) => ruta.fulfill({ json: { data: [carro] } }))
    await page.route("**/api/equipos?enCarro=false", (ruta) =>
      ruta.fulfill({ json: { data: [] } })
    )
    let falla = true
    await page.route("**/api/carros/carro-ux/equipos", (ruta) =>
      ruta.fulfill(
        falla
          ? { status: 503, json: { error: "Consulta temporalmente indisponible" } }
          : { json: { data: [] } }
      )
    )
    await page.goto("/admin/inventario")
    await page.getByRole("button", { name: "Gestionar equipos" }).click()
    await expect(
      page.getByText(/No pudimos cargar los equipos de este carro/)
    ).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText(/Este carro no tiene equipos activos/)).toHaveCount(0)
    await expect(
      page.getByText("Agregar un equipo a este carro", { exact: true })
    ).toHaveCount(0)
    await comprobarMovil(page, "inventario-error")
    falla = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await expect(page.getByText(/Este carro no tiene equipos activos/)).toBeVisible()
    await expect(
      page.getByText("Agregar un equipo a este carro", { exact: true })
    ).toBeHidden()
    await page
      .getByRole("button", { name: "Agregar un equipo al carro", exact: true })
      .click()
    await expect(
      page.getByText("Agregar un equipo a este carro", { exact: true })
    ).toBeVisible()
  })

  test("inventario: fallos en carros y equipos sueltos bloquean las altas dependientes", async ({
    page,
  }) => {
    test.skip(!adminEmail || !adminPassword, "requiere un administrador de prueba")
    await login(page, adminEmail, adminPassword)
    let fallaCarros = true
    let fallaEquipos = true
    await page.route("**/api/carros", (ruta) =>
      ruta.fulfill(
        fallaCarros
          ? { status: 503, json: { error: "Consulta temporalmente indisponible" } }
          : { json: { data: [carro] } }
      )
    )
    await page.route("**/api/equipos?enCarro=false", (ruta) =>
      ruta.fulfill(
        fallaEquipos
          ? { status: 503, json: { error: "Consulta temporalmente indisponible" } }
          : { json: { data: [] } }
      )
    )
    await page.goto("/admin/inventario")
    await expect(
      page.getByText(/No pudimos cargar los carros del inventario/)
    ).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText(/No pudimos cargar los equipos sueltos/)).toBeVisible()
    await page.getByRole("button", { name: "Nuevo carro", exact: true }).click()
    await page.getByLabel("Nombre", { exact: true }).fill("Carro pendiente")
    await expect(
      page.getByRole("button", { name: "Crear carro", exact: true })
    ).toBeDisabled()
    await expect(
      page.getByText("No hay ningún equipo cargado todavía.", { exact: true })
    ).toHaveCount(0)
    await expect(
      page.getByRole("button", { name: "Agregar equipo", exact: true })
    ).toHaveCount(0)
    fallaCarros = false
    await page
      .getByRole("alert")
      .filter({ hasText: "No pudimos cargar los carros" })
      .getByRole("button", { name: "Reintentar" })
      .click()
    await expect(
      page.getByRole("button", { name: "Crear carro", exact: true })
    ).toBeEnabled()
    await expect(page.getByLabel("Nombre", { exact: true })).toHaveValue(
      "Carro pendiente"
    )
    fallaEquipos = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await expect(
      page.getByText("No hay ningún equipo cargado todavía.", { exact: true })
    ).toBeVisible()
  })

  test("perfil: explica el fallo de materias, conserva el motivo y recupera el selector", async ({
    page,
  }) => {
    await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    await page.route("**/api/ciclos", (ruta) =>
      ruta.fulfill({
        json: { data: [{ id: "ciclo-ux", anio: 2026, activo: true, archivado: false }] },
      })
    )
    await page.route("**/api/ciclos/ciclo-ux/cursos", (ruta) =>
      ruta.fulfill({
        json: { data: [{ id: "curso-ux", nombre: "3°1", anio: 3, archivado: false }] },
      })
    )
    let falla = true
    await page.route("**/api/cursos/curso-ux/materias", (ruta) =>
      ruta.fulfill(
        falla
          ? { status: 503, json: { error: "Consulta temporalmente indisponible" } }
          : {
              json: {
                data: [
                  {
                    id: "materia-ux",
                    cursoId: "curso-ux",
                    nombre: "Materia de prueba",
                    archivado: false,
                  },
                ],
              },
            }
      )
    )
    await page.goto("/perfil")
    await page.getByRole("button", { name: "Pedir otra materia", exact: true }).click()
    await page.getByLabel("¿Por qué la pedís?").fill("Me asignaron otro curso")
    await expect(page.getByText(/No pudimos consultar las materias/)).toBeVisible({
      timeout: 15_000,
    })
    await expect(page.getByLabel("¿Cuál materia?")).toBeDisabled()
    await expect(
      page.getByRole("button", { name: "Mandar el pedido", exact: true })
    ).toBeDisabled()
    await comprobarMovil(page, "materias-error")
    falla = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await page.getByLabel("¿Cuál materia?").selectOption("materia-ux")
    await expect(page.getByLabel("¿Por qué la pedís?")).toHaveValue(
      "Me asignaron otro curso"
    )
    await expect(
      page.getByRole("button", { name: "Mandar el pedido", exact: true })
    ).toBeEnabled()
  })

  test("cambio de computadora: no confunde el error con falta de equipos y comprueba el alcance", async ({
    page,
  }) => {
    await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    await page.route("**/api/reservas?*", (ruta) =>
      ruta.fulfill({
        json: {
          data: [
            {
              id: "reserva-ux",
              reservaGrupoId: "grupo-ux",
              equipoId: "equipo-ux",
              materiaId: "materia-ux",
              materiaNombre: "Materia de prueba",
              cursoNombre: "3°1",
              fecha: "2026-11-02",
              horaInicio: "09:00",
              horaFin: "10:00",
              estado: "CONFIRMADA",
              tipo: "NORMAL",
              etiqueta: "PC 1",
              nombreDocenteSnapshot: "Docente de prueba",
              reglaRecurrenciaId: "serie-ux",
            },
          ],
          meta: { total: 1, page: 1, pageSize: 20 },
        },
      })
    )
    let falla = true
    await page.route("**/api/equipos-disponibles?*", (ruta) =>
      ruta.fulfill(
        falla
          ? { status: 503, json: { error: "Consulta temporalmente indisponible" } }
          : {
              json: {
                data: [
                  {
                    equipoId: "reemplazo-ux",
                    etiqueta: "PC de reemplazo",
                    tramo: "NEUTRAL",
                  },
                ],
              },
            }
      )
    )
    await page.goto("/reservas")
    await page.getByRole("button", { name: "Cambiar computadora", exact: true }).click()
    await expect(
      page.getByText(/No pudimos consultar las computadoras disponibles/)
    ).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText(/No hay ninguna computadora libre/)).toHaveCount(0)
    await expect(
      page.getByRole("button", { name: "Cambiar", exact: true })
    ).toBeDisabled()
    await comprobarMovil(page, "cambio-error")
    falla = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await page.getByLabel("¿Por cuál?").selectOption("reemplazo-ux")
    await expect(page.getByRole("button", { name: "Cambiar", exact: true })).toBeEnabled()
    await page.getByLabel("Esta fecha y todas las siguientes", { exact: true }).check()
    await expect(page.getByLabel("¿Por cuál?")).toHaveValue("")
    await expect(
      page.getByRole("button", { name: "Cambiar", exact: true })
    ).toBeDisabled()
    // Se verifica la preparación del cambio; no se modifica ninguna reserva.
  })
})
