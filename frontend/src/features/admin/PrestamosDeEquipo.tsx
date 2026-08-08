import { useQuery } from "@tanstack/react-query"

import { EstadoBadge } from "@/components/EstadoBadge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import * as reservasApi from "@/features/reservas/api"
import type { Prestamo } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-08.8 — el historial de entregas de una máquina, dentro de su ficha.
 *
 * Es de solo lectura: entregar y recibir se hace en /admin/entregas, que es
 * donde está la gente esperando. Lo que aporta acá es lo que no se ve en
 * ningún otro lado — las observaciones de cada devolución ("volvió sin el
 * cargador"), que son justo lo que se consulta cuando un equipo aparece con un
 * problema y hay que reconstruir por dónde anduvo.
 */

function cuando(iso: string): string {
  return new Date(iso).toLocaleString("es-AR", {
    day: "2-digit",
    month: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
}

function estadoDelPrestamo(p: Prestamo) {
  if (!p.abierto) return null
  if (p.demorado) return <EstadoBadge tono="peligro">Sin devolver</EstadoBadge>
  return <EstadoBadge tono="alerta">Afuera</EstadoBadge>
}

export function PrestamosDeEquipo({ equipoId }: { equipoId: string }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["prestamos", "equipo", equipoId],
    queryFn: () => reservasApi.historialDePrestamosDeEquipo(equipoId),
  })

  if (isLoading) {
    return <p className="text-muted-foreground text-sm">Cargando entregas…</p>
  }
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{getErrorMessage(error)}</AlertDescription>
      </Alert>
    )
  }

  const prestamos = data?.data ?? []

  return (
    <div className="grid gap-2 rounded-md border border-dashed p-3">
      {prestamos.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          Esta computadora nunca salió del laboratorio.
        </p>
      ) : (
        <ul className="grid gap-2 text-sm">
          {prestamos.map((p) => (
            <li key={p.id} className="grid gap-0.5">
              <span className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{p.entregadoANombre}</span>
                {estadoDelPrestamo(p)}
              </span>
              <span className="text-muted-foreground text-xs">
                Salió {cuando(p.entregadoEn)}
                {p.devueltoEn ? ` · volvió ${cuando(p.devueltoEn)}` : " · todavía afuera"}
                {p.materiaNombre && ` · ${p.materiaNombre}`}
                {p.motivo && ` · ${p.motivo}`}
              </span>
              {/* La observación es el renglón al margen del papel, y el
                  único lugar del sistema donde queda escrita. */}
              {p.observaciones && (
                <span className="text-foreground text-xs">↳ {p.observaciones}</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
