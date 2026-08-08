import { useState } from "react"
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
  nombreDePC,
  PRESTAMOS_KEY,
  REFRESCO_DEL_MOSTRADOR,
  textoDeDemora,
} from "@/features/admin/entregas/compartido"
import * as reservasApi from "@/features/reservas/api"
import { getErrorMessage } from "@/lib/api-client"

/**
 * Qué computadoras están fuera del laboratorio y el botón para recibirlas.
 *
 * Es EL lugar donde se marca que una máquina volvió, sin importar por qué
 * salió: da igual si fue contra una reserva o en un préstamo suelto para un
 * trámite. Por eso hay una sola lista y no dos — el Admin que la recibe no
 * necesita acordarse de cómo salió.
 *
 * Vive suelto y no dentro de una pantalla porque se usa en dos: en el panel
 * del laboratorio, que es donde el Admin está parado todo el día, y en la
 * pantalla de entregas.
 */
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
      reservasApi.recibirPCs({
        prestamoIds: ids,
        observaciones: observaciones || undefined,
      }),
    onSuccess: async (respuesta) => {
      const yaEstaban = respuesta.noRecibidos?.length ?? 0
      setResumen(
        yaEstaban === 0
          ? `Volvieron ${respuesta.recibidos.length} computadora(s).`
          : `Volvieron ${respuesta.recibidos.length}. ${yaEstaban} ya figuraba(n) adentro.`
      )
      setMarcados(new Set())
      setObservaciones("")
      await queryClient.invalidateQueries({ queryKey: PRESTAMOS_KEY })
    },
  })

  const prestamos = data?.data ?? []
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
            ? "No hay ninguna computadora entregada."
            : `${prestamos.length} computadora(s) entregada(s)${demorados > 0 ? `, ${demorados} sin devolver a horario` : ""}. Marcá acá cuando vuelvan.`}
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

        {/* El orden lo decide el backend: lo que debía haber vuelto hace más
            tiempo va primero, y lo que no tiene hora pactada al final. */}
        {prestamos.map((p) => (
          <div
            key={p.id}
            className="flex flex-col gap-2 rounded-md border p-3 sm:flex-row sm:items-start sm:justify-between"
          >
            <div className="flex min-w-0 items-start gap-2">
              <input
                type="checkbox"
                className="mt-1"
                checked={marcados.has(p.id)}
                aria-label={`Seleccionar ${nombreDePC(p)}`}
                onChange={() => alternar(p.id)}
              />
              <div className="min-w-0">
                <p className="font-medium">
                  {nombreDePC(p)}{" "}
                  {p.demorado && (
                    <EstadoBadge tono="peligro">
                      {textoDeDemora(p.minutosDeDemora ?? 0)}
                    </EstadoBadge>
                  )}
                </p>
                <p className="text-muted-foreground text-sm break-words">
                  {p.entregadoANombre}
                  {p.materiaNombre && ` · ${p.materiaNombre}`}
                  {p.motivo && ` · ${p.motivo}`}
                </p>
                {!compacto && (
                  <p className="text-muted-foreground text-xs">
                    Salió {hora(p.entregadoEn)}
                    {p.devolucionEstimada
                      ? ` · tiene que volver ${hora(p.devolucionEstimada)}`
                      : " · sin hora de devolución"}
                  </p>
                )}
              </div>
            </div>
            <Button
              size="sm"
              className="shrink-0"
              disabled={recibir.isPending}
              onClick={() => recibir.mutate([p.id])}
            >
              Recibir
            </Button>
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
