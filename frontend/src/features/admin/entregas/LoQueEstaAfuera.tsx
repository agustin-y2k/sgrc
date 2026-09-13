import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { EstadoBadge } from "@/components/EstadoBadge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  hora,
  nombreDeEquipo,
  PRESTAMOS_KEY,
  REFRESCO_DEL_MOSTRADOR,
  textoDeDemora,
} from "@/features/admin/entregas/compartido"
import * as reservasApi from "@/features/reservas/api"
import type { Prestamo } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"
import { contar, plural } from "@/lib/plural"

/**
 * Qué computadoras están fuera del laboratorio y el botón para recibirlas.
 *
 * **Agrupado por quién las tiene, no una máquina por renglón.** Las máquinas
 * no salen ni vuelven de a una: salen con una clase y vuelven con esa clase.
 * Con un día real cargado, la lista plana eran diecinueve renglones y mil
 * quinientos píxeles —lo más alto de la portada—, once de ellos repitiendo
 * palabra por palabra el mismo docente y la misma materia, y recibir un carro
 * eran once clics. Agrupado son cuatro bloques y un botón por grupo.
 *
 * Las máquinas sueltas siguen pudiendo recibirse de a una: cada una es una
 * casilla adentro de su grupo, y abajo está el botón que recibe lo marcado
 * con una observación en común.
 */

type Grupo = {
  clave: string
  /** Quién responde por estas máquinas. */
  nombre: string
  /** Materia, curso o destino: por qué salieron. */
  detalle: string
  prestamos: Prestamo[]
  /** La peor demora del grupo: es la que hay que ir a reclamar. */
  minutosDeDemora: number
  /** La salida y la devolución más tempranas del grupo. */
  salio: string
  vence?: string
}

/**
 * Un grupo es una entrega: la misma persona, por el mismo motivo. No se
 * agrupa por reserva porque una clase de doce máquinas puede haberse entregado
 * en dos tandas, y al mostrador le importa quién las tiene.
 */
export function agruparPrestamos(prestamos: Prestamo[]): Grupo[] {
  const grupos = new Map<string, Grupo>()

  for (const p of prestamos) {
    const detalle = [p.materiaNombre, p.destino].filter(Boolean).join(" · ")
    const clave = `${p.entregadoANombre}|${detalle}`
    const existente = grupos.get(clave)

    if (existente) {
      existente.prestamos.push(p)
      existente.minutosDeDemora = Math.max(existente.minutosDeDemora, p.minutosDeDemora ?? 0)
      if (p.entregadoEn < existente.salio) existente.salio = p.entregadoEn
      if (p.devolucionEstimada && (!existente.vence || p.devolucionEstimada < existente.vence)) {
        existente.vence = p.devolucionEstimada
      }
      continue
    }

    grupos.set(clave, {
      clave,
      nombre: p.entregadoANombre,
      detalle,
      prestamos: [p],
      minutosDeDemora: p.minutosDeDemora ?? 0,
      salio: p.entregadoEn,
      vence: p.devolucionEstimada,
    })
  }

  // El orden lo decide el backend —lo que debía haber vuelto hace más tiempo
  // va primero—, y el Map conserva el orden de inserción, así que el primer
  // préstamo de cada grupo manda.
  return [...grupos.values()]
}

