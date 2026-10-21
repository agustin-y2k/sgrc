import AxeBuilder from "@axe-core/playwright"
import { expect, test } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

test("borrador: vuelve con los datos, revalida y se limpia al cerrar sesión", async ({
  page,
}) => {
  await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
  await page.route("**/api/mis-materias", (r) =>
    r.fulfill({
      json: {
        data: [
          {
            materiaId: "materia-ux",
            materiaNombre: "TIC",
            cursoNombre: "3°1",
            cicloId: "ciclo-ux",
            cicloAnio: 2026,
          },
        ],
      },
    })
  )
  await page.route("**/api/jornada", (r) => r.fulfill({ json: { data: [] } }))
  let disponible = true
  await page.route("**/api/equipos-disponibles?*", (r) =>
    r.fulfill({
      json: {
        data: disponible
          ? [{ equipoId: "equipo-ux", etiqueta: "PC de prueba", tramo: "NEUTRAL" }]
          : [],
      },
    })
  )
  await page.goto("/reservas/nueva")
  await page.getByLabel("Materia", { exact: true }).selectOption("materia-ux")
  await page.getByLabel("Fecha", { exact: true }).fill("2026-11-02")
  await page.getByLabel("Hora de inicio: hora", { exact: true }).selectOption("08")
  await page.getByLabel("Hora de fin: hora", { exact: true }).selectOption("09")
  await page.getByRole("checkbox", { name: /PC de prueba/ }).check()
  await expect(
    page.getByRole("button", { name: "Confirmar reserva", exact: true })
  ).toBeEnabled()
  await page.goto("/inventario")
  await page.goto("/reservas/nueva")
  await expect(page.getByLabel("Materia", { exact: true })).toHaveValue("materia-ux")
  await expect(page.getByRole("checkbox", { name: /PC de prueba/ })).toBeChecked()
  await expect(page.getByText(/Borrador recuperado/)).toBeVisible()
  disponible = false
  await page.reload()
  await expect(
    page.getByText("No hay ningún equipo libre en esa franja.", { exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole("button", { name: "Confirmar reserva", exact: true })
  ).toBeDisabled()
  await page.getByRole("button", { name: "Salir", exact: true }).click()
  await expect(page).toHaveURL(/\/login$/)
  expect(
    await page.evaluate(() => sessionStorage.getItem("sgrc_borrador_reserva"))
  ).toBeNull()
  await login(
    page,
    process.env.E2E_OTRO_DOCENTE_EMAIL ?? "dibujo@local.test",
    process.env.E2E_OTRO_DOCENTE_PASSWORD ?? DOCENTE_PASSWORD
  )
  await page.goto("/reservas/nueva")
  await expect(page.getByLabel("Fecha", { exact: true })).toHaveValue("")
  await expect(page.getByText(/Borrador recuperado/)).toHaveCount(0)
})

for (const tema of ["claro", "oscuro"]) {
  test(`estados y menú móvil del administrador en tema ${tema}`, async ({ page }) => {
    await login(
      page,
      process.env.E2E_ADMIN_EMAIL ?? "",
      process.env.E2E_ADMIN_PASSWORD ?? ""
    )
    await page.evaluate((valor) => localStorage.setItem("sgrc-tema", valor), tema)
    const equipos = ["DISPONIBLE", "EN_MANTENIMIENTO", "FUERA_DE_SERVICIO"].map(
      (estado, i) => ({
        id: `equipo-${i}`,
        carroId: "carro-ux",
        identificador: i + 1,
        etiqueta: `PC ${i + 1}`,
        numeroSerie: `SERIE-UX-${i}`,
        estado,
        tipo: "PC",
        reservable: true,
        esComputadora: true,
        dadoDeBaja: false,
      })
    )
    await page.route("**/api/carros", (r) =>
      r.fulfill({
        json: {
          data: [{ id: "carro-ux", nombre: "Carro de prueba", dadoDeBaja: false }],
        },
      })
    )
    await page.route("**/api/carros/carro-ux/equipos", (r) =>
      r.fulfill({ json: { data: equipos } })
    )
    await page.route("**/api/equipos?enCarro=false", (r) =>
      r.fulfill({ json: { data: [] } })
    )
    await page.goto("/admin/inventario")
    await page.getByRole("button", { name: "Gestionar equipos", exact: true }).click()
    await expect(
      page.locator('[data-slot="badge"]').filter({ hasText: /^Fuera de servicio$/ })
    ).toBeVisible()
    await page.setViewportSize({ width: 320, height: 780 })
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations
    ).toEqual([])
    await page.getByRole("button", { name: /^Menú/ }).click()
    await page.getByText("Administración y configuración", { exact: true }).click()
    const menu = page.locator("#menu-principal")
    const limites = await menu.boundingBox()
    expect(limites!.y + limites!.height).toBeLessThanOrEqual(780)
    await menu.evaluate((n) => {
      n.scrollTop = n.scrollHeight
    })
    await expect(page.getByRole("link", { name: "Mi cuenta", exact: true })).toBeVisible()
    const cerrar = page.getByRole("button", { name: "Cerrar", exact: true })
    expect((await cerrar.boundingBox())!.y).toBeGreaterThanOrEqual(0)
    await page.screenshot({
      path: test.info().outputPath(`menu-${tema}.png`),
      fullPage: true,
    })
    await cerrar.click()
    await page.goto("/admin/bloquear-equipos")
    await expect(
      page.locator('[data-slot="badge"]').filter({ hasText: /^Fuera de servicio$/ })
    ).toBeVisible()
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations
    ).toEqual([])
  })
}
