import { useEffect } from "react"

export function useEnfocarEquipo(
  equipoId: string | null | undefined,
  disponible: boolean
) {
  useEffect(() => {
    if (!equipoId || !disponible) return
    const elemento = document.getElementById(`equipo-inventario-${equipoId}`)
    elemento?.scrollIntoView?.({ block: "center", behavior: "smooth" })
    elemento?.focus({ preventScroll: true })
  }, [equipoId, disponible])
}
