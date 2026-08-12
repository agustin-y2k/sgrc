import * as reservasApi from "@/features/reservas/api"
import * as tokenStore from "@/lib/token-store"

vi.mock("@/lib/token-store")

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  })
}

/** Deja el fetch espiado y devuelve la llamada para mirarla. */
function fetchFalso(respuesta = jsonResponse({ data: [] })) {
  const fetchMock = vi.fn().mockResolvedValue(respuesta)
  vi.stubGlobal("fetch", fetchMock)
  return fetchMock
}

function llamada(fetchMock: ReturnType<typeof vi.fn>) {
  const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
  return { url, init }
}

/**
 * Estas dos funciones nacieron con RF-04.12 y lo que se prueba es lo que
 * ningún test de pantalla ve: qué sale por el cable. Un componente que mockea
 * el módulo `api` da por buena cualquier cosa que este arme.
 */
describe("api de reservas, lo que se manda", () => {
  beforeEach(() => {
    vi.spyOn(tokenStore, "getToken").mockReturnValue(null)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  describe("pedirLiberacion", () => {
    /**
     * `apiFetch` ya serializa el body. Pasarle una cadena JSON lo codifica
     * dos veces y al backend le llega una cadena donde espera un objeto, que
     * es un 400 que ningún test de pantalla puede ver.
     */
    it("manda el mensaje como objeto, no como cadena ya serializada", async () => {
      const fetchMock = fetchFalso(new Response(null, { status: 204 }))

      await reservasApi.pedirLiberacion("res1", "Tengo una evaluación")

      const { init } = llamada(fetchMock)
      expect(JSON.parse(init.body as string)).toEqual({ mensaje: "Tengo una evaluación" })
    })

    it("postea contra la reserva que se quiere pedir", async () => {
      const fetchMock = fetchFalso(new Response(null, { status: 204 }))

      await reservasApi.pedirLiberacion("res1", "")

      const { url, init } = llamada(fetchMock)
      expect(url).toContain("/api/reservation/reservas/res1/pedido-de-liberacion")
      expect(init.method).toBe("POST")
    })
  })

  describe("equiposDisponibles", () => {
    it("manda la franja pedida", async () => {
      const fetchMock = fetchFalso()

      await reservasApi.equiposDisponibles("2026-08-11", "08:00", "09:00")

      const { url } = llamada(fetchMock)
      const params = new URLSearchParams(url.split("?")[1])
      expect(params.get("fecha")).toBe("2026-08-11")
      expect(params.get("horaInicio")).toBe("08:00")
      expect(params.get("horaFin")).toBe("09:00")
    })

    /**
     * Sin serie el parámetro no viaja: mandarlo vacío haría que el backend
     * cruce contra una serie inexistente en vez de contra la fecha sola.
     */
    it("no manda la serie cuando el cambio es de una sola fecha", async () => {
      const fetchMock = fetchFalso()

      await reservasApi.equiposDisponibles("2026-08-11", "08:00", "09:00")

      expect(llamada(fetchMock).url).not.toContain("serieDesdeGrupoId")
    })

    // RF-08.14: con la serie, los libres son los libres en TODAS las fechas
    // que faltan, y eso lo resuelve el backend a partir de este parámetro.
    it("manda la serie cuando el cambio alcanza a las siguientes", async () => {
      const fetchMock = fetchFalso()

      await reservasApi.equiposDisponibles("2026-08-11", "08:00", "09:00", "grupo1")

      const params = new URLSearchParams(llamada(fetchMock).url.split("?")[1])
      expect(params.get("serieDesdeGrupoId")).toBe("grupo1")
    })
  })

  describe("cambiarEquipoDeReserva", () => {
    it("manda el alcance junto con el equipo nuevo", async () => {
      const fetchMock = fetchFalso(jsonResponse({ id: "res1" }))

      await reservasApi.cambiarEquipoDeReserva("res1", "pc9", false)

      const { init } = llamada(fetchMock)
      expect(JSON.parse(init.body as string)).toEqual({ equipoId: "pc9", soloEsta: false })
    })

    // El alcance por defecto es el conservador: tocar una sola fecha.
    it("por defecto cambia solo esa fecha", async () => {
      const fetchMock = fetchFalso(jsonResponse({ id: "res1" }))

      await reservasApi.cambiarEquipoDeReserva("res1", "pc9")

      const { init } = llamada(fetchMock)
      expect(JSON.parse(init.body as string)).toMatchObject({ soloEsta: true })
    })
  })
})
