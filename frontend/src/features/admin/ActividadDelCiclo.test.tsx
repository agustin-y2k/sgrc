import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import { ActividadDelCiclo } from "./ActividadDelCiclo"
import * as api from "./actividad-api"
import { ApiError } from "@/lib/api-client"
vi.mock("./actividad-api")
const metricas = {
  reservasSolicitadas: 2,
  reservasAtendidas: 1,
  reservasNoRetiradas: 1,
  reservasCanceladas: 0,
  prestamosConReserva: 2,
  prestamosSinReserva: 3,
  prestamosOrigenNoDeterminado: 0,
  prestamosAbiertos: 1,
  prestamosDevueltos: 4,
}
function montar(archivado = false) {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <ActividadDelCiclo
        cicloId="ciclo"
        desde="2026-05-01"
        hasta="2026-05-04"
        archivado={archivado}
      />
    </QueryClientProvider>
  )
}
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.consultarActividad).mockResolvedValue({
    version: 1,
    anio: 2026,
    disponible: true,
    historico: false,
    totales: metricas,
    equipos: [{ equipoId: "pc", etiqueta: "PC 1", ...metricas }],
    responsables: [{ identidad: "u1", nombre: "Docente", usuarioId: "u1", ...metricas }],
  })
})
it("distingue reservas de movimientos y muestra los préstamos sin reserva", async () => {
  montar()
  await screen.findByText("Préstamos sin reserva")
  const tabla = screen.getByRole("table", { name: "Entregas por responsable" })
  expect(
    within(tabla).getByRole("row", { name: "Docente 1 2 3 0 1 4" })
  ).toBeInTheDocument()
  expect(api.consultarActividad).toHaveBeenCalledWith("ciclo", "2026-05-01", "2026-05-04")
})
it("consulta el histórico completo sin trasladarle los filtros de fecha", async () => {
  montar(true)
  await screen.findByText("Préstamos sin reserva")
  expect(api.consultarActividad).toHaveBeenCalledWith("ciclo", undefined, undefined)
})
it("un histórico anterior explica la falta de detalle sin inventar ceros", async () => {
  vi.mocked(api.consultarActividad).mockResolvedValue({
    version: 0,
    anio: 2025,
    disponible: false,
    historico: true,
    totales: metricas,
    equipos: [],
    responsables: [],
  })
  montar(true)
  await screen.findByText(/Este cierre es anterior/)
  expect(screen.queryByText("Préstamos sin reserva")).not.toBeInTheDocument()
})
it("un fallo de consulta no se muestra como ausencia de actividad", async () => {
  vi.mocked(api.consultarActividad).mockRejectedValue(new ApiError(503, "sin conexión"))
  montar()
  await screen.findByText(/No pudimos consultar reservas y entregas efectivas/)
  expect(screen.queryByText(/Sin actividad registrada/)).not.toBeInTheDocument()
})
