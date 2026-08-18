import { expect, type APIRequestContext, type Page } from "@playwright/test"

/**
 * Credenciales de un docente ya aprobado y asignado a al menos una materia
 * de un ciclo activo (RF-04.1).
 *
 * Por defecto son las del docente que siembra el entorno de desarrollo
 * (`scripts/sembrar-datos-de-prueba.sh`, que corre solo con `make run`), así
 * que los E2E andan sin configurar nada contra un `make run`. Contra otra
 * base se pasan por entorno.
 */
export const DOCENTE_EMAIL = process.env.E2E_DOCENTE_EMAIL ?? "docente@escuela.edu.ar"
export const DOCENTE_PASSWORD = process.env.E2E_DOCENTE_PASSWORD ?? "docente_password_123"

/** Loguea y espera a salir de /login. */
export async function login(page: Page, email: string, password: string) {
  await page.goto("/login")
  await page.getByLabel(/email/i).fill(email)
  await page.getByLabel(/contraseña/i).fill(password)
  await page.getByRole("button", { name: /iniciar sesión/i }).click()
  await expect(page).not.toHaveURL(/\/login$/)
}

/**
 * La tarjeta de una reserva en /reservas, ubicada por el texto que la
 * encabeza: "Lunes, 9 de marzo 08:00–09:00" (ojo: el guion es un en dash).
 *
 * La fecha se arma acá con el mismo Intl que usa la página (`src/lib/fechas`)
 * en vez de repetir el formato a mano: si mañana se decide mostrar el año o
 * abreviar el mes, este helper acompaña el cambio solo. Antes buscaba la
 * fecha en ISO, que es lo que viaja por la API pero no lo que se ve.
 */
const FECHA_LARGA = new Intl.DateTimeFormat("es-AR", {
  weekday: "long",
  day: "numeric",
  month: "long",
})

export function etiquetaDeReserva(fecha: string, horaInicio: string, horaFin: string) {
  const [anio, mes, dia] = fecha.split("-").map(Number)
  const texto = FECHA_LARGA.format(new Date(anio, mes - 1, dia))
  return `${texto.charAt(0).toUpperCase() + texto.slice(1)} ${horaInicio}–${horaFin}`
}

/** Un tramo de la jornada institucional, tal como lo devuelve GET /api/jornada. */
export type TramoDeJornada = {
  diaSemana: string
  horaInicio: string
  horaFin: string
}

/**
 * Los tramos en que la institución declaró estar abierta (RF-07.5).
 *
 * Los tests que reservan lo necesitan porque el backend rechaza todo lo que
 * caiga afuera: una franja elegida a ciegas funciona contra una instalación
 * recién sembrada —donde la jornada está vacía y por eso no hay restricción—
 * y falla contra cualquier escuela que ya haya declarado su horario. Que un
 * test del flujo crítico dependa de que nadie haya configurado el sistema es
 * exactamente al revés de lo que tiene que pasar.
 *
 * Se pide por la API y no por la pantalla porque es un dato del entorno, no
 * lo que este test ejercita. Si la consulta falla se devuelve vacío, que es
 * el caso "sin restricción" y deja el comportamiento de siempre.
 */
export async function jornadaDeclarada(
  request: APIRequestContext
): Promise<TramoDeJornada[]> {
  const ingreso = await request.post("/api/auth/login", {
    data: { email: DOCENTE_EMAIL, password: DOCENTE_PASSWORD },
  })
  if (!ingreso.ok()) return []
  const { token } = await ingreso.json()

  const respuesta = await request.get("/api/jornada", {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!respuesta.ok()) return []
  return (await respuesta.json()).data ?? []
}
