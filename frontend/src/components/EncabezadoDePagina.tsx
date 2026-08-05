import type { ReactNode } from "react"

/**
 * El encabezado de una pantalla: qué es, para qué sirve, y la acción
 * principal si la tiene.
 *
 * Existe como componente y no como un `<h1>` suelto en cada página porque
 * cada una lo resolvía distinto —algunas con descripción y otras no, con
 * separaciones diferentes, con el botón antes o después del título— y eso
 * hace que moverse entre pantallas se sienta como usar dos aplicaciones.
 *
 * En un teléfono el título y la acción se apilan: puestos en la misma línea,
 * el botón quedaba espachurrado contra el borde.
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
