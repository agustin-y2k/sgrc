import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
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
  nombreDeEquipo,
  PRESTAMOS_KEY,
  REFRESCO_DEL_MOSTRADOR,
} from "@/features/admin/entregas/compartido"
import * as reservasApi from "@/features/reservas/api"
import { hoyISO, type ReservaDetallada } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"
import { contar, plural } from "@/lib/plural"
import { todasLasPaginas } from "@/lib/todas-las-paginas"
import { diaAnteriorISO, intervaloDeReserva } from "@/features/reservas/intervalo"
import { useAhora } from "@/lib/useAhora"

/** Entregar las computadoras de una reserva del día. */
export function EntregarDeUnaReserva({ yaAfuera }: { yaAfuera: Set<string> }) {
  const queryClient = useQueryClient()
  const [marcadas, setMarcadas] = useState<Set<string>>(new Set())
  const [retiradoPor, setRetiradoPor] = useState("")
  const [esTodoLoQueSeLlevo, setEsTodoLoQueSeLlevo] = useState(false)
  const [resumen, setResumen] = useState<string | null>(null)

  const hoy = hoyISO()
  const ayer = diaAnteriorISO(hoy)
  const ahora = useAhora()
  const consultaPrestamos = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })
  const consultaReservas = useQuery({
    queryKey: ["reservas", "del-dia", ayer, hoy],
    // pageSize al máximo: el listado pagina de a 50 por defecto, y un día con
    // ocho clases de ocho máquinas son 64 reservas.
    queryFn: () =>
      todasLasPaginas((page) =>
        reservasApi.listarReservas({
          desde: ayer,
          hasta: hoy,
          pageSize: 200,
          ...(page > 1 ? { page } : {}),
        })
      ),
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })
  const { data } = consultaReservas
  const consultaLista = [consultaReservas, consultaPrestamos].every(
    (q) => q.isSuccess && !q.error && !q.isFetching
  )
  const errorConsulta = consultaReservas.error ?? consultaPrestamos.error

  const entregar = useMutation({
    mutationFn: (ids: string[]) =>
      reservasApi.entregarPorReserva({
        reservaIds: ids,
        liberarNoEntregadas: esTodoLoQueSeLlevo,
        retiradoPor: retiradoPor.trim() || undefined,
      }),
    onSuccess: async (respuesta) => {
      const noSalieron = respuesta.noEntregadas ?? []
      const partes = [
        noSalieron.length === 0
          ? `Salieron ${contar(respuesta.entregadas.length, "equipo")}.`
          : `Salieron ${respuesta.entregadas.length}. No salieron ${noSalieron.length}: ${noSalieron
              .map((n) => n.detalle)
              .join("; ")}`,
      ]
      if (respuesta.liberadas?.length)
        partes.push(
          `${plural(respuesta.liberadas.length, "Quedó libre", "Quedaron libres")} ${contar(respuesta.liberadas.length, "equipo")} de esta clase.`
        )
      // Solo llega cuando se entregó contra una reserva ya liberada: en el
      // rato que el equipo estuvo libre, otro pudo reservarlo.
      for (const a of respuesta.avisos ?? []) {
        partes.push(
          `Ojo: ese equipo tiene reserva ${a.fecha} de ${a.horaInicio} a ${a.horaFin}${a.docente ? ` (${a.docente})` : ""}.`
        )
      }
      setResumen(partes.join(" "))
      setMarcadas(new Set())
      setRetiradoPor("")
      setEsTodoLoQueSeLlevo(false)
      await queryClient.invalidateQueries({ queryKey: PRESTAMOS_KEY })
      await queryClient.invalidateQueries({ queryKey: ["reservas"] })
      await queryClient.invalidateQueries({ queryKey: ["equipos-disponibles"] })
    },
  })

  // Las de hoy que todavía están en el laboratorio.
  const pendientes = useMemo(() => {
    const reservas: ReservaDetallada[] = data?.data ?? []
    const afuera = new Set([
      ...yaAfuera,
      ...(consultaPrestamos.data?.data ?? []).map((p) => p.equipoId),
    ])
    return reservas.filter(
      (r) =>
        (r.estado === "CONFIRMADA" || r.estado === "NO_RETIRADA") &&
        // Un bloqueo administrativo no tiene docente: lo crea un Admin sobre
        // equipos sueltos, y no hay nadie esperando para retirarlas.
        r.tipo !== "BLOQUEO" &&
        !afuera.has(r.equipoId) &&
        (r.fecha === hoy ||
          (r.fecha === ayer && intervaloDeReserva(r).fin > ahora.getTime()))
    )
  }, [data, yaAfuera, consultaPrestamos.data, hoy, ayer, ahora])
  const seleccionValida =
    marcadas.size > 0 && [...marcadas].every((id) => pendientes.some((r) => r.id === id))

  const porGrupo = useMemo(() => {
    const grupos = new Map<string, ReservaDetallada[]>()
    for (const r of pendientes) {
      const clave = r.reservaGrupoId ?? r.id
      grupos.set(clave, [...(grupos.get(clave) ?? []), r])
    }
    return [...grupos.values()]
  }, [pendientes])

  const alternarGrupo = (reservas: ReservaDetallada[]) => {
    const nueva = new Set(marcadas)
    const todas = reservas.every((r) => nueva.has(r.id))
    for (const r of reservas) {
      if (todas) nueva.delete(r.id)
      else nueva.add(r.id)
    }
    setMarcadas(nueva)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Entregar las de una reserva</CardTitle>
        <CardDescription>
          Las reservas de hoy que todavía no se retiraron. La hora de devolución sale del
          fin de la reserva, no hay que cargarla. Las reservas liberadas ya no guardan el
          equipo: entregalo sólo si sigue en el laboratorio.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {!consultaLista && !errorConsulta && (
          <p className="text-muted-foreground text-sm" role="status">
            Consultando reservas y entregas…
          </p>
        )}
        {errorConsulta && (
          <ErrorDeConsulta
            mensaje="No pudimos consultar las reservas y sus entregas."
            reintentando={consultaReservas.isFetching || consultaPrestamos.isFetching}
            onReintentar={() => {
              void consultaReservas.refetch()
              void consultaPrestamos.refetch()
            }}
          />
        )}
        {entregar.error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(entregar.error)}</AlertDescription>
          </Alert>
        )}
        {resumen && (
          <Alert>
            <AlertDescription>{resumen}</AlertDescription>
          </Alert>
        )}

        {consultaLista && porGrupo.length === 0 && (
          <p className="text-muted-foreground text-sm">
            No queda ninguna reserva de hoy sin retirar.
          </p>
        )}

        {!errorConsulta &&
          porGrupo.map((reservas) => {
            const primera = reservas[0]
            const todasMarcadas = reservas.every((r) => marcadas.has(r.id))
            return (
              <div
                key={primera.reservaGrupoId ?? primera.id}
                className="grid gap-2 rounded-md border p-3"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="min-w-0">
                    <p className="font-medium">
                      {primera.materiaNombre ?? primera.motivoBloqueo ?? "Bloqueado"}
                      {primera.cursoNombre && ` · ${primera.cursoNombre}`}
                    </p>
                    <p className="text-muted-foreground text-sm">
                      {primera.horaInicio}–{primera.horaFin} ·{" "}
                      {primera.nombreDocenteSnapshot}
                    </p>
                  </div>
                  <button
                    type="button"
                    disabled={!consultaLista || entregar.isPending}
                    className="text-muted-foreground hover:text-foreground text-xs underline"
                    onClick={() => alternarGrupo(reservas)}
                  >
                    {todasMarcadas ? "Desmarcar todas" : "Marcar todas"}
                  </button>
                </div>
                {/* Máquina por máquina: puede llevarse tres de las cinco que
                  reservó, y las otras dos siguen disponibles para otro. */}
                <div className="grid gap-1 sm:grid-cols-3">
                  {reservas.map((r) => (
                    <label key={r.id} className="flex items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        disabled={!consultaLista || entregar.isPending}
                        checked={marcadas.has(r.id)}
                        onChange={() => {
                          const nueva = new Set(marcadas)
                          if (nueva.has(r.id)) nueva.delete(r.id)
                          else nueva.add(r.id)
                          setMarcadas(nueva)
                        }}
                      />
                      {nombreDeEquipo(r)}
                      {r.estado === "NO_RETIRADA" && (
                        <span className="text-muted-foreground"> · reserva liberada</span>
                      )}
                    </label>
                  ))}
                </div>
              </div>
            )
          })}

        {marcadas.size > 0 && (
          <div className="grid gap-2 rounded-md border border-dashed p-3">
            <div className="grid gap-1.5">
              <Label htmlFor="retirado-por">¿Quién las retira? (opcional)</Label>
              <Input
                id="retirado-por"
                value={retiradoPor}
                onChange={(e) => setRetiradoPor(e.target.value)}
                placeholder="Ej.: Juan (alumno de 5°A)"
              />
              {/* El docente queda como responsable pase lo que pase: él
                  reservó y a él se le reclama. Esto es solo quién pasó por el
                  mostrador, y por eso se puede dejar vacío. */}
              <p className="text-muted-foreground text-xs">
                Las máquinas quedan igual a cargo del docente de la reserva. Dejalo vacío
                si no hace falta anotar quién vino a buscarlas.
              </p>
            </div>
            {/* La otra mitad de una entrega parcial: decir que ESTO es todo.
                Marcado, lo que quedó sin tildar de esas mismas clases deja de
                estar guardado y vuelve al resto de la escuela en el acto —
                antes lo hacía un reloj de quince minutos que tenía que
                adivinar si el docente volvía—. Sin marcar, la reserva sigue
                siendo suya: el caso de "ya vuelvo por las otras dos". */}
            <label className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                className="mt-1"
                checked={esTodoLoQueSeLlevo}
                onChange={(e) => setEsTodoLoQueSeLlevo(e.target.checked)}
              />
              <span>
                Es todo lo que se llevó
                <span className="text-muted-foreground block text-xs">
                  Lo que no marcaste deja de estar guardado y queda libre para otro curso.
                  No afecta a las próximas fechas de una reserva que se repite.
                </span>
              </span>
            </label>
            <div>
              <Button
                disabled={entregar.isPending || !consultaLista || !seleccionValida}
                onClick={() => {
                  if (consultaLista && seleccionValida && !entregar.isPending)
                    entregar.mutate([...marcadas])
                }}
              >
                Entregar {contar(marcadas.size, "equipo")}
                {esTodoLoQueSeLlevo && " y liberar el resto"}
              </Button>
            </div>
            {(!seleccionValida || errorConsulta) && (
              <p className="text-sm">
                Revisá la selección: algunos equipos ya no están pendientes o la consulta
                no está disponible.
              </p>
            )}
            <Button
              variant="outline"
              type="button"
              onClick={() => {
                setMarcadas(new Set())
                setEsTodoLoQueSeLlevo(false)
              }}
            >
              Limpiar la selección
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
