import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { LoQueEstaAfuera } from "@/features/admin/entregas/LoQueEstaAfuera"
import * as api from "@/features/reservas/api"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/reservas/api")

function montarDosEntregas() {
  vi.mocked(api.listarPrestamosAbiertos).mockResolvedValue({
    data: ["Ana", "Bea"].map((nombre, i) => ({
      id: `p${i}`,
      equipoId: `e${i}`,
      entregadoANombre: nombre,
      entregadoEn: "2026-10-16T10:00:00Z",
      abierto: true,
      demorado: false,
      etiqueta: `PC ${i + 1}`,
    })),
  })
  vi.mocked(api.recibirEquipos).mockResolvedValue({ recibidos: [], noRecibidos: [] })
  const cliente = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={cliente}>
      <LoQueEstaAfuera />
    </QueryClientProvider>
  )
  return cliente
}

it("recibir otro grupo no copia la observación ni borra lo preparado para los seleccionados", async () => {
  const user = userEvent.setup()
  montarDosEntregas()
  await user.click(await screen.findByRole("checkbox", { name: "Seleccionar PC 1" }))
  await user.type(screen.getByLabelText(/Observaciones/), "Sin cargador")
  await user.click(screen.getByRole("button", { name: "Recibir 1 equipo de Bea" }))
  expect(api.recibirEquipos).toHaveBeenLastCalledWith({
    prestamoIds: ["p1"],
    observaciones: undefined,
  })
  expect(screen.getByLabelText(/Observaciones/)).toHaveValue("Sin cargador")
  expect(screen.getByRole("checkbox", { name: "Seleccionar PC 1" })).toBeChecked()
  await user.click(screen.getByRole("button", { name: "Recibir 1 equipo seleccionado" }))
  expect(api.recibirEquipos).toHaveBeenLastCalledWith({
    prestamoIds: ["p0"],
    observaciones: "Sin cargador",
  })
})

it("un préstamo recibido por otra persona deja de poder enviarse desde una selección vieja", async () => {
  const user = userEvent.setup()
  const cliente = montarDosEntregas()
  await user.click(await screen.findByRole("checkbox", { name: "Seleccionar PC 1" }))
  vi.mocked(api.listarPrestamosAbiertos).mockResolvedValue({ data: [] })
  await cliente.invalidateQueries({ queryKey: ["prestamos"] })
  await screen.findByText(/Cambió la lista de entregas abiertas/)
  expect(
    screen.getByRole("button", { name: "Recibir 1 equipo seleccionado" })
  ).toBeDisabled()
})

it("un fallo de consulta no se interpreta como que no hay equipos afuera", async () => {
  const cliente = montarDosEntregas()
  await screen.findByRole("button", { name: "Recibir 1 equipo de Ana" })
  vi.mocked(api.listarPrestamosAbiertos).mockRejectedValue(
    new ApiError(503, "sin conexión")
  )
  await cliente.invalidateQueries({ queryKey: ["prestamos"] })
  await screen.findByText(/No pudimos consultar las entregas abiertas/)
  expect(screen.queryByText("No hay ningún equipo entregado.")).not.toBeInTheDocument()
  expect(
    screen.queryByRole("button", { name: /Recibir 1 equipo/ })
  ).not.toBeInTheDocument()
})

it("recibir las máquinas de un docente no incluye las de otra cuenta con el mismo nombre", async () => {
  vi.mocked(api.listarPrestamosAbiertos).mockResolvedValue({
    data: ["usuario-1", "usuario-2"].map((id, i) => ({
      id: `prestamo-${i}`,
      equipoId: `equipo-${i}`,
      entregadoAUsuarioId: id,
      entregadoANombre: "Ana Docente",
      entregadoEn: "2026-10-12T12:00:00Z",
      abierto: true,
      demorado: false,
      etiqueta: `PC ${i + 1}`,
      materiaNombre: "Programación",
    })),
  })
  vi.mocked(api.recibirEquipos).mockResolvedValue({ recibidos: [], noRecibidos: [] })
  const cliente = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={cliente}>
      <LoQueEstaAfuera />
    </QueryClientProvider>
  )
  const recibir = await screen.findAllByRole("button", {
    name: "Recibir 1 equipo de Ana Docente",
  })
  expect(recibir).toHaveLength(2)
  await userEvent.click(recibir[0])
  expect(api.recibirEquipos).toHaveBeenCalledWith({
    prestamoIds: ["prestamo-0"],
    observaciones: undefined,
  })
})
