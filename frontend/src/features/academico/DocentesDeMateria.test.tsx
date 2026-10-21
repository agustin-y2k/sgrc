import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { DocentesDeMateria } from "./DocentesDeMateria"
import * as academicoApi from "./api"
import * as adminApi from "@/features/admin/api"
import type { Curso, DocenteMateria, Materia } from "./types"
import type { Usuario } from "@/features/auth/types"
import { ApiError } from "@/lib/api-client"

vi.mock("./api")
vi.mock("@/features/admin/api")

const CURSO: Curso = {
  id: "curso-3-2",
  cicloLectivoId: "ciclo-2026",
  nombre: "3°2",
  anio: 3,
  division: "2",
  modalidad: "Construcciones",
  archivado: false,
}
const MATERIA: Materia = {
  id: "materia-3-2",
  cursoId: CURSO.id,
  espacioId: "",
  nombre: "Tecnología de los Materiales",
  archivado: false,
}
const DOCENTES: Usuario[] = ["Ana", "Beatriz", "Carla", "Diana"].map((nombre, i) => ({
  id: `u${i}`,
  nombre,
  apellido: "Prueba",
  email: `${nombre}@revision.test`,
  rol: "DOCENTE",
  estado: "APROBADA",
  fechaRegistro: "2026-01-01T00:00:00Z",
  fechaAprobacion: null,
  debeCambiarPassword: false,
}))
const ASIGNADOS: DocenteMateria[] = DOCENTES.map((u, i) => ({
  id: `dm${i}`,
  usuarioId: u.id,
  rol: i < 2 ? "TITULAR" : "SUPLENTE",
}))

function renderPanel(asignados = ASIGNADOS.slice(0, 1), soloLectura = false) {
  vi.mocked(academicoApi.listarDocentesDeMateria).mockResolvedValue({ data: asignados })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <DocentesDeMateria materia={MATERIA} curso={CURSO} soloLectura={soloLectura} />
    </QueryClientProvider>
  )
  return client
}

async function abrirConfirmacion(indice = 0) {
  const user = userEvent.setup()
  await user.click((await screen.findAllByRole("button", { name: "Quitar" }))[indice])
  return user
}

