import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import { PanelDeSoporte } from "@/features/sugerencias/PanelDeSoporte"
import * as sugerenciasApi from "@/features/sugerencias/api"
import type { Sugerencia } from "@/features/sugerencias/types"

vi.mock("@/features/sugerencias/api")
vi.mock("@/features/auth/AuthContext")

function usuario(rol: Usuario["rol"]): Usuario {
  return {
    id: rol === "ADMIN" ? "admin1" : "docente1",
    nombre: "Ada",
    apellido: "Lovelace",
    email: "ada@escuela.edu.ar",
    rol,
    estado: "APROBADA",
    fechaRegistro: "2026-01-01T00:00:00Z",
    fechaAprobacion: null,
    debeCambiarPassword: false,
  }
}

function hilo(over: Partial<Sugerencia> = {}): Sugerencia {
  return {
    id: "s1",
    tipo: "AYUDA",
    asunto: "No arranca la PC 3",
    estado: "ABIERTA",
    esperaRespuesta: true,
    mensajes: [
      {
        id: "m1",
        deAdmin: false,
        texto: "La enciendo y no pasa nada",
        escritoEn: "2026-08-19T10:00:00Z",
      },
    ],
    creadaEn: "2026-08-19T10:00:00Z",
    ultimaActividadEn: "2026-08-19T10:00:00Z",
    ...over,
  }
}

