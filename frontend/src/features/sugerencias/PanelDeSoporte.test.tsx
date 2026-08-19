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

describe("PanelDeSoporte", () => {
  beforeEach(() => {
    vi.mocked(sugerenciasApi.misSugerencias).mockResolvedValue({ data: [hilo()] })
    vi.mocked(sugerenciasApi.listar).mockResolvedValue({ data: [hilo()] })
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

  // El Admin no pide ayuda: él es a quien se la piden.
  it("el Admin no ve el botón de pedir ayuda", async () => {
    montar("ADMIN")

    await screen.findByText(/esperando respuesta/)
    expect(screen.queryByRole("button", { name: "Pedir ayuda" })).not.toBeInTheDocument()
  })
})
