import { useEffect, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useLocation, useNavigate, useSearchParams } from "react-router"

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
import { etiquetaDeMateriaYLugar } from "@/features/academico/types"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { Input } from "@/components/ui/input"
import { Select } from "@/components/ui/select"
import { Label } from "@/components/ui/label"
import * as adminApi from "@/features/admin/api"
import { todasLasPaginas } from "@/lib/todas-las-paginas"

const RESERVAS_KEY = ["reservas"]

/** El aviso de "listo, se creó" que deja NuevaReservaPage al navegar acá. */
function useConfirmacionDeLlegada(): string | null {
  const { pathname, search, state } = useLocation()
  const navigate = useNavigate()
  const recibida =
    state && typeof state === "object" && "confirmacion" in state
      ? String((state as { confirmacion: unknown }).confirmacion)
      : null
  const [confirmacion] = useState(recibida)

  useEffect(() => {
    if (recibida !== null) navigate(pathname + search, { replace: true, state: null })
  }, [recibida, pathname, search, navigate])

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
  const [params, setParams] = useSearchParams()
  const incluirCanceladas =
    params.get("canceladas") === "true" || params.get("estado") === "CANCELADA"
  const numeroPagina = Number(params.get("pagina"))
  const pagina =
    Number.isSafeInteger(numeroPagina) && numeroPagina >= 1 ? numeroPagina : 1
  const fechasPersonalizadas = params.has("desde") || params.has("hasta")
  const vista =
    params.get("vista") === "historial"
      ? "historial"
      : params.get("vista") === "todas"
        ? "todas"
        : "proximas"
  const hoy = new Date()
  const fechaLocal = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
  const ayer = new Date(hoy.getFullYear(), hoy.getMonth(), hoy.getDate() - 1)
  const desde =
    params.get("desde") || (vista === "proximas" ? fechaLocal(hoy) : undefined)
  const hasta =
    params.get("hasta") || (vista === "historial" ? fechaLocal(ayer) : undefined)
  const busqueda = params.get("busqueda") || undefined
  const estado = (params.get("estado") || undefined) as EstadoReserva | undefined
  const creadoPor =
    user?.rol === "ADMIN" ? params.get("creadoPor") || undefined : undefined
  const rangoValido = !desde || !hasta || desde <= hasta
  const cambiarParams = (valores: Record<string, string>) => {
    const nuevos = new URLSearchParams(params)
    nuevos.delete("pagina")
    for (const [campo, valor] of Object.entries(valores)) {
      if (valor) nuevos.set(campo, valor)
      else nuevos.delete(campo)
    }
    setParams(nuevos)
  }
  const docentes = useQuery({
    queryKey: ["usuarios", "filtro-reservas"],
    queryFn: () =>
      todasLasPaginas((page) => adminApi.listarUsuarios({ page, pageSize: 200 })),
    enabled: user?.rol === "ADMIN",
  })
  /** La tarjeta con el panel de cancelación abierto, si hay alguna. */
  const [cancelando, setCancelando] = useState<string | null>(null)
  const [cambiandoEquipo, setCambiandoEquipo] = useState<string | null>(null)

  const { data, isPending, isSuccess, isFetching, error, refetch } = useQuery({
    queryKey: [
      ...RESERVAS_KEY,
      "por-clases",
      { incluirCanceladas, pagina, desde, hasta, busqueda, estado, creadoPor },
    ],
    enabled: rangoValido,
    queryFn: () =>
      reservasApi.listarReservas({
        incluirCanceladas,
        page: pagina,
        porClases: true,
        pageSize: 20,
        desde,
        hasta,
        busqueda,
        estado,
        creadoPor,
      }),
  })

  const grupos = agruparReservas(!error && rangoValido ? (data?.data ?? []) : [])
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
            <Link
              to="/reservas/nueva"
              state={{
                volverA: `/reservas${params.size ? `?${params.toString()}` : ""}`,
              }}
            >
              Nueva reserva
            </Link>
          </Button>
        }
      />

      <div className="mb-4 flex flex-wrap gap-2" aria-label="Vista de reservas">
        {(
          [
            ["proximas", "Próximas"],
            ["historial", "Historial"],
            ["todas", "Todas"],
          ] as const
        ).map(([valor, texto]) => (
          <Button
            key={valor}
            type="button"
            variant={!fechasPersonalizadas && vista === valor ? "default" : "outline"}
            className="h-11"
            aria-pressed={!fechasPersonalizadas && vista === valor}
            onClick={() => cambiarParams({ vista: valor, desde: "", hasta: "" })}
          >
            {texto}
          </Button>
        ))}
      </div>
      <p className="text-muted-foreground mb-4 text-sm">
        {fechasPersonalizadas
          ? "Consultando el rango de fechas indicado en los filtros."
          : vista === "proximas"
            ? "Clases de hoy y de las fechas posteriores."
            : vista === "historial"
              ? "Clases de las fechas anteriores a hoy."
              : "Todas las fechas."}
      </p>
      <form
        key={params.toString()}
        className="mb-4 grid gap-3 rounded-xl border p-3 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault()
          const valores = new FormData(e.currentTarget)
          cambiarParams({
            desde: String(valores.get("desde") || ""),
            hasta: String(valores.get("hasta") || ""),
            busqueda: String(valores.get("busqueda") || "").trim(),
            estado: String(valores.get("estado") || ""),
            creadoPor:
              user?.rol === "ADMIN"
                ? String(valores.get("creadoPor") ?? creadoPor ?? "")
                : "",
          })
        }}
      >
        <div className="grid gap-1.5">
          <Label htmlFor="desde-reservas">Desde</Label>
          <Input id="desde-reservas" name="desde" type="date" defaultValue={desde} />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="hasta-reservas">Hasta</Label>
          <Input id="hasta-reservas" name="hasta" type="date" defaultValue={hasta} />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="buscar-reservas">Buscar materia, curso o modalidad</Label>
          <Input
            id="buscar-reservas"
            name="busqueda"
            maxLength={100}
            defaultValue={busqueda}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="estado-reservas">Estado de los equipos</Label>
          <Select id="estado-reservas" name="estado" defaultValue={estado ?? ""}>
            <option value="">Todos los estados</option>
            {Object.entries(ETIQUETA_RESERVA).map(([valor, etiqueta]) => (
              <option key={valor} value={valor}>
                {etiqueta}
              </option>
            ))}
          </Select>
          <p className="text-muted-foreground text-xs">
            Muestra clases que contienen equipos en ese estado, con todos sus equipos para
            conservar el contexto.
          </p>
        </div>
        {user?.rol === "ADMIN" && (
          <div className="grid gap-1.5">
            <Label htmlFor="docente-reservas">Docente o responsable</Label>
            <Select
              key={docentes.data?.data.map((d) => d.id).join(",") ?? "cargando"}
              id="docente-reservas"
              name="creadoPor"
              defaultValue={creadoPor ?? ""}
              disabled={docentes.isPending || !!docentes.error}
            >
              <option value="">Todos los responsables</option>
              {creadoPor && !docentes.data?.data.some((d) => d.id === creadoPor) && (
                <option value={creadoPor}>Cuenta seleccionada</option>
              )}
              {docentes.data?.data.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.nombre} {d.apellido} · {d.email}
                </option>
              ))}
            </Select>
            {docentes.error && (
              <ErrorDeConsulta
                mensaje="No pudimos consultar los responsables."
                reintentando={docentes.isFetching}
                onReintentar={() => {
                  void docentes.refetch()
                }}
              />
            )}
          </div>
        )}
        <div className="flex flex-wrap items-end gap-2">
          <Button type="submit" className="h-11">
            Aplicar filtros
          </Button>
          <Button
            type="button"
            variant="outline"
            className="h-11"
            onClick={() => setParams(new URLSearchParams())}
          >
            Limpiar filtros
          </Button>
        </div>
      </form>
      {!rangoValido && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            La fecha «Hasta» debe ser igual o posterior a «Desde».
          </AlertDescription>
        </Alert>
      )}
      <label className="mb-4 flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={incluirCanceladas}
          disabled={estado === "CANCELADA"}
          onChange={(e) => {
            cambiarParams({ canceladas: e.target.checked ? "true" : "" })
            // Cambiar el filtro cambia la colección: quedarse en la página 5
            // de la anterior puede caer más allá del final y mostrar vacío.
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
        <div className="mb-4">
          <ErrorDeConsulta
            mensaje="No pudimos cargar las reservas."
            reintentando={isFetching}
            onReintentar={() => {
              void refetch()
            }}
          />
        </div>
      )}

      {rangoValido && isPending && (
        <p role="status" className="text-muted-foreground">
          Cargando…
        </p>
      )}
      {rangoValido && isSuccess && !error && grupos.length === 0 && (
        <p className="text-muted-foreground">
          No hay clases que coincidan con estos filtros.
        </p>
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
              <CardContent className="grid min-w-0 gap-3 pt-4">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0 break-words">
                    <p className="font-medium">
                      {grupo.materiaNombre
                        ? etiquetaDeMateriaYLugar({
                            materiaNombre: grupo.materiaNombre,
                            cursoNombre: grupo.cursoNombre ?? "",
                          })
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
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
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

      {/* Cada página contiene las clases y los bloqueos completos que resume. */}
      {data && !error && rangoValido && (
        <Paginador
          meta={data.meta}
          onCambiarPagina={(valor) => cambiarParams({ pagina: String(valor) })}
          etiqueta={user?.rol === "ADMIN" ? "clases y bloqueos" : "clases"}
        />
      )}
    </div>
  )
}
