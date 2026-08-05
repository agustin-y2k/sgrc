import { useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import * as reservasApi from "@/features/reservas/api"
import type { PCDisponible } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"

type Props = {
  fecha: string
  horaInicio: string
  horaFin: string
  seleccionadas: string[]
  onCambio: (pcIds: string[]) => void
}

/**
 * RF-04.2: "selecciona PCs de una lista (como tildar casillas) hasta juntar
 * la cantidad que necesita — la lista no está restringida a un solo carro".
 * Por eso se agrupa por carro pero la selección es única y cruzada.
 *
 * Solo consulta cuando la franja está completa: pedir disponibilidad sin
 * fecha u horario no tiene sentido y el backend responde 400.
 */
export function SelectorDePCs({
  fecha,
  horaInicio,
  horaFin,
  seleccionadas,
  onCambio,
}: Props) {
  const franjaCompleta = Boolean(fecha && horaInicio && horaFin && horaFin > horaInicio)

  const { data, isLoading, error } = useQuery({
    queryKey: ["pcs-disponibles", fecha, horaInicio, horaFin],
    queryFn: () => reservasApi.pcsDisponibles(fecha, horaInicio, horaFin),
    enabled: franjaCompleta,
  })

  if (!franjaCompleta) {
    return (
      <p className="text-muted-foreground text-sm">
        Elegí la fecha y el horario para ver qué PCs están libres.
      </p>
    )
  }
  if (isLoading)
    return <p className="text-muted-foreground text-sm">Buscando PCs libres…</p>
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{getErrorMessage(error)}</AlertDescription>
      </Alert>
    )
  }

  const pcs = data?.data ?? []
  if (pcs.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        No hay ninguna PC libre en esa franja.
      </p>
    )
  }

  // Agrupadas por carro solo para que sea legible; la selección cruza carros.
  const porCarro = new Map<string, PCDisponible[]>()
  for (const pc of pcs) {
    const delCarro = porCarro.get(pc.carroNombre)
    if (delCarro) delCarro.push(pc)
    else porCarro.set(pc.carroNombre, [pc])
  }

  function alternar(pcId: string, tildada: boolean) {
    onCambio(
      tildada ? [...seleccionadas, pcId] : seleccionadas.filter((id) => id !== pcId)
    )
  }

  return (
    <div className="grid gap-4">
      <p className="text-muted-foreground text-sm">
        {seleccionadas.length === 0
          ? `${pcs.length} PC(s) libres en esa franja.`
          : `${seleccionadas.length} de ${pcs.length} seleccionada(s).`}
      </p>

      {[...porCarro.entries()].map(([carro, delCarro]) => (
        <fieldset key={carro} className="grid gap-2">
          <legend className="mb-1 text-sm font-medium">{carro}</legend>
          <div className="grid gap-2 sm:grid-cols-2">
            {delCarro.map((pc) => {
              const id = `pc-${pc.pcId}`
              return (
                <div
                  key={pc.pcId}
                  className="flex items-start gap-2 rounded-md border p-2"
                >
                  <Checkbox
                    id={id}
                    checked={seleccionadas.includes(pc.pcId)}
                    onCheckedChange={(v) => alternar(pc.pcId, v === true)}
                  />
                  <div className="grid gap-0.5">
                    <Label htmlFor={id} className="cursor-pointer">
                      PC {pc.identificador}
                      {pc.freezado && (
                        <Badge variant="outline" className="ml-1">
                          Freezada
                        </Badge>
                      )}
                    </Label>
                    {/* RF-03.7: el software es lo que define la elección. */}
                    {pc.softwareInstalado && (
                      <span className="text-muted-foreground text-xs">
                        {pc.softwareInstalado}
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
