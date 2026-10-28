import { detalleAcademicoDePrestamo } from "./prestamos"
import type { ContextoAcademicoPrestamo, Prestamo } from "./types"
import { agruparPrestamos } from "@/features/admin/entregas/LoQueEstaAfuera"

const contexto: ContextoAcademicoPrestamo = {
  reservaIdOriginal: "reserva",
  materiaId: "materia",
  materiaNombre: "Dibujo",
  contenedorId: "curso",
  contenedorTipo: "CURSO",
  contenedorNombre: "3°2",
  cursoAnio: 3,
  cursoDivision: "2",
  modalidad: "Construcción",
  cicloId: "ciclo",
  cicloAnio: 2026,
  reconstruido: false,
}
const p: Prestamo = {
  id: "p",
  equipoId: "pc",
  entregadoANombre: "Docente",
  entregadoAUsuarioId: "docente",
  entregadoEn: "2026-05-04T08:30:00Z",
  abierto: true,
  demorado: false,
  destino: "3°2 · Construcción",
  contextoAcademico: contexto,
}

it("muestra la materia y el ciclo capturados incluso sin reserva vinculada", () => {
  expect(
    detalleAcademicoDePrestamo({
      ...p,
      reservaId: undefined,
      materiaNombre: "Nombre corregido",
    })
  ).toBe("Dibujo · 3°2 · Construcción · Ciclo 2026")
})
it("no agrupa entregas de distintos cursos o ciclos bajo una misma materia", () => {
  const siguiente = {
    ...p,
    id: "otro",
    equipoId: "pc2",
    contextoAcademico: { ...contexto, cicloId: "nuevo", cicloAnio: 2027 },
  }
  expect(agruparPrestamos([p, siguiente])).toHaveLength(2)
})
it("identifica un espacio sin inventar año ni división de curso", () => {
  expect(
    detalleAcademicoDePrestamo({
      ...p,
      destino: "Biblioteca",
      contextoAcademico: {
        ...contexto,
        materiaNombre: "Biblioteca",
        contenedorNombre: "Biblioteca",
        contenedorTipo: "ESPACIO",
        modalidad: undefined,
        cursoAnio: undefined,
        cursoDivision: undefined,
      },
    })
  ).toBe("Biblioteca · Ciclo 2026")
})
it("conserva un destino libre distinto del curso capturado", () => {
  expect(detalleAcademicoDePrestamo({ ...p, destino: "Sala de dibujo" })).toContain(
    "Ciclo 2026 · Sala de dibujo"
  )
})
it("respeta las entregas espontáneas y los registros anteriores sin contexto", () => {
  expect(
    detalleAcademicoDePrestamo({
      ...p,
      contextoAcademico: undefined,
      materiaNombre: undefined,
      destino: "Secretaría",
    })
  ).toBe("Secretaría")
  expect(
    detalleAcademicoDePrestamo({
      ...p,
      contextoAcademico: undefined,
      materiaNombre: "Materia anterior",
      destino: undefined,
    })
  ).toBe("Materia anterior")
})
