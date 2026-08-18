import { test, expect, type Page } from "@playwright/test"

import {
  DOCENTE_EMAIL,
  DOCENTE_PASSWORD,
  etiquetaDeReserva,
  jornadaDeclarada,
  login,
  type TramoDeJornada,
} from "./helpers"

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
 * Fecha y franja horaria propias de cada corrida, dentro de lo que la
 * escuela declaró abierto.
 *
 * Son propias de cada corrida porque el test cancela su reserva pero **no
 * puede borrarla** —la API no expone borrado de reservas, y no debería—, así
 * que cada corrida deja una fila CANCELADA. Con fecha y horario fijos, la
 * corrida siguiente encontraba dos tarjetas idénticas al marcar "mostrar
 * también las canceladas" y fallaba por ambigüedad, no por un problema del
 * sistema.
 *
 * Y van dentro de la jornada porque el backend rechaza lo de afuera. Antes
 * esto elegía siempre una franja entre las 05:00 y las 07:00 —una banda poco
 * habitual, justamente para no chocar con reservas de verdad—, lo que
 * funciona mientras la jornada esté sin declarar (el estado de un entorno
 * recién sembrado, en el que no hay restricción horaria) y falla apenas
 * alguien carga el horario real de la escuela: ninguna abre a las cinco de
 * la mañana. El flujo crítico dejaba de probarse justo en la instalación más
 * parecida a la de producción.
 *
 * Se siguen variando **dos ejes con períodos distintos** en vez de uno solo:
 * el día dentro de la ventana de candidatos y el arranque dentro del tramo.
 * Con un solo eje, dos corridas separadas por pocos segundos volvían a
 * chocar; combinándolos, el par tarda en repetirse mucho más que cualquier
 * par de corridas seguidas.
 */

/** Los días como los nombra la API, en el orden de Date.getDay(). */
const DIA_DE_LA_API = [
  "DOMINGO",
  "LUNES",
  "MARTES",
  "MIERCOLES",
  "JUEVES",
  "VIERNES",
  "SABADO",
]

/** Lo que dura la reserva del test, y la grilla que ofrece el selector. */
const DURACION_MINUTOS = 30
const PASO_MINUTOS = 5

function aMinutos(hhmm: string): number {
  const [hora, minutos] = hhmm.split(":").map(Number)
  return hora * 60 + minutos
}

function aHHMM(minutos: number): string {
  const h = String(Math.floor(minutos / 60)).padStart(2, "0")
  const m = String(minutos % 60).padStart(2, "0")
  return `${h}:${m}`
}

/**
 * Los arranques posibles de una reserva de 30 minutos dentro de un tramo,
 * sobre la grilla de 5 del selector.
 *
 * Un tramo que cierra al día siguiente (una escuela nocturna, 20:00–01:00)
 * se aprovecha solo hasta la medianoche: reservar cruzándola es un caso
 * legítimo y tiene sus propios tests, pero no es lo que este ejercita.
 */
function arranquesPosibles(tramo: TramoDeJornada): number[] {
  const inicio = aMinutos(tramo.horaInicio)
  const finDeclarado = aMinutos(tramo.horaFin)
  const fin = finDeclarado > inicio ? finDeclarado : 24 * 60

  const arranques: number[] = []
  const primero = Math.ceil(inicio / PASO_MINUTOS) * PASO_MINUTOS
  for (let m = primero; m + DURACION_MINUTOS <= fin; m += PASO_MINUTOS) {
    arranques.push(m)
  }
  return arranques
}

/**
 * Sin jornada declarada no hay restricción horaria, así que se conserva la
 * banda de siempre: entre las 05:00 y las 07:00, poco habitual a propósito
 * para no pisarse con reservas reales del entorno.
 */
function bandaSinJornada(): number[] {
  return Array.from({ length: 24 }, (_, i) => 5 * 60 + i * PASO_MINUTOS)
}

function huellaDeEstaCorrida(jornada: TramoDeJornada[]): {
  fecha: string
  inicio: string
  fin: string
} {
  const ahora = new Date()
  const segundos = ahora.getHours() * 3600 + ahora.getMinutes() * 60 + ahora.getSeconds()

  // Los días de la ventana en los que se puede reservar. Con jornada, los
  // que tengan algún tramo donde entren 30 minutos; sin jornada, los de
  // lunes a viernes, que es lo que asumía la versión anterior.
  const candidatos: { dia: Date; arranques: number[] }[] = []
  for (let corrimiento = 14; corrimiento <= 50; corrimiento++) {
    const dia = new Date()
    dia.setDate(dia.getDate() + corrimiento)

    if (jornada.length === 0) {
      if (dia.getDay() === 0 || dia.getDay() === 6) continue
      candidatos.push({ dia, arranques: bandaSinJornada() })
      continue
    }

    const arranques = jornada
      .filter((tramo) => tramo.diaSemana === DIA_DE_LA_API[dia.getDay()])
      .flatMap(arranquesPosibles)
    if (arranques.length > 0) candidatos.push({ dia, arranques })
  }

  if (candidatos.length === 0) {
    throw new Error(
      "la jornada declarada no deja ningún tramo de 30 minutos en las próximas semanas — " +
        "revisá el horario de la escuela en /admin/jornada, o fijá la fecha con E2E_FECHA_RESERVA"
    )
  }

  const elegido = candidatos[segundos % candidatos.length]
  const inicio = elegido.arranques[Math.floor(segundos / 37) % elegido.arranques.length]

  const mes = String(elegido.dia.getMonth() + 1).padStart(2, "0")
  const nroDia = String(elegido.dia.getDate()).padStart(2, "0")

  return {
    fecha: `${elegido.dia.getFullYear()}-${mes}-${nroDia}`,
    inicio: aHHMM(inicio),
    fin: aHHMM(inicio + DURACION_MINUTOS),
  }
}

/** Completa un SelectorDeHora: "#<id>-hora" y "#<id>-minutos". */
async function elegirHora(page: Page, id: string, hhmm: string) {
  const [hora, minutos] = hhmm.split(":")
  await page.locator(`#${id}-hora`).selectOption(hora)
  await page.locator(`#${id}-minutos`).selectOption(minutos)
}

test("un docente reserva una PC y después cancela la reserva", async ({
  page,
  request,
}) => {
  const huella = huellaDeEstaCorrida(await jornadaDeclarada(request))
  // E2E_FECHA_RESERVA sigue mandando si se fija por entorno.
  const fecha = process.env.E2E_FECHA_RESERVA ?? huella.fecha
  const HORA_INICIO = huella.inicio
  const HORA_FIN = huella.fin
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
