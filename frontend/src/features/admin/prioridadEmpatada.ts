import type { PreferenciaDeEquipo } from "@/features/inventory/types"

/** Dos nombres de materia son el mismo sin tildes, mayúsculas ni espacios de más. */
function esLaMisma(a: string, b: string) {
  return a.trim().localeCompare(b.trim(), "es", { sensitivity: "base" }) === 0
}

/**
 * Las OTRAS materias que ya usan esa prioridad en esos equipos.
 *
 * Existe porque la prioridad arranca en 1 en la base y en el formulario: dos
 * materias marcadas sin tocar el número empatan, y un empate significa que
 * ninguna le gana a la otra. Es un estado legítimo, así que esto avisa y no
 * bloquea — pero sin el aviso, quien marca cree haber establecido una
 * preferencia que no existe.
 */
export function materiasConLaMismaPrioridad(
  marcas: PreferenciaDeEquipo[],
  materiaNombre: string,
  prioridad: number,
): string[] {
  const nombres: string[] = []
  for (const m of marcas) {
    if (m.prioridad !== prioridad) continue
    if (esLaMisma(m.materiaNombre, materiaNombre)) continue
    if (nombres.some((n) => esLaMisma(n, m.materiaNombre))) continue
    nombres.push(m.materiaNombre)
  }
  return nombres
}
