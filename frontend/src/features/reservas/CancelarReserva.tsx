import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuth } from "@/features/auth/AuthContext"
import * as reservasApi from "@/features/reservas/api"
import type { GrupoDeReservas } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-04.6 / RF-04.8 — el panel de confirmación para cancelar una reserva.
 *
 * Vive en un componente y no adentro del listado porque se abre desde dos
 * lugares: la pantalla de reservas y la de inicio, donde el docente ve su
 * próxima clase. Dos copias del mismo panel es cómo termina pasando que en
 * una el motivo sea obligatorio y en la otra no, o que una cancele el grupo
 * entero y la otra un solo equipo.
 *
 * Cancelar es destructivo y sin deshacer, así que se confirma siempre: el
 * botón de la tarjeta abre esto, no dispara la cancelación.
 */
export function CancelarReserva({
  grupo,
  onListo,
}: {
  grupo: GrupoDeReservas
  onListo: () => void
}) {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  /** RF-04.6: true = solo esta fecha; false = esta y todas las siguientes. */
  const [soloEsta, setSoloEsta] = useState(true)
  const [motivo, setMotivo] = useState("")

  // El id de los campos tiene que ser único en la página: en inicio puede
  // haber varias tarjetas abiertas a la vez, y dos radios con el mismo
  // `name` se comportan como un solo grupo entre tarjetas distintas.
  const clave = grupo.grupoId ?? grupo.reservas[0]?.id ?? grupo.fecha

  // RF-04.8: el motivo solo es obligatorio si la reserva es de otra
  // persona, que es el caso de un Admin cancelando la de un docente.
  const esAjena = grupo.creadoPor !== user?.id
  const motivoFalta = esAjena && motivo.trim() === ""
  const cancelables = grupo.reservas.filter((r) => r.estado === "CONFIRMADA")

  const cancelar = useMutation({
    // RF-04.6: la elección se aplica "a todos los equipos del grupo en esa
    // fecha (o rango)", así que las dos ramas cancelan el grupo entero —
    // lo único que cambia es si además alcanza a las fechas siguientes.
    // Antes "solo esta fecha" llamaba a cancelarReserva y liberaba un
    // solo equipo, que no es lo que dice el requisito ni lo que sugiere el
    // texto de la opción.
    mutationFn: async (): Promise<void> => {
      if (grupo.grupoId) {
        await reservasApi.cancelarGrupo(grupo.grupoId, motivo, soloEsta)
        return
      }
      // Un bloqueo administrativo son N filas `reserva` sueltas, sin grupo
      // que las una en la base. Se cancelan TODAS: la tarjeta representa la
      // operación completa, y liberar solo un equipo dejaría el aula a medio
      // bloquear sin que nada lo diga. En serie y no en paralelo para que un
      // fallo a mitad de camino sea un error claro y no una carrera.
      for (const r of cancelables) {
        await reservasApi.cancelarReserva(r.id, motivo)
      }
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["reservas"] })
      onListo()
    },
  })

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <p className="text-sm">
        {grupo.esBloqueo
          ? `Se libera${cancelables.length === 1 ? "" : "n"} ${cancelables.length} equipo${cancelables.length === 1 ? "" : "s"} de este bloqueo. Vuelven a estar disponibles para reservar.`
          : `Se ${cancelables.length === 1 ? "cancela" : "cancelan"} ${cancelables.length} equipo${cancelables.length === 1 ? "" : "s"} de esta reserva.`}
      </p>

      {/* El error de la cancelación va acá y no arriba de la pantalla: es la
          respuesta a este botón, y a un metro de distancia del panel nadie
          lo relaciona con lo que acaba de apretar. */}
      {cancelar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(cancelar.error)}</AlertDescription>
        </Alert>
      )}

      {/* RF-04.6: elegir entre esta fecha o esta y las siguientes solo tiene
          sentido si es recurrente. */}
      {grupo.esRecurrente && (
        <div className="grid gap-1.5">
          <span className="text-sm font-medium">¿Qué querés cancelar?</span>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="radio"
              name={`alcance-${clave}`}
              checked={soloEsta}
              onChange={() => setSoloEsta(true)}
            />
            Solo esta fecha
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="radio"
              name={`alcance-${clave}`}
              checked={!soloEsta}
              onChange={() => setSoloEsta(false)}
            />
            Esta fecha y todas las siguientes
          </label>
        </div>
      )}

      <div className="grid gap-1.5">
        <Label htmlFor={`motivo-${clave}`}>
          Motivo {esAjena ? "(obligatorio)" : "(opcional)"}
        </Label>
        <Input
          id={`motivo-${clave}`}
          value={motivo}
          onChange={(e) => setMotivo(e.target.value)}
          placeholder={
            esAjena ? "El docente va a recibir este texto en la notificación" : ""
          }
        />
        {motivoFalta && (
          <p className="text-destructive text-sm">
            Al cancelar la reserva de otra persona el motivo es obligatorio.
          </p>
        )}
      </div>

      <div className="flex gap-2">
        <Button
          variant="destructive"
          size="sm"
          disabled={motivoFalta || cancelar.isPending}
          onClick={() => cancelar.mutate()}
        >
          Confirmar cancelación
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={cancelar.isPending}
          onClick={onListo}
        >
          Volver
        </Button>
      </div>
    </div>
  )
}
