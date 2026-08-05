import { test, expect, type Page } from "@playwright/test"

const ADMIN_EMAIL = process.env.E2E_ADMIN_EMAIL ?? ""
const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? ""

const CARPETA = process.env.CARPETA_CAPTURAS ?? ""

async function login(page: Page) {
  await page.goto("/login")
  await page.getByLabel(/email/i).fill(ADMIN_EMAIL)
  await page.getByLabel(/contraseña/i).fill(ADMIN_PASSWORD)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()
  await page.waitForURL((u) => !u.pathname.endsWith("/login"))
}

test.describe("modo oscuro", () => {
  test.skip(!ADMIN_EMAIL || !ADMIN_PASSWORD, "faltan credenciales de Admin")

  test("respeta la preferencia del sistema desde la primera pantalla", async ({
    browser,
  }) => {
    // Con el navegador en oscuro y sin ninguna elección previa, el login ya
    // tiene que salir oscuro: es la primera pantalla que ve cualquiera.
    const contexto = await browser.newContext({ colorScheme: "dark" })
    const page = await contexto.newPage()
    await page.goto("/login")

    await expect(page.locator("html")).toHaveClass(/dark/)
    await contexto.close()
  })

  test("el interruptor cambia el tema y la elección sobrevive a la recarga", async ({
    browser,
  }) => {
    const contexto = await browser.newContext({ colorScheme: "light" })
    const page = await contexto.newPage()
    await login(page)

    await expect(page.locator("html")).not.toHaveClass(/dark/)
    if (CARPETA) await page.screenshot({ path: `${CARPETA}/tema-claro.png` })

    await page.getByRole("button", { name: /cambiar a modo oscuro/i }).click()
    await expect(page.locator("html")).toHaveClass(/dark/)
    if (CARPETA) await page.screenshot({ path: `${CARPETA}/tema-oscuro.png` })

    // Recargar es donde se nota si la elección se guardó de verdad.
    await page.reload()
    await expect(page.locator("html")).toHaveClass(/dark/)
    await contexto.close()
  })

  test("la elección le gana a la preferencia del sistema", async ({ browser }) => {
    const contexto = await browser.newContext({ colorScheme: "dark" })
    const page = await contexto.newPage()
    await login(page)

    await page.getByRole("button", { name: /cambiar a modo claro/i }).click()
    await expect(page.locator("html")).not.toHaveClass(/dark/)

    await page.reload()
    // El sistema sigue en oscuro y aun así queda claro: mandó la persona.
    await expect(page.locator("html")).not.toHaveClass(/dark/)
    await contexto.close()
  })
})
