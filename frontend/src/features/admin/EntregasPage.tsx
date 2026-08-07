import { useMemo, useState } from "react"
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
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
import * as inventoryApi from "@/features/inventory/api"
import * as reservasApi from "@/features/reservas/api"
import { hoyISO, type Prestamo, type ReservaDetallada } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-08 — el mostrador: qué computadoras están afuera, quién se las llevó y
 * cuáles volvieron.
 *
 * Reemplaza el papel en el que los Admin anotan esto hoy. La diferencia que
 * justifica la pantalla no es la comodidad: una PC no puede figurar entregada
 * dos veces, porque eso lo garantiza un índice único en la base. En el papel,
 * que dos personas anoten la misma máquina no lo detecta nadie hasta que
 * aparece un docente sin computadora.
 *
 * Lo que se ve acá NO es "el estado de la PC": no hay ninguna columna que
 * diga "prestada". Se deriva de si existe un préstamo sin devolver, y por eso
 * no puede quedar desincronizado — que es exactamente lo que le pasa al papel
 * cuando alguien devuelve una máquina y nadie tacha el renglón.
 */

const PRESTAMOS_KEY = ["prestamos"]

/** "hace 25 minutos", "hace 2 h 10 min" — la demora, en castellano. */
function textoDeDemora(minutos: number): string {
  if (minutos < 60) return `${minutos} min tarde`
  const horas = Math.floor(minutos / 60)
  const resto = minutos % 60
  return resto === 0 ? `${horas} h tarde` : `${horas} h ${resto} min tarde`
}

function hora(iso: string): string {
  return new Date(iso).toLocaleTimeString("es-AR", { hour: "2-digit", minute: "2-digit" })
}

function nombreDePC(p: Prestamo): string {
  const pc = p.pcIdentificador ? `PC ${p.pcIdentificador}` : "PC"
  return p.carroNombre ? `${pc} · ${p.carroNombre}` : pc
}

// ── Lo que está afuera ──────────────────────────────────────────────────

