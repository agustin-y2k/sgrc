import { useState } from "react"
import { useMutation, useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import * as reservasApi from "@/features/reservas/api"
import type { EquipoDisponible, EquipoOcupado } from "@/features/reservas/types"
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
 * RF-04.2: "selecciona equipos de una lista (como tildar casillas) hasta juntar
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
        Elegí la fecha y el horario para ver qué equipos están libres.
      </p>
    )
  }
  if (isLoading)
    return <p className="text-muted-foreground text-sm">Buscando equipos libres…</p>
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{getErrorMessage(error)}</AlertDescription>
      </Alert>
    )
  }

  const equipos = data?.data ?? []
  const ocupados = data?.ocupados ?? []

  if (equipos.length === 0 && ocupados.length === 0) {
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
          : `${seleccionadas.length} de ${equipos.length} seleccionado(s).`}
      </p>

      {equipos.length === 0 && (
        <p className="text-sm">
          No queda ninguno libre en esa franja, pero abajo está quién tiene cada uno.
        </p>
      )}

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

      {ocupados.length > 0 && <EquiposTomados ocupados={ocupados} />}
    </div>
  )
}

/**
 * RF-04.11 + RF-04.12: los equipos que ya tiene alguien en esa franja.
 *
 * No están para tildarlos: están porque "no hay nada libre" y "los tiene
 * alguien con quien puedo hablar" son situaciones distintas y solo la segunda
 * tiene salida. El dato ya era público —está en el calendario de cada
 * equipo— pero ahí no es donde se decide.
 */
function EquiposTomados({ ocupados }: { ocupados: EquipoOcupado[] }) {
  return (
    <div className="grid gap-2 border-t pt-4">
      <h3 className="text-sm font-medium">Ya reservados en esa franja</h3>
      <p className="text-muted-foreground text-sm">
        No se pueden tildar. Si necesitás alguno, podés pedírselo a quien lo tiene: le
        llega un aviso y un correo, y decide él.
      </p>

      <div className="grid gap-2 sm:grid-cols-2">
        {ocupados.map((equipo) => (
          <EquipoTomado key={equipo.equipoId} equipo={equipo} />
        ))}
      </div>
    </div>
  )
}

function EquipoTomado({ equipo }: { equipo: EquipoOcupado }) {
  const [pidiendo, setPidiendo] = useState(false)
  const [mensaje, setMensaje] = useState("")

  const pedir = useMutation({
    mutationFn: () => reservasApi.pedirLiberacion(equipo.reservaId!, mensaje),
  })

  return (
    <div className="grid gap-2 rounded-md border border-dashed p-2">
      <div className="grid gap-0.5">
        <span className="text-sm font-medium">
          {equipo.etiqueta}
          {equipo.carroNombre && (
            <span className="text-muted-foreground font-normal"> · {equipo.carroNombre}</span>
          )}
        </span>
        {/* Un bloqueo administrativo no tiene docente: lo que explica la
            franja es el motivo que escribió el Admin. */}
        <span className="text-muted-foreground text-xs">
          {equipo.docenteNombre
            ? `${equipo.docenteNombre}${equipo.materiaNombre ? ` · ${equipo.materiaNombre}` : ""}`
            : equipo.motivo || "Tomado por administración"}
        </span>
        <span className="text-muted-foreground text-xs">
          {equipo.horaInicio} a {equipo.horaFin}
        </span>
      </div>

      {pedir.isSuccess ? (
        <p className="text-xs">
          Le avisamos. La reserva sigue siendo suya hasta que decida.
        </p>
      ) : equipo.puedePedirse ? (
        pidiendo ? (
          <div className="grid gap-2">
            <Label htmlFor={`pedido-${equipo.equipoId}`} className="text-xs">
              ¿Para qué la necesitás? (opcional)
            </Label>
            <Input
              id={`pedido-${equipo.equipoId}`}
              value={mensaje}
              maxLength={500}
              placeholder="Tengo una evaluación a esa hora"
              onChange={(e) => setMensaje(e.target.value)}
            />
            {pedir.isError && (
              <Alert variant="destructive">
                <AlertDescription>{getErrorMessage(pedir.error)}</AlertDescription>
              </Alert>
            )}
            <div className="flex gap-2">
              <Button
                type="button"
                size="sm"
                disabled={pedir.isPending}
                onClick={() => pedir.mutate()}
              >
                {pedir.isPending ? "Enviando…" : "Enviar pedido"}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => setPidiendo(false)}
              >
                Cancelar
              </Button>
            </div>
          </div>
        ) : (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setPidiendo(true)}
          >
            Pedírsela
          </Button>
        )
      ) : null}
    </div>
  )
}
