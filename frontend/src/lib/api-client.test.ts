import { ApiError, apiFetch, getErrorMessage } from "@/lib/api-client"
import * as tokenStore from "@/lib/token-store"

vi.mock("@/lib/token-store")

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  })
}

function textResponse(body: string, status = 200) {
  return new Response(body, {
    status,
    headers: { "content-type": "text/plain; charset=utf-8" },
  })
}

describe("apiFetch", () => {
  beforeEach(() => {
    vi.spyOn(tokenStore, "getToken").mockReturnValue(null)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("parsea una respuesta JSON exitosa", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ ok: true })))

    const result = await apiFetch<{ ok: boolean }>("/api/algo")

    expect(result).toEqual({ ok: true })
  })

  // El backend real responde texto plano en éxito para varios endpoints
  // (c.SendStatus, ej.
  it("no intenta parsear JSON cuando el content-type es text/plain", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(textResponse("Created", 201)))

    const result = await apiFetch<void>("/api/algo", { method: "POST" })

    expect(result).toBeUndefined()
  })

  // El error handler default de Fiber v2 manda texto plano, no {"error":
  // "..."} — este test cubre exactamente el bug que se encontró probando
  // contra el backend real (ver comentario en api-client.ts).
  it("usa el body de texto plano como mensaje de ApiError", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(textResponse("credenciales inválidas", 401))
    )

    await expect(apiFetch("/api/auth/login")).rejects.toMatchObject({
      status: 401,
      message: "credenciales inválidas",
    })
  })

  it("adjunta el header Authorization cuando hay token guardado", async () => {
    vi.spyOn(tokenStore, "getToken").mockReturnValue("un-token")
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}))
    vi.stubGlobal("fetch", fetchMock)

    await apiFetch("/api/algo")

    const [, init] = fetchMock.mock.calls[0]
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer un-token")
  })

  it("no adjunta Authorization cuando no hay token", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}))
    vi.stubGlobal("fetch", fetchMock)

    await apiFetch("/api/algo")

    const [, init] = fetchMock.mock.calls[0]
    expect((init.headers as Record<string, string>).Authorization).toBeUndefined()
  })
})

describe("getErrorMessage", () => {
  it("devuelve el mensaje de un ApiError", () => {
    expect(getErrorMessage(new ApiError(409, "email ya registrado"))).toBe(
      "email ya registrado"
    )
  })

  it("devuelve un mensaje genérico para errores no-API", () => {
    expect(getErrorMessage(new Error("boom"))).toBe("Ocurrió un error inesperado")
  })
})
