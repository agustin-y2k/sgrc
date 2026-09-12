import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as adminApi from "@/features/admin/api"
import { AuditoriaPage } from "@/features/auditoria/AuditoriaPage"
import * as auditoriaApi from "@/features/auditoria/api"
import type { EntradaDeAuditoria } from "@/features/auditoria/types"
import type { Usuario } from "@/features/auth/types"

vi.mock("@/features/auditoria/api")
vi.mock("@/features/admin/api")

function entrada(over: Partial<EntradaDeAuditoria> = {}): EntradaDeAuditoria {
  return {
    id: "e1",
    actorId: "u1",
    actorNombre: "Marta Fernández",
    accion: "CURSO_ELIMINADO",
    entidad: "curso",
    entidadId: "c1",
    ipOrigen: "10.0.0.7",
    creadoEn: "2026-09-11T05:00:57Z",
    ...over,
  }
}

function usuario(over: Partial<Usuario> = {}): Usuario {
  return {
    id: "u1",
    nombre: "Marta",
    apellido: "Fernández",
    email: "marta@example.com",
    rol: "ADMIN",
    estado: "APROBADA",
    fechaRegistro: "2026-01-01T00:00:00Z",
    fechaAprobacion: "2026-01-02T00:00:00Z",
    debeCambiarPassword: false,
    ...over,
  }
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <AuditoriaPage />
    </QueryClientProvider>
  )
}

