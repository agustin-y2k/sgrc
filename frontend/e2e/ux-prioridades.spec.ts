import AxeBuilder from "@axe-core/playwright"
import { expect, test } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

for (const { serie, sinLibres } of [
  { serie: false, sinLibres: true },
  { serie: false, sinLibres: false },
  { serie: true, sinLibres: false },
]) {
  test(`prioridad: resumen y alcance ${serie ? "recurrente" : "puntual"}${sinLibres ? " sin equipos libres" : " mixto"}`, async ({
    page,
  }) => {
    await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    await page.route("**/api/mis-materias", (r) =>
      r.fulfill({
        json: {
          data: [
            {
              materiaId: "materia-ux",
              materiaNombre: "Dibujo Técnico",
              cursoNombre: "3°1",
              cicloId: "ciclo-ux",
              cicloAnio: 2026,
            },
          ],
        },
      })
    )
    await page.route("**/api/jornada", (r) => r.fulfill({ json: { data: [] } }))
    await page.route("**/api/reservas/recurrentes/previsualizar", (r) =>
      r.fulfill({
        json: {
          cantidadClases: 3,
          puedeCrear: true,
          equiposATomar: ["tomable-ux"],
          fechas: [],
        },
      })
    )
    await page.route("**/api/equipos-disponibles?*", (r) =>
      r.fulfill({
        json: {
          data: sinLibres
            ? []
            : [
                {
                  equipoId: "libre-ux",
                  etiqueta: "PC libre",
                  carroNombre: "Carro de prueba",
                  tramo: "NEUTRAL",
                },
              ],
          ocupados: [
            {
              equipoId: "tomable-ux",
              etiqueta: "PC preferente",
              carroNombre: "Carro de prueba",
              docenteNombre: "Docente de TIC",
              materiaNombre: "TIC",
              horaInicio: "08:00",
              horaFin: "09:00",
              puedeTomarse: true,
              puedePedirse: false,
              reservaId: "reserva-ajena-ux",
            },
          ],
        },
      })
    )
    const enviados: unknown[] = []
    await page.route("**/api/reservas**", (r) => {
      if (r.request().url().endsWith("/previsualizar")) return r.fallback()
      if (r.request().method() !== "POST") return r.continue()
      enviados.push(r.request().postDataJSON())
      return r.fulfill({
        status: 201,
        json: {
          grupo: {},
          reservas: [],
          reservasReubicadas: 2,
          reservasCanceladas: 1,
          docentesAfectados: 2,
        },
      })
    })
    await page.goto("/reservas/nueva")
    await page.getByLabel("Materia", { exact: true }).selectOption("materia-ux")
    if (serie) {
      await page.getByRole("button", { name: "Se repite todas las semanas" }).click()
      await page.getByLabel("Desde", { exact: true }).fill("2026-11-02")
      await page.getByLabel("Hasta", { exact: true }).fill("2026-11-16")
    } else await page.getByLabel("Fecha", { exact: true }).fill("2026-11-02")
    await page.getByLabel("Hora de inicio: hora", { exact: true }).selectOption("08")
    await page.getByLabel("Hora de inicio: minutos", { exact: true }).selectOption("00")
    await page.getByLabel("Hora de fin: hora", { exact: true }).selectOption("09")
    await page.getByLabel("Hora de fin: minutos", { exact: true }).selectOption("00")
    await page
      .getByRole("checkbox", { name: "PC preferente (Carro de prueba)", exact: true })
      .check()
    await expect(page.getByRole("status")).toHaveText(
      /1 equipo seleccionado: 0 libres y 1 por prioridad/
    )
    await expect(page.getByText(/Vas a reservar 1 equipo/)).toContainText(
      "pueden quedar faltantes"
    )
    if (!sinLibres) {
      await page
        .getByRole("checkbox", { name: "PC libre (Carro de prueba)", exact: true })
        .check()
      await expect(page.getByRole("status")).toHaveText(
        /2 equipos seleccionados: 1 libre y 1 por prioridad/
      )
    }
    const resumen = page.getByText(
      sinLibres ? /Vas a reservar 1 equipo/ : /Vas a reservar 2 equipos/
    )
    await expect(resumen).toContainText(
      serie ? "El alcance son las fechas" : "El alcance es esta fecha"
    )
    if (serie) {
      await expect(page.getByText(/Las demás fechas de las series ajenas/)).toBeVisible()
      await page.getByRole("button", { name: "Comprobar serie" }).click()
      await page.getByRole("checkbox", { name: /Autorizo tomar los equipos/ }).check()
    }
    await page.setViewportSize({ width: 320, height: 780 })
    const ancho = await resumen.boundingBox()
    expect(ancho!.x + ancho!.width).toBeLessThanOrEqual(320)
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations
    ).toEqual([])
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.screenshot({
      path: test.info().outputPath("prioridad-movil.png"),
      fullPage: true,
    })
    await page.getByRole("button", { name: "Confirmar reserva", exact: true }).click()
    expect(enviados).toEqual([
      expect.objectContaining({
        equipoIds: sinLibres ? ["tomable-ux"] : ["tomable-ux", "libre-ux"],
        equiposATomar: ["tomable-ux"],
      }),
    ])
    await expect(
      page.getByRole("status").filter({ hasText: /Reserva.*confirmada/ })
    ).toContainText("Se repusieron 2 equipos en las reservas desplazadas")
    await expect(
      page.getByRole("status").filter({ hasText: /Reserva.*confirmada/ })
    ).toContainText("1 equipo quedó sin reemplazo disponible")
    // La escritura se intercepta; ninguna reserva local se crea o desplaza.
  })
}