function LoQueEstaAfuera() {
  const queryClient = useQueryClient()
  const [marcados, setMarcados] = useState<Set<string>>(new Set())
  const [observaciones, setObservaciones] = useState("")
  const [resumen, setResumen] = useState<string | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
  })

  const recibir = useMutation({
    mutationFn: (ids: string[]) =>
      reservasApi.recibirPCs({ prestamoIds: ids, observaciones: observaciones || undefined }),
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
            : `${prestamos.length} computadora(s) entregada(s)${demorados > 0 ? `, ${demorados} sin devolver a horario` : ""}.`}
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
          <div key={p.id} className="flex flex-col gap-2 rounded-md border p-3 sm:flex-row sm:items-start sm:justify-between">
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
                <p className="text-muted-foreground text-xs">
                  Salió {hora(p.entregadoEn)}
                  {p.devolucionEstimada
                    ? ` · tiene que volver ${hora(p.devolucionEstimada)}`
                    : " · sin hora de devolución"}
                </p>
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
              {/* La observación se guarda en TODAS las que se reciban de
                  una vez, así que si es de una sola máquina conviene
                  recibirla aparte con su botón. */}
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

// ── Entregar las de una reserva ─────────────────────────────────────────

function EntregarDeUnaReserva({ yaAfuera }: { yaAfuera: Set<string> }) {
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

  // Las de hoy que todavía están en el laboratorio. Se cruzan por pcId
  // contra lo que está afuera: el backend no marca la reserva, porque la
  // custodia es de la máquina y no de la reserva.
  const pendientes = useMemo(() => {
    const reservas: ReservaDetallada[] = data?.data ?? []
    return reservas.filter(
      (r) =>
        r.estado === "CONFIRMADA" &&
        // Un bloqueo por evaluación no tiene docente: lo crea un Admin sobre
        // PCs sueltas, y no hay nadie esperando para retirarlas. Si alguien
        // tiene que llevárselas para una mesa de examen, es una entrega
        // suelta con el nombre escrito a mano.
        r.tipo !== "EVALUACION_ESTATAL" &&
        !yaAfuera.has(r.pcId)
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
                    PC {r.pcIdentificador}
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

// ── Entrega suelta ──────────────────────────────────────────────────────

function EntregaSuelta({
  yaAfuera,
  onCerrar,
}: {
  yaAfuera: Set<string>
  onCerrar: () => void
}) {
  const queryClient = useQueryClient()
  const [nombre, setNombre] = useState("")
  const [motivo, setMotivo] = useState("")
  const [devolucion, setDevolucion] = useState("")
  const [seleccionadas, setSeleccionadas] = useState<Set<string>>(new Set())
  const [resumen, setResumen] = useState<string | null>(null)

  const { data: carros } = useQuery({
    queryKey: ["carros"],
    queryFn: inventoryApi.listarCarros,
  })

  const consultasDePCs = useQueries({
    queries: (carros?.data ?? []).map((c) => ({
      queryKey: ["pcs", c.id],
      queryFn: () => inventoryApi.listarPCsDeCarro(c.id),
    })),
  })

  // Se ofrecen todas las que estén en el inventario y no estén ya afuera.
  // No se filtra por estado a propósito: llevarle al técnico una PC en
  // mantenimiento es justamente un préstamo, y prohibirlo obligaría a
  // sacarla del sistema para poder anotarlo.
  const pcs = useMemo(() => {
    const lista: { id: string; identificador: number; carroNombre: string }[] = []
    ;(carros?.data ?? []).forEach((carro, i) => {
      const pcsDelCarro = consultasDePCs[i]?.data?.data ?? []
      pcsDelCarro
        .filter((pc) => !pc.dadaDeBaja && !yaAfuera.has(pc.id))
        .forEach((pc) =>
          lista.push({ id: pc.id, identificador: pc.identificador, carroNombre: carro.nombre })
        )
    })
    return lista
  }, [carros, consultasDePCs, yaAfuera])

  const entregar = useMutation({
    mutationFn: () =>
      reservasApi.entregarSuelta({
        pcIds: [...seleccionadas],
        nombre: nombre.trim(),
        motivo: motivo.trim() || undefined,
        devolucionEstimada: devolucion ? new Date(devolucion).toISOString() : undefined,
      }),
    onSuccess: async (respuesta) => {
      const avisos = respuesta.avisos ?? []
      const noSalieron = respuesta.noEntregadas ?? []
      const partes = [`Salieron ${respuesta.entregadas.length} computadora(s).`]
      if (noSalieron.length > 0) {
        partes.push(`No salieron ${noSalieron.length}: ${noSalieron.map((n) => n.detalle).join("; ")}`)
      }
      // El aviso no impidió nada: el sistema no sabe cuánto dura un trámite,
      // así que la decisión es del Admin.
      for (const a of avisos) {
        partes.push(
          `Ojo: esa máquina tiene reserva ${a.fecha} de ${a.horaInicio} a ${a.horaFin}${a.docente ? ` (${a.docente})` : ""}.`
        )
      }
      setResumen(partes.join(" "))
      setSeleccionadas(new Set())
      setNombre("")
      setMotivo("")
      setDevolucion("")
      await queryClient.invalidateQueries({ queryKey: PRESTAMOS_KEY })
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Entregar sin reserva</CardTitle>
        <CardDescription>
          Para cuando piden una computadora en el momento — un trámite, algo puntual. No
          hace falta que la persona tenga cuenta en el sistema.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            setResumen(null)
            entregar.mutate()
          }}
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="entrega-nombre">¿A quién?</Label>
              <Input
                id="entrega-nombre"
                value={nombre}
                onChange={(e) => setNombre(e.target.value)}
                placeholder="Ej.: Marta (secretaría)"
                required
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="entrega-motivo">¿Para qué? (opcional)</Label>
              <Input
                id="entrega-motivo"
                value={motivo}
                onChange={(e) => setMotivo(e.target.value)}
                placeholder="Ej.: trámite"
              />
            </div>
          </div>

          <div className="grid gap-1.5 sm:max-w-xs">
            <Label htmlFor="entrega-devolucion">¿Cuándo la devuelve? (opcional)</Label>
            <Input
              id="entrega-devolucion"
              type="datetime-local"
              value={devolucion}
              onChange={(e) => setDevolucion(e.target.value)}
            />
            {/* Sin hora pactada no se le reclama nada: "vengo en un rato" es
                una respuesta válida, y una hora inventada solo generaría
                reclamos falsos. */}
            <p className="text-muted-foreground text-xs">
              Si no la sabés, dejalo vacío: no se le va a reclamar la devolución.
            </p>
          </div>

          <div className="grid gap-2">
            <Label>¿Qué computadoras?</Label>
            <div className="grid max-h-56 gap-1 overflow-y-auto rounded-md border p-2 sm:grid-cols-2">
              {pcs.length === 0 && (
                <p className="text-muted-foreground text-sm">
                  No hay computadoras disponibles para entregar.
                </p>
              )}
              {pcs.map((pc) => (
                <label key={pc.id} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={seleccionadas.has(pc.id)}
                    onChange={() => {
                      const nueva = new Set(seleccionadas)
                      if (nueva.has(pc.id)) nueva.delete(pc.id)
                      else nueva.add(pc.id)
                      setSeleccionadas(nueva)
                    }}
                  />
                  PC {pc.identificador}
                  <span className="text-muted-foreground">({pc.carroNombre})</span>
                </label>
              ))}
            </div>
          </div>

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

          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={entregar.isPending || seleccionadas.size === 0}>
              Entregar {seleccionadas.size} computadora(s)
            </Button>
            <Button type="button" variant="outline" onClick={onCerrar}>
              Cerrar
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

export function EntregasPage() {
  const [entregandoSuelta, setEntregandoSuelta] = useState(false)

  const { data } = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
  })

  // Qué máquinas están afuera, para no ofrecerlas de nuevo. Se calcula acá y
  // se pasa a los dos formularios: es el mismo dato y una sola consulta.
  const yaAfuera = useMemo(
    () => new Set((data?.data ?? []).map((p) => p.pcId)),
    [data]
  )

  return (
    <div>
      <EncabezadoDePagina
        titulo="Entregas y devoluciones"
        descripcion="Qué computadoras están afuera del laboratorio, quién se las llevó y cuándo tienen que volver. Reemplaza el registro en papel."
        accion={
          !entregandoSuelta && (
            <Button variant="outline" onClick={() => setEntregandoSuelta(true)}>
              Entregar sin reserva
            </Button>
          )
        }
      />

      <div className="grid gap-4">
        {/* El formulario va en el cuerpo y no en el slot de acción del
            encabezado: ese slot es `shrink-0` y está pensado para botones,
            así que una tarjeta entera adentro queda apretada contra el borde
            en un teléfono. */}
        {entregandoSuelta && (
          <EntregaSuelta yaAfuera={yaAfuera} onCerrar={() => setEntregandoSuelta(false)} />
        )}
        <LoQueEstaAfuera />
        <EntregarDeUnaReserva yaAfuera={yaAfuera} />
      </div>
    </div>
  )
}
