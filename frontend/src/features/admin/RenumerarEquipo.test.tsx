import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { RenumerarEquipo } from "./RenumerarEquipo"
import { renumerarEquipo } from "./api"
import type { Equipo } from "@/features/inventory/types"
vi.mock("./api")
beforeEach(() => vi.clearAllMocks())

test("exige motivo y confirmación, mantiene la serie y permite cancelar el formulario", async () => {
  const onListo = vi.fn()
  vi.mocked(renumerarEquipo).mockResolvedValue(undefined)
  const cliente = new QueryClient()
  render(
    <QueryClientProvider client={cliente}>
      <RenumerarEquipo
        equipo={
          {
            id: "pc",
            identificador: 4,
            etiqueta: "PC 4",
            numeroSerie: "SERIE-FISICA",
          } as Equipo
        }
        onListo={onListo}
      />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole("button", { name: "Cambiar número de PC" }))
  const guardar = screen.getByRole("button", { name: "Confirmar nuevo rótulo" })
  expect(guardar).toBeDisabled()
  await user.type(screen.getByLabelText("Nuevo número de PC"), "18")
  await user.type(
    screen.getByLabelText("Motivo del cambio de rótulo"),
    "Rótulo deteriorado"
  )
  expect(guardar).toBeDisabled()
  await user.click(screen.getByRole("checkbox"))
  expect(guardar).toBeEnabled()
  await user.click(guardar)
  await waitFor(() => expect(onListo).toHaveBeenCalled())
  expect(renumerarEquipo).toHaveBeenCalledWith("pc", 4, 18, "Rótulo deteriorado")
  expect(screen.getAllByText(/SERIE-FISICA/).length).toBeGreaterThan(0)
})

test("cerrar el cambio de rótulo no envía ninguna modificación", async () => {
  const cliente = new QueryClient()
  render(
    <QueryClientProvider client={cliente}>
      <RenumerarEquipo
        equipo={
          { id: "pc", identificador: 4, etiqueta: "PC 4", numeroSerie: "SERIE" } as Equipo
        }
        onListo={vi.fn()}
      />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole("button", { name: "Cambiar número de PC" }))
  await user.click(screen.getByRole("button", { name: "Cancelar cambio de rótulo" }))
  expect(screen.queryByLabelText("Nuevo número de PC")).not.toBeInTheDocument()
  expect(renumerarEquipo).not.toHaveBeenCalled()
})
