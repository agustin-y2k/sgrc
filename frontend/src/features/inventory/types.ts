// Espeja internal/inventory/interfaces/http/dto.go — la fuente de verdad
// es ese archivo (ver la nota en lib/api-client.ts sobre por qué no se
// confía ciegamente en docs/08-api-spec.yaml).

export type EstadoPC = "DISPONIBLE" | "EN_MANTENIMIENTO" | "FUERA_DE_SERVICIO"

export type Carro = {
  id: string
  nombre: string
  descripcion?: string
}

export type PC = {
  id: string
  carroId: string
  identificador: number
  numeroSerie: number
  freezado: boolean
  cpu?: string
  ram?: string
  sistemaOperativo?: string
  softwareInstalado?: string
  estado: EstadoPC
  dadaDeBaja: boolean
  fechaBaja?: string
  fechaAlta: string
}

/**
 * RF-03.5 — una falla reportada sobre una PC.
 *
 * Vive acá y no en features/admin porque no es un concepto de
 * administración: el docente que usa la PC es quien reporta ("Docentes solo
 * pueden reportarlas"), y el Admin es quien después la gestiona.
 */
export type GravedadIncidencia = "LEVE" | "MODERADA" | "GRAVE"

/**
 * El backend NO impone una máquina de estados acá (a diferencia de Usuario o
 * PC): valida que el valor sea uno de los cuatro y nada más, así que
 * cualquier estado puede pasar a cualquier otro. La pantalla ofrece el
 * recorrido esperado sin bloquear el resto.
 */
export type EstadoIncidencia = "ABIERTA" | "EN_REPARACION" | "ENVIADA_DGE" | "RESUELTA"

export type Incidencia = {
  id: string
  pcId: string
  reportadoPor?: string
  descripcion: string
  gravedad: GravedadIncidencia
  /** ISO 8601 */
  fecha: string
  enviadoDge: boolean
  fechaEnvioDge?: string
  estado: EstadoIncidencia
}

// Los listados del backend responden { data: [...] }.
export type RespuestaLista<T> = { data: T[] }
