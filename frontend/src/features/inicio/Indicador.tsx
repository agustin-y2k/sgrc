import { Link } from "react-router"

/**
 * Un número grande con su rótulo, que además es un enlace.
 *
 * Vive en su propio archivo porque lo comparten la tira de estado de la
 * portada y «En el laboratorio ahora», que antes era una tarjeta aparte con
 * su propio tamaño de letra. Eran el mismo objeto dibujado de dos formas y
 * separados por mil ochocientos píxeles de página.
 */
export function Indicador({
  valor,
  sufijo,
  rotulo,
  detalle,
  a,
  destacado,
}: {
  /** null cuando la consulta que lo alimenta falló. */
  valor: number | null
  /** «de 90»: el total contra el que se lee el número, en chico y al lado. */
  sufijo?: string
  rotulo: string
  detalle: string
  a: string
  /** Pide atención: hay algo pendiente de hacer. */
  destacado?: boolean
}) {
  return (
    <Link
      to={a}
      className={[
        "focus-visible:ring-ring rounded-xl border p-4 transition-colors focus-visible:ring-2 focus-visible:outline-none",
        destacado
          ? "border-alerta/40 bg-alerta/10 hover:bg-alerta/20"
          : "bg-superficie hover:bg-muted",
      ].join(" ")}
    >
      <p className="text-3xl font-semibold tabular-nums">
        {valor ?? <span className="text-muted-foreground">—</span>}
        {/* El sufijo no compite con el número: es la referencia, no el dato. */}
        {sufijo && valor !== null && (
          <span className="text-muted-foreground text-lg font-normal">{` ${sufijo}`}</span>
        )}
      </p>
      <p className="mt-0.5 text-sm font-medium">{rotulo}</p>
      <p className="text-muted-foreground text-xs">{detalle}</p>
    </Link>
  )
}
