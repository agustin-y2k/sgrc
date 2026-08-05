import { Button } from "@/components/ui/button"

/**
 * Espeja `paginacion.Meta` del backend (internal/shared/paginacion): el
 * total es de la colección completa, no de la página que se está viendo.
 */
export type PaginacionMeta = {
  total: number
  page: number
  pageSize: number
}

/**
 * Controles de página para los tres listados que el backend pagina
 * —reservas, notificaciones y usuarios—. Sin esto la pantalla mostraría
 * las primeras 50 filas y las demás desaparecerían sin que nada lo diga,
 * que es peor que el listado sin cota que la paginación vino a arreglar.
 *
 * No se muestra si todo entra en una sola página: en una escuela ese va a
 * ser el caso normal y no tiene sentido ocupar espacio con dos botones que
 * nunca se van a poder tocar.
 */
export function Paginador({
  meta,
  onCambiarPagina,
  etiqueta,
}: {
  meta: PaginacionMeta
  onCambiarPagina: (pagina: number) => void
  /** Cómo se llama lo que se está listando, en plural ("reservas"). */
  etiqueta: string
}) {
  const totalPaginas = Math.max(1, Math.ceil(meta.total / meta.pageSize))
  if (totalPaginas <= 1) return null

  const desde = (meta.page - 1) * meta.pageSize + 1
  const hasta = Math.min(meta.page * meta.pageSize, meta.total)

  return (
    <div className="flex items-center justify-between gap-4 pt-2">
      <p className="text-muted-foreground text-sm">
        {desde}–{hasta} de {meta.total} {etiqueta}
      </p>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={meta.page <= 1}
          onClick={() => onCambiarPagina(meta.page - 1)}
        >
          Anterior
        </Button>
        <span className="text-muted-foreground text-sm">
          Página {meta.page} de {totalPaginas}
        </span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={meta.page >= totalPaginas}
          onClick={() => onCambiarPagina(meta.page + 1)}
        >
          Siguiente
        </Button>
      </div>
    </div>
  )
}
