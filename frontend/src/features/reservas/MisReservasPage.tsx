import { useEffect, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useLocation, useNavigate } from "react-router"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { EstadoBadge, TONO_RESERVA } from "@/components/EstadoBadge"
import { Paginador } from "@/components/Paginador"
import { formatearFechaLargaCapitalizada } from "@/lib/fechas"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuth } from "@/features/auth/AuthContext"
import * as reservasApi from "@/features/reservas/api"
import { agruparReservas } from "@/features/reservas/types"
import { CambiarPCDeReserva } from "@/features/reservas/CambiarPCDeReserva"
import type { EstadoReserva, GrupoDeReservas } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"

const RESERVAS_KEY = ["reservas"]

/**
 * El aviso de "listo, se creó" que deja NuevaReservaPage al navegar acá.
 *
 * Se copia a estado local y se borra del historial en el primer render: si
 * quedara en la entrada del historial, recargar la página o volver con el
 * botón "atrás" del navegador mostraría de nuevo la confirmación de algo
 * que se hizo hace media hora.
 */
function useConfirmacionDeLlegada(): string | null {
  const { pathname, state } = useLocation()
  const navigate = useNavigate()
  const recibida =
    state && typeof state === "object" && "confirmacion" in state
      ? String((state as { confirmacion: unknown }).confirmacion)
      : null
  const [confirmacion] = useState(recibida)

  useEffect(() => {
    if (recibida !== null) navigate(pathname, { replace: true, state: null })
  }, [recibida, pathname, navigate])

  return confirmacion
}

/**
 * Identifica una tarjeta del listado. Un bloqueo por evaluación no tiene
 * grupo en la base, así que se lo identifica por la operación que lo creó
 * (ver claveDeAgrupacion en types.ts) — usar el id de su primera reserva
 * rompía apenas esa PC se cancelaba y la tarjeta pasaba a empezar por otra.
 */
function claveDeTarjeta(grupo: GrupoDeReservas): string {
  if (grupo.grupoId) return grupo.grupoId
  if (grupo.esBloqueoEvaluacion) {
    return `evaluacion:${grupo.creadoPor ?? "sistema"}:${grupo.fecha}:${grupo.horaInicio}:${grupo.horaFin}`
  }
  return grupo.reservas[0].id
}

const ETIQUETA_RESERVA: Record<EstadoReserva, string> = {
  CONFIRMADA: "Confirmada",
  CANCELADA: "Cancelada",
  FINALIZADA: "Finalizada",
  // "No retirada" y no "Liberada": lo que le pasó al docente es que no la
  // fue a buscar, y esa es la palabra que le explica por qué la perdió.
  NO_RETIRADA: "No retirada",
}

function EstadoDeReserva({ estado }: { estado: EstadoReserva }) {
  return <EstadoBadge tono={TONO_RESERVA[estado]}>{ETIQUETA_RESERVA[estado]}</EstadoBadge>
}

type Cancelacion = {
  grupo: GrupoDeReservas
  /** RF-04.6: true = solo esta fecha; false = esta y todas las siguientes. */
  soloEsta: boolean
  motivo: string
}

/**
 * Estado del grupo a partir del de sus reservas.
 *
 * El backend mantiene un estado propio en reserva_grupo
 * (PARCIALMENTE_CANCELADA incluido), pero el listado devuelve Reserva, no
 * grupos, así que acá se deriva de las filas: si queda alguna confirmada la
 * reserva sigue viva, y si no, se muestra el estado que comparten todas.
 */
function estadoDelGrupo(grupo: GrupoDeReservas): EstadoReserva {
  if (grupo.reservas.some((r) => r.estado === "CONFIRMADA")) return "CONFIRMADA"
  if (grupo.reservas.every((r) => r.estado === "CANCELADA")) return "CANCELADA"
  // Si no quedó ninguna viva y alguna se liberó por no retiro, eso es lo que
  // hay que mostrar: "Finalizada" haría creer que la clase se dio.
  if (grupo.reservas.some((r) => r.estado === "NO_RETIRADA")) return "NO_RETIRADA"
  return "FINALIZADA"
}

