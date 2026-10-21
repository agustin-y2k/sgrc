import { z } from "zod"
import { BORRADOR_RESERVA_KEY, borrarBorradorReserva } from "@/lib/borradores"

const id = z.string().max(64)
const fecha = z
  .string()
  .max(10)
  .refine(
    (v) =>
      !v ||
      (/^\d{4}-\d{2}-\d{2}$/.test(v) &&
        Number.isFinite(Date.parse(`${v}T00:00:00Z`)) &&
        new Date(`${v}T00:00:00Z`).toISOString().slice(0, 10) === v)
  )
const hora = z.string().refine((v) => !v || /^([01]\d|2[0-3]):[0-5]\d$/.test(v))
const datos = z.object({
  modo: z.enum(["simple", "recurrente"]),
  materiaId: id,
  busquedaMateria: z.string().max(200),
  volverA: z
    .string()
    .max(2000)
    .refine((v) => /^\/reservas(?:\?|$)/.test(v))
    .default("/reservas"),
  fecha,
  horaInicio: hora,
  horaFin: hora,
  diaSemana: z.enum([
    "LUNES",
    "MARTES",
    "MIERCOLES",
    "JUEVES",
    "VIERNES",
    "SABADO",
    "DOMINGO",
  ]),
  fechaInicio: fecha,
  fechaFin: fecha,
  seleccion: z
    .object({
      clave: z.string().max(1024),
      ids: z
        .array(id)
        .max(1000)
        .transform((ids) => [...new Set(ids)]),
      aTomar: z
        .array(id)
        .max(1000)
        .transform((ids) => [...new Set(ids)]),
    })
    .refine((s) => s.aTomar.every((i) => s.ids.includes(i))),
})
const esquema = z.object({ version: z.literal(1), usuarioId: id, datos })
export type BorradorReserva = z.infer<typeof datos>

export function leerBorradorReserva(
  usuarioId: string | undefined
): BorradorReserva | null {
  if (!usuarioId) return null
  try {
    const texto = sessionStorage.getItem(BORRADOR_RESERVA_KEY)
    if (!texto) return null
    const valor = esquema.safeParse(JSON.parse(texto))
    if (!valor.success || valor.data.usuarioId !== usuarioId) {
      borrarBorradorReserva()
      return null
    }
    return valor.data.datos
  } catch {
    borrarBorradorReserva()
    return null
  }
}

export function hayDatosDeReserva(d: BorradorReserva): boolean {
  return !!(
    d.modo !== "simple" ||
    d.materiaId ||
    d.fecha ||
    d.horaInicio ||
    d.horaFin ||
    d.fechaInicio ||
    d.fechaFin ||
    d.seleccion.ids.length ||
    d.busquedaMateria
  )
}

export function guardarBorradorReserva(
  usuarioId: string | undefined,
  d: BorradorReserva
): boolean {
  if (!usuarioId) return true
  if (!hayDatosDeReserva(d)) {
    borrarBorradorReserva()
    return true
  }
  try {
    sessionStorage.setItem(
      BORRADOR_RESERVA_KEY,
      JSON.stringify({ version: 1, usuarioId, datos: d })
    )
    return true
  } catch {
    return false
  }
}
