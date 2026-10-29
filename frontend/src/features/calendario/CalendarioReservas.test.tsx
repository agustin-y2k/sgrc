import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"
import { CalendarioReservas } from "./CalendarioReservas"
import { cargarCalendario } from "./reservas-api"
import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import { aFechaISO } from "./semana"

vi.mock("./reservas-api")
vi.mock("@/features/auth/AuthContext")

function montar(rol: "ADMIN" | "DOCENTE") {
  vi.mocked(useAuth).mockReturnValue({
    user: { id: "persona", rol } as Usuario,
  } as ReturnType<typeof useAuth>)
  const cliente = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={cliente}>
      <MemoryRouter>
        <CalendarioReservas />
      </MemoryRouter>
    </QueryClientProvider>
  )
}
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(cargarCalendario).mockResolvedValue([])
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.setAttribute("open", "")
    },
  })
  Object.defineProperty(HTMLDialogElement.prototype, "close", {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.removeAttribute("open")
    },
  })
})

test.each([
  ["DOCENTE", "mias"],
  ["ADMIN", "todas"],
] as const)("%s accede al calendario con el ámbito %s", async (rol, ambito) => {
  montar(rol)
  await waitFor(() =>
    expect(cargarCalendario).toHaveBeenCalledWith(expect.objectContaining({ ambito }))
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole("button", { name: "Mes" }))
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Mes" })).toHaveAttribute(
      "aria-pressed",
      "true"
    )
  )
  const fecha = (screen.getByLabelText("Ir a fecha") as HTMLInputElement).value
  await user.click(screen.getByRole("button", { name: "Período siguiente" }))
  expect((screen.getByLabelText("Ir a fecha") as HTMLInputElement).value).not.toBe(fecha)
  await user.click(screen.getByRole("button", { name: "Hoy" }))
  expect((screen.getByLabelText("Ir a fecha") as HTMLInputElement).value).toBe(
    aFechaISO(new Date())
  )
})

test("otras materias se consultan sin ofrecer gestión y se cierra el detalle", async () => {
  vi.mocked(cargarCalendario).mockResolvedValue([
    {
      id: "r",
      equipoId: "pc",
      reservaGrupoId: "g",
      fecha: aFechaISO(new Date()),
      horaInicio: "08:00",
      horaFin: "09:00",
      estado: "CONFIRMADA",
      tipo: "NORMAL",
      identificador: 4,
      etiqueta: "PC 4",
      carroNombre: "Carro 1",
      materiaNombre: "Matemática",
      propia: false,
    },
  ])
  montar("DOCENTE")
  const user = userEvent.setup()
  await user.click(await screen.findByRole("button", { name: /Matemática,.*08:00/ }))
  const dialogo = screen.getByRole("dialog")
  expect(
    within(dialogo).queryByRole("link", { name: "Gestionar reserva" })
  ).not.toBeInTheDocument()
  expect(within(dialogo).getByRole("link", { name: /PC 4/ })).toBeInTheDocument()
  await user.click(within(dialogo).getByRole("button", { name: "Cerrar detalles" }))
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
})
test("un error de calendario permite reintentar y no muestra una falsa ausencia", async () => {
  vi.mocked(cargarCalendario).mockRejectedValueOnce(new Error("sin conexión"))
  montar("ADMIN")
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "No pudimos consultar el calendario"
  )
  expect(screen.queryByText(/No hay reservas que coincidan/)).not.toBeInTheDocument()
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Reintentar calendario" }))
  expect(await screen.findByText(/No hay reservas que coincidan/)).toBeInTheDocument()
})

test.each(["ADMIN", "DOCENTE"] as const)(
  "%s abre una reserva cancelada con los filtros del listado",
  async (rol) => {
    vi.mocked(cargarCalendario).mockResolvedValue([
      {
        id: "cancelada",
        equipoId: "pc",
        reservaGrupoId: "g",
        fecha: aFechaISO(new Date()),
        horaInicio: "08:00",
        horaFin: "09:00",
        estado: "CANCELADA",
        tipo: "NORMAL",
        identificador: 4,
        etiqueta: "PC 4",
        carroNombre: "Carro 1",
        materiaNombre: "Dibujo",
        propia: true,
        creadoPor: "persona",
      },
    ])
    montar(rol)
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: /Dibujo,.*08:00/ }))
    const href = within(screen.getByRole("dialog"))
      .getByRole("link", { name: "Gestionar reserva" })
      .getAttribute("href")!
    const params = new URL(href, "http://localhost").searchParams
    expect(params.get("canceladas")).toBe("true")
    expect(params.get("desde")).toBe(aFechaISO(new Date()))
    expect(params.get("hasta")).toBe(aFechaISO(new Date()))
    expect(params.get("creadoPor")).toBe(rol === "ADMIN" ? "persona" : null)
  }
)
