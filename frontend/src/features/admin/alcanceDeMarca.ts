import type { MateriaParaPreferencia } from "@/features/inventory/types"
import { sinTildes } from "@/lib/texto"

export const SIN_ATRIBUTO = "__SIN_ATRIBUTO__"
export type ValoresDeMarca = {
  materiaNombre: string
  modalidad: string
  anio: string
  division: string
  prioridad: string
  alcance: "CURSO" | "GENERAL"
}
export const MARCA_VACIA: ValoresDeMarca = {
  materiaNombre: "",
  modalidad: "",
  anio: "",
  division: "",
  prioridad: "1",
  alcance: "CURSO",
}
export function mismoTexto(a: string, b: string) {
  return sinTildes(a.trim()) === sinTildes(b.trim())
}
export function textoDeEje(v: string) {
  return v === SIN_ATRIBUTO ? "" : v
}
export function coincidenEjes(f: MateriaParaPreferencia, v: ValoresDeMarca) {
  return (
    (v.alcance !== "CURSO" || f.tipo === "CURSO") &&
    (!v.modalidad || mismoTexto(f.modalidad ?? "", textoDeEje(v.modalidad))) &&
    (!v.anio || f.anio === Number(v.anio)) &&
    (!v.division || mismoTexto(f.division ?? "", textoDeEje(v.division)))
  )
}
export function marcaValida(v: ValoresDeMarca, catalogo: MateriaParaPreferencia[]) {
  if (
    !v.materiaNombre ||
    (v.alcance === "CURSO" && (!v.modalidad || !v.anio || !v.division))
  )
    return false
  return catalogo.some(
    (f) => coincidenEjes(f, v) && mismoTexto(f.materiaNombre, v.materiaNombre)
  )
}
export function datosDeMarca(v: ValoresDeMarca) {
  return {
    materiaNombre: v.materiaNombre,
    modalidad: textoDeEje(v.modalidad).trim() || undefined,
    anio: v.anio ? Number(v.anio) : undefined,
    division: textoDeEje(v.division).trim() || undefined,
    prioridad: Number(v.prioridad),
    cursoExacto: v.alcance === "CURSO",
  }
}
