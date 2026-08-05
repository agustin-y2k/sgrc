import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { formatearPorcentaje, proporcion } from "@/features/admin/types"

export function Seccion({
  titulo,
  resumen,
  alDescargar,
  children,
}: {
  titulo: string
  /** Una línea con los totales: el contexto que le falta a cada fila. */
  resumen?: React.ReactNode
  /** Si viene, la sección ofrece bajarse a CSV. */
  alDescargar?: () => void
  children: React.ReactNode
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-2 space-y-0">
        <div className="grid gap-1">
          <CardTitle>{titulo}</CardTitle>
          {resumen && <p className="text-muted-foreground text-sm">{resumen}</p>}
        </div>
        {alDescargar && (
          <Button variant="outline" size="sm" onClick={alDescargar}>
            Descargar CSV
          </Button>
        )}
      </CardHeader>
      {/* Cada tabla scrollea sola en pantallas angostas (RNF-07). */}
      <CardContent className="overflow-x-auto">{children}</CardContent>
    </Card>
  )
}

/**
 * La participación de una fila en el total, como barra y como número.
 *
 * En columna y no en un gráfico aparte: es la misma comparación que la
 * tabla ya hace, y así se lee de un vistazo sin perder los valores exactos.
 * Un div con ancho en porcentaje alcanza — no justifica una librería.
 */
export function Proporcion({ parte, total }: { parte: number; total: number }) {
  const pct = proporcion(parte, total)
  return (
    <div className="flex items-center gap-2">
      <div
        className="bg-muted h-1.5 w-16 shrink-0 overflow-hidden rounded-full sm:w-24"
        // La barra es decorativa: repite el número que está al lado, y
        // anunciarla dos veces solo estorba a quien usa lector de pantalla.
        aria-hidden="true"
      >
        <div className="bg-primary h-full rounded-full" style={{ width: `${pct}%` }} />
      </div>
      <span className="text-muted-foreground w-12 shrink-0 text-right text-xs tabular-nums">
        {formatearPorcentaje(pct)}
      </span>
    </div>
  )
}

/** Suma una columna de una lista. */
export function sumar<T>(filas: T[], valor: (fila: T) => number): number {
  return filas.reduce((acc, fila) => acc + valor(fila), 0)
}

// RF-06: reportes de uso e incidencias. El uso depende del ciclo lectivo;
// las incidencias no, porque Incidencia sobrevive al archivado (RF-02.4).
