import { Link } from "react-router"

import { EstadoBadge, TONO_LICENCIA } from "@/components/EstadoBadge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import * as adminApi from "@/features/admin/api"
import type { Licencia } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaLarga } from "@/lib/fechas"
import { useQuery } from "@tanstack/react-query"

/**
 * RF-03.11 — las licencias de un equipo, dentro de la ficha de ese equipo.
 *
 * Es de solo lectura a propósito. Cargarlas, renovarlas y editarlas se hace
 * en /admin/licencias, donde las acciones son masivas: el mismo software
 * está en las ocho máquinas del carro y se renueva de una sola vez. Repetir
 * acá el alta y la renovación de a una invitaría a hacer ocho veces el
 * trabajo que la otra pantalla resuelve en uno, y a que las ocho fechas
 * queden desparejas sin motivo.
 *
 * Lo que sí aporta acá: mirando un equipo concreta —porque falló, porque un
 * docente preguntó por ella— se ve de una si su software está al día.
 */

const ETIQUETA_ESTADO: Record<string, string> = {
  SIN_FECHA: "Falta cargar el vencimiento",
  VENCIDA: "Vencida",
  POR_VENCER: "Por vencer",
  VIGENTE: "Vigente",
}

function contador(l: Licencia): string {
  if (l.diasRestantes == null) return "sin fecha de vencimiento"
  const d = l.diasRestantes
  if (d > 1) return `vence en ${d} días`
  if (d === 1) return "vence mañana"
  if (d === 0) return "vence hoy"
  if (d === -1) return "venció ayer"
  return `venció hace ${-d} días`
}

export function LicenciasDeEquipo({ equipoId }: { equipoId: string }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["licencias", "equipo", equipoId],
    queryFn: () => adminApi.listarLicenciasDeEquipo(equipoId),
  })

  if (isLoading) {
    return <p className="text-muted-foreground text-sm">Cargando licencias…</p>
  }
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{getErrorMessage(error)}</AlertDescription>
      </Alert>
    )
  }

  const licencias = data?.data ?? []

  return (
    <div className="grid gap-2 rounded-md border border-dashed p-3">
      {licencias.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          Este equipo no tiene licencias con vencimiento cargadas.
        </p>
      ) : (
        <ul className="grid gap-1 text-sm">
          {licencias.map((l) => (
            <li key={l.id} className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{l.nombre}</span>
              <EstadoBadge tono={TONO_LICENCIA[l.estado] ?? "neutro"}>
                {ETIQUETA_ESTADO[l.estado] ?? l.estado}
              </EstadoBadge>
              <span className="text-muted-foreground">
                {contador(l)}
                {l.fechaVencimiento && ` · ${formatearFechaLarga(l.fechaVencimiento)}`}
              </span>
            </li>
          ))}
        </ul>
      )}
      <div>
        <Button asChild variant="outline" size="sm">
          <Link to="/admin/licencias">Administrar licencias</Link>
        </Button>
      </div>
    </div>
  )
}
