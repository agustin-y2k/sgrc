import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { EstadoBadge, TONO_GRAVEDAD, TONO_INCIDENCIA } from "@/components/EstadoBadge"
import { Button } from "@/components/ui/button"
import * as adminApi from "@/features/admin/api"
import * as inventoryApi from "@/features/inventory/api"
import type { EstadoIncidencia, Incidencia } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

/** RF-03.5/RF-03.6 — el historial de fallas de un equipo y su gestión. */

const ETIQUETA_ESTADO_INCIDENCIA: Record<EstadoIncidencia, string> = {
  ABIERTA: "Abierta",
  EN_REPARACION: "En reparación",
  ENVIADA_A_SOPORTE: "Enviada a soporte técnico",
  RESUELTA: "Resuelta",
}

const ETIQUETA_GRAVEDAD = {
  LEVE: "Leve",
  MODERADA: "Moderada",
  GRAVE: "Grave",
} as const

function formatearFecha(iso: string): string {
  return new Date(iso).toLocaleDateString("es-AR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  })
}

export function IncidenciasDeEquipo({ equipoId }: { equipoId: string }) {
  const queryClient = useQueryClient()
  const incidenciasKey = ["incidencias", equipoId]

  const { data, isLoading, error } = useQuery({
    queryKey: incidenciasKey,
    queryFn: () => inventoryApi.listarIncidenciasDeEquipo(equipoId),
  })

  const editar = useMutation({
    mutationFn: ({
      incidencia,
      estado,
      marcarEnviadaASoporte,
    }: {
      incidencia: Incidencia
      estado?: EstadoIncidencia
      marcarEnviadaASoporte?: boolean
    }) => adminApi.editarIncidencia(incidencia.id, { estado, marcarEnviadaASoporte }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: incidenciasKey })
      // Los reportes de RF-06 agregan por estado: si no se invalidan, el
      // Admin cierra una incidencia acá y el reporte la sigue contando
      // abierta hasta que recargue la página.
      await queryClient.invalidateQueries({ queryKey: ["reporte"] })
    },
  })

  if (isLoading) {
    return <p className="text-muted-foreground text-sm">Cargando incidencias…</p>
  }

  const incidencias = data?.data ?? []
  const fallo = error ?? editar.error

  return (
    <div className="grid gap-2">
      {fallo && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(fallo)}</AlertDescription>
        </Alert>
      )}

      {incidencias.length === 0 && (
        <p className="text-muted-foreground text-sm">
          Este equipo no tiene incidencias reportadas.
        </p>
      )}

      {incidencias.map((i) => (
        <div key={i.id} className="grid gap-2 rounded-md border p-3">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <p className="text-sm">{i.descripcion}</p>
            <div className="flex flex-wrap gap-1.5">
              <EstadoBadge tono={TONO_GRAVEDAD[i.gravedad]}>
                {ETIQUETA_GRAVEDAD[i.gravedad]}
              </EstadoBadge>
              <EstadoBadge tono={TONO_INCIDENCIA[i.estado]}>
                {ETIQUETA_ESTADO_INCIDENCIA[i.estado]}
              </EstadoBadge>
            </div>
          </div>

          <p className="text-muted-foreground text-xs">
            Reportada el {formatearFecha(i.fecha)}
            {i.enviadoASoporte &&
              i.fechaEnvioASoporte &&
              ` · enviada a soporte el ${formatearFecha(i.fechaEnvioASoporte)}`}
          </p>

          {/* El backend no impone una máquina de estados: acepta cualquiera
              de los cuatro valores. Se ofrecen los que no son el actual, en
              el orden del recorrido esperado, sin bloquear ninguno — quien
              gestiona el equipo sabe mejor que la pantalla si algo volvió
              para atrás. */}
          <div className="flex flex-wrap gap-2">
            {(["ABIERTA", "EN_REPARACION", "RESUELTA"] as EstadoIncidencia[])
              .filter((e) => e !== i.estado)
              .map((e) => (
                <Button
                  key={e}
                  variant="outline"
                  size="sm"
                  disabled={editar.isPending}
                  onClick={() => editar.mutate({ incidencia: i, estado: e })}
                >
                  → {ETIQUETA_ESTADO_INCIDENCIA[e]}
                </Button>
              ))}
            {/* RF-03.6: marcar el envío a soporte técnico guarda la
                fecha, que es el dato que después hay que poder mostrar. Por
                eso es un botón aparte y no un estado más del grupo de
                arriba: el backend, con marcarEnviadaASoporte, además de mover el
                estado registra el día. */}
            {!i.enviadoASoporte && (
              <Button
                variant="outline"
                size="sm"
                disabled={editar.isPending}
                onClick={() =>
                  editar.mutate({ incidencia: i, marcarEnviadaASoporte: true })
                }
              >
                Marcar enviada a soporte
              </Button>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}
