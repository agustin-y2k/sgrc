import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "react-router"

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
  PRESTAMOS_KEY,
  REFRESCO_DEL_MOSTRADOR,
} from "@/features/admin/entregas/compartido"
import * as reservasApi from "@/features/reservas/api"
import { hoyISO, type ReservaDetallada } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"
import { contar, plural } from "@/lib/plural"

/**
 * La cola del mostrador: qué clase está en curso, qué viene después, y qué
 * máquinas hay que entregarle a cada docente.
 */

/** HH:MM a minutos, para comparar contra la hora actual. */
function enMinutos(hhmm: string): number {
  const [h, m] = hhmm.split(":").map(Number)
  return Number.isFinite(h) && Number.isFinite(m) ? h * 60 + m : 0
}

function minutosDeAhora(): number {
  const ahora = new Date()
  return ahora.getHours() * 60 + ahora.getMinutes()
}

type ReservaDelDia = {
  clave: string
  materia: string
  curso?: string
  docente: string
  horaInicio: string
  horaFin: string
  reservas: ReservaDetallada[]
}

function agruparPorClase(reservas: ReservaDetallada[]): ReservaDelDia[] {
  const porGrupo = new Map<string, ReservaDelDia>()

  for (const r of reservas) {
    const clave = r.reservaGrupoId ?? r.id
    const existente = porGrupo.get(clave)
    if (existente) {
      existente.reservas.push(r)
      continue
    }
    porGrupo.set(clave, {
      clave,
      materia: r.materiaNombre ?? r.motivoBloqueo ?? "Bloqueado",
      curso: r.cursoNombre,
      docente: r.nombreDocenteSnapshot ?? "",
      horaInicio: r.horaInicio,
      horaFin: r.horaFin,
      reservas: [r],
    })
  }

  return [...porGrupo.values()].sort((a, b) =>
    a.horaInicio.localeCompare(b.horaInicio)
  )
}

