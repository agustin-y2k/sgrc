import type { PaginacionMeta } from "@/components/Paginador"

/** Para selectores y resúmenes completos; los listados conservan su paginador. */
export async function todasLasPaginas<T>(
  leer: (pagina: number) => Promise<{ data: T[]; meta?: PaginacionMeta }>
): Promise<{ data: T[] }> {
  const primera = await leer(1)
  const data = [...primera.data]
  if (!primera.meta || data.length >= primera.meta.total) return { data }
  const paginas = Math.ceil(primera.meta.total / primera.meta.pageSize)
  for (let pagina = 2; pagina <= paginas; pagina++) {
    const siguiente = await leer(pagina)
    if (siguiente.data.length === 0) break
    data.push(...siguiente.data)
    if (data.length >= primera.meta.total) break
  }
  return { data }
}
