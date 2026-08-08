import { useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import * as reservasApi from "@/features/reservas/api"
import type { EquipoDisponible } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * Título del grupo de lo que no cuelga de ningún carro. Es lo que ve el
 * docente arriba del proyector cuando va a reservar.
 */
const SIN_CARRO = "Otros equipos"

type Props = {
  fecha: string
  horaInicio: string
  horaFin: string
  seleccionadas: string[]
  onCambio: (equipoIds: string[]) => void
}

/**
 * RF-04.2: "selecciona Equipos de una lista (como tildar casillas) hasta juntar
 * la cantidad que necesita — la lista no está restringida a un solo carro".
 * Por eso se agrupa por carro pero la selección es única y cruzada.
 *
 * Solo consulta cuando la franja está completa: pedir disponibilidad sin
 * fecha u horario no tiene sentido y el backend responde 400.
 */
export function SelectorDeEquipos({
  fecha,
  horaInicio,
  horaFin,
  seleccionadas,
  onCambio,
}: Props) {
  const franjaCompleta = Boolean(fecha && horaInicio && horaFin && horaFin > horaInicio)

  const { data, isLoading, error } = useQuery({
    queryKey: ["equipos-disponibles", fecha, horaInicio, horaFin],
    queryFn: () => reservasApi.equiposDisponibles(fecha, horaInicio, horaFin),
    enabled: franjaCompleta,
  })

  if (!franjaCompleta) {
    return (
      <p className="text-muted-foreground text-sm">
        Elegí la fecha y el horario para ver qué Equipos están libres.
      </p>
    )
  }
  if (isLoading)
    return <p className="text-muted-foreground text-sm">Buscando Equipos libres…</p>
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{getErrorMessage(error)}</AlertDescription>
      </Alert>
    )
  }

  const equipos = data?.data ?? []
  if (equipos.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        No hay ningún equipo libre en esa franja.
      </p>
    )
  }

  // Agrupadas por carro solo para que sea legible; la selección cruza carros.
  //
  // Lo que no está en ningún carro (015 — el proyector) va bajo su propio
  // título: con `carroNombre` vacío caía en un grupo sin leyenda, y ahí no
  // hay nada que le diga al docente qué está mirando.
  const porCarro = new Map<string, EquipoDisponible[]>()
  for (const equipo of equipos) {
    const grupo = equipo.carroNombre || SIN_CARRO
    const delCarro = porCarro.get(grupo)
    if (delCarro) delCarro.push(equipo)
    else porCarro.set(grupo, [equipo])
  }

  function alternar(equipoId: string, tildada: boolean) {
    onCambio(
      tildada ? [...seleccionadas, equipoId] : seleccionadas.filter((id) => id !== equipoId)
    )
  }

  return (
    <div className="grid gap-4">
      <p className="text-muted-foreground text-sm">
        {seleccionadas.length === 0
          ? `${equipos.length} equipo(s) libres en esa franja.`
          : `${seleccionadas.length} de ${equipos.length} seleccionada(s).`}
      </p>

      {[...porCarro.entries()].map(([carro, delCarro]) => (
        <fieldset key={carro} className="grid gap-2">
          <legend className="mb-1 text-sm font-medium">{carro}</legend>
          <div className="grid gap-2 sm:grid-cols-2">
            {delCarro.map((equipo) => {
              const id = `equipo-${equipo.equipoId}`
              return (
                <div
                  key={equipo.equipoId}
                  className="flex items-start gap-2 rounded-md border p-2"
                >
                  <Checkbox
                    id={id}
                    checked={seleccionadas.includes(equipo.equipoId)}
                    onCheckedChange={(v) => alternar(equipo.equipoId, v === true)}
                  />
                  <div className="grid gap-0.5">
                    <Label htmlFor={id} className="cursor-pointer">
                      {equipo.etiqueta}
                      {equipo.freezado && (
                        <Badge variant="outline" className="ml-1">
                          Freezada
                        </Badge>
                      )}
                    </Label>
                    {/* RF-03.7: el software es lo que define la elección. */}
                    {equipo.softwareInstalado && (
                      <span className="text-muted-foreground text-xs">
                        {equipo.softwareInstalado}
                      </span>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        </fieldset>
      ))}
    </div>
  )
}
