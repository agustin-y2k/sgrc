import { descargarFoto, subirMiFoto } from "@/features/perfil/api"
import { ApiError, registrarManejadorDeSesionRechazada } from "@/lib/api-client"
import { clearToken, setToken } from "@/lib/token-store"

afterEach(() => {
  vi.restoreAllMocks()
  clearToken()
})

it("la subida multipart conserva el mensaje y código de rechazo del backend", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response("La foto excede las dimensiones permitidas", { status: 400 })
  )
  await expect(subirMiFoto(new Blob(["x"]))).rejects.toMatchObject({
    status: 400,
    message: "La foto excede las dimensiones permitidas",
  })
})

it("la lectura de una foto revocada usa el mismo cierre de sesión que JSON", async () => {
  setToken("sesion-actual")
  const cerrada = vi.fn()
  const quitar = registrarManejadorDeSesionRechazada(cerrada)
  try {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("Cuenta dada de baja", {
        status: 401,
        headers: { "X-Sesion-Motivo": "revocada" },
      })
    )
    await expect(descargarFoto("persona")).rejects.toBeInstanceOf(ApiError)
    expect(cerrada).toHaveBeenCalledWith("Cuenta dada de baja", "revocada")
  } finally {
    quitar()
  }
})

it("una foto inexistente sigue siendo una respuesta cacheable", async () => {
  const respuesta = new Response("No hay foto de perfil", { status: 404 })
  vi.spyOn(globalThis, "fetch").mockResolvedValue(respuesta)
  await expect(descargarFoto("persona")).resolves.toBeNull()
  expect(respuesta.bodyUsed).toBe(true)
})
