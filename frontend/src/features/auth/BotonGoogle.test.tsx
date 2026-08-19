import { render, screen, waitFor } from "@testing-library/react"

import * as authApi from "@/features/auth/api"
import { BotonGoogle } from "@/features/auth/BotonGoogle"
import { cargarGoogleIdentity } from "@/lib/google-identity"

vi.mock("@/features/auth/api")
vi.mock("@/lib/google-identity")

/**
 * El botón lo dibuja Google dentro de su propio iframe, así que acá no hay
 * nada visual que probar: lo que importa es cuándo se inicializa la
 * biblioteca de Google y cuándo NO, y que el token termine en manos de quien
 * lo pidió.
 */
function googleFalso() {
  const initialize = vi.fn()
  const renderButton = vi.fn()
  vi.mocked(cargarGoogleIdentity).mockResolvedValue({
    accounts: { id: { initialize, renderButton } },
  })
  return { initialize, renderButton }
}

describe("BotonGoogle", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  // Sin GOOGLE_CLIENT_ID en el backend, el formulario de email y contraseña
  // sigue siendo un camino completo: no se dibuja nada ni se carga el script
  // de un tercero.
  it("sin client ID configurado, no carga nada de Google", async () => {
    const { initialize } = googleFalso()
    vi.mocked(authApi.configPublica).mockResolvedValue({ googleClientId: "" })

    render(<BotonGoogle onCredential={vi.fn()} />)

    await waitFor(() => expect(authApi.configPublica).toHaveBeenCalled())
    expect(cargarGoogleIdentity).not.toHaveBeenCalled()
    expect(initialize).not.toHaveBeenCalled()
    expect(screen.getByTestId("boton-google")).toHaveClass("hidden")
  })

  it("con client ID, inicializa Google y dibuja el botón", async () => {
    const { initialize, renderButton } = googleFalso()
    vi.mocked(authApi.configPublica).mockResolvedValue({
      googleClientId: "123-abc.apps.googleusercontent.com",
    })

    render(<BotonGoogle onCredential={vi.fn()} />)

    await waitFor(() => expect(initialize).toHaveBeenCalled())
    expect(initialize.mock.calls[0][0]).toMatchObject({
      client_id: "123-abc.apps.googleusercontent.com",
      // One Tap apagado: el diálogo flotante de Google no puede aparecer
      // solo encima del formulario.
      auto_select: false,
    })
    // renderButton vive en OTRO useEffect, disparado por el estado que deja
    // el primero: cuando initialize ya corrió, todavía falta un ciclo de
    // render.
    await waitFor(() => expect(renderButton).toHaveBeenCalled())
    await waitFor(() =>
      expect(screen.getByTestId("boton-google")).not.toHaveClass("hidden")
    )
  })

  // El iframe de Google no se puede tocar con CSS, así que lo único que hace
  // que el botón no desentone son las opciones de renderButton.
  it("pide el botón con el tema claro cuando la app está en claro", async () => {
    document.documentElement.classList.remove("dark")
    const { renderButton } = googleFalso()
    vi.mocked(authApi.configPublica).mockResolvedValue({ googleClientId: "123-abc" })

    render(<BotonGoogle onCredential={vi.fn()} />)

    await waitFor(() => expect(renderButton).toHaveBeenCalled())
    expect(renderButton.mock.calls[0][1]).toMatchObject({
      theme: "outline",
      logo_alignment: "left",
    })
  })

  // En oscuro, el botón "outline" es una tarjeta blanca en medio de un
  // formulario oscuro.
  it("pide el botón oscuro cuando la app está en oscuro", async () => {
    document.documentElement.classList.add("dark")
    const { renderButton } = googleFalso()
    vi.mocked(authApi.configPublica).mockResolvedValue({ googleClientId: "123-abc" })

    render(<BotonGoogle onCredential={vi.fn()} />)

    await waitFor(() => expect(renderButton).toHaveBeenCalled())
    expect(renderButton.mock.calls[0][1]).toMatchObject({ theme: "filled_black" })
    document.documentElement.classList.remove("dark")
  })

  // jsdom no hace layout, así que el ancho medido es 0. Sin medida hay que
  // omitir `width`: mandarle 0 a Google dibuja un botón de ancho cero.
  it("sin poder medir el ancho, no le manda width a Google", async () => {
    const { renderButton } = googleFalso()
    vi.mocked(authApi.configPublica).mockResolvedValue({ googleClientId: "123-abc" })

    render(<BotonGoogle onCredential={vi.fn()} />)

    await waitFor(() => expect(renderButton).toHaveBeenCalled())
    expect(renderButton.mock.calls[0][1]).not.toHaveProperty("width")
  })

  it("entrega el token que devuelve Google", async () => {
    const { initialize } = googleFalso()
    vi.mocked(authApi.configPublica).mockResolvedValue({ googleClientId: "123-abc" })
    const onCredential = vi.fn()

    render(<BotonGoogle onCredential={onCredential} />)
    await waitFor(() => expect(initialize).toHaveBeenCalled())

    // Google llama al callback que se le pasó al inicializar.
    initialize.mock.calls[0][0].callback({ credential: "el-id-token" })

    expect(onCredential).toHaveBeenCalledWith("el-id-token")
  })

  it("ignora una respuesta de Google sin token", async () => {
    const { initialize } = googleFalso()
    vi.mocked(authApi.configPublica).mockResolvedValue({ googleClientId: "123-abc" })
    const onCredential = vi.fn()

    render(<BotonGoogle onCredential={onCredential} />)
    await waitFor(() => expect(initialize).toHaveBeenCalled())

    initialize.mock.calls[0][0].callback({})

    expect(onCredential).not.toHaveBeenCalled()
  })

  // Si el script de Google no carga (sin conexión, bloqueado por una
  // extensión), no se muestra ningún error: para quien iba a entrar con email
  // y contraseña sería ruido sobre algo que no pensaba usar.
  it("si el script de Google no carga, no rompe la pantalla", async () => {
    vi.mocked(authApi.configPublica).mockResolvedValue({ googleClientId: "123-abc" })
    vi.mocked(cargarGoogleIdentity).mockRejectedValue(new Error("sin conexión"))

    render(<BotonGoogle onCredential={vi.fn()} />)

    await waitFor(() => expect(cargarGoogleIdentity).toHaveBeenCalled())
    expect(screen.getByTestId("boton-google")).toHaveClass("hidden")
  })

  it("si la config del backend falla, tampoco rompe", async () => {
    googleFalso()
    vi.mocked(authApi.configPublica).mockRejectedValue(new Error("backend caído"))

    render(<BotonGoogle onCredential={vi.fn()} />)

    await waitFor(() => expect(authApi.configPublica).toHaveBeenCalled())
    expect(screen.getByTestId("boton-google")).toHaveClass("hidden")
  })
})
