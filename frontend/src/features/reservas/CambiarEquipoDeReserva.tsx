import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as reservasApi from "@/features/reservas/api"
import type { GrupoDeReservas } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-08.14 — cambiar una computadora de una reserva ya hecha.
 *
 * Sirve cuando el sistema avisa que una máquina no volvió al laboratorio y
 * puede no estar para tu clase. Hasta ahora la única salida era cancelar esa
 * Equipo y hacer otra reserva, que arma un grupo nuevo: la misma clase terminaba
 * mostrada como dos tarjetas separadas en esta misma pantalla.
 *
 * Solo se ofrecen los equipos libres en esa franja. La lista sale del mismo
 * endpoint que usa el formulario de reserva, así que lo que se ve acá es lo
 * mismo que se vería al reservar de cero.
 */
export function CambiarEquipoDeReserva({
  grupo,
  onListo,
}: {
  grupo: GrupoDeReservas
  onListo: () => void
}) {
  const queryClient = useQueryClient()
  const cambiables = grupo.reservas.filter((r) => r.estado === "CONFIRMADA")
  const [reservaID, setReservaID] = useState(cambiables[0]?.id ?? "")
  const [equipoID, setEquipoID] = useState("")
  // RF-08.14: el alcance solo se pregunta si hay serie. En una reserva
  // suelta las dos opciones significan lo mismo, y ofrecer la pregunta
  // igual haría dudar sobre algo que no tiene consecuencias.
  const [soloEsta, setSoloEsta] = useState(true)

  // Con la serie elegida, los equipos que se ofrecen son los libres en TODAS
  // las fechas que faltan: ofrecer los de esta y rechazar el cambio cuando
  // choca en la tercera es hacerle adivinar al docente.
  const serieDesdeGrupoId = !soloEsta && grupo.esRecurrente ? grupo.grupoId : undefined

  const { data, isLoading } = useQuery({
    queryKey: [
      "equipos-disponibles",
      grupo.fecha,
      grupo.horaInicio,
      grupo.horaFin,
      serieDesdeGrupoId ?? "",
    ],
    queryFn: () =>
      reservasApi.equiposDisponibles(
        grupo.fecha,
        grupo.horaInicio,
        grupo.horaFin,
        serieDesdeGrupoId
      ),
  })

  const cambiar = useMutation({
    mutationFn: () => reservasApi.cambiarEquipoDeReserva(reservaID, equipoID, soloEsta),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["reservas"] })
      onListo()
    },
  })

  const disponibles = data?.data ?? []

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <p className="text-sm">
        Cambiá una de tus computadoras por otra que esté libre en el mismo horario. La
        clase queda igual: no se cancela ni se vuelve a reservar.
      </p>

      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor={`cambiar-de-${grupo.grupoId ?? reservaID}`}>¿Cuál cambiás?</Label>
          <Select
            id={`cambiar-de-${grupo.grupoId ?? reservaID}`}
            value={reservaID}
            onChange={(e) => setReservaID(e.target.value)}
          >
            {cambiables.map((r) => (
              <option key={r.id} value={r.id}>
                {r.etiqueta}
                {r.carroNombre && ` · ${r.carroNombre}`}
              </option>
            ))}
          </Select>
        </div>

        <div className="grid gap-1.5">
          <Label htmlFor={`cambiar-a-${grupo.grupoId ?? reservaID}`}>¿Por cuál?</Label>
          <Select
            id={`cambiar-a-${grupo.grupoId ?? reservaID}`}
            value={equipoID}
            onChange={(e) => setEquipoID(e.target.value)}
            disabled={isLoading || disponibles.length === 0}
          >
            <option value="">Elegí una computadora</option>
            {disponibles.map((equipo) => (
              <option key={equipo.equipoId} value={equipo.equipoId}>
                {equipo.etiqueta}
                {equipo.carroNombre && ` · ${equipo.carroNombre}`}
                {equipo.softwareInstalado ? ` · ${equipo.softwareInstalado}` : ""}
              </option>
            ))}
          </Select>
          {/* El software instalado va en la etiqueta porque es el dato por
              el que se elige una máquina (RF-03.7): cambiar a una que no
              tenga el programa de la clase no resuelve nada. */}
          {!isLoading && disponibles.length === 0 && (
            <p className="text-muted-foreground text-xs">
              No hay ninguna computadora libre en ese horario.
            </p>
          )}
        </div>
      </div>

      {grupo.esRecurrente && (
        <fieldset className="grid gap-2">
          <legend className="mb-1 text-sm font-medium">¿Hasta cuándo?</legend>
          <div className="flex items-start gap-2">
            <input
              type="radio"
              id={`alcance-esta-${grupo.grupoId ?? reservaID}`}
              name={`alcance-cambio-${grupo.grupoId ?? reservaID}`}
              className="mt-1"
              checked={soloEsta}
              onChange={() => setSoloEsta(true)}
            />
            <Label htmlFor={`alcance-esta-${grupo.grupoId ?? reservaID}`} className="font-normal">
              Solo esta fecha
            </Label>
          </div>
          <div className="flex items-start gap-2">
            <input
              type="radio"
              id={`alcance-siguientes-${grupo.grupoId ?? reservaID}`}
              name={`alcance-cambio-${grupo.grupoId ?? reservaID}`}
              className="mt-1"
              checked={!soloEsta}
              onChange={() => setSoloEsta(false)}
            />
            <Label
              htmlFor={`alcance-siguientes-${grupo.grupoId ?? reservaID}`}
              className="font-normal"
            >
              Esta fecha y todas las siguientes
            </Label>
          </div>
          {!soloEsta && (
            <p className="text-muted-foreground text-xs">
              Solo se ofrecen las computadoras libres en todas las fechas que faltan. Si
              alguna fecha choca, no se cambia ninguna.
            </p>
          )}
        </fieldset>
      )}

      {cambiar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(cambiar.error)}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          disabled={cambiar.isPending || !reservaID || !equipoID}
          onClick={() => cambiar.mutate()}
        >
          Cambiar
        </Button>
        <Button variant="outline" size="sm" onClick={onListo}>
          Cancelar
        </Button>
      </div>
    </div>
  )
}
