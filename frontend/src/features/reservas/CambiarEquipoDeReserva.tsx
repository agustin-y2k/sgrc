import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import * as reservasApi from "@/features/reservas/api"
import { puedeCambiarEquipo } from "@/features/reservas/types"
import type { GrupoDeReservas } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"

/** RF-08.14 — cambiar una computadora de una reserva ya hecha. */
export function CambiarEquipoDeReserva({
  grupo,
  onListo,
}: {
  grupo: GrupoDeReservas
  onListo: () => void
}) {
  const queryClient = useQueryClient()
  const cambiables = grupo.reservas.filter(puedeCambiarEquipo)
  const [reservaID, setReservaID] = useState(cambiables[0]?.id ?? "")
  const [equipoID, setEquipoID] = useState("")
  // RF-08.14: el alcance solo se pregunta si hay serie.
  const [soloEsta, setSoloEsta] = useState(true)

  // Con la serie elegida, los equipos que se ofrecen son los libres en TODAS
  // las fechas que faltan: ofrecer los de esta y rechazar el cambio cuando
  // choca en la tercera es hacerle adivinar al docente.
  const serieDesdeGrupoId = !soloEsta && grupo.esRecurrente ? grupo.grupoId : undefined

  // La materia de la reserva que se está cambiando: es lo que ordena la lista
  // (RF-03.21), para que cambiar de máquina ofrezca lo mismo que ofrecería
  // reservar de cero.
  const materiaId = cambiables.find((r) => r.id === reservaID)?.materiaId

  const { data, isPending, isFetching, isSuccess, error, refetch } = useQuery({
    queryKey: [
      "equipos-disponibles",
      grupo.fecha,
      grupo.horaInicio,
      grupo.horaFin,
      serieDesdeGrupoId ?? "",
      materiaId ?? "",
    ],
    queryFn: () =>
      reservasApi.equiposDisponibles({
        fecha: grupo.fecha,
        horaInicio: grupo.horaInicio,
        horaFin: grupo.horaFin,
        serieDesdeGrupoId,
        materiaId,
      }),
  })

  const cambiar = useMutation({
    mutationFn: () => reservasApi.cambiarEquipoDeReserva(reservaID, equipoID, soloEsta),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["reservas"] })
      onListo()
    },
  })

  const disponibles = data?.data ?? []
  const consultaLista = isSuccess && !isFetching && !error
  const puedeCambiar =
    consultaLista &&
    cambiables.some((r) => r.id === reservaID) &&
    disponibles.some((e) => e.equipoId === equipoID) &&
    !cambiar.isPending

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <p className="text-sm">
        Cambiá una de tus computadoras por otra que esté libre en el mismo horario. La
        clase queda igual: no se cancela ni se vuelve a reservar.
      </p>

      {grupo.reservas.some((r) => r.tuvoEntrega) && (
        <p role="status" className="text-sm text-muted-foreground">
          Las computadoras que ya tuvieron una entrega conservan su equipo, aunque se
          hayan devuelto. Para cambiar las próximas fechas, abrí una clase sin entregas.
        </p>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor={`cambiar-de-${grupo.grupoId ?? reservaID}`}>
            ¿Cuál cambiás?
          </Label>
          <Select
            id={`cambiar-de-${grupo.grupoId ?? reservaID}`}
            value={reservaID}
            onChange={(e) => {
              setReservaID(e.target.value)
              setEquipoID("")
            }}
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
            disabled={!consultaLista || disponibles.length === 0}
          >
            <option value="">Elegí una computadora</option>
            {(consultaLista ? disponibles : []).map((equipo) => (
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
          {!error && (isPending || isFetching) && (
            <p role="status" className="text-muted-foreground text-sm">
              Consultando computadoras disponibles…
            </p>
          )}
          {consultaLista && disponibles.length === 0 && (
            <p className="text-muted-foreground text-xs">
              {serieDesdeGrupoId
                ? "No hay ninguna computadora libre en todas las fechas que faltan."
                : "No hay ninguna computadora libre en ese horario."}
            </p>
          )}
        </div>
      </div>

      {error && (
        <ErrorDeConsulta
          mensaje="No pudimos consultar las computadoras disponibles."
          reintentando={isFetching}
          onReintentar={() => {
            void refetch()
          }}
        />
      )}

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
              onChange={() => {
                setSoloEsta(true)
                setEquipoID("")
              }}
            />
            <Label
              htmlFor={`alcance-esta-${grupo.grupoId ?? reservaID}`}
              className="font-normal"
            >
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
              onChange={() => {
                setSoloEsta(false)
                setEquipoID("")
              }}
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

      {/* h-11 en teléfono, igual que los botones que abren estos paneles
          (ver TarjetaDeClase): 44px es el mínimo táctil de WCAG 2.5.5. El
          criterio se había aplicado al primer paso del flujo y se perdía en
          el segundo, que es donde se confirma de verdad — el `sm` del
          sistema son 28px, la mitad de un dedo. */}
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          className="h-11 px-4 sm:h-9"
          disabled={!puedeCambiar}
          onClick={() => {
            if (puedeCambiar) cambiar.mutate()
          }}
        >
          Cambiar
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="h-11 px-4 sm:h-9"
          onClick={onListo}
        >
          Cancelar
        </Button>
      </div>
    </div>
  )
}