test("administrador: prioridad de dispositivos, alcances y empates", async ({ page }) => {
  const email = process.env.E2E_ADMIN_EMAIL ?? ""
  const password = process.env.E2E_ADMIN_PASSWORD ?? ""
  test.skip(!email || !password, "requiere administrador de prueba")
  await login(page, email, password)
  await page.route("**/api/carros", (r) =>
    r.fulfill({ json: { data: [{ id: "carro-ux", nombre: "Carro de prueba" }] } })
  )
  await page.route("**/api/carros/carro-ux/equipos", (r) =>
    r.fulfill({
      json: {
        data: [
          {
            id: "equipo-ux",
            etiqueta: "PC de prueba",
            reservable: true,
            dadoDeBaja: false,
          },
        ],
      },
    })
  )
  await page.route("**/api/catalogo-prioridades", (r) =>
    r.fulfill({
      json: {
        data: ["Dibujo Técnico", "TIC"].map((materiaNombre) => ({
          materiaId: materiaNombre,
          materiaNombre,
          contenedorId: "curso-ux",
          contenedorNombre: "3°2 Construcción",
          tipo: "CURSO",
          modalidad: "Construcción",
          anio: 3,
          division: "2",
        })),
      },
    })
  )
  await page.route("**/api/carros/carro-ux/preferencias", (r) =>
    r.fulfill({
      json: {
        data: [
          {
            id: "marca-ux",
            equipoId: "equipo-ux",
            materiaNombre: "TIC",
            prioridad: 1,
          },
        ],
      },
    })
  )
  await page.goto("/admin/inventario")
  await page.getByRole("button", { name: "Preferencia del carro", exact: true }).click()
  const panel = page
    .locator("div")
    .filter({
      has: page.getByRole("heading", {
        name: "Marcar preferencia para todo el carro",
        exact: true,
      }),
    })
    .last()
  await expect(
    page.getByText(/La misma marca se guarda en cada dispositivo/)
  ).toBeVisible()
  await expect(page.getByLabel("Materia", { exact: true })).toBeDisabled()
  await page.getByLabel("Modalidad", { exact: true }).selectOption("Construcción")
  await page.getByLabel("Año", { exact: true }).selectOption("3")
  await page.getByLabel("División", { exact: true }).selectOption("2")
  await page.getByLabel("Materia", { exact: true }).selectOption("Dibujo Técnico")
  await expect(page.getByText(/Cuando ambas marcas aplican/)).toBeVisible()
  await page.getByText("Cómo se resuelven las prioridades", { exact: true }).click()
  await expect(page.getByText(/En un empate ninguna desplaza/)).toBeVisible()
  await expect(page.getByText(/reservas que empiezan en una hora o menos/)).toBeVisible()
  await expect(page.getByLabel("Prioridad", { exact: true })).toHaveAccessibleDescription(
    "1 = mayor prioridad; 9 = menor prioridad."
  )
  await page.setViewportSize({ width: 320, height: 780 })
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze())
      .violations
  ).toEqual([])
  await panel.screenshot({ path: test.info().outputPath("ayuda-admin.png") })
})
