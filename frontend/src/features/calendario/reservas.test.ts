import {
  clasesDelCalendario,
  crearEjeSemanal,
  distribuirClases,
  fechasDelCalendario,
  moverCalendario,
  segmentosDeClase,
} from "./reservas"
import type { ReservaCalendario } from "./reservas"

export function reservaDeCalendario(
  over: Partial<ReservaCalendario> = {}
): ReservaCalendario {
  return {
    id: "r1",
    equipoId: "pc1",
    reservaGrupoId: "g1",
    fecha: "2026-10-22",
    horaInicio: "08:00",
    horaFin: "09:00",
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    etiqueta: "PC 4",
    identificador: 4,
    carroNombre: "Carro 1",
    materiaNombre: "Dibujo técnico",
    cursoNombre: "3°2",
    propia: true,
    ...over,
  }
}

test("agrupa equipos de la misma clase, sin fusionar ocurrencias recurrentes", () => {
  const clases = clasesDelCalendario([
    reservaDeCalendario(),
    reservaDeCalendario({ id: "r2", equipoId: "pc2" }),
    reservaDeCalendario({ id: "r3", reservaGrupoId: "g2", fecha: "2026-10-29" }),
  ])
  expect(clases).toHaveLength(2)
  expect(clases[0].reservas).toHaveLength(2)
})
test("el mes tiene 42 fechas consecutivas y navega correctamente entre años", () => {
  const fechas = fechasDelCalendario(new Date(2026, 11, 20), "mes")
  expect(fechas).toHaveLength(42)
  expect(fechas[0]).toBe("2026-11-30")
  expect(fechas[41]).toBe("2027-01-10")
  expect(moverCalendario(new Date(2026, 11, 31), "mes", 1)).toEqual(new Date(2027, 0, 1))
})
test("una nocturna aparece en ambos días, sin inventar un tramo al finalizar a medianoche", () => {
  const c = clasesDelCalendario([
    reservaDeCalendario({ horaInicio: "23:00", horaFin: "01:00" }),
  ])[0]
  expect(segmentosDeClase(c).map((s) => [s.fecha, s.inicio, s.fin, s.continua])).toEqual([
    ["2026-10-22", 1380, 1440, false],
    ["2026-10-23", 0, 60, true],
  ])
  c.horaFin = "00:00"
  expect(segmentosDeClase(c)).toHaveLength(1)
})
test("las clases simultáneas tienen columnas distintas y las siguientes recuperan el ancho", () => {
  const c = clasesDelCalendario([reservaDeCalendario()])[0]
  const partes = [
    { clase: c, fecha: c.fecha, inicio: 480, fin: 600, continua: false },
    {
      clase: { ...c, clave: "otra" },
      fecha: c.fecha,
      inicio: 540,
      fin: 660,
      continua: false,
    },
    {
      clase: { ...c, clave: "despues" },
      fecha: c.fecha,
      inicio: 660,
      fin: 720,
      continua: false,
    },
  ]
  expect(distribuirClases(partes).map((s) => [s.columna, s.columnas])).toEqual([
    [0, 2],
    [1, 2],
    [0, 1],
  ])
})

test("separa operaciones de bloqueo coincidentes y conserva sus motivos y equipos", () => {
  const bloqueo = (id: string, operacion: string | undefined, motivo: string) =>
    reservaDeCalendario({
      id,
      reservaGrupoId: undefined,
      tipo: "BLOQUEO",
      creadoPor: "admin",
      bloqueoOperacionId: operacion,
      motivoBloqueo: motivo,
    })
  const clases = clasesDelCalendario([
    bloqueo("b1", "op1", "Mantenimiento"),
    bloqueo("b2", "op1", "Mantenimiento"),
    bloqueo("b3", "op2", "Evaluación"),
    bloqueo("b4", undefined, "Histórico"),
    bloqueo("b5", undefined, "Otro histórico"),
  ])
  expect(clases.map((c) => [c.clave, c.motivoBloqueo, c.reservas.length])).toEqual([
    ["op1", "Mantenimiento", 2],
    ["op2", "Evaluación", 1],
    ["b4", "Histórico", 1],
    ["b5", "Otro histórico", 1],
  ])
})

test("un eje sin mínimo conserva la escala aunque haya intervalos cortos", () => {
  const segmentos = segmentosDeClase(
    clasesDelCalendario([
      reservaDeCalendario({ horaInicio: "08:00", horaFin: "08:05" }),
    ])[0]
  )
  const eje = crearEjeSemanal(420, 1140, segmentos, 0)
  expect(eje.alto).toBe(864)
  expect(eje.posicion(480)).toBe(72)
  expect(eje.posicion(485)).toBe(78)
})

test("el eje táctil separa clases cortas consecutivas sin agrandar toda la jornada", () => {
  const clases = clasesDelCalendario([
    reservaDeCalendario({ horaInicio: "08:00", horaFin: "08:05" }),
    reservaDeCalendario({
      id: "r2",
      reservaGrupoId: "g2",
      horaInicio: "08:05",
      horaFin: "08:10",
    }),
  ])
  const eje = crearEjeSemanal(420, 1140, clases.flatMap(segmentosDeClase), 46)
  expect(eje.posicion(485) - eje.posicion(480) - 2).toBeGreaterThanOrEqual(44)
  expect(eje.posicion(490) - eje.posicion(485) - 2).toBeGreaterThanOrEqual(44)
  expect(eje.posicion(487.5)).toBe((eje.posicion(485) + eje.posicion(490)) / 2)
  expect(eje.alto).toBeLessThan(1000)
})

test("la ampliación de una clase breve no agranda el hueco hasta la siguiente", () => {
  const clases = clasesDelCalendario([
    reservaDeCalendario({ horaInicio: "08:00", horaFin: "08:05" }),
    reservaDeCalendario({
      id: "r2",
      reservaGrupoId: "g2",
      horaInicio: "08:06",
      horaFin: "09:00",
    }),
  ])
  const eje = crearEjeSemanal(420, 1140, clases.flatMap(segmentosDeClase), 90)
  expect(eje.posicion(480)).toBe(72)
  expect(eje.posicion(485) - eje.posicion(480)).toBeGreaterThanOrEqual(90)
  expect(eje.posicion(486) - eje.posicion(485)).toBeCloseTo(1.2)
  expect(eje.posicion(540) - eje.posicion(486)).toBeGreaterThanOrEqual(90)
})

test("las clases coincidentes conservan espacio de lectura y una consecutiva no las tapa", () => {
  const clases = clasesDelCalendario([
    reservaDeCalendario({ horaInicio: "08:00", horaFin: "09:00" }),
    reservaDeCalendario({
      id: "r2",
      reservaGrupoId: "g2",
      horaInicio: "08:30",
      horaFin: "09:30",
    }),
    reservaDeCalendario({
      id: "r3",
      reservaGrupoId: "g3",
      horaInicio: "09:30",
      horaFin: "09:35",
    }),
  ])
  const segmentos = clases.flatMap(segmentosDeClase)
  const eje = crearEjeSemanal(420, 1140, segmentos, 90)
  for (const s of segmentos)
    expect(eje.posicion(s.fin) - eje.posicion(s.inicio)).toBeGreaterThanOrEqual(90)
  expect(eje.posicion(480)).toBe(72)
  expect(eje.posicion(570)).toBeGreaterThan(eje.posicion(540))
})
