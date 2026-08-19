import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import { MisMensajesPage } from "@/features/sugerencias/MisMensajesPage"
import * as sugerenciasApi from "@/features/sugerencias/api"
import type { Sugerencia } from "@/features/sugerencias/types"

vi.mock("@/features/sugerencias/api")

function mensaje(over: Partial<Sugerencia> = {}): Sugerencia {
  return {
    id: "s1",
    tipo: "PROBLEMA",
    texto: "No me aparece ninguna computadora para el jueves",
    estado: "ABIERTA",
    creadaEn: "2026-08-18T10:00:00Z",
    ...over,
  }
}

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/mis-mensajes"]}>
        <MisMensajesPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("MisMensajesPage", () => {
  beforeEach(() => {
    vi.mocked(sugerenciasApi.misSugerencias).mockResolvedValue({ data: [] })
  })

  it("no deja mandar un mensaje vacío", async () => {
    montar()
    expect(await screen.findByRole("button", { name: "Mandar" })).toBeDisabled()
  })

  /**
   * La pantalla desde la que se escribió la manda la interfaz, no la persona:
   * sin ese dato, un "no me deja" obliga a ir a buscar a quien lo escribió
   * para preguntarle qué estaba haciendo.
   */
  it("manda la pantalla desde la que se llegó, junto con el texto", async () => {
    vi.mocked(sugerenciasApi.escribir).mockResolvedValue(mensaje())
    const user = userEvent.setup()
    montar()

    await user.type(
      await screen.findByLabelText(/contalo con tus palabras/i),
      "No me deja reservar"
    )
    await user.click(screen.getByRole("button", { name: "Mandar" }))

    await waitFor(() =>
      expect(sugerenciasApi.escribir).toHaveBeenCalledWith(
        "PROBLEMA",
        "No me deja reservar",
        "/mis-mensajes"
      )
    )
  })

  it("distingue contar un problema de proponer una idea", async () => {
    vi.mocked(sugerenciasApi.escribir).mockResolvedValue(mensaje({ tipo: "SUGERENCIA" }))
    const user = userEvent.setup()
    montar()

    await user.click(await screen.findByRole("button", { name: /se me ocurre una idea/i }))
    await user.type(screen.getByLabelText(/contalo con tus palabras/i), "Un botón para repetir")
    await user.click(screen.getByRole("button", { name: "Mandar" }))

    await waitFor(() =>
      expect(sugerenciasApi.escribir).toHaveBeenCalledWith(
        "SUGERENCIA",
        expect.any(String),
        expect.any(String)
      )
    )
  })

  /**
   * Ver la respuesta es lo que sostiene el buzón: sin ella, quien escribió
   * siente que habló al vacío y dos veces así alcanzan para que no vuelva a
   * escribir.
   */
  it("muestra lo que contestaron", async () => {
    vi.mocked(sugerenciasApi.misSugerencias).mockResolvedValue({
      data: [
        mensaje({
          estado: "RESUELTA",
          respuesta: "Era la jornada, ya lo cargamos.",
        }),
      ],
    })
    montar()

    expect(await screen.findByText(/Era la jornada, ya lo cargamos\./)).toBeInTheDocument()
    expect(screen.getByText("Contestado")).toBeInTheDocument()
  })

  it("mientras no contestan, lo dice", async () => {
    vi.mocked(sugerenciasApi.misSugerencias).mockResolvedValue({ data: [mensaje()] })
    montar()

    expect(await screen.findByText("Esperando respuesta")).toBeInTheDocument()
  })
})
