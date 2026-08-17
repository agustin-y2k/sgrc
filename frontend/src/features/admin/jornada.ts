import { DIAS_SEMANA, etiquetaDia } from "@/features/disponibilidad/types"
import type { BloqueHorario, DiaSemana } from "@/features/disponibilidad/types"

/**
 * Presentación de la jornada institucional: cómo se agrupa para leerla y
 * cómo se nombran los días.
 *
 * Está separado de la pantalla porque es la parte que tiene reglas —qué es
 * "lunes a viernes", cuándo un conjunto de días se dice como un rango— y esa
 * es la parte que conviene poder probar sin renderizar nada.
 *
 * El backend guarda un bloque por día: "lunes a viernes de 7:30 a 12:30" son
 * cinco filas. Eso está bien como modelo —cada día puede moverse solo— pero
 * es una forma pésima de leerlo y de cargarlo. Acá se hace el camino de
 * vuelta: cinco bloques con el mismo horario se muestran como una sola línea.
 */

const ORDEN_SEMANA: DiaSemana[] = DIAS_SEMANA.map((d) => d.valor)

function indiceDeDia(dia: DiaSemana): number {
  return ORDEN_SEMANA.indexOf(dia)
}

/** Los días como transcurre la semana, sin repetidos. */
export function ordenarDias(dias: DiaSemana[]): DiaSemana[] {
  return ORDEN_SEMANA.filter((d) => dias.includes(d))
}

/**
 * Un mismo horario aplicado a varios días, con los bloques reales que lo
 * componen: uno por día, cada uno con su ID, porque es lo que hay que
 * mandarle al backend para editarlo o borrarlo.
 */
export type TramoAgrupado = {
  horaInicio: string
  horaFin: string
  /** En orden de semana. */
  dias: DiaSemana[]
  /** Uno por día, en el mismo orden que `dias`. */
  bloques: BloqueHorario[]
}

/**
 * Junta los bloques que comparten horario, sin importar el día.
 *
 * No fusiona horarios distintos aunque se toquen: 07:00–12:00 y 12:00–18:00
 * del mismo día se muestran como dos líneas porque son dos cosas que alguien
 * cargó por separado y va a querer tocar por separado. (El backend sí los
 * fusiona para decidir si una reserva entra — ver `dentroDeLaJornada` —, pero
 * eso es la regla, no la pantalla.)
 *
 * El orden es el de lectura: primero por el día más temprano del grupo, y
 * dentro del mismo día por hora de apertura.
 */
export function agruparTramos(bloques: BloqueHorario[]): TramoAgrupado[] {
  const porHorario = new Map<string, BloqueHorario[]>()
  for (const b of bloques) {
    const clave = `${b.horaInicio}-${b.horaFin}`
    porHorario.set(clave, [...(porHorario.get(clave) ?? []), b])
  }

  const grupos: TramoAgrupado[] = []
  for (const delMismoHorario of porHorario.values()) {
    const ordenados = [...delMismoHorario].sort(
      (a, b) => indiceDeDia(a.diaSemana) - indiceDeDia(b.diaSemana)
    )
    grupos.push({
      horaInicio: ordenados[0].horaInicio,
      horaFin: ordenados[0].horaFin,
      dias: ordenados.map((b) => b.diaSemana),
      bloques: ordenados,
    })
  }

  return grupos.sort((a, b) => {
    const dia = indiceDeDia(a.dias[0]) - indiceDeDia(b.dias[0])
    return dia !== 0 ? dia : a.horaInicio.localeCompare(b.horaInicio)
  })
}

/**
 * Cómo se dice un conjunto de días: "Todos los días", "Lunes a viernes",
 * "Lunes, miércoles y viernes".
 *
 * El rango ("de X a Y") se usa solo desde tres días seguidos. Con dos, "lunes
 * a martes" es más largo y más raro que "lunes y martes".
 */
export function etiquetaDeDias(dias: DiaSemana[]): string {
  const ordenados = ordenarDias(dias)
  if (ordenados.length === 0) return ""
  if (ordenados.length === ORDEN_SEMANA.length) return "Todos los días"

  const primero = indiceDeDia(ordenados[0])
  const ultimo = indiceDeDia(ordenados[ordenados.length - 1])
  const seguidos = ultimo - primero === ordenados.length - 1

  if (seguidos && ordenados.length >= 3) {
    return `${etiquetaDia(ordenados[0])} a ${enMinuscula(ordenados[ordenados.length - 1])}`
  }

  const nombres = [etiquetaDia(ordenados[0]), ...ordenados.slice(1).map(enMinuscula)]
  if (nombres.length === 1) return nombres[0]
  return `${nombres.slice(0, -1).join(", ")} y ${nombres[nombres.length - 1]}`
}

function enMinuscula(dia: DiaSemana): string {
  return etiquetaDia(dia).toLocaleLowerCase("es")
}

const DIAS_HABILES: DiaSemana[] = ["LUNES", "MARTES", "MIERCOLES", "JUEVES", "VIERNES"]
const FIN_DE_SEMANA: DiaSemana[] = ["SABADO", "DOMINGO"]

/**
 * Los tres repartos de la semana que cubren casi todos los casos reales: la
 * escuela de siempre, la de jornada extendida o albergue que dicta los siete
 * días, y el fin de semana con su propio horario.
 *
 * Son atajos y no opciones excluyentes: después de aplicar uno se pueden
 * marcar o desmarcar días sueltos, porque las escuelas que no entran en
 * ninguno de los tres existen y son justamente el motivo por el que la
 * jornada se declara en vez de suponerse.
 */
export const ATAJOS_DE_DIAS: { etiqueta: string; dias: DiaSemana[] }[] = [
  { etiqueta: "Lunes a viernes", dias: DIAS_HABILES },
  { etiqueta: "Todos los días", dias: ORDEN_SEMANA },
  { etiqueta: "Fin de semana", dias: FIN_DE_SEMANA },
]

/** Si el atajo describe exactamente lo que está marcado, para poder resaltarlo. */
export function mismosDias(unos: DiaSemana[], otros: DiaSemana[]): boolean {
  const a = ordenarDias(unos)
  const b = ordenarDias(otros)
  return a.length === b.length && a.every((d, i) => d === b[i])
}