function montar(rol: Usuario["rol"] = "DOCENTE") {
  vi.mocked(useAuth).mockReturnValue({
    user: usuario(rol),
    isLoading: false,
    errorDeSesion: null,
    motivoDeCierre: null,
    login: vi.fn(),
    logout: vi.fn(),
    loginConGoogle: vi.fn(),
    refetchUser: vi.fn(),
  })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <PanelDeSoporte />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

/** Una respuesta del buzón: los dos listados vienen paginados de a 50. */
function pagina(hilos: Sugerencia[], total = hilos.length, page = 1) {
  return { data: hilos, meta: { total, page, pageSize: 50 } }
}

describe("PanelDeSoporte", () => {
  beforeEach(() => {
    vi.mocked(sugerenciasApi.misSugerencias).mockResolvedValue(pagina([hilo()]))
    vi.mocked(sugerenciasApi.listar).mockResolvedValue(pagina([hilo()]))
    vi.mocked(sugerenciasApi.responder).mockResolvedValue(hilo())
    vi.mocked(sugerenciasApi.resolver).mockResolvedValue(hilo({ estado: "RESUELTA" }))
    vi.mocked(sugerenciasApi.escribir).mockResolvedValue(hilo())
  })

  // Lo que el Admin necesita saber sin abrir nada: si hay alguien esperando.
  it("al Admin le dice cuántas conversaciones esperan respuesta", async () => {
    montar("ADMIN")

    expect(
      await screen.findByText(/Hay 1 conversación esperando respuesta/)
    ).toBeInTheDocument()
  })

  it("el Admin contesta sin salir del sistema", async () => {
    const user = userEvent.setup()
    montar("ADMIN")

    await user.click(await screen.findByRole("button", { name: "Ver conversaciones" }))
    // El hilo que espera respuesta viene desplegado: es en el que hay que
    // trabajar.
    await user.type(await screen.findByPlaceholderText("Contestale…"), "vamos para allá")
    await user.click(screen.getByRole("button", { name: "Responder" }))

    await waitFor(() =>
      expect(sugerenciasApi.responder).toHaveBeenCalledWith("s1", "vamos para allá")
    )
  })

  it("el Admin puede dar por resuelta una conversación", async () => {
    const user = userEvent.setup()
    montar("ADMIN")

    await user.click(await screen.findByRole("button", { name: "Ver conversaciones" }))
    await user.click(await screen.findByRole("button", { name: "Dar por resuelta" }))

    await waitFor(() => expect(sugerenciasApi.resolver).toHaveBeenCalledWith("s1"))
  })

  // El docente ve el seguimiento adentro, que es el punto de todo esto.
  it("el docente ve su conversación y puede seguirla", async () => {
    const user = userEvent.setup()
    montar("DOCENTE")

    await user.click(await screen.findByRole("button", { name: "Ver conversaciones" }))

    expect(await screen.findByText("No arranca la PC 3")).toBeInTheDocument()
    expect(screen.getByText("La enciendo y no pasa nada")).toBeInTheDocument()
    await user.type(
      screen.getByPlaceholderText("Seguí la conversación acá mismo…"),
      "ya probé con otro cable"
    )
    await user.click(screen.getByRole("button", { name: "Responder" }))

    await waitFor(() =>
      expect(sugerenciasApi.responder).toHaveBeenCalledWith(
        "s1",
        "ya probé con otro cable"
      )
    )
  })

  it("el docente pide ayuda con asunto y mensaje", async () => {
    const user = userEvent.setup()
    montar("DOCENTE")

    await user.click(await screen.findByRole("button", { name: "Pedir ayuda" }))
    await user.type(screen.getByLabelText("¿De qué se trata?"), "No arranca la PC 3")
    await user.type(
      screen.getByLabelText("Contalo con tus palabras"),
      "la enciendo y no pasa nada"
    )
    await user.click(screen.getByRole("button", { name: "Mandar" }))

    await waitFor(() =>
      expect(sugerenciasApi.escribir).toHaveBeenCalledWith(
        "AYUDA",
        "No arranca la PC 3",
        "la enciendo y no pasa nada",
        expect.any(String)
      )
    )
  })

  // Con el formulario y la lista abiertos hay dos botones de cerrar, uno al
  // lado del otro. Cuando los dos decían "Cerrar" eran indistinguibles: había
  // que apretar uno para descubrir cuál era.
  it("los dos botones de cerrar dicen qué cierran", async () => {
    const user = userEvent.setup()
    montar("DOCENTE")

    await user.click(await screen.findByRole("button", { name: "Pedir ayuda" }))

    const rotulos = screen.getAllByRole("button").map((b) => b.textContent?.trim())

    expect(rotulos).toContain("Cerrar el formulario")
    expect(rotulos).toContain("Ocultar conversaciones")
    // Ningún rótulo repetido: dos botones con el mismo texto en la misma
    // pantalla obligan a apretar uno para saber cuál era.
    expect(new Set(rotulos).size).toBe(rotulos.length)
  })

  // El Admin no pide ayuda: él es a quien se la piden.
  it("el Admin no ve el botón de pedir ayuda", async () => {
    montar("ADMIN")

    await screen.findByText(/esperando respuesta/)
    expect(screen.queryByRole("button", { name: "Pedir ayuda" })).not.toBeInTheDocument()
  })

  // El backend pagina de a 50 desde siempre; la pantalla pedía la primera
  // página y la mostraba como si fuera el buzón entero.
  describe("paginación", () => {
    it("deja ir a la página siguiente", async () => {
      const user = userEvent.setup()
      vi.mocked(sugerenciasApi.listar).mockResolvedValue(pagina([hilo()], 60))
      montar("ADMIN")

      await user.click(await screen.findByRole("button", { name: "Ver conversaciones" }))
      await user.click(await screen.findByRole("button", { name: "Siguiente" }))

      await waitFor(() => {
        expect(sugerenciasApi.listar).toHaveBeenCalledWith(true, 2)
      })
    })

    it("no muestra controles cuando entra todo en una página", async () => {
      const user = userEvent.setup()
      montar("ADMIN")

      await user.click(await screen.findByRole("button", { name: "Ver conversaciones" }))
      await screen.findByText("No arranca la PC 3")

      expect(screen.queryByRole("button", { name: "Siguiente" })).not.toBeInTheDocument()
    })

    // Contando sobre la página, una bandeja con 60 pendientes decía "hay 50":
    // el tamaño de la página, no un dato del buzón.
    it("cuenta los pendientes con el total del servidor, no con la página", async () => {
      vi.mocked(sugerenciasApi.listar).mockResolvedValue(pagina([hilo()], 60))
      montar("ADMIN")

      expect(
        await screen.findByText(/Hay 60 conversaciones esperando respuesta/)
      ).toBeInTheDocument()
    })

    // Quedarse en la página 4 de una lista que ahora tiene 2 es mirar el vacío.
    it("vuelve a la primera página al cambiar el filtro", async () => {
      const user = userEvent.setup()
      vi.mocked(sugerenciasApi.listar).mockResolvedValue(pagina([hilo()], 60))
      montar("ADMIN")

      await user.click(await screen.findByRole("button", { name: "Ver conversaciones" }))
      await user.click(await screen.findByRole("button", { name: "Siguiente" }))
      await waitFor(() => {
        expect(sugerenciasApi.listar).toHaveBeenCalledWith(true, 2)
      })

      await user.click(screen.getByLabelText("Ver solo lo que falta contestar"))

      await waitFor(() => {
        expect(sugerenciasApi.listar).toHaveBeenCalledWith(false, 1)
      })
    })
  })
})
