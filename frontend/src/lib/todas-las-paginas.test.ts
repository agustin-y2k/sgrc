import { todasLasPaginas } from "@/lib/todas-las-paginas"
import { ApiError } from "@/lib/api-client"

it("recupera también los registros posteriores al límite de 200 de la API", async () => {
  const leer = vi.fn(async (page: number) => ({
    data: Array.from({ length: page === 1 ? 200 : 1 }, (_, i) => (page - 1) * 200 + i),
    meta: { total: 201, page, pageSize: 200 },
  }))
  const resultado = await todasLasPaginas(leer)
  expect(resultado.data).toHaveLength(201)
  expect(resultado.data[200]).toBe(200)
  expect(leer.mock.calls.map(([pagina]) => pagina)).toEqual([1, 2])
})

it("un fallo intermedio no devuelve una lista parcial como si estuviera completa", async () => {
  const leer = vi
    .fn()
    .mockResolvedValueOnce({ data: [1], meta: { total: 2, page: 1, pageSize: 1 } })
    .mockRejectedValueOnce(new ApiError(500, "consulta fallida"))
  await expect(todasLasPaginas(leer)).rejects.toThrow("consulta fallida")
})
