import { test, expect } from "@playwright/test"

/**
 * Ninguna pantalla puede desbordar a lo ancho (RNF-07).
 *
 * Existe por un caso concreto: la barra de navegación con los diez ítems de
 * Admin más el nombre del usuario medía 1190px, así que en un portátil de
 * 1024 —una máquina de escritorio común en una escuela, no un caso raro—
 * toda la página quedaba con scroll horizontal. No se rompía nada visible a
 * primera vista; simplemente el contenido se corría al arrastrar y las
 * columnas de la derecha quedaban fuera de la pantalla.
 *
 * Es un desborde de los que no se notan mirando una captura y sí se notan
 * usando el sistema, y vuelve solo cada vez que se agrega un ítem al menú.
 * Por eso se mide, y se mide en los anchos donde duele.
 */

const ADMIN_EMAIL = process.env.E2E_ADMIN_EMAIL ?? ""
const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? ""

// Un Admin ve el menú completo: es el caso peor y el único que hace falta.
const RUTAS = [
  "/",
  "/reservas",
  "/reservas/nueva",
  "/inventario",
  "/disponibilidad",
  "/notificaciones",
  "/admin/aprobacion",
  "/admin/usuarios",
  "/admin/academico",
  "/admin/inventario",
  "/admin/reportes",
  "/admin/bloquear-equipos",
]

// 320 es el teléfono más angosto que sigue en uso; 1024 y 1180 son los dos
// anchos donde la barra reventaba y los que ningún monitor de desarrollo
// reproduce por su cuenta.
const ANCHOS = [320, 375, 768, 1024, 1180, 1440]

test("ninguna pantalla desborda a lo ancho", async ({ page }) => {
  test.skip(
    !ADMIN_EMAIL || !ADMIN_PASSWORD,
    "no se encontraron SEED_ADMIN_* en el .env ni E2E_ADMIN_* en el entorno"
  )
  test.setTimeout(240_000)

  await page.goto("/login")
  await page.getByLabel(/email/i).fill(ADMIN_EMAIL)
  await page.getByLabel(/contraseña/i).fill(ADMIN_PASSWORD)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()
  await page.waitForURL((u) => !u.pathname.endsWith("/login"))

  const desbordes: string[] = []

  for (const ancho of ANCHOS) {
    await page.setViewportSize({ width: ancho, height: 900 })
    for (const ruta of RUTAS) {
      await page.goto(ruta)
      await page.waitForLoadState("networkidle")

      const info = await page.evaluate(() => {
        const raiz = document.documentElement
        // Se nombra al primer elemento que se sale: sin eso, el fallo dice
        // "hay un desborde" y hay que salir a buscarlo a mano.
        const culpable = [...document.querySelectorAll("body *")].find((el) => {
          const caja = el.getBoundingClientRect()
          return caja.width > 0 && caja.right > raiz.clientWidth + 1
        })
        return {
          documento: raiz.scrollWidth,
          ventana: raiz.clientWidth,
          culpable: culpable
            ? `${culpable.tagName} class="${String(culpable.className).slice(0, 80)}"`
            : "desconocido",
        }
      })

      // El +1 absorbe el redondeo de subpíxeles del navegador.
      if (info.documento > info.ventana + 1) {
        desbordes.push(
          `${ancho}px ${ruta}: ${info.documento} > ${info.ventana} — ${info.culpable}`
        )
      }
    }
  }

  expect(desbordes, `desbordes horizontales:\n${desbordes.join("\n")}`).toEqual([])
})
