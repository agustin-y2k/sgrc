import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { PrestamosDeEquipo } from "./PrestamosDeEquipo"
import * as api from "@/features/reservas/api"

vi.mock("@/features/reservas/api")
it("conserva la procedencia histórica e identifica una reconstrucción", async () => {
  vi.mocked(api.historialDePrestamosDeEquipo).mockResolvedValue({
    data: [
      {
        id: "p",
        equipoId: "pc",
        entregadoANombre: "Docente",
        entregadoEn: "2026-05-04T08:30:00Z",
        devueltoEn: "2026-05-04T09:00:00Z",
        abierto: false,
        demorado: false,
        contextoAcademico: {
          reservaIdOriginal: "reserva",
          materiaId: "materia",
          materiaNombre: "Dibujo",
          contenedorId: "curso",
          contenedorTipo: "CURSO",
          contenedorNombre: "3°2",
          modalidad: "Construcción",
          cicloId: "ciclo",
          cicloAnio: 2026,
          reconstruido: true,
        },
      },
    ],
  })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <PrestamosDeEquipo equipoId="pc" />
    </QueryClientProvider>
  )
  expect(
    await screen.findByText(/Dibujo · 3°2 · Construcción · Ciclo 2026/)
  ).toBeInTheDocument()
  expect(
    screen.getByText(/Contexto reconstruido a partir de la reserva vinculada/)
  ).toBeInTheDocument()
})
