import { useState } from "react"
import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { CamposDeMarca, MARCA_VACIA, type ValoresDeMarca } from "./CamposDeMarca"
import { datosDeMarca, marcaValida, SIN_ATRIBUTO } from "./alcanceDeMarca"
import type { MateriaParaPreferencia } from "@/features/inventory/types"

const catalogo: MateriaParaPreferencia[] = [
  {
    materiaId: "m1",
    materiaNombre: "Tecnología de los Materiales",
    contenedorId: "c1",
    contenedorNombre: "3°2 Construcción",
    tipo: "CURSO",
    modalidad: "Construcción",
    anio: 3,
    division: "2",
  },
  {
    materiaId: "m2",
    materiaNombre: "TIC",
    contenedorId: "c2",
    contenedorNombre: "3°1 Electromecánica",
    tipo: "CURSO",
    modalidad: "Electromecánica",
    anio: 3,
    division: "1",
  },
  {
    materiaId: "m3",
    materiaNombre: "Matemática",
    contenedorId: "c3",
    contenedorNombre: "1°",
    tipo: "CURSO",
    anio: 1,
  },
  {
    materiaId: "m4",
    materiaNombre: "Biblioteca",
    contenedorId: "e1",
    contenedorNombre: "Biblioteca",
    tipo: "ESPACIO",
  },
]
function Formulario() {
  const [valores, setValores] = useState(MARCA_VACIA)
  return (
    <>
      <CamposDeMarca
        valores={valores}
        onChange={setValores}
        catalogo={catalogo}
        idBase="prueba"
      />
      <button disabled={!marcaValida(valores, catalogo)}>Guardar prueba</button>
    </>
  )
}
function opciones(etiqueta: string) {
  return within(screen.getByLabelText(etiqueta))
    .getAllByRole("option")
    .map((o) => o.getAttribute("value"))
}

test("Tecnología de los Materiales sólo se ofrece dentro de su modalidad, año y división", async () => {
  const user = userEvent.setup()
  render(<Formulario />)
  await user.selectOptions(screen.getByLabelText("Modalidad"), "Electromecánica")
  expect(opciones("Año")).toEqual(["", "3"])
  await user.selectOptions(screen.getByLabelText("Año"), "3")
  expect(opciones("División")).toEqual(["", "1"])
  await user.selectOptions(screen.getByLabelText("División"), "1")
  expect(opciones("Materia")).toEqual(["", "TIC"])
  await user.selectOptions(screen.getByLabelText("Modalidad"), "Construcción")
  await user.selectOptions(screen.getByLabelText("Año"), "3")
  await user.selectOptions(screen.getByLabelText("División"), "2")
  expect(opciones("Materia")).toEqual(["", "Tecnología de los Materiales"])
  await user.selectOptions(
    screen.getByLabelText("Materia"),
    "Tecnología de los Materiales"
  )
  expect(screen.getByRole("button")).toBeEnabled()
  await user.selectOptions(screen.getByLabelText("Año"), "")
  expect(screen.getByLabelText("División")).toHaveValue("")
  expect(screen.getByLabelText("Materia")).toHaveValue("")
  expect(screen.getByRole("button")).toBeDisabled()
})

test("la regla general filtra combinaciones existentes y permite materias de espacios", async () => {
  const user = userEvent.setup()
  render(<Formulario />)
  await user.selectOptions(screen.getByLabelText("Alcance"), "GENERAL")
  expect(opciones("Materia")).toContain("Biblioteca")
  await user.selectOptions(screen.getByLabelText("Modalidad"), "Construcción")
  expect(opciones("Materia")).toEqual(["", "Tecnología de los Materiales"])
  expect(opciones("División")).toEqual(["", "2"])
  await user.selectOptions(
    screen.getByLabelText("Materia"),
    "Tecnología de los Materiales"
  )
  expect(screen.getByRole("button")).toBeEnabled()
  await user.selectOptions(screen.getByLabelText("Alcance"), "CURSO")
  expect(screen.getByLabelText("Materia")).toBeDisabled()
  expect(screen.getByRole("button")).toBeDisabled()
})

test("sin modalidad y sin división se envían como ausencias exactas, conservando el año", () => {
  const v: ValoresDeMarca = {
    ...MARCA_VACIA,
    materiaNombre: "Matemática",
    modalidad: SIN_ATRIBUTO,
    anio: "1",
    division: SIN_ATRIBUTO,
  }
  expect(marcaValida(v, catalogo)).toBe(true)
  expect(datosDeMarca(v)).toEqual({
    materiaNombre: "Matemática",
    modalidad: undefined,
    anio: 1,
    division: undefined,
    cursoExacto: true,
    prioridad: 1,
  })
  expect(
    marcaValida({ ...v, materiaNombre: "Tecnología de los Materiales" }, catalogo)
  ).toBe(false)
  expect(marcaValida({ ...v, anio: "3" }, catalogo)).toBe(false)
})

test("una marca vieja o un catálogo actualizado no habilitan una combinación que dejó de existir", () => {
  const v: ValoresDeMarca = {
    ...MARCA_VACIA,
    materiaNombre: "Tecnología de los Materiales",
    modalidad: "Construcción",
    anio: "3",
    division: "2",
  }
  expect(marcaValida(v, catalogo)).toBe(true)
  expect(
    marcaValida(
      v,
      catalogo.filter((f) => f.materiaId !== "m1")
    )
  ).toBe(false)
})