describe("AuditoriaPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(auditoriaApi.opcionesDeAuditoria).mockResolvedValue({
      acciones: ["CURSO_ELIMINADO", "EQUIPO_DADO_DE_BAJA"],
      entidades: ["curso", "equipo"],
    })
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [entrada()],
      meta: { total: 1, page: 1, pageSize: 50 },
    })
    vi.mocked(adminApi.listarUsuarios).mockResolvedValue({
      data: [usuario(), usuario({ id: "u2", nombre: "Juan", apellido: "Pérez" })],
      meta: { total: 2, page: 1, pageSize: 200 },
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("muestra quién hizo qué, sobre qué y desde dónde", async () => {
    renderPagina()

    expect(await screen.findByText(/Marta Fernández/)).toBeInTheDocument()
    expect(screen.getByText(/desde 10\.0\.0\.7/)).toBeInTheDocument()
    // «Curso eliminado» aparece dos veces a propósito: como título de la fila y
    // como opción del selector, que se arma con las acciones que ocurrieron.
    expect(screen.getAllByText("Curso eliminado")).toHaveLength(2)
  })

  // Lo que hizo una cuenta sobrevive a su eliminación (RF-01.9). Que el actor
  // ya no exista es parte de la respuesta, no un dato que falta: se dice con
  // todas las letras en vez de dejar el renglón a medias.
  it("dice cuándo la cuenta que hizo la acción ya no existe", async () => {
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [entrada({ actorNombre: undefined })],
      meta: { total: 1, page: 1, pageSize: 50 },
    })
    renderPagina()

    expect(
      await screen.findByText(/Una cuenta que después se eliminó/)
    ).toBeInTheDocument()
  })

  // Las opciones salen de lo que de verdad ocurrió en esta instalación: el
  // catálogo completo llenaría el selector de callejones sin salida.
  it("ofrece como filtro sólo las acciones que ocurrieron", async () => {
    renderPagina()

    // Se espera a que la consulta de opciones resuelva: el selector existe
    // desde el primer render, con «Todas» sola.
    await screen.findByRole("option", { name: "Equipo dado de baja" })
    const selector = screen.getByLabelText("Acción")
    const opciones = [...selector.querySelectorAll("option")].map((o) => o.textContent)

    expect(opciones).toEqual(["Todas", "Curso eliminado", "Equipo dado de baja"])
  })

  it("filtra por acción", async () => {
    const user = userEvent.setup()
    renderPagina()

    await screen.findByRole("option", { name: "Curso eliminado" })
    await user.selectOptions(screen.getByLabelText("Acción"), "CURSO_ELIMINADO")

    await waitFor(() => {
      expect(auditoriaApi.listarAuditoria).toHaveBeenCalledWith(
        { accion: "CURSO_ELIMINADO" },
        1
      )
    })
  })

  // Quedarse en la página 4 después de acotar la búsqueda muestra una lista
  // vacía que parece "no hay nada".
  it("vuelve a la primera página al cambiar un filtro", async () => {
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [entrada()],
      meta: { total: 300, page: 3, pageSize: 50 },
    })
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: /Siguiente/i }))
    await screen.findByRole("option", { name: "Curso" })
    await user.selectOptions(screen.getByLabelText("Sobre qué"), "curso")

    await waitFor(() => {
      expect(auditoriaApi.listarAuditoria).toHaveBeenLastCalledWith(
        { entidad: "curso" },
        1
      )
    })
  })

  it("distingue «no hay nada» de «nada coincide con el filtro»", async () => {
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [],
      meta: { total: 0, page: 1, pageSize: 50 },
    })
    const user = userEvent.setup()
    renderPagina()

    expect(
      await screen.findByText(/Todavía no hay ninguna acción registrada/)
    ).toBeInTheDocument()

    await screen.findByRole("option", { name: "Curso eliminado" })
    await user.selectOptions(screen.getByLabelText("Acción"), "CURSO_ELIMINADO")

    expect(
      await screen.findByText(/No hay ninguna acción registrada que coincida/)
    ).toBeInTheDocument()
  })

  // El detalle se muestra crudo a propósito: cada acción guarda lo suyo, con su
  // propia forma, y darle formato obligaría a conocer las treinta del catálogo.
  it("despliega el detalle de la acción", async () => {
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [entrada({ detalle: { cursosCreados: 26, materiasCreadas: 270 } })],
      meta: { total: 1, page: 1, pageSize: 50 },
    })
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Ver el detalle" }))

    expect(screen.getByText(/cursosCreados/)).toBeInTheDocument()
    expect(screen.getByText(/270/)).toBeInTheDocument()
  })

  it("no ofrece desplegar un detalle que no existe", async () => {
    renderPagina()

    await screen.findByText(/Marta Fernández/)
    expect(
      screen.queryByRole("button", { name: "Ver el detalle" })
    ).not.toBeInTheDocument()
  })

  // Las dos preguntas para las que existe un registro de auditoría, y que el
  // backend sabía contestar desde el primer día: qué hizo esta persona y qué le
  // pasó a esta cosa.
  describe("quién y sobre qué ficha", () => {
    it("filtra por persona, eligiéndola por nombre y no por id", async () => {
      const user = userEvent.setup()
      renderPagina()

      // La lista de personas llega por su propia consulta: sin esperarla, el
      // selector todavía tiene sólo «Cualquiera».
      await screen.findByRole("option", { name: "Pérez, Juan" })
      await user.selectOptions(screen.getByLabelText("Quién"), "u2")

      await waitFor(() => {
        expect(auditoriaApi.listarAuditoria).toHaveBeenCalledWith(
          expect.objectContaining({ usuarioId: "u2" }),
          1
        )
      })
    })

    it("filtra por la ficha de una fila sin escribir el id", async () => {
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: /Ver todo lo de este curso/ }))

      await waitFor(() => {
        expect(auditoriaApi.listarAuditoria).toHaveBeenCalledWith(
          expect.objectContaining({ entidad: "curso", entidadId: "c1" }),
          1
        )
      })
    })

    // Acotada sin explicación, la lista parece "no pasó casi nada".
    it("dice que está mirando una sola ficha y deja volver", async () => {
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: /Ver todo lo de este curso/ }))

      expect(
        await screen.findByText(/Mostrando solo lo que le pasó a/)
      ).toBeInTheDocument()
      // Y deja de ofrecerse en la fila: ya se está mirando esa ficha.
      expect(
        screen.queryByRole("button", { name: /Ver todo lo de este curso/ })
      ).not.toBeInTheDocument()

      await user.click(screen.getByRole("button", { name: "Ver todas" }))

      await waitFor(() => {
        expect(auditoriaApi.listarAuditoria).toHaveBeenLastCalledWith(
          expect.objectContaining({ entidadId: undefined }),
          1
        )
      })
    })
  })
})
