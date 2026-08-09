import { Badge } from "@/components/ui/badge"

/**
 * Los estados del dominio, con un color cada uno.
 *
 * Antes salían todos de las tres variantes de shadcn (`default`,
 * `secondary`, `destructive`), así que "en mantenimiento" y "fuera de
 * servicio" se veían igual —las dos en rojo— y "disponible" era el mismo
 * gris que "finalizada". En una grilla de treinta equipos eso obliga a leer
 * cada etiqueta en vez de barrer con la vista.
 *
 * El color nunca va solo: cada badge lleva su texto. Alrededor de un 8% de
 * los varones no distingue rojo de verde, y este sistema lo va a usar toda
 * la escuela.
 */

type Tono = "exito" | "alerta" | "peligro" | "info" | "neutro"

const CLASES: Record<Tono, string> = {
  exito: "bg-exito text-exito-foreground border-transparent",
  alerta: "bg-alerta text-alerta-foreground border-transparent",
  peligro: "bg-destructive text-white border-transparent",
  info: "bg-info text-info-foreground border-transparent",
  neutro: "bg-muted text-muted-foreground border-transparent",
}

export function EstadoBadge({
  tono,
  children,
}: {
  tono: Tono
  children: React.ReactNode
}) {
  return <Badge className={CLASES[tono]}>{children}</Badge>
}

/** RF-03.3 — estado de un equipo. */
export const TONO_PC: Record<string, Tono> = {
  DISPONIBLE: "exito",
  EN_MANTENIMIENTO: "alerta",
  FUERA_DE_SERVICIO: "peligro",
}

/** RF-04 — estado de una reserva o de su grupo. */
export const TONO_RESERVA: Record<string, Tono> = {
  CONFIRMADA: "exito",
  PARCIALMENTE_CANCELADA: "alerta",
  CANCELADA: "peligro",
  // Finalizada no es ni bueno ni malo: ya pasó.
  FINALIZADA: "neutro",
  // NO_RETIRADA en alerta y no en peligro: no es una cancelación ni un
  // error, es que se pasó el plazo. Y la máquina puede seguir disponible.
  NO_RETIRADA: "alerta",
}

/** RF-01/RF-02 — estado de una cuenta. */
export const TONO_CUENTA: Record<string, Tono> = {
  PENDIENTE: "alerta",
  APROBADA: "exito",
  RECHAZADA: "peligro",
  BAJA: "neutro",
}

/** RF-03.5 — gravedad y estado de una incidencia. */
export const TONO_GRAVEDAD: Record<string, Tono> = {
  LEVE: "neutro",
  MODERADA: "alerta",
  GRAVE: "peligro",
}

export const TONO_INCIDENCIA: Record<string, Tono> = {
  ABIERTA: "peligro",
  EN_REPARACION: "alerta",
  ENVIADA_A_SOPORTE: "info",
  RESUELTA: "exito",
}

/**
 * RF-03.11 — estado de una licencia de software.
 *
 * SIN_FECHA va en `info` y no en `neutro` a propósito: no es un estado
 * pasivo del que no haya nada que decir, es una tarea pendiente —hay que ir
 * hasta la máquina a mirar cuántos días le quedan— y en la lista aparece
 * arriba de todo.
 */
export const TONO_LICENCIA: Record<string, Tono> = {
  SIN_FECHA: "info",
  VENCIDA: "peligro",
  POR_VENCER: "alerta",
  VIGENTE: "exito",
}
