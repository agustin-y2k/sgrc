import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import { AprobacionPage } from "@/features/auth/AprobacionPage"
import * as authApi from "@/features/auth/api"
import type { ListarUsuariosResponse } from "@/features/auth/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/auth/api")

function renderAprobacionPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    // Con router: la tarjeta enlaza con Académico para asignar la materia.
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/admin/aprobacion"]}>
        <AprobacionPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

const pendientesMock: ListarUsuariosResponse = {
  data: [
    {
      id: "1",
      nombre: "Alan",
      apellido: "Turing",
      email: "alan@escuela.edu.ar",
      rol: "DOCENTE",
      estado: "PENDIENTE",
      fechaRegistro: "2026-01-01T00:00:00Z",
      fechaAprobacion: null,
      debeCambiarPassword: false,
    },
  ],
  meta: { total: 1, page: 1, pageSize: 1 },
}

describe("AprobacionPage", () => {
  beforeEach(() => {
    // vi.mock() genera mocks automáticos cuyo historial de llamadas NO limpia
    // restoreAllMocks — sin esto, un "no fue llamado" ve las llamadas del
    // test anterior.
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("lista las cuentas PENDIENTE", async () => {
    vi.mocked(authApi.listarUsuarios).mockResolvedValue(pendientesMock)
    renderAprobacionPage()

    expect(await screen.findByText("Alan Turing")).toBeInTheDocument()
    expect(authApi.listarUsuarios).toHaveBeenCalledWith({ estado: "PENDIENTE" })
  })

  it("sin pendientes, muestra el mensaje vacío", async () => {
    vi.mocked(authApi.listarUsuarios).mockResolvedValue({
      data: [],
      meta: { total: 0, page: 1, pageSize: 0 },
    })
    renderAprobacionPage()

    expect(await screen.findByText("No hay cuentas pendientes.")).toBeInTheDocument()
  })

  it("Aprobar dispara PATCH estado=APROBADA con el id correcto", async () => {
    vi.mocked(authApi.listarUsuarios).mockResolvedValue(pendientesMock)
    vi.mocked(authApi.cambiarEstado).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderAprobacionPage()

    await screen.findByText("Alan Turing")
    await user.click(screen.getByRole("button", { name: "Aprobar" }))

    expect(authApi.cambiarEstado).toHaveBeenCalledWith("1", { estado: "APROBADA" })
  })

  // Rechazar no se deshace (RECHAZADA es terminal y no transiciona a ningún
  // lado), así que un solo click no debe alcanzar.
  it("Rechazar NO dispara el PATCH hasta confirmar, y avisa que no se deshace", async () => {
    vi.mocked(authApi.listarUsuarios).mockResolvedValue(pendientesMock)
    vi.mocked(authApi.cambiarEstado).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderAprobacionPage()

    await screen.findByText("Alan Turing")
    await user.click(screen.getByRole("button", { name: "Rechazar" }))

    expect(authApi.cambiarEstado).not.toHaveBeenCalled()
    expect(screen.getByText(/no se puede deshacer/)).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "Usuarios" })).toBeInTheDocument()
  })

  it("Rechazar confirmado dispara PATCH estado=RECHAZADA con el id correcto", async () => {
    vi.mocked(authApi.listarUsuarios).mockResolvedValue(pendientesMock)
    vi.mocked(authApi.cambiarEstado).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderAprobacionPage()

    await screen.findByText("Alan Turing")
    await user.click(screen.getByRole("button", { name: "Rechazar" }))
    await user.click(screen.getByRole("button", { name: "Confirmar rechazo" }))

    expect(authApi.cambiarEstado).toHaveBeenCalledWith("1", { estado: "RECHAZADA" })
  })

  it("Cancelar la confirmación no rechaza nada", async () => {
    vi.mocked(authApi.listarUsuarios).mockResolvedValue(pendientesMock)
    vi.mocked(authApi.cambiarEstado).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderAprobacionPage()

    await screen.findByText("Alan Turing")
    await user.click(screen.getByRole("button", { name: "Rechazar" }))
    await user.click(screen.getByRole("button", { name: "Cancelar" }))

    expect(authApi.cambiarEstado).not.toHaveBeenCalled()
    expect(screen.getByRole("button", { name: "Aprobar" })).toBeInTheDocument()
  })

  it("si el PATCH falla, muestra el mensaje del backend (ej. último Admin activo)", async () => {
    vi.mocked(authApi.listarUsuarios).mockResolvedValue(pendientesMock)
    vi.mocked(authApi.cambiarEstado).mockRejectedValue(
      new ApiError(409, "no se puede dejar la institución sin ningún Admin activo")
    )
    const user = userEvent.setup()
    renderAprobacionPage()

    await screen.findByText("Alan Turing")
    await user.click(screen.getByRole("button", { name: "Aprobar" }))

    expect(await screen.findByText(/sin ningún Admin activo/)).toBeInTheDocument()
  })
})

// RF-01.3 + RF-02.6: lo que el docente declaró al registrarse es lo que el
// Admin necesita para saber a qué materia y curso asignarlo.
describe("AprobacionPage — lo que el docente pidió dictar", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("muestra la materia y el curso que declaró", async () => {
    vi.mocked(authApi.listarUsuarios).mockResolvedValue({
      data: [
        {
          ...pendientesMock.data[0],
          cursoSolicitado: "5°A",
          materiaSolicitada: "Programación",
        },
      ],
      meta: { total: 1, page: 1, pageSize: 1 },
    })
    renderAprobacionPage()

    expect(await screen.findByText("Pidió dictar")).toBeInTheDocument()
    expect(screen.getByText("Programación · 5°A")).toBeInTheDocument()
  })

  // Son opcionales: quien se registró sin completarlos no tiene que dejar un
  // recuadro vacío en la tarjeta.
  it("no muestra el recuadro si no declaró nada", async () => {
    vi.mocked(authApi.listarUsuarios).mockResolvedValue(pendientesMock)
    renderAprobacionPage()

    await screen.findByText(/Alan Turing/)
    expect(screen.queryByText("Pidió dictar")).not.toBeInTheDocument()
  })
})
