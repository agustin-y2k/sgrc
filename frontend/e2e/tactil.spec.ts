import { test, expect, devices } from "@playwright/test"

import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

/**
 * En un teléfono, todo lo que se toca tiene que ser tocable (RF-04.13).
 *
 * Existe por un caso concreto: los botones "Cambiar una computadora" y
 * "Cancelar esta clase" de la pantalla de inicio salieron con el tamaño `sm`
 * del sistema de diseño, que son 28px de alto — la mitad del ancho de un
 * dedo. En una captura se ven perfectos; el problema aparece recién con el
 * teléfono en la mano, y justo en la pantalla pensada para quien no maneja
 * bien un teléfono.
 *
 * Es el mismo tipo de defecto que persigue `responsive.spec.ts`: invisible
 * mirando, evidente usando, y vuelve solo cada vez que alguien agrega un
 * control con el tamaño por defecto.
 *
 * **Se mide la pantalla entera**: el cuerpo, la barra —incluido el menú
 * desplegado, que es el otro camino a todo— y el pie. La barra tiene su propia
 * tensión: en escritorio no puede engordar porque desborda a lo ancho (ver
 * `responsive.spec.ts`), así que sus controles crecen solo en el teléfono. Un
 * test que midiera únicamente el cuerpo dejaría fuera justo el control que
 * abre todo lo demás.
 */

/** WCAG 2.5.5 (AAA): el mínimo cómodo para un dedo. */
const MINIMO = 44

/**
 * WCAG 2.5.8 (AA): el piso para un enlace embebido en una frase, que no
 * puede crecer a 44px sin partir el renglón que lo contiene.
 */
const MINIMO_EN_TEXTO = 24

test.use({ ...devices["Pixel 7"] })

/** Mide todo lo cliqueable que está a la vista y devuelve lo que no llega. */
async function controlesChicos(page: import("@playwright/test").Page) {
  return page.evaluate(
    ({ minimo, minimoEnTexto }) => {
      const malos: string[] = []
      for (const el of document.querySelectorAll("a, button")) {
        const caja = el.getBoundingClientRect()
        // Lo que no se ve no se toca: un panel plegado no es un defecto.
        if (caja.width === 0 && caja.height === 0) continue

        // Un enlace dentro de un párrafo es parte de una frase, no un botón
        // disfrazado, y se lo mide con el piso de AA.
        const enTexto = el.tagName === "A" && el.closest("p") !== null
        const piso = enTexto ? minimoEnTexto : minimo

        if (caja.height < piso) {
          const zona = el.closest("header")
            ? "barra"
            : el.closest("footer")
              ? "pie"
              : "cuerpo"
          const texto = (el.textContent ?? "").trim().slice(0, 40) || "(sin texto)"
          malos.push(
            `[${zona}] "${texto}" mide ${Math.round(caja.height)}px y necesita ${piso}px`
          )
        }
      }
      return malos
    },
    { minimo: MINIMO, minimoEnTexto: MINIMO_EN_TEXTO }
  )
}

test("en un teléfono, los controles del docente se pueden tocar", async ({ page }) => {
  await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
  await page.getByRole("heading", { level: 1 }).waitFor()
  await page.waitForLoadState("networkidle")

  const chicos = await controlesChicos(page)

  expect(
    chicos,
    `controles demasiado chicos para un dedo:\n${chicos.join("\n")}`
  ).toEqual([])
})

/**
 * El menú desplegado se mide aparte porque no existe hasta que alguien lo
 * abre: con el menú cerrado, sus enlaces no están en el DOM y el test de
 * arriba no puede verlos. Y son el camino a todas las pantallas del sistema
 * desde un teléfono.
 */
test("en un teléfono, los enlaces del menú se pueden tocar", async ({ page }) => {
  await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
  await page.getByRole("heading", { level: 1 }).waitFor()
  await page.getByRole("button", { name: "Menú" }).click()
  await page
    .getByRole("navigation", { name: /principal/i })
    .or(page.locator("#menu-principal"))
    .first()
    .waitFor()

  const chicos = await controlesChicos(page)

  expect(
    chicos,
    `controles del menú demasiado chicos para un dedo:\n${chicos.join("\n")}`
  ).toEqual([])
})

/**
 * Las dos acciones que se resuelven sin salir del inicio (RF-04.13) tienen
 * que estar al alcance en un teléfono, no solo existir en el DOM: si el menú
 * o una tarjeta las tapara, el docente no las encontraría.
 */
test("las acciones de una clase están a la vista en un teléfono", async ({ page }) => {
  await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
  await page.getByRole("heading", { level: 1 }).waitFor()
  await page.waitForLoadState("networkidle")

  const sinReservas = await page
    .getByText(/No tenés ninguna clase con computadoras reservadas/)
    .isVisible()
    .catch(() => false)
  test.skip(sinReservas, "el docente de prueba no tiene reservas próximas")

  await expect(
    page.getByRole("button", { name: "Cambiar una computadora" }).first()
  ).toBeVisible()
  await expect(
    page.getByRole("button", { name: "Cancelar esta clase" }).first()
  ).toBeVisible()
})
