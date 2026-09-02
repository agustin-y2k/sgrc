import type { ReactNode } from "react"

/**
 * El encabezado de una pantalla: qué es, para qué sirve, y la acción
 * principal si la tiene.
 *
 * `accion` es para UN CONTROL CHICO —un botón, a lo sumo dos—, nunca para un
 * panel o un formulario. El slot es `shrink-0` a propósito, para que un botón
 * como "Nueva reserva" no se parta en dos renglones; el precio es que lo que
 * se ponga ahí no encoge, y si es ancho aplasta el título y la descripción
 * —que sí encogen— hasta dejarlos en una palabra por renglón.
 *
 * Pasó con los formularios de alta de Licencias y de Usuarios, que eran un
 * botón cuando estaban cerrados y una tarjeta entera cuando se abrían. El
 * arreglo es el patrón que usa EntregasPage: la página guarda si el panel está
 * abierto, le pasa el botón a `accion` mientras está cerrado, y dibuja el
 * panel debajo del encabezado, a lo ancho.
 */
export function EncabezadoDePagina({
  titulo,
  descripcion,
  accion,
}: {
  titulo: string
  descripcion?: ReactNode
  /** La acción principal de la pantalla, si hay una sola clara. */
  accion?: ReactNode
}) {
  return (
    <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0">
        <h1 className="text-2xl font-semibold tracking-tight text-balance">{titulo}</h1>
        {descripcion && (
          <p className="text-muted-foreground mt-1 max-w-prose text-sm">{descripcion}</p>
        )}
      </div>
      {accion && <div className="flex shrink-0 flex-wrap gap-2">{accion}</div>}
    </div>
  )
}
