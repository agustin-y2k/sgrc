import AxeBuilder from "@axe-core/playwright"
import { expect, test } from "@playwright/test"
import { login } from "./helpers"

test.describe("recuperación y paginación de la revisión final", () => {
  test.skip(
    process.env.E2E_REVISION_AISLADA !== "1",
    "requiere el bloque vistas con base efímera"
  )
  test.setTimeout(120_000)

  for (const directa of [true, false]) {
    test(`una vista que no se descarga se recupera al recargar: entrada ${directa ? "directa" : "desde el menú"}`, async ({
      page,
    }) => {
      let descargas = 0
      const escrituras: string[] = []
      page.on("request", (r) => {
        if (
          new URL(r.url()).pathname.startsWith("/api/") &&
          r.method() !== "GET" &&
          !r.url().includes("/auth/login")
        )
          escrituras.push(r.url())
      })
      await page.route("**/assets/MisReservasPage-*.js", (route) => {
        descargas++
        return descargas === 1 ? route.abort("failed") : route.continue()
      })
      await login(page, process.env.E2E_ADMIN_EMAIL!, process.env.E2E_ADMIN_PASSWORD!)
      const sesion = await page.evaluate(() => localStorage.getItem("sgrc_token"))
      if (directa) await page.goto("/reservas")
      else await page.getByRole("link", { name: "Reservas", exact: true }).first().click()
      await expect(
        page.getByRole("heading", { name: "No pudimos abrir esta pantalla", exact: true })
      ).toBeVisible()
      await expect(
        page.getByRole("link", { name: "Inicio", exact: true }).first()
      ).toBeVisible()
      await expect(
        page.getByRole("link", { name: "Volver al inicio", exact: true })
      ).toHaveAttribute("href", "/")
      await expect(
        page.getByText(
          /Unexpected Application Error|Failed to fetch dynamically imported module/
        )
      ).toHaveCount(0)
      const axe = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
        .analyze()
      expect(axe.violations.map((v) => v.id)).toEqual([])
      await page.getByRole("button", { name: "Recargar la página", exact: true }).focus()
      await page.keyboard.press("Enter")
      await expect(page.locator("main h1")).toHaveText("Reservas")
      expect(descargas).toBe(2)
      expect(await page.evaluate(() => localStorage.getItem("sgrc_token"))).toBe(sesion)
      expect(escrituras).toEqual([])
    })
  }

  for (const admin of [true, false]) {
    test(`paginadores de avisos y conversaciones caben y permiten ir y volver: ${admin ? "admin" : "docente"}`, async ({
      page,
    }) => {
      await login(
        page,
        admin ? process.env.E2E_ADMIN_EMAIL! : process.env.E2E_DOCENTE_EMAIL!,
        admin ? process.env.E2E_ADMIN_PASSWORD! : process.env.E2E_DOCENTE_PASSWORD!
      )
      await page.route(
        (u) =>
          ["/api/notifications", "/api/sugerencias", "/api/mis-sugerencias"].includes(
            u.pathname.replace(/\/$/, "")
          ),
        (route) => {
          if (route.request().method() !== "GET") return route.continue()
          const u = new URL(route.request().url())
          const pagina = Number(u.searchParams.get("page") || 1)
          const inicio = (pagina - 1) * 50
          const data = Array.from({ length: Math.min(50, 125 - inicio) }, (_, i) => {
            const id = `prueba-${inicio + i}`
            const fecha = "2026-10-20T12:00:00Z"
            return u.pathname.includes("notifications")
              ? {
                  id,
                  mensaje: `Aviso de prueba ${inicio + i}`,
                  tipo: "GENERAL",
                  estado: "NO_LEIDA",
                  creadaEn: fecha,
                }
              : {
                  id,
                  tipo: "AYUDA",
                  asunto: `Consulta de prueba ${inicio + i}`,
                  estado: "ABIERTA",
                  esperaRespuesta: true,
                  mensajes: [
                    {
                      id: `${id}-mensaje`,
                      deAdmin: false,
                      texto: "Consulta",
                      escritoEn: fecha,
                    },
                  ],
                  creadaEn: fecha,
                  ultimaActividadEn: fecha,
                }
          })
          return route.fulfill({
            status: 200,
            json: { data, meta: { page: pagina, pageSize: 50, total: 125 } },
          })
        }
      )
      await page.goto("/notificaciones")
      await page.getByRole("button", { name: "Ver conversaciones", exact: true }).click()
      await expect(
        page.getByRole("button", { name: "Siguiente", exact: true })
      ).toHaveCount(2)
      for (const width of [320, 390, 768, 1280]) {
        await page.setViewportSize({ width, height: 844 })
        const recortes = await page
          .getByRole("button", { name: /^(Anterior|Siguiente)$/ })
          .evaluateAll((botones) =>
            botones.flatMap((b) => {
              const r = b.getBoundingClientRect()
              let izquierda = 0,
                derecha = innerWidth
              for (let p = b.parentElement; p; p = p.parentElement) {
                if (
                  ["hidden", "clip", "auto", "scroll"].includes(
                    getComputedStyle(p).overflowX
                  )
                ) {
                  const limite = p.getBoundingClientRect()
                  izquierda = Math.max(izquierda, limite.left)
                  derecha = Math.min(derecha, limite.right)
                }
              }
              return r.left < izquierda - 1 || r.right > derecha + 1
                ? [b.textContent]
                : []
            })
          )
        expect(recortes, `controles completos a ${width}px`).toEqual([])
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1
          )
        ).toBe(true)
      }
      await page.setViewportSize({ width: 320, height: 844 })
      for (const indice of [0, 1]) {
        const siguiente = page
          .getByRole("button", { name: "Siguiente", exact: true })
          .nth(indice)
        const anterior = page
          .getByRole("button", { name: "Anterior", exact: true })
          .nth(indice)
        await siguiente.click()
        await expect(page.getByText("Página 2 de 3", { exact: true })).toHaveCount(1)
        await siguiente.click()
        await expect(page.getByText("Página 3 de 3", { exact: true })).toHaveCount(1)
        await expect(siguiente).toBeDisabled()
        await anterior.click()
        await expect(page.getByText("Página 2 de 3", { exact: true })).toHaveCount(1)
        await anterior.click()
        await expect(page.getByText("Página 1 de 3", { exact: true })).toHaveCount(2)
        await expect(anterior).toBeDisabled()
      }
    })
  }
})