export function MisReservasPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [incluirCanceladas, setIncluirCanceladas] = useState(false)
  const [pagina, setPagina] = useState(1)
  const [cancelando, setCancelando] = useState<Cancelacion | null>(null)
  const [cambiandoPC, setCambiandoPC] = useState<string | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: [...RESERVAS_KEY, { incluirCanceladas, pagina }],
    queryFn: () => reservasApi.listarReservas({ incluirCanceladas, page: pagina }),
  })

  const cancelar = useMutation({
    // RF-04.6: la elección se aplica "a todas las PCs del grupo en esa
    // fecha (o rango)", así que las dos ramas cancelan el grupo entero —
    // lo único que cambia es si además alcanza a las fechas siguientes.
    // Antes "solo esta fecha" llamaba a cancelarReserva y liberaba una
    // sola PC, que no es lo que dice el requisito ni lo que sugiere el
    // texto de la opción.
    mutationFn: async ({ grupo, soloEsta, motivo }: Cancelacion): Promise<void> => {
      if (grupo.grupoId) {
        await reservasApi.cancelarGrupo(grupo.grupoId, motivo, soloEsta)
        return
      }
      // Un bloqueo por evaluación son N filas `reserva` sueltas, sin grupo
      // que las una en la base. Se cancelan TODAS: la tarjeta representa la
      // operación completa, y liberar solo una PC dejaría el aula a medio
      // bloquear sin que nada lo diga. En serie y no en paralelo para que un
      // fallo a mitad de camino sea un error claro y no una carrera.
      for (const r of grupo.reservas.filter((x) => x.estado === "CONFIRMADA")) {
        await reservasApi.cancelarReserva(r.id, motivo)
      }
    },
    onSuccess: async () => {
      setCancelando(null)
      await queryClient.invalidateQueries({ queryKey: RESERVAS_KEY })
    },
  })

  const grupos = agruparReservas(data?.data ?? [])

  // RF-04.8: el motivo solo es obligatorio si la reserva es de otra
  // persona, que es el caso de un Admin cancelando la de un docente.
  const esAjena = cancelando ? cancelando.grupo.creadoPor !== user?.id : false
  const motivoFalta = esAjena && cancelando?.motivo.trim() === ""
  const confirmacion = useConfirmacionDeLlegada()

  return (
    <div className="mx-auto max-w-4xl">
      <EncabezadoDePagina
        titulo={user?.rol === "ADMIN" ? "Reservas" : "Mis reservas"}
        descripcion={
          user?.rol === "ADMIN"
            ? "Todas las reservas del sistema y los bloqueos por evaluación."
            : "Tus reservas. Cada tarjeta es una clase, con todas sus PCs adentro."
        }
        accion={
          <Button asChild>
            <Link to="/reservas/nueva">Nueva reserva</Link>
          </Button>
        }
      />

      <label className="mb-4 flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={incluirCanceladas}
          onChange={(e) => {
            setIncluirCanceladas(e.target.checked)
            // Cambiar el filtro cambia la colección: quedarse en la página 5
            // de la anterior puede caer más allá del final y mostrar vacío.
            setPagina(1)
          }}
        />
        Mostrar también las canceladas
      </label>

      {confirmacion && (
        <Alert className="border-exito/40 bg-exito/10 mb-4" role="status">
          <AlertDescription>{confirmacion}</AlertDescription>
        </Alert>
      )}

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}
      {cancelar.error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(cancelar.error)}</AlertDescription>
        </Alert>
      )}

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}
      {!isLoading && grupos.length === 0 && (
        <p className="text-muted-foreground">Todavía no hay reservas.</p>
      )}

      <div className="grid gap-3">
        {grupos.map((grupo) => {
          const clave = claveDeTarjeta(grupo)
          // Antes exigía grupoId, así que en un bloqueo por evaluación el
          // panel de confirmación NUNCA se abría: apretar "Cancelar" no
          // hacía absolutamente nada.
          const enCurso =
            cancelando !== null && claveDeTarjeta(cancelando.grupo) === clave
          const estado = estadoDelGrupo(grupo)
          const cancelables = grupo.reservas.filter((r) => r.estado === "CONFIRMADA")

          return (
            <Card key={clave}>
              <CardContent className="grid gap-3 pt-4">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div>
                    <p className="font-medium">
                      {grupo.materiaNombre
                        ? `${grupo.materiaNombre} — ${grupo.cursoNombre}`
                        : `Bloqueo por evaluación · ${grupo.reservas.length} PC${grupo.reservas.length === 1 ? "" : "s"}`}
                    </p>
                    <p className="text-sm">
                      {/* Capitaliza la función y no la clase `capitalize`:
                          esa pone en mayúscula cada palabra y en castellano
                          quedaba "Martes, 4 De Agosto". */}
                      {formatearFechaLargaCapitalizada(grupo.fecha)}{" "}
                      <span className="text-muted-foreground tabular-nums">
                        {grupo.horaInicio}–{grupo.horaFin}
                      </span>
                    </p>
                    {/* Un bloqueo por evaluación no es de ningún docente: la
                        línea con el guion suelto no decía nada. */}
                    {(grupo.nombreDocenteSnapshot || grupo.esRecurrente) && (
                      <p className="text-muted-foreground text-sm">
                        {grupo.nombreDocenteSnapshot}
                        {grupo.esRecurrente &&
                          `${grupo.nombreDocenteSnapshot ? " · " : ""}reserva recurrente`}
                      </p>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <EstadoDeReserva estado={estado} />
                    {estado === "CONFIRMADA" && !enCurso && (
                      <>
                        {/* Cambiar de máquina no tiene sentido en un bloqueo
                            por evaluación: ahí las PCs se eligen a mano y no
                            hay un docente esperando una en particular. */}
                        {!grupo.esBloqueoEvaluacion && (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() =>
                              setCambiandoPC(cambiandoPC === clave ? null : clave)
                            }
                          >
                            Cambiar computadora
                          </Button>
                        )}
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() =>
                            setCancelando({ grupo, soloEsta: true, motivo: "" })
                          }
                        >
                          {grupo.esBloqueoEvaluacion ? "Levantar bloqueo" : "Cancelar"}
                        </Button>
                      </>
                    )}
                  </div>
                </div>

                {/* Las PCs de la reserva. Es lo que antes faltaba: sin esto
                    una reserva de ocho PCs eran ocho tarjetas idénticas. */}
                <div className="flex flex-wrap gap-1.5">
                  {grupo.reservas.map((r) => (
                    <Badge
                      key={r.id}
                      variant={
                        r.estado === "CANCELADA" || r.estado === "NO_RETIRADA"
                          ? "destructive"
                          : "outline"
                      }
                      title={
                        r.estado === "CANCELADA" && r.motivoCancelacion
                          ? `Cancelada: ${r.motivoCancelacion}`
                          : r.estado === "NO_RETIRADA"
                            ? "No se retiró a tiempo, así que quedó libre para otro docente"
                            : undefined
                      }
                    >
                      {r.etiqueta}
                      {r.carroNombre && ` · ${r.carroNombre}`}
                    </Badge>
                  ))}
                </div>

                {/* RF-04.7 / RF-03.8: una cascada puede cancelar algunas PCs
                    del grupo y dejar el resto en pie. El motivo de cada una
                    se muestra acá porque es la explicación de por qué la
                    reserva quedó incompleta. */}
                {grupo.reservas
                  .filter((r) => r.estado === "CANCELADA" && r.motivoCancelacion)
                  .map((r) => (
                    <p key={r.id} className="text-muted-foreground text-sm">
                      {r.etiqueta}: {r.motivoCancelacion}
                    </p>
                  ))}

                {cambiandoPC === clave && (
                  <CambiarPCDeReserva grupo={grupo} onListo={() => setCambiandoPC(null)} />
                )}

                {enCurso && cancelando && (
                  <div className="grid gap-3 rounded-md border p-3">
                    <p className="text-sm">
                      {grupo.esBloqueoEvaluacion
                        ? `Se libera${cancelables.length === 1 ? "" : "n"} ${cancelables.length} PC${cancelables.length === 1 ? "" : "s"} de este bloqueo. Vuelven a estar disponibles para reservar.`
                        : `Se ${cancelables.length === 1 ? "cancela" : "cancelan"} las ${cancelables.length} PC${cancelables.length === 1 ? "" : "s"} de esta reserva.`}
                    </p>

                    {/* RF-04.6: elegir entre esta fecha o esta y las
                        siguientes solo tiene sentido si es recurrente. */}
                    {grupo.esRecurrente && (
                      <div className="grid gap-1.5">
                        <span className="text-sm font-medium">¿Qué querés cancelar?</span>
                        <label className="flex items-center gap-2 text-sm">
                          <input
                            type="radio"
                            name={`alcance-${clave}`}
                            checked={cancelando.soloEsta}
                            onChange={() =>
                              setCancelando({ ...cancelando, soloEsta: true })
                            }
                          />
                          Solo esta fecha
                        </label>
                        <label className="flex items-center gap-2 text-sm">
                          <input
                            type="radio"
                            name={`alcance-${clave}`}
                            checked={!cancelando.soloEsta}
                            onChange={() =>
                              setCancelando({ ...cancelando, soloEsta: false })
                            }
                          />
                          Esta fecha y todas las siguientes
                        </label>
                      </div>
                    )}

                    <div className="grid gap-1.5">
                      <Label htmlFor={`motivo-${clave}`}>
                        Motivo {esAjena ? "(obligatorio)" : "(opcional)"}
                      </Label>
                      <Input
                        id={`motivo-${clave}`}
                        value={cancelando.motivo}
                        onChange={(e) =>
                          setCancelando({ ...cancelando, motivo: e.target.value })
                        }
                        placeholder={
                          esAjena
                            ? "El docente va a recibir este texto en la notificación"
                            : ""
                        }
                      />
                      {motivoFalta && (
                        <p className="text-destructive text-sm">
                          Al cancelar la reserva de otra persona el motivo es obligatorio.
                        </p>
                      )}
                    </div>

                    <div className="flex gap-2">
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={motivoFalta || cancelar.isPending}
                        onClick={() => cancelar.mutate(cancelando)}
                      >
                        Confirmar cancelación
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={cancelar.isPending}
                        onClick={() => setCancelando(null)}
                      >
                        Volver
                      </Button>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          )
        })}
      </div>

      {/* El paginador cuenta reservas (una fila por PC), no las tarjetas
          agrupadas que se ven arriba: es lo que pagina el backend, y
          contarlo de otra forma daría un total que no cierra con las
          páginas. */}
      {data && (
        <Paginador meta={data.meta} onCambiarPagina={setPagina} etiqueta="reservas" />
      )}
    </div>
  )
}
