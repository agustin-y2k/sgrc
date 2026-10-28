import { useMemo, useRef, useState, type ReactNode } from "react"
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
import {
  conflictoDeEntregaAnticipada,
  diaAnteriorISO,
  diaSiguienteISO,
  intervaloDeReserva,
} from "@/features/reservas/intervalo"
import { useAhora } from "@/lib/useAhora"
import { todasLasPaginas } from "@/lib/todas-las-paginas"
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

type ReservaDelDia = {
  clave: string
  materia: string
  curso?: string
  docente: string
  fecha: string
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
      fecha: r.fecha,
      horaInicio: r.horaInicio,
      horaFin: r.horaFin,
      reservas: [r],
    })
  }

  return [...porGrupo.values()].sort(
    (a, b) => intervaloDeReserva(a).inicio - intervaloDeReserva(b).inicio
  )
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
  informar,
  todasLasReservas,
  ahora,
  consultaLista,
}: {
  clase: ReservaDelDia
  afuera: Afuera
  enCurso: boolean
  informar: (detalles: string[]) => void
  todasLasReservas: ReservaDetallada[]
  ahora: Date
  consultaLista: boolean
}) {
  const queryClient = useQueryClient()
  const [retiradoPor, setRetiradoPor] = useState("")
  const [abriendoNombre, setAbriendoNombre] = useState(false)
  const botonDelNombre = useRef<HTMLButtonElement>(null)

  function cancelarAnotacion() {
    setRetiradoPor("")
    setAbriendoNombre(false)
    botonDelNombre.current?.focus()
  }
  // La entrega parcial es un modo aparte y no el caso por defecto: lo normal
  // es que el docente se lleve todo, y eso tiene que ser un clic.
  const [modoParcial, setModoParcial] = useState(false)
  const [marcadas, setMarcadas] = useState<Map<string, string>>(new Map())

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
  const conflictos = new Map<string, ReservaDetallada>()
  for (const r of sinRetirar) {
    const conflicto = conflictoDeEntregaAnticipada(r, todasLasReservas, ahora)
    if (conflicto) conflictos.set(r.id, conflicto)
  }
  const habilitadas = sinRetirar.filter((r) => !conflictos.has(r.id))
  const liberadas = clase.reservas.filter((r) => r.estado === "NO_RETIRADA")

  const entregar = useMutation({
    mutationFn: ({ ids, liberarElResto }: { ids: string[]; liberarElResto: boolean }) =>
      reservasApi.entregarPorReserva({
        reservaIds: ids,
        equiposEsperados: Object.fromEntries(
          ids.map((id) => [
            id,
            marcadas.get(id) ?? clase.reservas.find((r) => r.id === id)!.equipoId,
          ])
        ),
        retiradoPor: retiradoPor.trim() || undefined,
        liberarNoEntregadas: liberarElResto,
      }),
    onError: () => queryClient.invalidateQueries({ queryKey: ["reservas"] }),
    onSuccess: async (respuesta) => {
      informar(
        (respuesta.noEntregadas ?? []).map((rechazo) => {
          const reserva = clase.reservas.find((r) => r.equipoId === rechazo.equipoId)
          return `${reserva ? nombreDeEquipo(reserva) : "Equipo"}: ${rechazo.detalle}`
        })
      )
      setRetiradoPor("")
      setAbriendoNombre(false)
      setModoParcial(false)
      setMarcadas(new Map())
      await queryClient.invalidateQueries({ queryKey: PRESTAMOS_KEY })
      await queryClient.invalidateQueries({ queryKey: ["reservas"] })
    },
  })

  const alternar = (id: string) => {
    const nueva = new Map(marcadas)
    if (nueva.has(id)) nueva.delete(id)
    else nueva.set(id, clase.reservas.find((r) => r.id === id)!.equipoId)
    setMarcadas(nueva)
  }
  const seleccionValida = [...marcadas].every(([id, equipo]) =>
    habilitadas.some((r) => r.id === id && r.equipoId === equipo)
  )
  const quedanSueltas = Math.max(0, habilitadas.length - marcadas.size)

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

      {sinRetirar
        .filter((r) => conflictos.has(r.id))
        .map((r) => (
          <p
            key={r.id}
            id={`impedimento-${r.id}`}
            className="text-muted-foreground text-sm"
          >
            {nombreDeEquipo(r)}: no se puede adelantar porque hay{" "}
            {conflictos.get(r.id)!.tipo === "BLOQUEO"
              ? "un bloqueo administrativo"
              : "otra reserva"}{" "}
            el {conflictos.get(r.id)!.fecha} de {conflictos.get(r.id)!.horaInicio} a{" "}
            {conflictos.get(r.id)!.horaFin}
            {conflictos.get(r.id)!.horaFin < conflictos.get(r.id)!.horaInicio &&
              " del día siguiente"}
            . La reserva para retirar después se mantiene.
          </p>
        ))}
      {!seleccionValida && (
        <p role="alert" className="text-sm">
          Cambió un equipo o apareció una reserva intermedia. Volvé a marcar las
          computadoras antes de entregar.
        </p>
      )}
      {entregar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(entregar.error)}</AlertDescription>
        </Alert>
      )}

      {sinRetirar.length > 0 && (
        <div className="grid gap-2">
          {abriendoNombre && (
            <div id={`anotacion-${clase.clave}`} className="grid gap-1.5">
              <Label htmlFor={`quien-${clase.clave}`}>
                ¿Quién las retira? (opcional)
              </Label>
              <Input
                id={`quien-${clase.clave}`}
                autoFocus
                value={retiradoPor}
                onChange={(e) => setRetiradoPor(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Escape") {
                    e.preventDefault()
                    cancelarAnotacion()
                  }
                }}
                placeholder="Ej.: Juan (alumno de 5°A)"
              />
              {/* Anotarlo no cambia de quién son: el docente reservó y él
                  responde. Es solo quién pasó por el mostrador. */}
              <p className="text-muted-foreground text-xs">
                Quedan igual a cargo de {clase.docente}.
              </p>
            </div>
          )}
          {/* Máquina por máquina, sólo en el modo parcial. Lo que no se
              marca deja de estar guardado para este docente en cuanto se
              confirma: es el Admin diciendo "esto es todo lo que se llevó",
              que es un dato que él tiene enfrente y el sistema no. */}
          {modoParcial && (
            <div className="grid gap-2 rounded-md border border-dashed p-3">
              <p className="text-sm font-medium">¿Cuáles se llevó?</p>
              <div className="grid gap-1 sm:grid-cols-2">
                {sinRetirar.map((r) => (
                  <label key={r.id} className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      disabled={
                        !consultaLista || entregar.isPending || conflictos.has(r.id)
                      }
                      aria-describedby={
                        conflictos.has(r.id) ? `impedimento-${r.id}` : undefined
                      }
                      checked={marcadas.has(r.id)}
                      aria-label={`Se llevó ${nombreDeEquipo(r)}`}
                      onChange={() => alternar(r.id)}
                    />
                    {nombreDeEquipo(r)}
                  </label>
                ))}
              </div>
              <p className="text-muted-foreground text-xs">
                {quedanSueltas === 0
                  ? "Si se las lleva todas, usá «Entregar todas»."
                  : `${contar(quedanSueltas, "computadora")} ${plural(quedanSueltas, "queda", "quedan")} libre${plural(quedanSueltas, "", "s")} para otro curso en el acto.`}
                {conflictos.size > 0 &&
                  " Los equipos que no pueden adelantarse siguen guardados para después."}
              </p>
            </div>
          )}

          <div className="flex flex-wrap gap-2">
            {modoParcial ? (
              <>
                <Button
                  size="sm"
                  disabled={
                    !consultaLista ||
                    entregar.isPending ||
                    marcadas.size === 0 ||
                    !seleccionValida
                  }
                  onClick={() =>
                    entregar.mutate({ ids: [...marcadas.keys()], liberarElResto: true })
                  }
                >
                  Entregar {marcadas.size} y liberar el resto
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setModoParcial(false)
                    setMarcadas(new Map())
                  }}
                >
                  Volver
                </Button>
              </>
            ) : (
              <>
                <Button
                  size="sm"
                  disabled={
                    !consultaLista || entregar.isPending || habilitadas.length === 0
                  }
                  onClick={() =>
                    entregar.mutate({
                      ids: habilitadas.map((r) => r.id),
                      liberarElResto: false,
                    })
                  }
                >
                  {conflictos.size > 0 ? "Entregar disponibles" : "Entregar todas"} (
                  {habilitadas.length})
                </Button>
                {/* Con una sola máquina no hay nada que partir. */}
                {habilitadas.length > 1 && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setModoParcial(true)}
                  >
                    Entrega parcial
                  </Button>
                )}
              </>
            )}
            <Button
              ref={botonDelNombre}
              type="button"
              variant="outline"
              size="sm"
              aria-expanded={abriendoNombre}
              aria-controls={`anotacion-${clase.clave}`}
              disabled={entregar.isPending}
              onClick={() => {
                if (abriendoNombre) cancelarAnotacion()
                else setAbriendoNombre(true)
              }}
            >
              {abriendoNombre ? "Cancelar anotación" : "Anotar quién las retira"}
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

