import { archivarCiclo } from "./api"

describe("cierre anual: datos enviados a la API", () => {
  const fetchMock = vi.fn()
  beforeEach(() => {
    fetchMock.mockReset()
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ archivado: true }), {
        headers: { "content-type": "application/json" },
      })
    )
    vi.stubGlobal("fetch", fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  it.each([NaN, Infinity, -Infinity, 1999, 2101, 2027.5])(
    "rechaza %s antes de enviar la petición",
    async (anio) => {
      await expect(archivarCiclo("ciclo", anio)).rejects.toThrow(
        /entero entre 2000 y 2100/
      )
      expect(fetchMock).not.toHaveBeenCalled()
    }
  )
  it.each([2000, 2027, 2100])("conserva el destino válido %s", async (anio) => {
    await archivarCiclo("ciclo", anio)
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ clonarA: anio })
  })
  it("cierra sin clonar cuando se solicita expresamente sin destino", async () => {
    await archivarCiclo("ciclo")
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({})
  })
})
