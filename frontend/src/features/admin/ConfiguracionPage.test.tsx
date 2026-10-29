import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { ConfiguracionPage } from "./ConfiguracionPage"
import * as api from "./configuracion-api"
import { ApiError } from "@/lib/api-client"
vi.mock("./configuracion-api")
const configuracion = {
  retiroGraciaMinutos: 40,
  recordatorioAntelacionMinutos: 60,
  cierreGraciaMinutos: 60,
  cierreHoraSinJornada: 23,
  licenciasHoraAviso: 8,
  version: 1,
}
function montar() {
  return render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <ConfiguracionPage />
    </QueryClientProvider>
  )
}
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.consultarConfiguracion).mockResolvedValue(configuracion)
})
it("guarda un plazo nuevo junto a su versión y muestra cuándo se aplica", async () => {
  vi.mocked(api.guardarConfiguracion).mockResolvedValue({
    ...configuracion,
    retiroGraciaMinutos: 10,
    version: 2,
  })
  const user = userEvent.setup()
  montar()
  const campo = await screen.findByLabelText("Espera para retirar una reserva (minutos)")
  await user.clear(campo)
  await user.type(campo, "10")
  await user.click(screen.getByRole("button", { name: "Guardar configuración" }))
  await screen.findByText(/Configuración guardada/)
  expect(api.guardarConfiguracion).toHaveBeenCalledWith({
    ...configuracion,
    retiroGraciaMinutos: 10,
  })
  expect(screen.getByRole("button", { name: "Guardar configuración" })).toBeDisabled()
})
it("bloquea valores vacíos o fuera de rango", async () => {
  const user = userEvent.setup()
  montar()
  const campo = await screen.findByLabelText("Espera para retirar una reserva (minutos)")
  await user.clear(campo)
  expect(screen.getByRole("button", { name: "Guardar configuración" })).toBeDisabled()
  await user.type(campo, "0")
  expect(screen.getByRole("button", { name: "Guardar configuración" })).toBeDisabled()
  expect(api.guardarConfiguracion).not.toHaveBeenCalled()
})
it("un conflicto conserva la edición y permite recargar la versión vigente", async () => {
  vi.mocked(api.guardarConfiguracion).mockRejectedValue(
    new ApiError(409, "otra persona cambió la configuración")
  )
  const user = userEvent.setup()
  montar()
  const campo = await screen.findByLabelText("Espera para retirar una reserva (minutos)")
  await user.clear(campo)
  await user.type(campo, "10")
  await user.click(screen.getByRole("button", { name: "Guardar configuración" }))
  await screen.findByText(/otra persona cambió/)
  expect(campo).toHaveValue(10)
  vi.mocked(api.consultarConfiguracion).mockResolvedValue({
    ...configuracion,
    retiroGraciaMinutos: 25,
    version: 2,
  })
  await user.click(screen.getByRole("button", { name: "Recargar configuración" }))
  await waitFor(() => expect(campo).toHaveValue(25))
})
it("un fallo de lectura no ofrece guardar valores por defecto", async () => {
  vi.mocked(api.consultarConfiguracion).mockRejectedValue(
    new ApiError(503, "sin configuración")
  )
  montar()
  await screen.findByText(/No pudimos consultar la configuración/)
  expect(
    screen.queryByRole("button", { name: "Guardar configuración" })
  ).not.toBeInTheDocument()
})