function Clase({
  clase,
  afuera,
  enCurso,
}: {
  clase: ReservaDelDia
  afuera: Set<string>
  enCurso: boolean
}) {
  const queryClient = useQueryClient()
  const [retiradoPor, setRetiradoPor] = useState("")
  const [abriendoNombre, setAbriendoNombre] = useState(false)

  const sinRetirar = clase.reservas.filter(
    (r) => r.estado === "CONFIRMADA" && !afuera.has(r.equipoId)
  )
  const entregadas = clase.reservas.filter((r) => afuera.has(r.equipoId))
  const liberadas = clase.reservas.filter((r) => r.estado === "NO_RETIRADA")

  const entregar = useMutation({
    mutationFn: (ids: string[]) =>
      reservasApi.entregarPorReserva({
        reservaIds: ids,
        retiradoPor: retiradoPor.trim() || undefined,
      }),
    onSuccess: async () => {
      setRetiradoPor("")
      setAbriendoNombre(false)
      await queryClient.invalidateQueries({ queryKey: PRESTAMOS_KEY })
      await queryClient.invalidateQueries({ queryKey: ["reservas"] })
    },
  })

  return (
    <div className="grid gap-2 rounded-md border p-3">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <div className="min-w-0">
          <p className="font-medium">
            {clase.horaInicio}–{clase.horaFin} · {clase.materia}
            {clase.curso && (
              <span className="text-muted-foreground font-normal"> · {clase.curso}</span>
            )}
          </p>
          <p className="text-muted-foreground text-sm">{clase.docente}</p>
        </div>
        {enCurso && <EstadoBadge tono="info">En curso</EstadoBadge>}
      </div>

      <div className="flex flex-wrap gap-1.5 text-xs">
        {entregadas.map((r) => (
          <EstadoBadge key={r.id} tono="exito">
            {r.etiqueta} entregada
          </EstadoBadge>
        ))}
        {/* "Sin retirar" no es lo mismo que "liberada": la primera todavía
            está guardada para este docente, la segunda ya no. */}
        {sinRetirar.map((r) => (
          <EstadoBadge key={r.id} tono="neutro">
            {r.etiqueta} sin retirar
          </EstadoBadge>
        ))}
        {liberadas.map((r) => (
          <EstadoBadge key={r.id} tono="alerta">
            {r.etiqueta} liberada
          </EstadoBadge>
        ))}
      </div>

      {entregar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(entregar.error)}</AlertDescription>
        </Alert>
      )}

      {sinRetirar.length > 0 && (
        <div className="grid gap-2">
          {abriendoNombre && (
            <div className="grid gap-1.5">
              <Label htmlFor={`quien-${clase.clave}`}>¿Quién las retira? (opcional)</Label>
              <Input
                id={`quien-${clase.clave}`}
                value={retiradoPor}
                onChange={(e) => setRetiradoPor(e.target.value)}
                placeholder="Ej.: Juan (alumno de 5°A)"
              />
              {/* Anotarlo no cambia de quién son: el docente reservó y él
                  responde. Es solo quién pasó por el mostrador. */}
              <p className="text-muted-foreground text-xs">
                Quedan igual a cargo de {clase.docente}.
              </p>
            </div>
          )}
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              disabled={entregar.isPending}
              onClick={() => entregar.mutate(sinRetirar.map((r) => r.id))}
            >
              Entregar {sinRetirar.length === clase.reservas.length ? "todas" : ""} (
              {sinRetirar.length})
            </Button>
            {!abriendoNombre && (
              <Button variant="outline" size="sm" onClick={() => setAbriendoNombre(true)}>
                Anotar quién las retira
              </Button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

export function PanelDelLaboratorio() {
  const hoy = hoyISO()

  const { data: prestamos } = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  const { data: reservas, isLoading } = useQuery({
    queryKey: ["reservas", "del-dia", hoy],
    // pageSize al máximo: el listado pagina de a 50 por defecto y un día con
    // ocho clases de ocho máquinas son 64 reservas.
    queryFn: () => reservasApi.listarReservas({ desde: hoy, hasta: hoy, pageSize: 200 }),
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  const afuera = useMemo(
    () => new Set((prestamos?.data ?? []).map((p) => p.equipoId)),
    [prestamos]
  )

  const { enCurso, siguientes, terminadas } = useMemo(() => {
    const ahora = minutosDeAhora()
    // Los bloqueos administrativos no se entregan a nadie: nadie viene a
    // buscarlos, los crea un Admin para sacar máquinas de circulación.
    const delDia = (reservas?.data ?? []).filter((r) => r.tipo !== "BLOQUEO")
    const clases = agruparPorClase(delDia)

    return {
      enCurso: clases.filter(
        (c) => enMinutos(c.horaInicio) <= ahora && ahora < enMinutos(c.horaFin)
      ),
      siguientes: clases.filter((c) => enMinutos(c.horaInicio) > ahora),
      terminadas: clases.filter((c) => enMinutos(c.horaFin) <= ahora),
    }
  }, [reservas])

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Para entregar ahora</CardTitle>
          <CardDescription>
            {enCurso.length === 0
              ? "No hay ninguna clase en curso."
              : `${contar(enCurso.length, "clase")} en curso. Entregá las máquinas desde acá.`}
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          {isLoading && <p className="text-muted-foreground text-sm">Cargando…</p>}
          {enCurso.map((c) => (
            <Clase key={c.clave} clase={c} afuera={afuera} enCurso />
          ))}
          {!isLoading && enCurso.length === 0 && terminadas.length > 0 && (
            <p className="text-muted-foreground text-sm">
              Hoy ya {plural(terminadas.length, "pasó", "pasaron")} {contar(terminadas.length, "clase")}.
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Lo que sigue hoy</CardTitle>
          <CardDescription>
            {siguientes.length === 0
              ? "No queda ninguna clase por empezar hoy."
              : `${contar(siguientes.length, "clase")} por empezar. Se pueden entregar antes de hora.`}
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          {siguientes.slice(0, 4).map((c) => (
            <Clase key={c.clave} clase={c} afuera={afuera} enCurso={false} />
          ))}
          {siguientes.length > 4 && (
            <Button asChild variant="outline" size="sm" className="justify-self-start">
              <Link to="/admin/entregas">Ver las {siguientes.length} del día</Link>
            </Button>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
