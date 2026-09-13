import { useMemo, useState, type ReactNode } from "react"
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
  nombreDeEquipo,
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
 *
 * **Una sola tarjeta y no dos.** «Para entregar ahora» y «Lo que sigue hoy»
 * eran la misma lista partida por un `if` de hora, con el mismo componente
 * adentro. En dos columnas se desbalanceaban —dos clases de un lado, cinco del
 * otro— y en un día sin clases quedaban dos cajas vacías ocupando el mejor
 * lugar de la portada. Una lista cronológica dice lo mismo, se lee de arriba a
 * abajo como pasa el día, y cuando no hay nada es un renglón.
 */

/** Cuántas clases por empezar se resumen antes de mandar al listado. */
const MAX_SIGUIENTES = 4

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

  return [...porGrupo.values()].sort((a, b) => a.horaInicio.localeCompare(b.horaInicio))
}

/** Qué está afuera, mirado desde las dos claves que hacen falta. */
type Afuera = {
  /**
   * Las RESERVAS que tienen un préstamo abierto. Esta es la que dice si una
   * clase ya recibió sus máquinas.
   */
  reservas: Set<string>
  /**
   * Los EQUIPOS que están fuera del laboratorio, por el motivo que sea. Una
   * máquina que salió con la clase anterior no se le puede entregar a la
   * siguiente aunque la tenga reservada.
   */
  equipos: Set<string>
}

function Clase({
  clase,
  afuera,
  enCurso,
}: {
  clase: ReservaDelDia
  afuera: Afuera
  enCurso: boolean
}) {
  const queryClient = useQueryClient()
  const [retiradoPor, setRetiradoPor] = useState("")
  const [abriendoNombre, setAbriendoNombre] = useState(false)

  // Entregada es la reserva que tiene SU préstamo abierto, no la que usa un
  // equipo que está afuera.
  //
  // Mirar el equipo era el error: la misma máquina la reservan varias clases
  // a lo largo del día, así que una entrega a las once pintaba de verde el día
  // entero. Con un día real cargado, nueve de once clases decían tener
  // entregadas máquinas que nadie había sacado, y el botón ofrecía "Entregar
  // (2)" cuando faltaban las diez.
  const entregadas = clase.reservas.filter((r) => afuera.reservas.has(r.id))
  const pendientes = clase.reservas.filter(
    (r) => r.estado === "CONFIRMADA" && !afuera.reservas.has(r.id)
  )
  // Reservada para esta clase pero todavía afuera con otro: no se puede
  // entregar, y decirlo es más útil que no nombrarla.
  const ocupadas = pendientes.filter((r) => afuera.equipos.has(r.equipoId))
  const sinRetirar = pendientes.filter((r) => !afuera.equipos.has(r.equipoId))
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

      {/* El resumen antes que el detalle: con doce máquinas, doce chips que
          dicen "PC 7 · Carro 1 entregada" son doscientos píxeles para contar
          hasta doce. El renglón dice cuántas de cuántas; los chips quedan
          para lo que hay que ir a buscar. */}
      <p className="text-muted-foreground text-sm">
        {/* La concordancia va con las entregadas, que es el sujeto: "1
            entregada de 12 computadoras", "0 entregadas de 1 computadora". */}
        {entregadas.length} {plural(entregadas.length, "entregada")} de{" "}
        {contar(clase.reservas.length, "computadora")}
        {ocupadas.length > 0 && ` · ${ocupadas.length} todavía con otra clase`}
      </p>

      {/* El detalle equipo por equipo, SOLO en la clase que está pasando.
          Para una de la tarde no es información, es ruido: sus máquinas están
          afuera con la clase de ahora y van a volver antes. Con un día a full
          eran doce chips naranjas por cada clase futura —ciento treinta
          píxeles cada una— contando algo que a las once no se hace. */}
      {(enCurso || liberadas.length > 0) && (
        <div className="flex flex-wrap gap-1.5 text-xs">
          {/* "Sin retirar" no es lo mismo que "liberada": la primera todavía
              está guardada para este docente, la segunda ya no. */}
          {enCurso &&
            sinRetirar.map((r) => (
              <EstadoBadge key={r.id} tono="neutro">
                {nombreDeEquipo(r)} sin retirar
              </EstadoBadge>
            ))}
          {enCurso &&
            ocupadas.map((r) => (
              <EstadoBadge key={r.id} tono="alerta">
                {nombreDeEquipo(r)} todavía afuera
              </EstadoBadge>
            ))}
          {/* Las liberadas se nombran siempre: es la excepción que alguien
              tiene que ver, empezó o no la clase. */}
          {liberadas.map((r) => (
            <EstadoBadge key={r.id} tono="alerta">
              {nombreDeEquipo(r)} liberada
            </EstadoBadge>
          ))}
        </div>
      )}

      {entregar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(entregar.error)}</AlertDescription>
        </Alert>
      )}

      {sinRetirar.length > 0 && (
        <div className="grid gap-2">
          {abriendoNombre && (
            <div className="grid gap-1.5">
              <Label htmlFor={`quien-${clase.clave}`}>
                ¿Quién las retira? (opcional)
              </Label>
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

export function PanelDelLaboratorio({ accion }: { accion?: ReactNode }) {
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

  const afuera: Afuera = useMemo(() => {
    const abiertos = prestamos?.data ?? []
    return {
      reservas: new Set(
        abiertos.map((p) => p.reservaId).filter((id): id is string => !!id)
      ),
      equipos: new Set(abiertos.map((p) => p.equipoId)),
    }
  }, [prestamos])

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

  const hayAlgo = enCurso.length > 0 || siguientes.length > 0

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-2 space-y-0">
        <div className="grid gap-1.5">
          {/* "Para entregar hoy" y no "Hoy en el laboratorio": la tarjeta
              se nombra por lo que se hace ahí. Ya se había descartado
              "Ahora en el laboratorio" por lo mismo —suena a un informe del
              lugar y no a una cola de gente esperando su máquina—, y al
              fusionar las dos tarjetas casi vuelve por la ventana. */}
          <CardTitle>Para entregar hoy</CardTitle>
          <CardDescription>
            {enCurso.length > 0 && `${contar(enCurso.length, "clase")} en curso`}
            {enCurso.length > 0 && siguientes.length > 0 && " · "}
            {siguientes.length > 0 && `${siguientes.length} por empezar`}
            {!hayAlgo &&
              (terminadas.length > 0
                ? `Hoy ya ${plural(terminadas.length, "pasó", "pasaron")} ${contar(terminadas.length, "clase")}. No queda ninguna.`
                : "Hoy no hay ninguna clase con equipos reservados.")}
          </CardDescription>
        </div>
        {accion}
      </CardHeader>
      <CardContent className="grid gap-3">
        {isLoading && <p className="text-muted-foreground text-sm">Cargando…</p>}

        {/* En curso primero y después lo que viene: es el orden del día, y lo
            que está pasando ahora es lo único que tiene a alguien esperando
            del otro lado del mostrador. */}
        {enCurso.map((c) => (
          <Clase key={c.clave} clase={c} afuera={afuera} enCurso />
        ))}
        {siguientes.slice(0, MAX_SIGUIENTES).map((c) => (
          <Clase key={c.clave} clase={c} afuera={afuera} enCurso={false} />
        ))}

        {siguientes.length > MAX_SIGUIENTES && (
          <Button asChild variant="outline" size="sm" className="justify-self-start">
            <Link to="/admin/entregas">
              Ver las {enCurso.length + siguientes.length} del día
            </Link>
          </Button>
        )}
      </CardContent>
    </Card>
  )
}