describe("Quitar docentes de una materia concreta", () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.mocked(adminApi.listarUsuarios).mockResolvedValue({
      data: DOCENTES,
      meta: { total: 4, page: 1, pageSize: 200 },
    })
    vi.mocked(academicoApi.removerDocenteMateria).mockResolvedValue({
      reservasCanceladas: 0,
    })
  })

  it("abrir y volver no escribe; identifica persona, materia y curso y devuelve el foco", async () => {
    renderPanel()
    const user = await abrirConfirmacion()
    expect(academicoApi.removerDocenteMateria).not.toHaveBeenCalled()
    const confirmacion = screen.getByRole("group", { name: "Confirmar quitar docente" })
    expect(confirmacion).toHaveTextContent("Ana Prueba")
    expect(confirmacion).toHaveTextContent(MATERIA.nombre)
    expect(confirmacion).toHaveTextContent("3°2")
    expect(confirmacion).toHaveTextContent("Construcciones")
    expect(confirmacion).toHaveTextContent(/se cancelarán sus reservas futuras/)
    await user.click(within(confirmacion).getByRole("button", { name: "Volver" }))
    expect(academicoApi.removerDocenteMateria).not.toHaveBeenCalled()
    expect(screen.getByRole("button", { name: "Quitar" })).toHaveFocus()
  })

  it.each([0, 2])(
    "con dos titulares y dos suplentes, quitar el docente %i conserva las reservas",
    async (indice) => {
      renderPanel(ASIGNADOS)
      await abrirConfirmacion(indice)
      const confirmacion = screen.getByRole("group", { name: "Confirmar quitar docente" })
      expect(confirmacion).toHaveTextContent(/Quedan 3 docentes/)
      expect(confirmacion).toHaveTextContent(/Las reservas se mantienen/)
      expect(confirmacion).not.toHaveTextContent(/se cancelarán/)
      expect(academicoApi.removerDocenteMateria).not.toHaveBeenCalled()
    }
  )

  it("una confirmación rápida se consume una sola vez e invalida las vistas afectadas", async () => {
    const client = renderPanel(ASIGNADOS)
    const invalidar = vi.spyOn(client, "invalidateQueries")
    await abrirConfirmacion(1)
    const confirmar = screen.getByRole("button", {
      name: "Confirmar quitar docente",
    })
    act(() => {
      fireEvent.click(confirmar)
      fireEvent.click(confirmar)
    })
    await waitFor(() =>
      expect(academicoApi.removerDocenteMateria).toHaveBeenCalledTimes(1)
    )
    expect(academicoApi.removerDocenteMateria).toHaveBeenCalledWith(MATERIA.id, "dm1")
    await waitFor(() =>
      expect(invalidar).toHaveBeenCalledWith({ queryKey: ["reservas"] })
    )
    for (const key of [
      "asignaciones",
      "docentes-materia",
      "mis-materias",
      "materias-de-docente",
      "equipos-disponibles",
      "notificaciones",
      "reporte",
      "auditoria",
    ])
      expect(invalidar.mock.calls.some(([filtro]) => filtro?.queryKey?.[0] === key)).toBe(
        true
      )
  })

  it("mientras envía, bloquea asignaciones y cambios de rol y muestra el resultado real", async () => {
    let resolver!: (r: { reservasCanceladas: number }) => void
    vi.mocked(academicoApi.removerDocenteMateria).mockReturnValue(
      new Promise((r) => {
        resolver = r
      })
    )
    renderPanel()
    const user = await abrirConfirmacion()
    await user.click(screen.getByRole("button", { name: "Confirmar quitar docente" }))
    expect(screen.getByLabelText("Rol de Ana Prueba")).toBeDisabled()
    expect(screen.getByRole("button", { name: "Asignar" })).toBeDisabled()
    expect(screen.getByRole("button", { name: "Quitar" })).toBeDisabled()
    await act(async () => resolver({ reservasCanceladas: 2 }))
    expect(
      await screen.findByText(/Se cancelaron 2 reservas futuras/)
    ).toBeInTheDocument()
  })

  it("un rechazo no se reintenta automáticamente y exige confirmar de nuevo", async () => {
    vi.mocked(academicoApi.removerDocenteMateria).mockRejectedValue(
      new ApiError(409, "La asignación cambió")
    )
    renderPanel()
    const user = await abrirConfirmacion()
    await user.click(screen.getByRole("button", { name: "Confirmar quitar docente" }))
    expect(await screen.findByText("La asignación cambió")).toBeInTheDocument()
    expect(academicoApi.removerDocenteMateria).toHaveBeenCalledTimes(1)
    expect(
      screen.queryByRole("button", { name: "Confirmar quitar docente" })
    ).not.toBeInTheDocument()
    await user.click(await screen.findByRole("button", { name: "Quitar" }))
    expect(academicoApi.removerDocenteMateria).toHaveBeenCalledTimes(1)
  })

  it("si cambia la lista durante la confirmación, obliga a revisar antes de escribir", async () => {
    const client = renderPanel(ASIGNADOS)
    await abrirConfirmacion()
    await act(async () => {
      client.setQueryData(["docentes-materia", MATERIA.id], { data: [ASIGNADOS[0]] })
    })
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Confirmar quitar docente" })
      ).toBeDisabled()
    )
    expect(screen.getByText(/Las asignaciones cambiaron/)).toBeInTheDocument()
    expect(academicoApi.removerDocenteMateria).not.toHaveBeenCalled()
  })

  it("una consulta fallida ofrece reintento y no permite modificar asignaciones", async () => {
    vi.mocked(adminApi.listarUsuarios).mockRejectedValueOnce(
      new ApiError(503, "Sin conexión")
    )
    renderPanel()
    const user = userEvent.setup()
    await screen.findByRole("button", { name: "Reintentar" })
    expect(screen.queryByRole("button", { name: "Quitar" })).not.toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Reintentar" }))
    expect(await screen.findByRole("button", { name: "Quitar" })).toBeEnabled()
    expect(academicoApi.removerDocenteMateria).not.toHaveBeenCalled()
  })

  it("en un ciclo archivado no permite quitar ni asignar docentes", async () => {
    renderPanel(ASIGNADOS, true)
    await screen.findByText(/Ana Prueba/)
    expect(screen.queryByRole("button", { name: "Quitar" })).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Asignar" })).not.toBeInTheDocument()
  })
})
