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
import {
  conflictoDeEntregaAnticipada,
  diaAnteriorISO,
  diaSiguienteISO,
  puedeEntregarseReserva,
} from "@/features/reservas/intervalo"
import { useAhora } from "@/lib/useAhora"

/** Entregar las computadoras de una reserva del día. */
export function EntregarDeUnaReserva({ yaAfuera }: { yaAfuera: Set<string> }) {
  const queryClient = useQueryClient()
  const [marcadas, setMarcadas] = useState<Map<string, string>>(new Map())
  const [retiradoPor, setRetiradoPor] = useState("")
  const [esTodoLoQueSeLlevo, setEsTodoLoQueSeLlevo] = useState(false)
  const [resumen, setResumen] = useState<string | null>(null)

  const hoy = hoyISO()
  const ayer = diaAnteriorISO(hoy)
  const manana = diaSiguienteISO(hoy)
  const ahora = useAhora()
  const consultaPrestamos = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })
  const consultaReservas = useQuery({
    queryKey: ["reservas", "del-dia", ayer, manana],
    // pageSize al máximo: el listado pagina de a 50 por defecto, y un día con
    // ocho clases de ocho máquinas son 64 reservas.
    queryFn: () =>
      todasLasPaginas((page) =>
        reservasApi.listarReservas({
          desde: ayer,
          hasta: manana,
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
        equiposEsperados: Object.fromEntries(ids.map((id) => [id, marcadas.get(id)!])),
        liberarNoEntregadas: esTodoLoQueSeLlevo,
        retiradoPor: retiradoPor.trim() || undefined,
      }),
    onError: () => queryClient.invalidateQueries({ queryKey: ["reservas"] }),
    onSuccess: async (respuesta) => {
      const noSalieron = respuesta.noEntregadas ?? []
      const partes = [
        noSalieron.length === 0
          ? `Salieron ${contar(respuesta.entregadas.length, "equipo")}.`
          : `Salieron ${respuesta.entregadas.length}. No salieron ${noSalieron.length}: ${noSalieron
              .map((n) => {
                const reserva = data?.data.find((r) => r.equipoId === n.equipoId)
                return `${reserva ? nombreDeEquipo(reserva) + ": " : ""}${n.detalle}`
              })
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
      setMarcadas(new Map())
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
        puedeEntregarseReserva(r, ahora) &&
        // Un bloqueo administrativo no tiene docente: lo crea un Admin sobre
        // equipos sueltos, y no hay nadie esperando para retirarlas.
        r.tipo !== "BLOQUEO" &&
        !afuera.has(r.equipoId)
    )
  }, [data, yaAfuera, consultaPrestamos.data, ahora])
  const conflictos = useMemo(() => {
    const mapa = new Map<string, ReservaDetallada>()
    for (const r of pendientes) {
      const conflicto = conflictoDeEntregaAnticipada(r, data?.data ?? [], ahora)
      if (conflicto) mapa.set(r.id, conflicto)
    }
    return mapa
  }, [pendientes, data, ahora])
  const seleccionValida =
    marcadas.size > 0 &&
    [...marcadas.keys()].every((id) =>
      pendientes.some(
        (r) => r.id === id && r.equipoId === marcadas.get(id) && !conflictos.has(id)
      )
    )

  const porGrupo = useMemo(() => {
    const grupos = new Map<string, ReservaDetallada[]>()
    for (const r of pendientes) {
      const clave = r.reservaGrupoId ?? r.id
      grupos.set(clave, [...(grupos.get(clave) ?? []), r])
    }
    return [...grupos.values()]
  }, [pendientes])

  const alternarGrupo = (reservas: ReservaDetallada[]) => {
    reservas = reservas.filter((r) => !conflictos.has(r.id))
    const nueva = new Map(marcadas)
    const todas = reservas.every((r) => nueva.has(r.id))
    for (const r of reservas) {
      if (todas) nueva.delete(r.id)
      else nueva.set(r.id, r.equipoId)
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
          equipo: entregalo sólo si sigue en el laboratorio. Adelantar el retiro requiere
          que no haya otra reserva o bloqueo de esa PC antes de la devolución prevista.
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
            const habilitadas = reservas.filter((r) => !conflictos.has(r.id))
            const todasMarcadas =
              habilitadas.length > 0 && habilitadas.every((r) => marcadas.has(r.id))
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
                    disabled={
                      !consultaLista || entregar.isPending || habilitadas.length === 0
                    }
                    className="text-muted-foreground hover:text-foreground text-xs underline"
                    onClick={() => alternarGrupo(reservas)}
                  >
                    {todasMarcadas ? "Desmarcar todas" : "Marcar todas"}
                  </button>
                </div>
                {/* Máquina por máquina: puede llevarse tres de las cinco que
                  reservó, y las otras dos siguen disponibles para otro. */}
                <div className="grid gap-1 sm:grid-cols-3">
                  {reservas.map((r) => {
                    const conflicto = conflictos.get(r.id)
                    const avisoId = `conflicto-entrega-${r.id}`
                    return (
                      <div key={r.id} className="grid content-start gap-1">
                        <label className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            disabled={!consultaLista || entregar.isPending || !!conflicto}
                            aria-describedby={conflicto ? avisoId : undefined}
                            checked={marcadas.has(r.id)}
                            onChange={() => {
                              const nueva = new Map(marcadas)
                              if (nueva.has(r.id)) nueva.delete(r.id)
                              else nueva.set(r.id, r.equipoId)
                              setMarcadas(nueva)
                            }}
                          />
                          {nombreDeEquipo(r)}
                          {r.estado === "NO_RETIRADA" && (
                            <span className="text-muted-foreground">
                              {" "}
                              · reserva liberada
                            </span>
                          )}
                        </label>
                        {conflicto && (
                          <p id={avisoId} className="text-muted-foreground text-xs">
                            No se puede adelantar:{" "}
                            {conflicto.tipo === "BLOQUEO"
                              ? "bloqueo administrativo"
                              : "otra reserva"}{" "}
                            el {conflicto.fecha} de {conflicto.horaInicio} a{" "}
                            {conflicto.horaFin}
                            {conflicto.horaFin < conflicto.horaInicio &&
                              " del día siguiente"}
                            .{conflicto.materiaNombre && ` ${conflicto.materiaNombre}.`}{" "}
                            La reserva para retirar después se mantiene.
                          </p>
                        )}
                      </div>
                    )
                  })}
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
                  Los equipos que no pueden adelantarse por otra reserva o bloqueo siguen
                  guardados para después. No afecta a las próximas fechas de una reserva
                  que se repite.
                </span>
              </span>
            </label>
            <div>
              <Button
                disabled={entregar.isPending || !consultaLista || !seleccionValida}
                onClick={() => {
                  if (consultaLista && seleccionValida && !entregar.isPending)
                    entregar.mutate([...marcadas.keys()])
                }}
              >
                Entregar {contar(marcadas.size, "equipo")}
                {esTodoLoQueSeLlevo && " y liberar el resto"}
              </Button>
            </div>
            {(!seleccionValida || errorConsulta) && (
              <p className="text-sm">
                Revisá la selección: algunos equipos cambiaron, ya no están pendientes o
                tienen otra reserva antes del retiro. También puede faltar actualizar la
                consulta.
              </p>
            )}
            <Button
              variant="outline"
              type="button"
              onClick={() => {
                setMarcadas(new Map())
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
