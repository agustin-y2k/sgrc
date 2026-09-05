import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { AcademicoPage } from "@/features/academico/AcademicoPage"
import * as academicoApi from "@/features/academico/api"
import type {
  CicloLectivo,
  Curso,
  DocenteMateria,
  Materia,
} from "@/features/academico/types"
import * as adminApi from "@/features/admin/api"
import type { Usuario } from "@/features/auth/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/academico/api")
vi.mock("@/features/admin/api")

function ciclo(over: Partial<CicloLectivo> = {}): CicloLectivo {
  return { id: "ciclo1", anio: 2026, activo: true, archivado: false, ...over }
}

function curso(over: Partial<Curso> = {}): Curso {
  return {
    id: "curso1",
    cicloLectivoId: "ciclo1",
    nombre: "1°A",
    anio: 1,
    division: "A",
    activo: true,
    archivado: false,
    ...over,
  }
}

function materia(over: Partial<Materia> = {}): Materia {
  return {
    id: "materia1",
    cursoId: "curso1",
    nombre: "Matemáticas",
    activo: true,
    archivado: false,
    ...over,
  }
}

function usuario(over: Partial<Usuario> = {}): Usuario {
  return {
    id: "u1",
    nombre: "Ada",
    apellido: "Lovelace",
    email: "ada@escuela.edu.ar",
    rol: "DOCENTE",
    estado: "APROBADA",
    fechaRegistro: "2026-01-01T00:00:00Z",
    fechaAprobacion: "2026-01-02T00:00:00Z",
    debeCambiarPassword: false,
    ...over,
  }
}

function docenteMateria(over: Partial<DocenteMateria> = {}): DocenteMateria {
  return { id: "dm1", usuarioId: "u1", rol: "TITULAR", ...over }
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <AcademicoPage />
    </QueryClientProvider>
  )
}

