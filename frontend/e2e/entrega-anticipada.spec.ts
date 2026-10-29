import { expect, test } from "@playwright/test"
import { login } from "./helpers"

// Reloj y listados controlados para comprobar también bordes de medianoche sin
// depender de la hora de ejecución. La regla transaccional se prueba con PostgreSQL.
test.describe("explicación del retiro anticipado", () => {
  test.skip(process.env.E2E_ENTREGA_ANTICIPADA_AISLADA !== "1", "requiere base efímera")
  for (const ancho of [390, 1280]) {
    for (const portada of [true, false]) {
      test(`${portada ? "portada" : "entregas"} a ${ancho}px conserva la PC de la clase intermedia`, async ({
        page,
      }) => {
        await page.setViewportSize({ width: ancho, height: 900 })
        await page.clock.setFixedTime(new Date("2026-05-04T09:00:00Z"))
        const comun = {
          fecha: "2026-05-04",
          estado: "CONFIRMADA",
          tipo: "NORMAL",
          nombreDocenteSnapshot: "Docente de prueba",
          cursoNombre: "1°1°",
          carroNombre: "Carro de prueba",
        }
        const reservas = [
          {
            ...comun,
            id: "intermedia",
            reservaGrupoId: "grupo-previo",
            equipoId: "pc1",
            etiqueta: "PC 1",
            identificador: 1,
            materiaNombre: "Clase intermedia",
            horaInicio: "08:00",
            horaFin: "09:00",
          },
          ...[1, 2].map((n) => ({
            ...comun,
            id: `posterior-${n}`,
            reservaGrupoId: "grupo-posterior",
            equipoId: `pc${n}`,
            etiqueta: `PC ${n}`,
            identificador: n,
            materiaNombre: "Clase posterior",
            horaInicio: "12:00",
            horaFin: "13:00",
          })),
        ]
        await page.route("**/api/reservas?**", async (route) => {
          if (route.request().method() !== "GET") return route.continue()
          await route.fulfill({
            json: { data: reservas, total: reservas.length, page: 1, pageSize: 200 },
          })
        })
        await page.route("**/api/prestamos", async (route) => {
          if (route.request().method() !== "GET") return route.continue()
          await route.fulfill({ json: { data: [] } })
        })
        let entregas = 0
        await page.route("**/api/prestamos/por-reserva", async (route) => {
          entregas++
          await route.abort()
        })
        await login(page, process.env.E2E_ADMIN_EMAIL!, process.env.E2E_ADMIN_PASSWORD!)
        await page.goto(portada ? "/" : "/admin/entregas")
        const aviso = page.getByText(/no se puede adelantar/i)
        await expect(aviso).toContainText("08:00")
        await expect(aviso).toContainText("09:00")
        await expect(aviso).toContainText("se mantiene")
        if (portada) {
          await expect(
            page.getByRole("button", { name: "Entregar disponibles (1)", exact: true })
          ).toBeEnabled()
        } else {
          const grupo = page
            .locator("div.rounded-md.border.p-3")
            .filter({ has: page.getByText("Clase posterior · 1°1°", { exact: true }) })
          const impedida = grupo.getByRole("checkbox", { name: /^PC 1/ })
          await expect(impedida).toBeDisabled()
          await grupo.getByRole("button", { name: "Marcar todas" }).click()
          await expect(impedida).not.toBeChecked()
          await expect(grupo.getByRole("checkbox", { name: /^PC 2/ })).toBeChecked()
          await expect(
            page.getByRole("button", { name: "Entregar 1 equipo", exact: true })
          ).toBeEnabled()
        }
        expect(entregas).toBe(0)
        expect(
          await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
        ).toBe(true)
      })
    }
  }
})
