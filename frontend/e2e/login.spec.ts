import { test, expect, type Page } from "@playwright/test"

// Smoke test del flujo de login contra el backend real. Las credenciales
// del Admin salen del `.env` del proyecto (ver playwright.config.ts), que es
// el mismo que usa el backend para sembrarlo (RF-01.4), así que con
// `make run` no hay nada que configurar.
const ADMIN_EMAIL = process.env.E2E_ADMIN_EMAIL ?? ""
const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? ""

test("un Admin puede loguearse", async ({ page }) => {
  // El skip va por test y no por archivo: el de credenciales inválidas no
  // necesita ninguna cuenta, y saltearlo por una configuración que no usa
  // era perder cobertura sin razón.
  test.skip(
    !ADMIN_EMAIL || !ADMIN_PASSWORD,
    "no se encontraron SEED_ADMIN_* en el .env ni E2E_ADMIN_* en el entorno"
  )

  await page.goto("/login")

  await page.getByLabel(/email/i).fill(ADMIN_EMAIL)
  await page.getByLabel(/contraseña/i).fill(ADMIN_PASSWORD)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()

  await expect(page).not.toHaveURL(/\/login$/)
})

test("credenciales inválidas muestran un error", async ({ page }) => {
  await page.goto("/login")

  await page.getByLabel(/email/i).fill("nadie@ejemplo.com")
  await page.getByLabel(/contraseña/i).fill("password-incorrecta")
  await page.getByRole("button", { name: /iniciar sesión/i }).click()

  await expect(page.getByText(/credenciales inválidas/i)).toBeVisible()
})

// La casilla "Mantener la sesión iniciada" (RF-01.13) no cambia nada visible
// después de entrar: lo único que cambia es el `exp` del token. Por eso el
// test lo lee de localStorage y lo decodifica, en vez de mirar la pantalla.
function vencimientoDelToken(payloadB64: string): number {
  const json = JSON.parse(
    Buffer.from(payloadB64.split(".")[1], "base64").toString("utf8")
  ) as { exp: number }
  return json.exp * 1000
}

test.describe("duración de la sesión", () => {
  test.skip(
    !ADMIN_EMAIL || !ADMIN_PASSWORD,
    "no se encontraron SEED_ADMIN_* en el .env ni E2E_ADMIN_* en el entorno"
  )

  async function entrar(page: Page, conLaCasilla: boolean) {
    await page.goto("/login")
    await page.getByLabel(/email/i).fill(ADMIN_EMAIL)
    await page.getByLabel(/contraseña/i).fill(ADMIN_PASSWORD)
    if (conLaCasilla) {
      // Exacto y no /…/i: en la pantalla hay DOS casillas de mantener la
      // sesión —esta y la del botón de Google, que se llama "… con Google"—
      // y un match parcial las agarra a las dos.
      await page.getByLabel("Mantener la sesión iniciada", { exact: true }).click()
    }
    await page.getByRole("button", { name: /iniciar sesión/i }).click()
    await expect(page).not.toHaveURL(/\/login$/)

    const token = await page.evaluate(() => localStorage.getItem("sgrc_token"))
    expect(token).toBeTruthy()
    return vencimientoDelToken(token as string)
  }

  test("sin la casilla, la sesión es la normal", async ({ page }) => {
    const vence = await entrar(page, false)

    // Los umbrales son holgados a propósito: el test afirma "corta" y "larga",
    // no los valores exactos, que el despliegue configura por .env.
    expect(vence - Date.now()).toBeLessThan(48 * 60 * 60 * 1000)
  })

  test("con la casilla tildada, la sesión dura mucho más", async ({ page }) => {
    const vence = await entrar(page, true)

    expect(vence - Date.now()).toBeGreaterThan(7 * 24 * 60 * 60 * 1000)
  })
})
