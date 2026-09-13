import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"

import {
  SelectorDeCursoSolicitado,
  SelectorDeMateriaSolicitada,
} from "@/features/auth/SelectorDeCursoSolicitado"
import type { LugarParaRegistro } from "@/features/auth/types"

/**
 * Lo que importa acá son tres cosas a la vez:
 *
 * - que lo que la escuela tiene cargado se **vea** —un desplegable, no un campo
 *   de texto con sugerencias que nadie descubre—,
 * - que se pueda **escribir igual** lo que no está en la lista, y
 * - que un lugar que no dicta materias (RF-02.13) no pregunte cuál.
 */
const CURSOS: LugarParaRegistro[] = [
  { nombre: "1°A", tipo: "CURSO", materias: ["Matemática", "Lengua"] },
  {
    nombre: "4°2",
    modalidad: "Electromecánica",
    tipo: "CURSO",
    materias: ["Taller"],
  },
  { nombre: "Biblioteca", tipo: "ESPACIO", materias: ["Biblioteca"] },
]

function AnfitrionCurso({ lugares = CURSOS, cargado = true }) {
  const [valor, setValor] = useState("")
  return (
    <>
      <SelectorDeCursoSolicitado
        idPrefijo="t"
        value={valor}
        onChange={setValor}
        lugares={lugares}
        cargado={cargado}
      />
      <output data-testid="valor">{valor}</output>
    </>
  )
}

function AnfitrionMateria({ lugar }: { lugar: LugarParaRegistro | null }) {
  const [valor, setValor] = useState("")
  return (
    <>
      <SelectorDeMateriaSolicitada
        idPrefijo="t"
        value={valor}
        onChange={setValor}
        lugarElegido={lugar}
      />
      <output data-testid="valor">{valor}</output>
    </>
  )
}

const ROTULO_CURSO = "Curso o lugar donde trabajás"

describe("SelectorDeCursoSolicitado", () => {
  it("muestra lo que la escuela tiene cargado, con la modalidad al lado", () => {
    render(<AnfitrionCurso />)

    const opciones = [...screen.getByLabelText(ROTULO_CURSO).querySelectorAll("option")]
      .map((o) => o.textContent)
      .filter((t) => t && !t.includes("─"))

    expect(opciones).toContain("1°A")
    expect(opciones).toContain("4°2 · Electromecánica")
    expect(opciones).toContain("Biblioteca")
  })

  it("elegir de la lista manda ese nombre", async () => {
    const user = userEvent.setup()
    render(<AnfitrionCurso />)

    await user.selectOptions(screen.getByLabelText(ROTULO_CURSO), "1°A")

    expect(screen.getByTestId("valor")).toHaveTextContent("1°A")
  })

  // La salida de emergencia: un preceptor no está en ninguna lista.
  it("«Otro» abre un campo para escribir lo que no está", async () => {
    const user = userEvent.setup()
    render(<AnfitrionCurso />)

    expect(screen.queryByLabelText(/escribilo/)).not.toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText(ROTULO_CURSO), "__otro__")
    await user.type(screen.getByLabelText(/escribilo/), "Preceptoría")

    expect(screen.getByTestId("valor")).toHaveTextContent("Preceptoría")
  })

  // Elegir «Otro» no puede dejar puesto lo que estaba seleccionado antes: el
  // campo de texto nacería con algo que la persona no escribió.
  it("«Otro» limpia lo que estaba elegido", async () => {
    const user = userEvent.setup()
    render(<AnfitrionCurso />)

    await user.selectOptions(screen.getByLabelText(ROTULO_CURSO), "1°A")
    await user.selectOptions(screen.getByLabelText(ROTULO_CURSO), "__otro__")

    expect(screen.getByTestId("valor")).toHaveTextContent("")
  })

  // Sin nada cargado, un desplegable cuya única opción es «Otro» sería una
  // puerta con un cartel que dice «puerta».
  it("sin lista, es un campo de texto común", async () => {
    const user = userEvent.setup()
    render(<AnfitrionCurso lugares={[]} />)

    await user.type(screen.getByLabelText(ROTULO_CURSO), "Sala Verde")

    expect(screen.getByTestId("valor")).toHaveTextContent("Sala Verde")
  })
})

describe("SelectorDeMateriaSolicitada", () => {
  it("ofrece las materias DE ESE curso, y ninguna otra", () => {
    render(<AnfitrionMateria lugar={CURSOS[0]} />)

    const opciones = [...screen.getByLabelText("Materia").querySelectorAll("option")]
      .map((o) => o.textContent)
      .filter((t) => t && !t.includes("─"))

    expect(opciones).toContain("Matemática")
    expect(opciones).toContain("Lengua")
    // «Taller» es de 4°2: ofrecerla acá sería mandar una combinación inexistente.
    expect(opciones).not.toContain("Taller")
  })

  it("también deja escribir una materia que no está", async () => {
    const user = userEvent.setup()
    render(<AnfitrionMateria lugar={CURSOS[0]} />)

    await user.selectOptions(screen.getByLabelText("Materia"), "__otro__")
    await user.type(screen.getByLabelText(/escribilo/), "Robótica")

    expect(screen.getByTestId("valor")).toHaveTextContent("Robótica")
  })

  // Si el curso se escribió a mano, no existe todavía: no hay materias que
  // ofrecer, así que se escribe directamente.
  it("con un curso escrito a mano, la materia es texto libre", () => {
    render(<AnfitrionMateria lugar={null} />)

    expect(screen.getByLabelText("Materia")).toHaveAttribute("placeholder")
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument()
  })

  // RF-02.13: en la Biblioteca no se dicta una materia. Preguntar cuál sería
  // devolver la ceremonia que ese modelo saca.
  it("en un lugar sin materias no pregunta nada y se completa solo", async () => {
    render(<AnfitrionMateria lugar={CURSOS[2]} />)

    expect(screen.getByText(/no se dicta una materia/)).toBeInTheDocument()
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument()
    expect(await screen.findByTestId("valor")).toHaveTextContent("Biblioteca")
  })
})
