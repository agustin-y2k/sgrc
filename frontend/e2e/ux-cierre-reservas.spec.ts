import AxeBuilder from "@axe-core/playwright"
import { expect, test } from "@playwright/test"
import { login } from "./helpers"

const email = process.env.E2E_ADMIN_EMAIL ?? ""
const password = process.env.E2E_ADMIN_PASSWORD ?? ""

test.describe("claridad del cierre y las clases completas", () => {
  test.skip(!email || !password, "requiere credenciales de administrador de prueba")

  test("un año inválido no cambia el alcance del cierre", async ({ page }) => {
    await login(page, email, password)
    // Datos y respuestas del cierre simulados: esta prueba no archiva ningún ciclo.
    await page.route("**/api/ciclos", (ruta) =>
      ruta.fulfill({
        json: {
          data: [{ id: "ciclo-ux", anio: 2026, activo: true, archivado: false }],
        },
      })
    )
    const enviados: unknown[] = []
    await page.route("**/api/ciclos/*/archivar", (ruta) => {
      enviados.push(ruta.request().postDataJSON())
      return ruta.fulfill({
        status: 400,
        json: { error: "Petición interceptada; el ciclo se conserva." },
      })
    })
    await page.goto("/admin/academico")
    await page
      .getByRole("button", { name: "Cerrar el año y abrir el siguiente", exact: true })
      .click()
    const anio = page.getByLabel(/Año del ciclo siguiente/)
    for (const valor of ["", "2027/2028", "2O27", "Infinity", "2101"]) {
      await anio.fill(valor)
      await expect(
        page.getByRole("button", { name: /Cerrar 2026 y crear/ })
      ).toBeDisabled()
      await expect(anio).toHaveAttribute("aria-invalid", "true")
    }
    expect(enviados).toEqual([])
    await page.setViewportSize({ width: 320, height: 780 })
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
    ).toBe(true)
    await anio.fill("2028")
    await page
      .getByRole("button", { name: "Cerrar 2026 y crear 2028", exact: true })
      .click()
    await expect(
      page.getByText("Petición interceptada; el ciclo se conserva.")
    ).toBeVisible()
    expect(enviados).toEqual([{ clonarA: 2028 }])
    await page.getByLabel("Cerrar sin crear otro ciclo", { exact: true }).check()
    await expect(anio).toHaveCount(0)
    await page
      .getByRole("button", { name: "Cerrar 2026 sin abrir otro ciclo", exact: true })
      .click()
    await expect.poll(() => enviados.length).toBe(2)
    expect(enviados[1]).toEqual({})
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations
    ).toEqual([])
  })

  test("las clases conservan todos sus equipos al cambiar de página", async ({
    page,
  }) => {
    await login(page, email, password)
    const llamadas: URL[] = []
    const clases = Array.from({ length: 21 }, (_, clase) =>
      Array.from({ length: clase === 0 || clase === 20 ? 30 : 1 }, (_, equipo) => ({
        id: `reserva-ux-${clase}-${equipo}`,
        reservaGrupoId: `grupo-ux-${clase}`,
        equipoId: `equipo-ux-${clase}-${equipo}`,
        materiaId: "materia-ux",
        materiaNombre: `Clase de prueba ${clase + 1}`,
        cursoNombre: "3°1",
        fecha: "2026-11-02",
        horaInicio: "09:00",
        horaFin: "10:00",
        estado: "CONFIRMADA",
        tipo: "NORMAL",
        creadoPor: "docente-ux",
        nombreDocenteSnapshot: "Docente de prueba",
        etiqueta: `PC ${equipo + 1}`,
        carroNombre: "Carro de prueba",
      }))
    )
    await page.route("**/api/reservas*", (ruta) => {
      const url = new URL(ruta.request().url())
      if (url.pathname !== "/api/reservas" || ruta.request().method() !== "GET")
        return ruta.continue()
      llamadas.push(url)
      const numero = Number(url.searchParams.get("page") ?? 1)
      const cantidad = Number(url.searchParams.get("pageSize") ?? 50)
      const agrupadas = url.searchParams.get("porClases") === "true"
      const datos = agrupadas
        ? clases.slice((numero - 1) * cantidad, numero * cantidad).flat()
        : clases.flat().slice((numero - 1) * cantidad, numero * cantidad)
      return ruta.fulfill({
        json: {
          data: datos,
          meta: {
            total: agrupadas ? clases.length : clases.flat().length,
            page: numero,
            pageSize: cantidad,
          },
        },
      })
    })
    await page.goto("/reservas")
    const primera = page
      .locator('[data-slot="card"]')
      .filter({ hasText: /^Clase de prueba 1 —/ })
    await expect(primera).toBeVisible()
    await expect(
      primera.getByText("PC 30 · Carro de prueba", { exact: true })
    ).toBeVisible()
    expect(llamadas.at(-1)?.searchParams.get("porClases")).toBe("true")
    await expect(
      page.getByText("1–20 de 21 clases y bloqueos", { exact: true })
    ).toBeVisible()
    await page.getByRole("button", { name: "Siguiente", exact: true }).click()
    const ultima = page
      .locator('[data-slot="card"]')
      .filter({ hasText: "Clase de prueba 21 —" })
    await expect(
      ultima.getByText("PC 30 · Carro de prueba", { exact: true })
    ).toBeVisible()
    await expect(primera).toHaveCount(0)
    await ultima.getByRole("button", { name: "Cancelar", exact: true }).click()
    await expect(
      ultima.getByText("Se cancelan 30 equipos de esta reserva.", { exact: true })
    ).toBeVisible()
    await page.setViewportSize({ width: 320, height: 780 })
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
    // Se abre la confirmación; no se envía una cancelación al backend.
  })
})