export function PanelDelLaboratorio({ accion }: { accion?: ReactNode }) {
  const [advertencias, setAdvertencias] = useState<string[]>([])
  const ahora = useAhora()
  const hoy = hoyISO()
  const ayer = diaAnteriorISO(hoy)
  const manana = diaSiguienteISO(hoy)

  const consultaPrestamos = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  const consultaReservas = useQuery({
    queryKey: ["reservas", "del-dia", ayer, manana],
    // pageSize al máximo: el listado pagina de a 50 por defecto y un día con
    // ocho clases de ocho máquinas son 64 reservas.
    queryFn: () =>
      todasLasPaginas((pagina) =>
        reservasApi.listarReservas({
          desde: ayer,
          hasta: manana,
          pageSize: 200,
          ...(pagina > 1 ? { page: pagina } : {}),
        })
      ),
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  const { data: prestamos, error: errorPrestamos } = consultaPrestamos
  const { data: reservas, error: errorReservas, isLoading } = consultaReservas
  const consultaLista = [consultaReservas, consultaPrestamos].every(
    (q) => q.isSuccess && !q.error && !q.isFetching
  )

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
    // Los bloqueos administrativos no se entregan a nadie: nadie viene a
    // buscarlos, los crea un Admin para sacar máquinas de circulación.
    const delDia = (reservas?.data ?? []).filter(
      (r) => r.tipo !== "BLOQUEO" && r.fecha <= hoy
    )
    const clases = agruparPorClase(delDia)

    return {
      enCurso: clases.filter(
        (c) =>
          intervaloDeReserva(c).inicio <= ahora.getTime() &&
          ahora.getTime() < intervaloDeReserva(c).fin
      ),
      siguientes: clases.filter((c) => intervaloDeReserva(c).inicio > ahora.getTime()),
      terminadas: clases.filter(
        (c) => c.fecha === hoy && intervaloDeReserva(c).fin <= ahora.getTime()
      ),
    }
  }, [reservas, ahora, hoy])

  const hayAlgo = enCurso.length > 0 || siguientes.length > 0

  if (errorPrestamos || errorReservas)
    return (
      <Alert variant="destructive">
        <AlertDescription>
          {getErrorMessage(errorPrestamos ?? errorReservas)}
        </AlertDescription>
      </Alert>
    )

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
        {advertencias.length > 0 && (
          <Alert>
            <AlertDescription>
              <p>No se pudieron entregar estos equipos:</p>
              <ul>
                {advertencias.map((detalle, i) => (
                  <li key={i}>{detalle}</li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        )}
        {isLoading && <p className="text-muted-foreground text-sm">Cargando…</p>}

        {/* En curso primero y después lo que viene: es el orden del día, y lo
            que está pasando ahora es lo único que tiene a alguien esperando
            del otro lado del mostrador. */}
        {enCurso.map((c) => (
          <Clase
            key={c.clave}
            clase={c}
            afuera={afuera}
            enCurso
            informar={setAdvertencias}
            todasLasReservas={reservas?.data ?? []}
            ahora={ahora}
            consultaLista={consultaLista}
          />
        ))}
        {siguientes.slice(0, MAX_SIGUIENTES).map((c) => (
          <Clase
            key={c.clave}
            clase={c}
            afuera={afuera}
            enCurso={false}
            informar={setAdvertencias}
            todasLasReservas={reservas?.data ?? []}
            ahora={ahora}
            consultaLista={consultaLista}
          />
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
