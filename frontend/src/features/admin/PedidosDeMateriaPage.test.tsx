import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import { PedidosDeMateriaPage } from "@/features/admin/PedidosDeMateriaPage"
import * as academicoApi from "@/features/academico/api"
import * as perfilApi from "@/features/perfil/api"
import type { PedidoDeMateria } from "@/features/perfil/types"

vi.mock("@/features/perfil/api")
vi.mock("@/features/academico/api")

function pedido(over: Partial<PedidoDeMateria> = {}): PedidoDeMateria {
  return {
    id: "p1",
    usuarioId: "docente2",
    materiaId: "m1",
    esMateriaNueva: false,
    motivo: "Me asignaron el segundo turno",
    estado: "PENDIENTE",
    creadoEn: "2026-08-18T10:00:00Z",
    ...over,
  }
}

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <PedidosDeMateriaPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("PedidosDeMateriaPage", () => {
  beforeEach(() => {
    vi.mocked(academicoApi.listarCiclos).mockResolvedValue({
      data: [{ id: "c1", anio: 2026, activo: true, archivado: false }],
    })
    vi.mocked(academicoApi.listarCursos).mockResolvedValue({
      data: [
        {
          id: "cur1",
          cicloLectivoId: "c1",
          nombre: "3°C",
          activo: true,
          archivado: false,
        },
      ],
    })
    vi.mocked(academicoApi.listarDocentesDeMateria).mockResolvedValue({ data: [] })
  })

  /**
   * Sin esto el Admin aprueba a ciegas: el pedido guarda `materiaId` y nada
   * más, así que la tarjeta decía «Una materia existente» y dos pedidos de
   * materias distintas se veían idénticos. Con una sola materia cargada no se
   * notaba; en una escuela con varias, sí.
   */
  it("nombra la materia y el curso de un pedido de una materia que ya existe", async () => {
    vi.mocked(perfilApi.listarPedidos).mockResolvedValue({
      data: [pedido({ materiaNombre: "Programación", cursoNombre: "5°A" })],
    })
    montar()

    expect(await screen.findByText("Programación de 5°A")).toBeInTheDocument()
    expect(screen.queryByText(/Una materia existente/)).not.toBeInTheDocument()
  })

  /**
   * Aprobar es asignar a ESA persona a la materia, así que quién pide es el
   * dato sobre el que se decide. El pedido guarda un UUID: sin el nombre
   * resuelto por el servidor, la tarjeta no lo decía en ningún lado.
   */
  it("dice quién pide", async () => {
    vi.mocked(perfilApi.listarPedidos).mockResolvedValue({
      data: [
        pedido({
          materiaNombre: "Bases de Datos",
          cursoNombre: "6°A",
          docenteNombre: "Silvia Bermúdez",
        }),
      ],
    })
    montar()

    expect(await screen.findByText("Silvia Bermúdez")).toBeInTheDocument()
  })

  /**
   * `creadoEn` es un instante, no una fecha de calendario: cortarle los diez
   * primeros caracteres tomaba su día en UTC, y un pedido hecho a las 23:40 de
   * Argentina aparecía fechado al día siguiente.
   */
  it("fecha el pedido en la zona de quien mira, no en UTC", async () => {
    vi.mocked(perfilApi.listarPedidos).mockResolvedValue({
      data: [pedido({ creadoEn: "2026-08-28T02:40:00Z" })],
    })
    montar()

    const esperado = new Intl.DateTimeFormat("es-AR", {
      weekday: "long",
      day: "numeric",
      month: "long",
    }).format(new Date("2026-08-28T02:40:00Z"))
    expect(await screen.findByText(`Pedido el ${esperado}`)).toBeInTheDocument()
  })

  /**
   * El otro camino: la materia todavía no existe y lo pedido es el texto que
   * escribió la persona.
   */
  it("nombra lo que se escribió a mano cuando la materia no existe", async () => {
    vi.mocked(perfilApi.listarPedidos).mockResolvedValue({
      data: [
        pedido({
          materiaId: undefined,
          esMateriaNueva: true,
          materiaSolicitada: "Laboratorio de Redes",
          cursoSolicitado: "6°A",
        }),
      ],
    })
    montar()

    expect(await screen.findByText("Laboratorio de Redes de 6°A")).toBeInTheDocument()
  })

  /**
   * El motivo es lo único con lo que cuenta quien decide antes de ir a hablar
   * con la persona.
   */
  it("muestra con qué palabras lo pidió", async () => {
    vi.mocked(perfilApi.listarPedidos).mockResolvedValue({ data: [pedido()] })
    montar()

    expect(await screen.findByText(/Me asignaron el segundo turno/)).toBeInTheDocument()
  })

  /**
   * Rechazar sin explicación manda a la persona a preguntar por qué, y esa
   * conversación empieza mal: del otro lado hay alguien que se expuso
   * contando para qué la quería.
   */
  it("no deja rechazar sin explicar por qué", async () => {
    vi.mocked(perfilApi.listarPedidos).mockResolvedValue({ data: [pedido()] })
    const user = userEvent.setup()
    montar()

    await user.click(await screen.findByRole("button", { name: "No aprobar" }))
    expect(screen.getByRole("button", { name: /confirmar el rechazo/i })).toBeDisabled()

    await user.type(screen.getByLabelText(/por qué no/i), "La materia queda como está")
    expect(screen.getByRole("button", { name: /confirmar el rechazo/i })).toBeEnabled()
  })

  /**
   * Una materia que no existe hay que crearla en algún curso, y el sistema no
   * lo adivina del texto que escribió el docente ("Robótica de 5°B" es una
   * frase, no un curso).
   */
  it("para aprobar una materia que no existe, pide el curso", async () => {
    vi.mocked(perfilApi.listarPedidos).mockResolvedValue({
      data: [
        pedido({
          materiaId: undefined,
          esMateriaNueva: true,
          materiaSolicitada: "Robótica",
          cursoSolicitado: "5°B",
        }),
      ],
    })
    const user = userEvent.setup()
    montar()

    await user.click(await screen.findByRole("button", { name: "Aprobar" }))
    expect(screen.getByRole("button", { name: /confirmar y habilitar/i })).toBeDisabled()

    await user.selectOptions(
      await screen.findByLabelText(/en qué curso se crea/i),
      "cur1"
    )
    expect(screen.getByRole("button", { name: /confirmar y habilitar/i })).toBeEnabled()
  })

  /**
   * El rol no da ni quita permisos, pero es el dato que después alguien lee
   * para saber quién es quién: hay suplentes que cubren un cargo por años.
   */
  it("deja elegir el rol, y por defecto lo decide el sistema", async () => {
    vi.mocked(perfilApi.listarPedidos).mockResolvedValue({ data: [pedido()] })
    vi.mocked(perfilApi.resolverPedido).mockResolvedValue(pedido({ estado: "APROBADO" }))
    const user = userEvent.setup()
    montar()

    await user.click(await screen.findByRole("button", { name: "Aprobar" }))
    await user.click(screen.getByRole("button", { name: /confirmar y habilitar/i }))

    await waitFor(() =>
      expect(perfilApi.resolverPedido).toHaveBeenCalledWith("p1", {
        aprobar: true,
        respuesta: "",
        cursoId: undefined,
        rol: undefined,
      })
    )
  })

  it("cuando no hay nada pendiente, lo dice", async () => {
    vi.mocked(perfilApi.listarPedidos).mockResolvedValue({ data: [] })
    montar()

    expect(await screen.findByText(/No hay pedidos sin resolver/)).toBeInTheDocument()
  })
})
