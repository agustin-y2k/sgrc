import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"

import { SelectorDeCursoSolicitado } from "@/features/auth/SelectorDeCursoSolicitado"

/**
 * Lo que importa acá es que el valor que sale sea el nombre canónico del
 * curso —el mismo que el Admin ve del otro lado— y que "no lo declaré" siga
 * siendo posible: el campo es opcional y un desplegable siempre tiene algo
 * elegido.
 */
function Anfitrion({ inicial = "" }: { inicial?: string }) {
  const [valor, setValor] = useState(inicial)
  return (
    <>
      <SelectorDeCursoSolicitado idPrefijo="t" value={valor} onChange={setValor} />
      <output data-testid="valor">{valor}</output>
    </>
  )
}

describe("SelectorDeCursoSolicitado", () => {
  it("arranca sin nada elegido y con la división bloqueada", () => {
    render(<Anfitrion />)

    expect(screen.getByLabelText("Año")).toHaveValue("")
    // Una división suelta no se puede componer en un nombre de curso.
    expect(screen.getByLabelText("División")).toBeDisabled()
    expect(screen.getByTestId("valor")).toHaveTextContent("")
  })

  // Elegir el año solo ya alcanza para tener un curso válido: la "A" existe
  // en todos los años y el desplegable de al lado queda a la vista.
  it("con solo elegir el año compone el curso con la división A", async () => {
    const user = userEvent.setup()
    render(<Anfitrion />)

    await user.selectOptions(screen.getByLabelText("Año"), "5")

    expect(screen.getByTestId("valor")).toHaveTextContent("5°A")
    expect(screen.getByLabelText("División")).toBeEnabled()
  })

  it("compone el nombre con el ° que pone el sistema", async () => {
    const user = userEvent.setup()
    render(<Anfitrion />)

    await user.selectOptions(screen.getByLabelText("Año"), "3")
    await user.selectOptions(screen.getByLabelText("División"), "C")

    expect(screen.getByTestId("valor")).toHaveTextContent("3°C")
  })

  // Sin esta salida, quien abrió el desplegable por curiosidad se queda con
  // un curso declarado que nunca quiso declarar, y el Admin no puede
  // distinguirlo de uno elegido a propósito.
  it("volver a Sin especificar limpia el campo entero", async () => {
    const user = userEvent.setup()
    render(<Anfitrion inicial="4°B" />)

    await user.selectOptions(screen.getByLabelText("Año"), "")

    expect(screen.getByTestId("valor")).toHaveTextContent("")
    expect(screen.getByLabelText("División")).toBeDisabled()
  })

  it("muestra los dos selects ya puestos si viene con un valor", () => {
    render(<Anfitrion inicial="6°D" />)

    expect(screen.getByLabelText("Año")).toHaveValue("6")
    expect(screen.getByLabelText("División")).toHaveValue("D")
  })
})
