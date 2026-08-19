import { useEffect, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useLocation, useNavigate } from "react-router"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { EstadoBadge, TONO_RESERVA } from "@/components/EstadoBadge"
import { Paginador } from "@/components/Paginador"
import { formatearFechaLargaCapitalizada } from "@/lib/fechas"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { useAuth } from "@/features/auth/AuthContext"
import * as reservasApi from "@/features/reservas/api"
import { agruparReservas } from "@/features/reservas/types"
import { CambiarEquipoDeReserva } from "@/features/reservas/CambiarEquipoDeReserva"
import { CancelarReserva } from "@/features/reservas/CancelarReserva"
import type { EstadoReserva, GrupoDeReservas } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"

const RESERVAS_KEY = ["reservas"]

/** El aviso de "listo, se creó" que deja NuevaReservaPage al navegar acá. */
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

/** Identifica una tarjeta del listado. */
function claveDeTarjeta(grupo: GrupoDeReservas): string {
  if (grupo.grupoId) return grupo.grupoId
  if (grupo.esBloqueo) {
    return `bloqueo:${grupo.creadoPor ?? "sistema"}:${grupo.fecha}:${grupo.horaInicio}:${grupo.horaFin}`
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

/** Estado del grupo a partir del de sus reservas. */
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
  const [incluirCanceladas, setIncluirCanceladas] = useState(false)
  const [pagina, setPagina] = useState(1)
  /** La tarjeta con el panel de cancelación abierto, si hay alguna. */
  const [cancelando, setCancelando] = useState<string | null>(null)
  const [cambiandoEquipo, setCambiandoEquipo] = useState<string | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: [...RESERVAS_KEY, { incluirCanceladas, pagina }],
    queryFn: () => reservasApi.listarReservas({ incluirCanceladas, page: pagina }),
  })

  const grupos = agruparReservas(data?.data ?? [])
  const confirmacion = useConfirmacionDeLlegada()

  return (
    <div className="mx-auto max-w-4xl">
      <EncabezadoDePagina
        titulo={user?.rol === "ADMIN" ? "Reservas" : "Mis reservas"}
        descripcion={
          user?.rol === "ADMIN"
            ? "Todas las reservas del sistema y los bloqueos administrativos."
            : "Cada tarjeta es una clase, con todas las computadoras que reservaste para ella."
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

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}
      {!isLoading && grupos.length === 0 && (
        <p className="text-muted-foreground">Todavía no hay reservas.</p>
      )}

      <div className="grid gap-3">
        {grupos.map((grupo) => {
          const clave = claveDeTarjeta(grupo)
          // La clave no puede ser grupoId: un bloqueo administrativo no tiene
          // grupo, y el panel de confirmación no se abriría nunca — apretar
          // "Cancelar" no haría absolutamente nada.
          const enCurso = cancelando === clave
          const estado = estadoDelGrupo(grupo)

          return (
            <Card key={clave}>
              <CardContent className="grid gap-3 pt-4">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div>
                    <p className="font-medium">
                      {grupo.materiaNombre
                        ? `${grupo.materiaNombre} — ${grupo.cursoNombre}`
                        : `${grupo.motivoBloqueo ?? "Bloqueado"} · ${grupo.reservas.length} equipo${grupo.reservas.length === 1 ? "" : "s"}`}
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
                    {/* Un bloqueo administrativo no es de ningún docente: la
                        línea con el guion suelto no decía nada. */}
                    {(grupo.nombreDocenteSnapshot || grupo.esRecurrente) && (
                      <p className="text-muted-foreground text-sm">
                        {grupo.nombreDocenteSnapshot}
                        {grupo.esRecurrente &&
                          `${grupo.nombreDocenteSnapshot ? " · " : ""}se repite todas las semanas`}
                      </p>
                    )}
                    {/* Un cartel naranja que dice "No retirada" y nada más
                        deja al docente sin saber qué le pasó ni qué puede
                        hacer: si perdió las máquinas para siempre, si tiene
                        que avisarle a alguien, si le van a reclamar algo.
                        Justo el estado que más preocupa era el que menos
                        explicaba. La regla real es que se liberaron, pero que
                        si siguen en el laboratorio se las entregan igual
                        (RF-08), y eso es lo único accionable que hay para
                        decirle. */}
                    {estado === "NO_RETIRADA" && (
                      <p className="text-muted-foreground text-sm">
                        Pasó el plazo para retirarlas y quedaron libres para otro docente.
                        Si todavía están en el laboratorio te las pueden entregar igual:
                        preguntá en el mostrador.
                      </p>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <EstadoDeReserva estado={estado} />
                    {estado === "CONFIRMADA" && !enCurso && (
                      <>
                        {/* Cambiar de máquina no tiene sentido en un bloqueo
                            por evaluación: ahí los equipos se eligen a mano y no
                            hay un docente esperando una en particular. */}
                        {!grupo.esBloqueo && (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() =>
                              setCambiandoEquipo(cambiandoEquipo === clave ? null : clave)
                            }
                          >
                            Cambiar computadora
                          </Button>
                        )}
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => setCancelando(clave)}
                        >
                          {grupo.esBloqueo ? "Levantar bloqueo" : "Cancelar"}
                        </Button>
                      </>
                    )}
                  </div>
                </div>

                {/* Los equipos de la reserva. Es lo que antes faltaba: sin esto
                    una reserva de ocho equipos eran ocho tarjetas idénticas. */}
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

                {/* RF-04.7 / RF-03.8: una cascada puede cancelar algunas equipos
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

                {cambiandoEquipo === clave && (
                  <CambiarEquipoDeReserva
                    grupo={grupo}
                    onListo={() => setCambiandoEquipo(null)}
                  />
                )}

                {enCurso && (
                  <CancelarReserva grupo={grupo} onListo={() => setCancelando(null)} />
                )}
              </CardContent>
            </Card>
          )
        })}
      </div>

      {/* El paginador cuenta reservas (una fila por equipo), no las tarjetas
          agrupadas que se ven arriba: es lo que pagina el backend, y
          contarlo de otra forma daría un total que no cierra con las
          páginas. */}
      {data && (
        <Paginador meta={data.meta} onCambiarPagina={setPagina} etiqueta="reservas" />
      )}
    </div>
  )
}
