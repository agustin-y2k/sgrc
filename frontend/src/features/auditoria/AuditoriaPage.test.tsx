import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, render, screen, waitFor, within } from "@testing-library/react"
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
  return queryClient
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

  it("despliega el detalle de la acción con etiquetas comprensibles", async () => {
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [entrada({ detalle: { cursosCreados: 26, materiasCreadas: 270 } })],
      meta: { total: 1, page: 1, pageSize: 50 },
    })
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Ver el detalle" }))

    expect(screen.getByText("Cursos creados")).toBeInTheDocument()
    expect(screen.getByText("Materias creadas")).toBeInTheDocument()
    expect(screen.getByText("270")).toBeInTheDocument()
    expect(screen.queryByText(/cursosCreados/)).not.toBeInTheDocument()
    expect(document.querySelector("pre")).toBeNull()
  })

  it("explica un cambio transaccional con su ficha y valores anteriores y nuevos", async () => {
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [
        entrada({
          accion: "CAMBIO_TRANSACCIONAL",
          entidad: "equipo",
          entidadNombre: "PC 7 del Carro 1",
          detalle: {
            operacion: "UPDATE",
            campos: ["estado", "password_hash"],
            antes: { estado: "DISPONIBLE", password_hash: "secreto-anterior" },
            despues: { estado: "EN_MANTENIMIENTO", password_hash: "secreto-nuevo" },
          },
        }),
      ],
      meta: { total: 1, page: 1, pageSize: 50 },
    })
    const user = userEvent.setup()
    renderPagina()
    expect(await screen.findByText("Se actualizó el equipo")).toBeInTheDocument()
    expect(screen.getByText("PC 7 del Carro 1")).toBeInTheDocument()
    const boton = screen.getByRole("button", { name: "Ver el detalle" })
    expect(boton).toHaveAttribute("aria-expanded", "false")
    await user.click(boton)
    const tabla = screen.getByRole("table", { name: "Cambios registrados" })
    expect(within(tabla).getByRole("columnheader", { name: "Antes" })).toBeInTheDocument()
    expect(
      within(tabla).getByRole("columnheader", { name: "Después" })
    ).toBeInTheDocument()
    expect(within(tabla).getByText("Disponible")).toBeInTheDocument()
    expect(within(tabla).getByText("En mantenimiento")).toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(
      /secreto-|password_hash|CAMBIO_TRANSACCIONAL/
    )
    expect(document.querySelector("pre")).toBeNull()
    await user.click(screen.getByRole("button", { name: "Ocultar el detalle" }))
    expect(screen.queryByRole("table")).not.toBeInTheDocument()
  })

  it("distingue los cambios sin usuario identificado de una cuenta eliminada", async () => {
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [
        entrada({
          actorId: "00000000-0000-0000-0000-000000000000",
          actorNombre: undefined,
        }),
      ],
      meta: { total: 1, page: 1, pageSize: 50 },
    })
    renderPagina()
    expect(await screen.findByText(/Sin usuario identificado/)).toBeInTheDocument()
    expect(
      screen.queryByText(/Una cuenta que después se eliminó/)
    ).not.toBeInTheDocument()
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

      await user.click(
        await screen.findByRole("button", { name: /Ver todo lo de este curso/ })
      )

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

      await user.click(
        await screen.findByRole("button", { name: /Ver todo lo de este curso/ })
      )

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
  it("un fallo del registro no anuncia vacío y permite reintentar", async () => {
    vi.mocked(auditoriaApi.listarAuditoria).mockRejectedValueOnce(
      new Error("Sin conexión")
    )
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/No pudimos consultar el registro de auditoría/)
    expect(screen.queryByText(/Todavía no hay ninguna acción/)).not.toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Reintentar" }))
    await screen.findByText(/desde 10.0.0.7/)
  })

  it("un error al actualizar oculta entradas anteriores y sus filtros por ficha", async () => {
    const qc = renderPagina()
    await screen.findByText(/desde 10.0.0.7/)
    vi.mocked(auditoriaApi.listarAuditoria).mockRejectedValueOnce(new Error("Caída"))
    await act(async () => {
      await qc.invalidateQueries({ queryKey: ["auditoria"] })
    })
    await screen.findByText(/No pudimos consultar el registro de auditoría/)
    expect(screen.queryByText(/desde 10.0.0.7/)).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: /Ver todo lo de/ })
    ).not.toBeInTheDocument()
  })

  it("los catálogos fallidos no se presentan como opciones vacías", async () => {
    vi.mocked(auditoriaApi.opcionesDeAuditoria).mockRejectedValueOnce(new Error("Caída"))
    vi.mocked(adminApi.listarUsuarios).mockRejectedValueOnce(new Error("Caída"))
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/No pudimos consultar las acciones/)
    await screen.findByText(/No pudimos consultar las personas/)
    expect(screen.getByLabelText("Acción")).toBeDisabled()
    expect(screen.getByLabelText("Sobre qué")).toBeDisabled()
    expect(screen.getByLabelText("Quién")).toBeDisabled()
    await screen.findByText(/desde 10.0.0.7/)
    await user.click(screen.getAllByRole("button", { name: "Reintentar" })[0])
    await screen.findByRole("option", { name: "Curso eliminado" })
    await user.click(screen.getByRole("button", { name: "Reintentar" }))
    await screen.findByRole("option", { name: "Pérez, Juan" })
    expect(screen.getByLabelText("Quién")).toBeEnabled()
  })

  it("cambiar el tipo de ficha elimina la restricción del identificador anterior", async () => {
    const user = userEvent.setup()
    renderPagina()
    await user.click(
      await screen.findByRole("button", { name: "Ver todo lo de este curso" })
    )
    await waitFor(() =>
      expect(auditoriaApi.listarAuditoria).toHaveBeenLastCalledWith(
        expect.objectContaining({ entidadId: "c1" }),
        1
      )
    )
    await user.selectOptions(screen.getByLabelText("Sobre qué"), "equipo")
    await waitFor(() =>
      expect(auditoriaApi.listarAuditoria).toHaveBeenLastCalledWith(
        expect.objectContaining({ entidad: "equipo", entidadId: undefined }),
        1
      )
    )
    expect(screen.queryByRole("button", { name: "Ver todas" })).not.toBeInTheDocument()
  })

  it("filtra una cuenta eliminada desde la entrada y mantiene una opción legible seleccionada", async () => {
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [entrada({ actorId: "borrada", actorNombre: undefined })],
      meta: { total: 1, page: 1, pageSize: 50 },
    })
    const user = userEvent.setup()
    renderPagina()
    await user.click(
      await screen.findByRole("button", { name: "Ver acciones de esta cuenta" })
    )
    await waitFor(() =>
      expect(auditoriaApi.listarAuditoria).toHaveBeenLastCalledWith(
        { usuarioId: "borrada" },
        1
      )
    )
    expect(screen.getByLabelText("Quién")).toHaveValue("borrada")
    expect(
      screen.getByRole("option", { name: "Una cuenta que después se eliminó" })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Ver acciones de esta cuenta" })
    ).not.toBeInTheDocument()
  })

  it("bloquea rangos invertidos y mayores que el límite del backend", async () => {
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/desde 10.0.0.7/)
    await user.type(screen.getByLabelText("Desde"), "2026-08-05")
    const cantidad = vi.mocked(auditoriaApi.listarAuditoria).mock.calls.length
    await user.type(screen.getByLabelText("Hasta"), "2026-08-04")
    await screen.findByText("La fecha Desde no puede ser posterior a Hasta.")
    expect(auditoriaApi.listarAuditoria).toHaveBeenCalledTimes(cantidad)
    expect(screen.queryByText(/desde 10.0.0.7/)).not.toBeInTheDocument()
    await user.clear(screen.getByLabelText("Hasta"))
    await user.type(screen.getByLabelText("Hasta"), "2028-08-05")
    await screen.findByText("El intervalo entre las fechas no puede superar 366 días.")
  })

  it("el detalle de cambios permite entrar con teclado al área de la tabla", async () => {
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [
        entrada({
          accion: "CAMBIO_TRANSACCIONAL",
          detalle: {
            operacion: "UPDATE",
            antes: { estado: "DISPONIBLE" },
            despues: { estado: "EN_MANTENIMIENTO" },
          },
        }),
      ],
      meta: { total: 1, page: 1, pageSize: 50 },
    })
    const user = userEvent.setup()
    renderPagina()
    await user.click(await screen.findByRole("button", { name: "Ver el detalle" }))
    expect(screen.getByRole("region", { name: "Cambios registrados" })).toHaveAttribute(
      "tabindex",
      "0"
    )
  })
  it("la ficha de un evento antiguo selecciona el filtro actual de equipos", async () => {
    vi.mocked(auditoriaApi.opcionesDeAuditoria).mockResolvedValue({
      acciones: ["CAMBIO_TRANSACCIONAL"],
      entidades: ["equipo"],
    })
    vi.mocked(auditoriaApi.listarAuditoria).mockResolvedValue({
      data: [entrada({ entidad: "pc" })],
      meta: { total: 1, page: 1, pageSize: 50 },
    })
    const user = userEvent.setup()
    renderPagina()
    await user.click(
      await screen.findByRole("button", { name: "Ver todo lo de este equipo" })
    )
    await waitFor(() =>
      expect(auditoriaApi.listarAuditoria).toHaveBeenLastCalledWith(
        expect.objectContaining({ entidad: "equipo", entidadId: "c1" }),
        1
      )
    )
    expect(screen.getByLabelText("Sobre qué")).toHaveValue("equipo")
  })
})
