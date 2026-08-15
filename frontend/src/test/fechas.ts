/**
 * Fechas relativas a hoy para los tests de formularios.
 *
 * Los inputs de fecha de reserva y de bloqueo administrativo tienen
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
 * Una fecha dentro de `dias` días.
 *
 * Antes esto saltaba al lunes cuando caía en fin de semana, porque los
 * formularios deshabilitaban el botón esos días y el test fallaba o no
 * según el día en que se corriera. Ya no hace falta: el fin de semana es
 * reservable como cualquier otro día, y saltearlo escondería justamente el
 * caso que ahora interesa que funcione.
 */
export function fechaFuturaEnDias(dias: number): string {
  const d = new Date()
  d.setDate(d.getDate() + dias)
  return aISO(d)
}
