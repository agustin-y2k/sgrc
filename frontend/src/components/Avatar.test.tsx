import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"

import { Avatar } from "@/components/Avatar"
import { setToken, clearToken } from "@/lib/token-store"

function renderAvatar(props: Partial<Parameters<typeof Avatar>[0]> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  // El avatar es decorativo —aria-hidden, alt vacío— así que no expone rol y
  // hay que buscarlo por el DOM.
  return render(
    <QueryClientProvider client={queryClient}>
      <Avatar usuarioId="u1" nombre="Ada" apellido="Lovelace" {...props} />
    </QueryClientProvider>
  )
}

describe("Avatar", () => {
  const fetchOriginal = globalThis.fetch

  beforeEach(() => {
    clearToken()
    // jsdom no implementa createObjectURL.
    globalThis.URL.createObjectURL = vi.fn(() => "blob:la-foto")
    globalThis.URL.revokeObjectURL = vi.fn()
  })

  afterEach(() => {
    globalThis.fetch = fetchOriginal
    clearToken()
  })

  // El bug que esto fija: la foto se pedía con un <img src> pelado, y una
  // etiqueta <img> no manda ningún header. Como el token vive en
  // localStorage, el pedido llegaba sin autenticar a una ruta que exige
  // estarlo: TODA foto daba 401 y nadie lo notaba, porque caer a las
  // iniciales se ve intencional.
  it("pide la foto con el token, no con un img suelto", async () => {
    setToken("un-token")
    const espia: ReturnType<typeof vi.fn<typeof fetch>> = vi.fn(async () =>
      Promise.resolve(new Response(new Blob(["x"]), { status: 200 }))
    )
    globalThis.fetch = espia

    const { container } = renderAvatar()

    await waitFor(() => expect(espia).toHaveBeenCalled())
    const [url, opciones] = espia.mock.calls[0]
    expect(String(url)).toContain("/api/auth/usuarios/u1/foto")
    expect((opciones?.headers as Record<string, string>).Authorization).toBe(
      "Bearer un-token"
    )
    // Y el token NO viaja en la URL: nginx registra la ruta completa en su
    // log de acceso, así que ahí quedaría un JWT escrito en disco.
    expect(String(url)).not.toContain("un-token")

    await waitFor(() =>
      expect(container.querySelector("img")).toHaveAttribute("src", "blob:la-foto")
    )
  })

  // Que alguien no tenga foto es el caso más común, no un error.
  it("sin foto dibuja las iniciales", async () => {
    setToken("un-token")
    globalThis.fetch = (async () =>
      new Response("", { status: 404 })) as unknown as typeof fetch

    const { container } = renderAvatar()

    expect(await screen.findByText("AL")).toBeInTheDocument()
    expect(container.querySelector("img")).toBeNull()
  })

  // Cuando la pantalla ya sabe que no hay foto, ni se pide.
  it("con tieneFoto en false no consulta nada", async () => {
    const espia: ReturnType<typeof vi.fn<typeof fetch>> = vi.fn()
    globalThis.fetch = espia

    renderAvatar({ tieneFoto: false })

    expect(screen.getByText("AL")).toBeInTheDocument()
    expect(espia).not.toHaveBeenCalled()
  })
})
