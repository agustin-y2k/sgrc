import AxeBuilder from "@axe-core/playwright"
import { expect, test } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

for (const admin of [false, true]) {
  test(`soporte: entrada y seguimiento para ${admin ? "administrador" : "docente"} sin depender del correo`, async ({
    page,
  }) => {
    await login(
      page,
      admin ? process.env.E2E_ADMIN_EMAIL! : DOCENTE_EMAIL,
      admin ? process.env.E2E_ADMIN_PASSWORD! : DOCENTE_PASSWORD
    )
    await page.route(/\/api\/notifications\/?(?:\?.*)?$/, (r) =>
      r.fulfill({
        json: { data: [], meta: { page: 1, pageSize: 50, total: 0, totalPages: 1 } },
      })
    )
    await page.route("**/api/notifications/preferencias-email", (r) =>
      r.fulfill({ json: { data: [] } })
    )
    await page.route("**/api/sugerencias**", (r) => {
      expect(r.request().method()).toBe("GET")
      return r.fulfill({
        json: { data: [], meta: { page: 1, pageSize: 50, total: 0, totalPages: 1 } },
      })
    })
    await page.route("**/api/mis-sugerencias*", (r) =>
      r.fulfill({
        json: { data: [], meta: { page: 1, pageSize: 50, total: 0 } },
      })
    )
    await page.goto("/reservas")
    await page.setViewportSize({ width: 1140, height: 900 })
    const etiqueta = admin ? "Responder consultas" : "Pedir ayuda"
    const abrir = page.getByRole("button", { name: etiqueta, exact: true })
    await expect(abrir).toBeVisible()
    await abrir.click()
    await expect(page).toHaveURL(new RegExp(`soporte=${admin ? "consultas" : "nuevo"}`))
    await expect(
      page.getByRole("button", { name: "Ocultar conversaciones" })
    ).toBeVisible()
    if (admin) {
      await expect(page.getByText("Consultas de docentes", { exact: true })).toBeVisible()
      await expect(
        page.getByRole("button", { name: "Pedir ayuda", exact: true })
      ).toHaveCount(0)
    } else
      await expect(
        page.getByRole("button", { name: "Cerrar el formulario" })
      ).toBeVisible()
    await expect(page.getByText(/si la escuela tiene habilitado el envío/)).toBeVisible()
    await page.getByRole("button", { name: "Ocultar conversaciones" }).click()
    await abrir.click()
    await expect(
      page.getByRole("button", { name: "Ocultar conversaciones" })
    ).toBeVisible()
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
    ).toBe(true)
    await page.setViewportSize({ width: 320, height: 780 })
    await page.getByRole("button", { name: "Menú", exact: true }).click()
    await page.getByRole("link", { name: etiqueta, exact: true }).click()
    await expect(
      page.getByRole("button", { name: "Ocultar conversaciones" })
    ).toBeVisible()
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations
    ).toEqual([])
    await page.screenshot({
      path: test.info().outputPath("soporte-movil.png"),
      fullPage: true,
    })
  })
}
