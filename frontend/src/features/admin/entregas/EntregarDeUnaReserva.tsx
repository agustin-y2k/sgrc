import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

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
import { PRESTAMOS_KEY } from "@/features/admin/entregas/compartido"
import * as reservasApi from "@/features/reservas/api"
import { hoyISO, type ReservaDetallada } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * Entregar las computadoras de una reserva del día.
 *
 * Máquina por máquina: el docente puede llevarse tres de las cinco que
 * reservó, y las otras dos siguen disponibles para otro.
 */
export function EntregarDeUnaReserva({ yaAfuera }: { yaAfuera: Set<string> }) {
  const queryClient = useQueryClient()
  const [marcadas, setMarcadas] = useState<Set<string>>(new Set())
  const [nombreAlternativo, setNombreAlternativo] = useState("")
  const [resumen, setResumen] = useState<string | null>(null)

  const hoy = hoyISO()
  const { data, isLoading } = useQuery({
    queryKey: ["reservas", "del-dia", hoy],
    // pageSize al máximo: el listado pagina de a 50 por defecto, y un día
    // con ocho clases de ocho máquinas son 64 reservas. Con el default, las
    // últimas del día no aparecían para entregar y nada lo avisaba.
    queryFn: () =>
      reservasApi.listarReservas({ desde: hoy, hasta: hoy, pageSize: 200 }),
  })

  const entregar = useMutation({
    mutationFn: (ids: string[]) =>
      reservasApi.entregarPorReserva({
        reservaIds: ids,
        nombreAlternativo: nombreAlternativo.trim() || undefined,
      }),
    onSuccess: async (respuesta) => {
      const noSalieron = respuesta.noEntregadas ?? []
      setResumen(
        noSalieron.length === 0
          ? `Salieron ${respuesta.entregadas.length} computadora(s).`
          : `Salieron ${respuesta.entregadas.length}. No salieron ${noSalieron.length}: ${noSalieron
              .map((n) => n.detalle)
              .join("; ")}`
      )
      setMarcadas(new Set())
      setNombreAlternativo("")
      await queryClient.invalidateQueries({ queryKey: PRESTAMOS_KEY })
    },
  })

  // Las de hoy que todavía están en el laboratorio. Se cruzan por equipoId
  // contra lo que está afuera: el backend no marca la reserva, porque la
  // custodia es de la máquina y no de la reserva.
  const pendientes = useMemo(() => {
    const reservas: ReservaDetallada[] = data?.data ?? []
    return reservas.filter(
      (r) =>
        r.estado === "CONFIRMADA" &&
        // Un bloqueo por evaluación no tiene docente: lo crea un Admin sobre
        // equipos sueltos, y no hay nadie esperando para retirarlas. Si alguien
        // tiene que llevárselas para una mesa de examen, es una entrega
        // suelta con el nombre escrito a mano.
        r.tipo !== "EVALUACION_ESTATAL" &&
        !yaAfuera.has(r.equipoId)
    )
  }, [data, yaAfuera])

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
          fin de la reserva, no hay que cargarla.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {isLoading && <p className="text-muted-foreground text-sm">Cargando…</p>}
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

        {!isLoading && porGrupo.length === 0 && (
          <p className="text-muted-foreground text-sm">
            No queda ninguna reserva de hoy sin retirar.
          </p>
        )}

        {porGrupo.map((reservas) => {
          const primera = reservas[0]
          const todasMarcadas = reservas.every((r) => marcadas.has(r.id))
          return (
            <div key={primera.reservaGrupoId ?? primera.id} className="grid gap-2 rounded-md border p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="min-w-0">
                  <p className="font-medium">
                    {primera.materiaNombre ?? "Bloqueo por evaluación"}
                    {primera.cursoNombre && ` · ${primera.cursoNombre}`}
                  </p>
                  <p className="text-muted-foreground text-sm">
                    {primera.horaInicio}–{primera.horaFin} · {primera.nombreDocenteSnapshot}
                  </p>
                </div>
                <button
                  type="button"
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
                      checked={marcadas.has(r.id)}
                      onChange={() => {
                        const nueva = new Set(marcadas)
                        if (nueva.has(r.id)) nueva.delete(r.id)
                        else nueva.add(r.id)
                        setMarcadas(nueva)
                      }}
                    />
                    {r.etiqueta}
                    <span className="text-muted-foreground">({r.carroNombre})</span>
                  </label>
                ))}
              </div>
            </div>
          )
        })}

        {marcadas.size > 0 && (
          <div className="grid gap-2 rounded-md border border-dashed p-3">
            <div className="grid gap-1.5">
              <Label htmlFor="nombre-alternativo">¿Se las llevó otra persona?</Label>
              <Input
                id="nombre-alternativo"
                value={nombreAlternativo}
                onChange={(e) => setNombreAlternativo(e.target.value)}
                placeholder="Ej.: Juan (alumno de 5°A)"
              />
              <p className="text-muted-foreground text-xs">
                Dejalo vacío si las retiró el docente de la reserva.
              </p>
            </div>
            <div>
              <Button
                disabled={entregar.isPending}
                onClick={() => entregar.mutate([...marcadas])}
              >
                Entregar {marcadas.size} computadora(s)
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
