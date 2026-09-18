import { materiasConLaMismaPrioridad } from "@/features/admin/prioridadEmpatada"
import type { PreferenciaDeEquipo } from "@/features/inventory/types"

function marca(materiaNombre: string, prioridad: number): PreferenciaDeEquipo {
  return {
    id: `${materiaNombre}-${prioridad}`,
    equipoId: "pc1",
    materiaNombre,
    prioridad,
    alcance: materiaNombre,
  }
}

test("nombra las otras materias que ya usan esa prioridad", () => {
  const marcas = [marca("Matemática", 1), marca("Lengua", 3)]
  expect(materiasConLaMismaPrioridad(marcas, "Dibujo Técnico", 1)).toEqual(["Matemática"])
})

test("no cuenta a la materia que se está marcando", () => {
  const marcas = [marca("Matemática", 1)]
  expect(materiasConLaMismaPrioridad(marcas, "Matemática", 1)).toEqual([])
})

test("ignora tildes y mayúsculas al compararse consigo misma", () => {
  const marcas = [marca("Matemática", 1)]
  expect(materiasConLaMismaPrioridad(marcas, "  matematica ", 1)).toEqual([])
})

test("no repite una materia marcada dos veces con la misma prioridad", () => {
  const marcas = [marca("Matemática", 1), { ...marca("Matemática", 1), id: "otra" }]
  expect(materiasConLaMismaPrioridad(marcas, "Dibujo Técnico", 1)).toEqual(["Matemática"])
})

test("sin empate no dice nada", () => {
  expect(materiasConLaMismaPrioridad([marca("Matemática", 2)], "Dibujo Técnico", 1)).toEqual([])
})
