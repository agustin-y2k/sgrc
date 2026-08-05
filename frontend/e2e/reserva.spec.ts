import { test, expect, type Page } from "@playwright/test"

import { DOCENTE_EMAIL, DOCENTE_PASSWORD, etiquetaDeReserva, login } from "./helpers"

/**
 * Flujo E2E completo de docs/10-testing.md: login → reservar →
 * cancelar. Es el camino crítico del sistema y el único que cruza los cuatro
 * paquetes que importan (auth → academic → reservation → notification), así
 * que vale probarlo contra el backend real y no solo con mocks.
 *
 * Requisitos del entorno: `make run` los deja todos listos (backend,
 * frontend en :8081, y un docente aprobado y asignado con una PC
 * disponible). Contra otra base se pasan por entorno, ver e2e/helpers.ts.
 *
 * Correr con:  npx playwright test
 */
test.skip(
  !DOCENTE_EMAIL || !DOCENTE_PASSWORD,
  "faltan E2E_DOCENTE_EMAIL/E2E_DOCENTE_PASSWORD de un docente aprobado y asignado a una materia"
)

/**
 * Fecha y franja horaria propias de cada corrida.
 *
 * Hacen falta porque el test cancela su reserva pero **no puede borrarla**
 * —la API no expone borrado de reservas, y no debería—, así que cada corrida
 * deja una fila CANCELADA. Con fecha y horario fijos, la corrida siguiente
 * encontraba dos tarjetas idénticas al marcar "mostrar también las
 * canceladas" y fallaba por ambigüedad, no por un problema del sistema.
 *
 * Se varían **dos ejes con períodos distintos** en vez de uno solo: la
 * franja tiene 24 valores posibles (la grilla de 5 minutos que ofrece el
 * selector, dentro de una banda poco habitual), así que variando solo eso
 * dos corridas separadas por 24 segundos volvían a chocar. Combinando día y
 * franja, el par se repite recién después de 37 × 24 = 888 segundos, y dos
 * corridas seguidas nunca coinciden.
 */
function huellaDeEstaCorrida(): { fecha: string; inicio: string; fin: string } {
  const ahora = new Date()
  const segundos = ahora.getHours() * 3600 + ahora.getMinutes() * 60 + ahora.getSeconds()

  const dia = new Date()
  dia.setDate(dia.getDate() + 14 + (segundos % 37))
  // La semana lectiva es de lunes a viernes: el backend rechaza el resto.
  while (dia.getDay() === 0 || dia.getDay() === 6) {
    dia.setDate(dia.getDate() + 1)
  }
  const mes = String(dia.getMonth() + 1).padStart(2, "0")
  const nroDia = String(dia.getDate()).padStart(2, "0")

  // Minutos múltiplo de 5: es la grilla del selector (el backend acepta
  // cualquiera, los horarios son libres).
  const desdeLas5 = 5 * 60 + (Math.floor(segundos / 37) % 24) * 5
  const hhmm = (m: number) =>
    `${String(Math.floor(m / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`

  return {
    fecha: `${dia.getFullYear()}-${mes}-${nroDia}`,
    inicio: hhmm(desdeLas5),
    fin: hhmm(desdeLas5 + 30),
  }
}

const { fecha: FECHA, inicio: HORA_INICIO, fin: HORA_FIN } = huellaDeEstaCorrida()

/** Completa un SelectorDeHora: "#<id>-hora" y "#<id>-minutos". */
async function elegirHora(page: Page, id: string, hhmm: string) {
  const [hora, minutos] = hhmm.split(":")
  await page.locator(`#${id}-hora`).selectOption(hora)
  await page.locator(`#${id}-minutos`).selectOption(minutos)
}

test("un docente reserva una PC y después cancela la reserva", async ({ page }) => {
  // E2E_FECHA_RESERVA sigue mandando si se fija por entorno.
  const fecha = process.env.E2E_FECHA_RESERVA ?? FECHA
  const etiqueta = etiquetaDeReserva(fecha, HORA_INICIO, HORA_FIN)

  await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)

  // --- Crear (RF-04.2) ---
  await page.goto("/reservas/nueva")

  const sinMaterias = page.getByText(/no estás asignado a ninguna materia/i)
  if (await sinMaterias.isVisible().catch(() => false)) {
    throw new Error(
      "el docente de E2E_DOCENTE_EMAIL no tiene materias asignadas en un ciclo activo — " +
        "asignale una desde el panel de Admin antes de correr este test"
    )
  }

  await page.locator("#materia").selectOption({ index: 1 })
  await page.locator("#fecha").fill(fecha)
  // La hora se elige en dos listas (hora y minutos), no se escribe: el
  // control nativo mostraba AM/PM según el idioma del navegador.
  await elegirHora(page, "horaInicio", HORA_INICIO)
  await elegirHora(page, "horaFin", HORA_FIN)

  const sinPCs = page.getByText(/no hay ninguna pc libre/i)
  const primeraPC = page.getByRole("checkbox").first()
  await expect(primeraPC.or(sinPCs)).toBeVisible()
  if (await sinPCs.isVisible()) {
    throw new Error(
      `no hay PCs libres el ${fecha} de ${HORA_INICIO} a ${HORA_FIN} — ` +
        "cargá una PC DISPONIBLE o fijá otra fecha con E2E_FECHA_RESERVA"
    )
  }

  await primeraPC.check()
  await page.getByRole("button", { name: /confirmar reserva/i }).click()

  // Al confirmar, NuevaReservaPage navega al listado.
  await expect(page).toHaveURL(/\/reservas$/)

  const tarjeta = page.locator('[data-slot="card"]').filter({ hasText: etiqueta })
  await expect(tarjeta).toBeVisible()
  await expect(tarjeta.getByText("Confirmada")).toBeVisible()

  // --- Cancelar (RF-04.8: reserva propia, motivo opcional) ---
  await tarjeta.getByRole("button", { name: /^cancelar$/i }).click()
  await tarjeta.getByRole("button", { name: /confirmar cancelación/i }).click()

  // El listado por defecto oculta las canceladas, así que desaparece.
  await expect(tarjeta).toHaveCount(0)

  // Y reaparece como CANCELADA al pedir que se muestren.
  await page.getByLabel(/mostrar también las canceladas/i).check()
  await expect(tarjeta).toBeVisible()
  await expect(tarjeta.getByText("Cancelada")).toBeVisible()
})
