import type { PaginacionMeta } from "@/components/Paginador"

/**
 * Envuelve una lista como la devuelve un endpoint paginado, con el `meta`
 * que agrega el backend (internal/shared/paginacion).
 *
 * Existe para que un mock de test no tenga que repetir el meta a mano en
 * cada archivo: lo que el test quiere decir es "el endpoint devuelve estas
 * filas", y el resto es contrato.
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
