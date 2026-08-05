import { test, expect } from "@playwright/test"

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
