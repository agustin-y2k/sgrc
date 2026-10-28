import type { Prestamo } from "./types"

/** El contexto histórico tiene prioridad sobre los datos académicos actuales. */
export function detalleAcademicoDePrestamo(p: Prestamo): string {
  const c = p.contextoAcademico
  if (!c) return [p.materiaNombre, p.destino].filter(Boolean).join(" · ")
  const lugar = [c.contenedorNombre, c.modalidad].filter(Boolean).join(" · ")
  return [
    c.materiaNombre,
    lugar !== c.materiaNombre ? lugar : undefined,
    `Ciclo ${c.cicloAnio}`,
    p.destino && p.destino !== lugar ? p.destino : undefined,
  ]
    .filter(Boolean)
    .join(" · ")
}