describe("AcademicoPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(academicoApi.listarCiclos).mockResolvedValue({ data: [ciclo()] })
    vi.mocked(academicoApi.listarCursos).mockResolvedValue({ data: [curso()] })
    vi.mocked(academicoApi.listarMaterias).mockResolvedValue({ data: [materia()] })
    vi.mocked(academicoApi.listarDocentesDeMateria).mockResolvedValue({ data: [] })
    vi.mocked(academicoApi.listarAsignaciones).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.listarUsuarios).mockResolvedValue({
      data: [usuario()],
      meta: { total: 1, page: 1, pageSize: 1 },
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // Es el estado en el que queda un deploy limpio: sin esto el Admin no
  // tiene forma de saber que todo el sistema depende de crear un ciclo.
  it("cuando no hay ciclos explica que nadie va a poder reservar", async () => {
    vi.mocked(academicoApi.listarCiclos).mockResolvedValue({ data: [] })
    renderPagina()

    expect(
      await screen.findByText(/Todavía no hay ningún ciclo lectivo/)
    ).toBeInTheDocument()
  })

  it("crea un ciclo lectivo con el año indicado", async () => {
    vi.mocked(academicoApi.listarCiclos).mockResolvedValue({ data: [] })
    vi.mocked(academicoApi.crearCiclo).mockResolvedValue(ciclo({ id: "nuevo" }))
    const user = userEvent.setup()
    renderPagina()

    const anio = await screen.findByLabelText("Año")
    await user.clear(anio)
    await user.type(anio, "2027")
    await user.click(screen.getByRole("button", { name: "Crear ciclo" }))

    await waitFor(() => {
      expect(academicoApi.crearCiclo).toHaveBeenCalledWith(2027)
    })
  })

  // RF-02.1: el índice único de Postgres lo garantiza, pero conviene
  // decirlo antes de que el backend responda 409.
  it("no deja crear otro ciclo si ya hay uno activo", async () => {
    renderPagina()

    expect(await screen.findByText(/Ya hay un ciclo activo/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Crear ciclo" })).toBeDisabled()
  })

  it("permite crear otro ciclo cuando el existente está archivado", async () => {
    vi.mocked(academicoApi.listarCiclos).mockResolvedValue({
      data: [ciclo({ activo: false, archivado: true })],
    })
    renderPagina()

    await screen.findByText("Archivado")
    expect(screen.getByRole("button", { name: "Crear ciclo" })).toBeEnabled()
  })

  describe("cursos", () => {
    async function abrirCiclo(user: ReturnType<typeof userEvent.setup>) {
      await user.click(await screen.findByRole("button", { name: "Cursos" }))
    }

    /**
     * RF-02.2: el año es lo único obligatorio y el nombre no se escribe — sale
     * del año y la división, y el eco muestra cómo va a quedar.
     */
    it("crea el curso con el año, la división y la modalidad", async () => {
      vi.mocked(academicoApi.crearCurso).mockResolvedValue(curso({ id: "nuevo" }))
      const user = userEvent.setup()
      renderPagina()
      await abrirCiclo(user)

      await user.selectOptions(await screen.findByLabelText("Año del curso"), "4")
      await user.type(screen.getByLabelText("División (opcional)"), "2")
      await user.type(
        screen.getByLabelText("Modalidad o carrera (opcional)"),
        "Electromecánica"
      )

      // El eco: el `°` lo pone el sistema, no quien carga.
      expect(screen.getByText("4°2", { selector: "strong" })).toBeInTheDocument()

      await user.click(screen.getByRole("button", { name: "Agregar" }))

      await waitFor(() => {
        expect(academicoApi.crearCurso).toHaveBeenCalledWith("ciclo1", {
          anio: 4,
          division: "2",
          modalidad: "Electromecánica",
        })
      })
    })

    /**
     * La división y la modalidad son opcionales porque no todos los ámbitos
     * las tienen: una universidad no divide sus cursos, una primaria no tiene
     * modalidades. El año sí va siempre.
     */
    it("crea un curso sin división ni modalidad", async () => {
      vi.mocked(academicoApi.crearCurso).mockResolvedValue(curso({ id: "nuevo" }))
      const user = userEvent.setup()
      renderPagina()
      await abrirCiclo(user)

      await user.selectOptions(await screen.findByLabelText("Año del curso"), "2")

      expect(screen.getByText("2°", { selector: "strong" })).toBeInTheDocument()

      await user.click(screen.getByRole("button", { name: "Agregar" }))

      await waitFor(() => {
        expect(academicoApi.crearCurso).toHaveBeenCalledWith("ciclo1", {
          anio: 2,
          division: undefined,
          modalidad: undefined,
        })
      })
    })

    // Séptimo año existe en primaria; el tope alto cubre además una carrera de
    // grado. El rango es de sanidad, no una regla de dominio.
    it("ofrece años más allá del sexto", async () => {
      const user = userEvent.setup()
      renderPagina()
      await abrirCiclo(user)

      const anio = await screen.findByLabelText("Año del curso")
      const opciones = [...anio.querySelectorAll("option")].map((o) => o.value)

      expect(opciones).toContain("7")
      expect(opciones).toContain("15")
    })

    it("muestra el error del backend al eliminar un curso con reservas", async () => {
      vi.mocked(academicoApi.eliminarCurso).mockRejectedValue(
        new ApiError(409, "el curso tiene reservas asociadas")
      )
      const user = userEvent.setup()
      renderPagina()
      await abrirCiclo(user)

      await user.click(await screen.findByRole("button", { name: "Eliminar" }))
      await user.click(screen.getByRole("button", { name: "Confirmar" }))

      expect(
        await screen.findByText("el curso tiene reservas asociadas")
      ).toBeInTheDocument()
    })

    // RF-02.4: un ciclo archivado conserva cursos y materias como
    // referencia, pero no admite cambios.
    it("un ciclo archivado no ofrece acciones de edición", async () => {
      vi.mocked(academicoApi.listarCiclos).mockResolvedValue({
        data: [ciclo({ activo: false, archivado: true })],
      })
      const user = userEvent.setup()
      renderPagina()
      await abrirCiclo(user)

      expect(await screen.findByText(/Este ciclo está archivado/)).toBeInTheDocument()
      expect(screen.queryByLabelText("Nuevo curso")).not.toBeInTheDocument()
      expect(screen.queryByRole("button", { name: "Renombrar" })).not.toBeInTheDocument()
      expect(screen.queryByRole("button", { name: "Eliminar" })).not.toBeInTheDocument()
    })
  })

  describe("materias", () => {
    async function abrirMaterias(user: ReturnType<typeof userEvent.setup>) {
      await user.click(await screen.findByRole("button", { name: "Cursos" }))
      await user.click(await screen.findByRole("button", { name: "Materias" }))
    }

    it("lista las materias del curso abierto", async () => {
      const user = userEvent.setup()
      renderPagina()
      await abrirMaterias(user)

      expect(await screen.findByText("Matemáticas")).toBeInTheDocument()
      expect(academicoApi.listarMaterias).toHaveBeenCalledWith("curso1")
    })

    it("crea una materia en el curso", async () => {
      vi.mocked(academicoApi.crearMateria).mockResolvedValue(materia({ id: "nueva" }))
      const user = userEvent.setup()
      renderPagina()
      await abrirMaterias(user)

      await user.type(await screen.findByLabelText("Nueva materia"), "Historia")
      await user.click(screen.getAllByRole("button", { name: "Agregar" })[1])

      await waitFor(() => {
        expect(academicoApi.crearMateria).toHaveBeenCalledWith("curso1", "Historia")
      })
    })

    it("renombra una materia", async () => {
      vi.mocked(academicoApi.editarMateria).mockResolvedValue(undefined)
      const user = userEvent.setup()
      renderPagina()
      await abrirMaterias(user)

      // Hay dos "Renombrar" en pantalla: el del curso y el de la materia.
      const renombrar = await screen.findAllByRole("button", { name: "Renombrar" })
      await user.click(renombrar[1])
      const campo = screen.getByLabelText("Nombre")
      await user.clear(campo)
      await user.type(campo, "Análisis Matemático")
      await user.click(screen.getByRole("button", { name: "Guardar" }))

      await waitFor(() => {
        expect(academicoApi.editarMateria).toHaveBeenCalledWith(
          "materia1",
          "Análisis Matemático"
        )
      })
    })

    it("avisa cuando el curso no tiene materias", async () => {
      vi.mocked(academicoApi.listarMaterias).mockResolvedValue({ data: [] })
      const user = userEvent.setup()
      renderPagina()
      await abrirMaterias(user)

      expect(
        await screen.findByText("Este curso todavía no tiene materias.")
      ).toBeInTheDocument()
    })
  })

  describe("docentes de una materia (RF-02.6)", () => {
    async function abrirDocentes(user: ReturnType<typeof userEvent.setup>) {
      await user.click(await screen.findByRole("button", { name: "Cursos" }))
      await user.click(await screen.findByRole("button", { name: "Materias" }))
      await user.click(await screen.findByRole("button", { name: "Docentes" }))
    }

    // Sin docentes la materia existe pero es inútil para un docente: solo
    // un Admin puede reservar sobre ella (RF-04.1).
    it("avisa que sin docentes asignados solo un Admin puede reservar", async () => {
      const user = userEvent.setup()
      renderPagina()
      await abrirDocentes(user)

      expect(
        await screen.findByText(/solo un Admin puede reservar esta materia/)
      ).toBeInTheDocument()
    })

    it("asigna un docente con el rol elegido", async () => {
      vi.mocked(academicoApi.asignarDocente).mockResolvedValue(docenteMateria())
      const user = userEvent.setup()
      renderPagina()
      await abrirDocentes(user)

      await user.selectOptions(await screen.findByLabelText("Asignar docente"), "u1")
      await user.selectOptions(screen.getByLabelText("Rol"), "SUPLENTE")
      await user.click(screen.getByRole("button", { name: "Asignar" }))

      await waitFor(() => {
        expect(academicoApi.asignarDocente).toHaveBeenCalledWith(
          "materia1",
          "u1",
          "SUPLENTE"
        )
      })
    })

    // El endpoint de academic devuelve solo usuarioId; el nombre se cruza
    // contra la lista de usuarios de auth.
    it("resuelve el nombre del docente asignado", async () => {
      vi.mocked(academicoApi.listarDocentesDeMateria).mockResolvedValue({
        data: [docenteMateria()],
      })
      const user = userEvent.setup()
      renderPagina()
      await abrirDocentes(user)

      expect(await screen.findByText(/Ada Lovelace/)).toBeInTheDocument()
      // El rol del asignado se muestra como el valor del selector con el que
      // se corrige, no como un cartel aparte.
      expect(screen.getByLabelText("Rol de Ada Lovelace")).toHaveValue("TITULAR")
    })

    // Cambiar el rol tiene su propio endpoint: quitar y volver a asignar pasa
    // por la cascada de RF-02.10 y, si es el único docente, le cancela las
    // reservas futuras a la materia.
    it("corrige el rol sin quitar la asignación", async () => {
      vi.mocked(academicoApi.listarDocentesDeMateria).mockResolvedValue({
        data: [docenteMateria()],
      })
      const user = userEvent.setup()
      renderPagina()
      await abrirDocentes(user)

      await user.selectOptions(
        await screen.findByLabelText("Rol de Ada Lovelace"),
        "SUPLENTE"
      )

      await waitFor(() => {
        expect(academicoApi.cambiarRolDocente).toHaveBeenCalledWith(
          "materia1",
          docenteMateria().id,
          "SUPLENTE"
        )
      })
      expect(academicoApi.removerDocenteMateria).not.toHaveBeenCalled()
    })

    it("no ofrece asignar a alguien que ya está asignado", async () => {
      vi.mocked(academicoApi.listarDocentesDeMateria).mockResolvedValue({
        data: [docenteMateria()],
      })
      const user = userEvent.setup()
      renderPagina()
      await abrirDocentes(user)

      const select = await screen.findByLabelText("Asignar docente")
      expect(select).not.toHaveTextContent("ada@escuela.edu.ar")
    })

    // RF-02.10: quitar al último cancela las reservas futuras de la materia.
    it("advierte antes de quitar al único docente asignado", async () => {
      vi.mocked(academicoApi.listarDocentesDeMateria).mockResolvedValue({
        data: [docenteMateria()],
      })
      const user = userEvent.setup()
      renderPagina()
      await abrirDocentes(user)

      expect(
        await screen.findByText(/se cancelan las reservas futuras de esta materia/)
      ).toBeInTheDocument()
    })

    it("no advierte si queda otro docente asignado", async () => {
      vi.mocked(academicoApi.listarDocentesDeMateria).mockResolvedValue({
        data: [docenteMateria(), docenteMateria({ id: "dm2", usuarioId: "u2" })],
      })
      const user = userEvent.setup()
      renderPagina()
      await abrirDocentes(user)

      await screen.findAllByRole("button", { name: "Quitar" })
      expect(
        screen.queryByText(/se cancelan las reservas futuras de esta materia/)
      ).not.toBeInTheDocument()
    })
  })

  describe("archivado (RF-02.4 / RF-02.5)", () => {
    async function abrirCierre(user: ReturnType<typeof userEvent.setup>) {
      await user.click(await screen.findByRole("button", { name: "Cerrar el año" }))
    }

    it("advierte que el borrado de reservas es definitivo", async () => {
      const user = userEvent.setup()
      renderPagina()
      await abrirCierre(user)

      expect(
        await screen.findByText(/elimina definitivamente todas sus reservas/)
      ).toBeInTheDocument()
      expect(screen.getByText(/No se puede deshacer/)).toBeInTheDocument()
    })

    it("archiva clonando al año siguiente por defecto", async () => {
      vi.mocked(academicoApi.archivarCiclo).mockResolvedValue({
        archivado: true,
        nuevoCicloId: "ciclo2",
        cursosClonados: 3,
        materiasClonadas: 9,
      })
      const user = userEvent.setup()
      renderPagina()
      await abrirCierre(user)

      await user.click(await screen.findByRole("button", { name: "Cerrar 2026" }))

      await waitFor(() => {
        expect(academicoApi.archivarCiclo).toHaveBeenCalledWith("ciclo1", 2027)
      })
      expect(await screen.findByText(/3 cursos y 9 materias/)).toBeInTheDocument()
    })

    it("archiva sin clonar si se vacía el año destino", async () => {
      vi.mocked(academicoApi.archivarCiclo).mockResolvedValue({
        archivado: true,
        cursosClonados: 0,
        materiasClonadas: 0,
      })
      const user = userEvent.setup()
      renderPagina()
      await abrirCierre(user)

      await user.clear(
        screen.getByLabelText(/Crear el ciclo siguiente copiando cursos y materias/)
      )
      await user.click(screen.getByRole("button", { name: "Cerrar 2026" }))

      await waitFor(() => {
        expect(academicoApi.archivarCiclo).toHaveBeenCalledWith("ciclo1", undefined)
      })
      expect(await screen.findByText(/No se creó un ciclo nuevo/)).toBeInTheDocument()
    })

    it("no ofrece cerrar un ciclo ya archivado", async () => {
      vi.mocked(academicoApi.listarCiclos).mockResolvedValue({
        data: [ciclo({ activo: false, archivado: true })],
      })
      renderPagina()

      await screen.findByText("Archivado")
      expect(
        screen.queryByRole("button", { name: "Cerrar el año" })
      ).not.toBeInTheDocument()
    })
  })
})