export function LoQueEstaAfuera({ compacto = false }: { compacto?: boolean }) {
  const queryClient = useQueryClient()
  const [marcados, setMarcados] = useState<Set<string>>(new Set())
  const [observaciones, setObservaciones] = useState("")
  const [resumen, setResumen] = useState<string | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    // El mostrador lo atienden varios: si un colega recibe una máquina, esta
    // pantalla tiene que enterarse sin que nadie apriete recargar.
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  const recibir = useMutation({
    mutationFn: (ids: string[]) =>
      reservasApi.recibirEquipos({
        prestamoIds: ids,
        observaciones: observaciones || undefined,
      }),
    onSuccess: async (respuesta) => {
      const yaEstaban = respuesta.noRecibidos?.length ?? 0
      setResumen(
        yaEstaban === 0
          ? `Volvieron ${contar(respuesta.recibidos.length, "equipo")}.`
          : `Volvieron ${respuesta.recibidos.length}. ${yaEstaban} ya figuraba(n) adentro.`
      )
      setMarcados(new Set())
      setObservaciones("")
      await queryClient.invalidateQueries({ queryKey: PRESTAMOS_KEY })
    },
  })

  const prestamos = useMemo(() => data?.data ?? [], [data])
  const grupos = useMemo(() => agruparPrestamos(prestamos), [prestamos])
  const demorados = prestamos.filter((p) => p.demorado).length

  const alternar = (id: string) => {
    const nueva = new Set(marcados)
    if (nueva.has(id)) nueva.delete(id)
    else nueva.add(id)
    setMarcados(nueva)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Afuera del laboratorio</CardTitle>
        <CardDescription>
          {prestamos.length === 0
            ? "No hay ningún equipo entregado."
            : `${contar(prestamos.length, "equipo")} ${plural(prestamos.length, "entregado")}${demorados > 0 ? `, ${demorados} sin devolver a horario` : ""}. Marcá acá cuando vuelvan.`}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {isLoading && <p className="text-muted-foreground text-sm">Cargando…</p>}
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(error)}</AlertDescription>
          </Alert>
        )}
        {recibir.error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(recibir.error)}</AlertDescription>
          </Alert>
        )}
        {resumen && (
          <Alert>
            <AlertDescription>{resumen}</AlertDescription>
          </Alert>
        )}

        {grupos.map((g) => (
          <div key={g.clave} className="grid gap-2 rounded-md border p-3">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0">
                {/* El nombre que va primero es el de quien RESPONDE por la
                    máquina, que contra una reserva es siempre el docente. */}
                <p className="font-medium break-words">
                  {g.nombre}{" "}
                  {g.minutosDeDemora > 0 && (
                    <EstadoBadge tono="peligro">
                      {textoDeDemora(g.minutosDeDemora)}
                    </EstadoBadge>
                  )}
                </p>
                <p className="text-muted-foreground text-sm break-words">
                  {[g.detalle, contar(g.prestamos.length, "equipo")]
                    .filter(Boolean)
                    .join(" · ")}
                </p>
                {!compacto && (
                  <p className="text-muted-foreground text-xs">
                    Salió {hora(g.salio)}
                    {g.vence
                      ? ` · tiene que volver ${hora(g.vence)}`
                      : " · sin hora de devolución"}
                  </p>
                )}
              </div>
              <Button
                size="sm"
                className="shrink-0"
                disabled={recibir.isPending}
                // El rótulo visible se repite entre grupos; el que se lee en
                // voz alta, no: dos botones "Recibir las 10" no dicen de quién.
                aria-label={`Recibir ${contar(g.prestamos.length, "equipo")} de ${g.nombre}`}
                onClick={() => recibir.mutate(g.prestamos.map((p) => p.id))}
              >
                {g.prestamos.length === 1 ? "Recibir" : `Recibir las ${g.prestamos.length}`}
              </Button>
            </div>

            {/* Cada máquina, una casilla. Que no volvieron todas juntas es lo
                normal —falta una, alguien se la olvidó—, así que recibir de a
                una tiene que seguir siendo un clic y no un menú. */}
            <div className="flex flex-wrap gap-x-4 gap-y-1.5">
              {g.prestamos.map((p) => (
                <label key={p.id} className="flex items-center gap-1.5 text-sm">
                  <input
                    type="checkbox"
                    checked={marcados.has(p.id)}
                    aria-label={`Seleccionar ${nombreDeEquipo(p)}`}
                    onChange={() => alternar(p.id)}
                  />
                  <span className="min-w-0 break-words">
                    {nombreDeEquipo(p)}
                    {p.retiradoPor && (
                      <span className="text-muted-foreground"> · retiró {p.retiradoPor}</span>
                    )}
                  </span>
                </label>
              ))}
            </div>
          </div>
        ))}

        {marcados.size > 0 && (
          <div className="grid gap-2 rounded-md border border-dashed p-3">
            <div className="grid gap-1.5">
              <Label htmlFor="observaciones">Observaciones (opcional)</Label>
              <Input
                id="observaciones"
                value={observaciones}
                onChange={(e) => setObservaciones(e.target.value)}
                placeholder="Ej.: volvió sin el cargador"
              />
              {/* La observación se guarda en TODAS las que se reciban de una
                  vez, así que si es de una sola máquina conviene recibirla
                  aparte con su botón. */}
              <p className="text-muted-foreground text-xs">
                Se guarda en las {marcados.size} que recibas juntas. Si es sobre una sola,
                recibila con su propio botón.
              </p>
            </div>
            <div>
              <Button
                disabled={recibir.isPending}
                onClick={() => recibir.mutate([...marcados])}
              >
                Recibir las {marcados.size} seleccionadas
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
