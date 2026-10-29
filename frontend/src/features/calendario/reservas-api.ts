import { apiFetch } from "@/lib/api-client"
import type { PaginacionMeta } from "@/components/Paginador"
import type { ReservaCalendario } from "./reservas"

export type FiltrosCalendario = {
  desde: string
  hasta: string
  ambito: "mias" | "todas"
  busqueda: string
  incluirCanceladas: boolean
}

export async function cargarCalendario(filtros: FiltrosCalendario) {
  const leer = (page: number) => {
    const params = new URLSearchParams({
      desde: filtros.desde,
      hasta: filtros.hasta,
      ambito: filtros.ambito,
      page: String(page),
      pageSize: "100",
    })
    if (filtros.busqueda) params.set("busqueda", filtros.busqueda)
    if (filtros.incluirCanceladas) params.set("incluirCanceladas", "true")
    return apiFetch<{ data: ReservaCalendario[]; meta: PaginacionMeta }>(
      `/api/calendario/reservas?${params}`
    )
  }
  const primera = await leer(1)
  const datos = [...primera.data]
  // meta.total cuenta clases, no equipos: no se compara con datos.length.
  const paginas = Math.ceil(primera.meta.total / primera.meta.pageSize)
  for (let pagina = 2; pagina <= paginas; pagina++) {
    const siguiente = await leer(pagina)
    datos.push(...siguiente.data)
  }
  return [...new Map(datos.map((r) => [r.id, r])).values()]
}
