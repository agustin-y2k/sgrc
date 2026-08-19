import type { PaginacionMeta } from "@/components/Paginador"

/**
 * Envuelve una lista como la devuelve un endpoint paginado, con el `meta` que
 * agrega el backend (internal/shared/paginacion).
 */
export function paginada<T>(
  data: T[],
  meta?: Partial<PaginacionMeta>
): { data: T[]; meta: PaginacionMeta } {
  return {
    data,
    meta: {
      total: meta?.total ?? data.length,
      page: meta?.page ?? 1,
      pageSize: meta?.pageSize ?? 50,
    },
  }
}
