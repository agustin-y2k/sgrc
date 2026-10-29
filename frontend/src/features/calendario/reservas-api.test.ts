import { cargarCalendario } from "./reservas-api"
import { apiFetch } from "@/lib/api-client"
import type { ReservaCalendario } from "./reservas"
vi.mock("@/lib/api-client")

test("carga todas las páginas aunque cada clase tenga varios equipos", async () => {
  const filas = (prefijo: string) =>
    Array.from(
      { length: 400 },
      (_, i) =>
        ({
          id: `${prefijo}-${i}`,
          reservaGrupoId: `g-${prefijo}-${Math.floor(i / 4)}`,
        }) as ReservaCalendario
    )
  vi.mocked(apiFetch)
    .mockResolvedValueOnce({
      data: filas("a"),
      meta: { total: 200, pageSize: 100, page: 1 },
    })
    .mockResolvedValueOnce({
      data: filas("b"),
      meta: { total: 200, pageSize: 100, page: 2 },
    })
  const datos = await cargarCalendario({
    desde: "2026-10-01",
    hasta: "2026-10-31",
    ambito: "todas",
    busqueda: "",
    incluirCanceladas: false,
  })
  expect(datos).toHaveLength(800)
  expect(apiFetch).toHaveBeenCalledTimes(2)
  expect(vi.mocked(apiFetch).mock.calls[1][0]).toContain("page=2")
})
