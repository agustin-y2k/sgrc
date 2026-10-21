import { Button } from "@/components/ui/button"

/**
 * Espeja `paginacion.Meta` del backend (internal/shared/paginacion): el total
 * es de la colección completa, no de la página que se está viendo.
 */
export type PaginacionMeta = {
  total: number
  page: number
  pageSize: number
}

/**
 * Controles compartidos por los listados paginados de la API.
 */
export function Paginador({
  meta,
  onCambiarPagina,
  etiqueta,
  disabled = false,
}: {
  meta: PaginacionMeta
  onCambiarPagina: (pagina: number) => void
  /** Cómo se llama lo que se está listando, en plural ("reservas"). */
  etiqueta: string
  disabled?: boolean
}) {
  const totalPaginas = Math.max(1, Math.ceil(meta.total / meta.pageSize))
  if (totalPaginas <= 1) return null

  const desde = (meta.page - 1) * meta.pageSize + 1
  const hasta = Math.min(meta.page * meta.pageSize, meta.total)

  return (
    <div className="flex min-w-0 flex-col gap-3 pt-2 sm:flex-row sm:items-center sm:justify-between">
      <p className="text-muted-foreground min-w-0 text-sm wrap-anywhere">
        {desde}–{hasta} de {meta.total} {etiqueta}
      </p>
      <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-11 px-3 sm:h-8"
          disabled={disabled || meta.page <= 1}
          onClick={() => onCambiarPagina(meta.page - 1)}
        >
          Anterior
        </Button>
        <span className="text-muted-foreground min-w-0 text-center text-sm wrap-anywhere">
          Página {meta.page} de {totalPaginas}
        </span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-11 px-3 sm:h-8"
          disabled={disabled || meta.page >= totalPaginas}
          onClick={() => onCambiarPagina(meta.page + 1)}
        >
          Siguiente
        </Button>
      </div>
    </div>
  )
}
