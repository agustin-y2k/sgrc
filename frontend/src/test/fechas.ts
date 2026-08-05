/**
 * Fechas relativas a hoy para los tests de formularios.
 *
 * Los inputs de fecha de reserva y de bloqueo por evaluación tienen
 * `min={hoyISO()}`, porque el backend rechaza un bloque que ya terminó
 * (domain.ErrReservaEnElPasado). jsdom implementa la validación de
 * restricciones, así que un valor por debajo del `min` no bloquea el tipeo
 * pero sí el submit: una fecha constante en el test funciona hasta que esa
 * fecha queda atrás y entonces el test empieza a fallar sin que nadie haya
 * tocado nada.
 */

function aISO(d: Date): string {
  const mes = String(d.getMonth() + 1).padStart(2, "0")
  const dia = String(d.getDate()).padStart(2, "0")
  return `${d.getFullYear()}-${mes}-${dia}`
}

/**
 * Un día lectivo dentro de `dias` días. Si cae sábado o domingo se corre al
 * lunes siguiente: los formularios avisan del fin de semana y deshabilitan
 * el botón (RF-04.2), así que un test que cayera ahí fallaría según el día
 * en que se corriera.
 */
export function diaLectivoEnDias(dias: number): string {
  const d = new Date()
  d.setDate(d.getDate() + dias)
  while (d.getDay() === 0 || d.getDay() === 6) {
    d.setDate(d.getDate() + 1)
  }
  return aISO(d)
}
