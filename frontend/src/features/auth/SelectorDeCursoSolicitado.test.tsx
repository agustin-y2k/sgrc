import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"

import { SelectorDeCursoSolicitado } from "@/features/auth/SelectorDeCursoSolicitado"

/**
 * Lo que importa acá es que el curso que se declara sea **texto libre**: esta
 * pantalla se ve sin haber iniciado sesión, así que no puede ofrecer los
 * cursos de la institución, y de todos modos el nombre de un curso no tiene
 * formato (RF-02.2). Y que "no lo declaré" siga siendo posible: el campo es
 * opcional.
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
  it("arranca vacío: el curso es opcional", () => {
    render(<Anfitrion />)

    expect(screen.getByLabelText("Curso")).toHaveValue("")
    expect(screen.getByTestId("valor")).toHaveTextContent("")
  })

  // El nombre viaja tal como se escribió: el sistema no lo compone ni lo
  // corrige, porque cada institución nombra sus cursos como los nombra.
  it("manda el curso tal como se escribió", async () => {
    const user = userEvent.setup()
    render(<Anfitrion />)

    await user.type(screen.getByLabelText("Curso"), "4°2")

    expect(screen.getByTestId("valor")).toHaveTextContent("4°2")
  })

  // Los cuatro ámbitos que el sistema tiene que poder atender.
  it("acepta el nombre de cualquier ámbito", async () => {
    const user = userEvent.setup()
    for (const curso of ["1°1", "2° Enfermería", "Comisión 3B", "Sala Verde"]) {
      const { unmount } = render(<Anfitrion />)
      await user.type(screen.getByLabelText("Curso"), curso)
      expect(screen.getByTestId("valor")).toHaveTextContent(curso)
      unmount()
    }
  })

  it("muestra el valor que viene puesto", () => {
    render(<Anfitrion inicial="6°D" />)

    expect(screen.getByLabelText("Curso")).toHaveValue("6°D")
  })
})
