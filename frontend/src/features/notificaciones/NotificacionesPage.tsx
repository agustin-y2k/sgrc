import { useEffect, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useLocation, useSearchParams } from "react-router"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { Paginador } from "@/components/Paginador"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import * as notificacionesApi from "@/features/notificaciones/api"
import { PreferenciasDeCorreo } from "@/features/notificaciones/PreferenciasDeCorreo"
import { PanelDeSoporte } from "@/features/sugerencias/PanelDeSoporte"
import type { Notificacion, TipoNotificacion } from "@/features/notificaciones/types"
import { NOTIFICACIONES_KEY } from "@/features/notificaciones/useNoLeidas"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaYHora } from "@/lib/fechas"

/** Fecha y hora en la zona del navegador. */
function formatearFecha(iso: string): string {
  return formatearFechaYHora(iso)
}

/**
 * A qué pantalla lleva cada aviso.
 *
 * Es un Record COMPLETO sobre TipoNotificacion y no una cadena de `if`: con la
 * cadena, agregar un tipo nuevo al backend no rompía nada acá, simplemente el
 * aviso llegaba sin botón y quedaba sin salida. Así pasó con los tres pedidos
 * —el de liberación, el de materia y su resolución—, que estuvieron llegando a
 * la campana sin ninguna forma de actuar sobre ellos. Ahora TypeScript exige
 * que cada tipo diga a dónde va, aunque sea `null`.
 *
 * `null` indica que no hay una acción asociada.
 */
const ACCION_POR_TIPO: Record<TipoNotificacion, { a: string; texto: string } | null> = {
  GENERAL: null,
  DOCENTE_PENDIENTE: { a: "/admin/aprobacion", texto: "Ir a aprobar" },
  RESERVA_CANCELADA: { a: "/reservas", texto: "Ver mis reservas" },
  RESERVA_REUBICADA: { a: "/reservas", texto: "Ver mis reservas" },
  LICENCIA_POR_VENCER: { a: "/admin/licencias", texto: "Ver licencias" },
  EQUIPO_SIN_DEVOLVER: { a: "/admin/entregas", texto: "Ver entregas" },
  // Le llega al dueño de la reserva: la decisión de liberar o no es suya y se
  // toma desde sus reservas.
  PEDIDO_DE_LIBERACION: { a: "/reservas", texto: "Ver mis reservas" },
  PEDIDO_DE_MATERIA: { a: "/admin/pedidos-de-materia", texto: "Ver los pedidos" },
  // La resolución le llega al docente, y sus materias se ven en su perfil,
  // que es desde donde pidió.
  PEDIDO_DE_MATERIA_RESUELTO: { a: "/perfil", texto: "Ver mi perfil" },
  SUGERENCIA: { a: "/notificaciones?soporte=consultas", texto: "Ver conversación" },
  SUGERENCIA_RESPONDIDA: {
    a: "/notificaciones?soporte=consultas",
    texto: "Ver conversación",
  },
}

/** RF-05: notificaciones internas. */
export function NotificacionesPage() {
  const queryClient = useQueryClient()
  // El botón "Pedir ayuda" de la barra de arriba entra por acá: abre la
  // pantalla con el formulario desplegado, en vez de dejar a alguien que
  // necesita algo buscando dónde escribir.
  const [parametros] = useSearchParams()
  const ubicacion = useLocation()
  const abrirSoporte = ["nuevo", "consultas"].includes(parametros.get("soporte") ?? "")
  const [mostrarLeidas, setMostrarLeidas] = useState(false)
  const [pagina, setPagina] = useState(1)
  const [resultado, setResultado] = useState("")
  const [errorAccion, setErrorAccion] = useState("")

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: [...NOTIFICACIONES_KEY, mostrarLeidas ? "TODAS" : "NO_LEIDA", pagina],
    queryFn: () =>
      notificacionesApi.listarNotificaciones(
        mostrarLeidas ? undefined : "NO_LEIDA",
        pagina
      ),
  })
  useEffect(() => {
    if (!data || error || isFetching) return
    const ultima = Math.max(1, Math.ceil(data.meta.total / data.meta.pageSize))
    if (pagina > ultima) setPagina(ultima)
  }, [data, error, isFetching, pagina])

  // Invalida toda la familia: el contador de la barra de navegación usa
  // otra clave y también tiene que bajar al marcar algo como leído.
  const invalidar = () => queryClient.invalidateQueries({ queryKey: NOTIFICACIONES_KEY })

  const marcarLeida = useMutation({
    onMutate: iniciarAccion,
    onError: (e) => setErrorAccion(getErrorMessage(e)),
    mutationFn: (n: Notificacion) => notificacionesApi.marcarLeida(n.id),
    onSuccess: invalidar,
  })

  const marcarTodas = useMutation({
    onMutate: iniciarAccion,
    onError: (e) => setErrorAccion(getErrorMessage(e)),
    mutationFn: notificacionesApi.marcarTodasLeidas,
    onSuccess: async (r) => {
      setResultado(`Avisos marcados como leídos: ${r.marcadas}.`)
      await invalidar()
    },
  })

  const borrar = useMutation({
    onMutate: iniciarAccion,
    onError: (e) => setErrorAccion(getErrorMessage(e)),
    mutationFn: (n: Notificacion) => notificacionesApi.borrar(n.id),
    onSuccess: invalidar,
  })

  const borrarLeidas = useMutation({
    onMutate: iniciarAccion,
    onError: (e) => setErrorAccion(getErrorMessage(e)),
    mutationFn: notificacionesApi.borrarLeidas,
    onSuccess: async (r) => {
      setResultado(`Avisos leídos borrados: ${r.borradas}.`)
      await invalidar()
    },
  })
  function iniciarAccion() {
    setResultado("")
    setErrorAccion("")
  }
  const ocupado =
    marcarLeida.isPending ||
    marcarTodas.isPending ||
    borrar.isPending ||
    borrarLeidas.isPending
  const bloquear = ocupado || isFetching || !!error || !data

  const notificaciones = data?.data ?? []
  const noLeidas = notificaciones.filter((n) => n.estado === "NO_LEIDA")
  const leidas = notificaciones.filter((n) => n.estado === "LEIDA")

  return (
    <div className="mx-auto max-w-3xl">
      <EncabezadoDePagina
        titulo="Avisos"
        descripcion="Avisos del sistema: cancelaciones, cuentas pendientes y cambios que te afectan."
        accion={
          /* Los dos gestos del mismo movimiento: cerrar lo pendiente y, un paso
             después, sacárselo de encima. Cada botón aparece sólo si hay algo
             sobre lo que actuar. */
          noLeidas.length > 0 || leidas.length > 0 ? (
            <span className="flex flex-wrap gap-2">
              {noLeidas.length > 0 && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={bloquear}
                  onClick={() => marcarTodas.mutate()}
                >
                  Marcar todas como leídas
                </Button>
              )}
              {leidas.length > 0 && (
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={bloquear}
                  onClick={() => borrarLeidas.mutate()}
                >
                  Borrar todos los avisos leídos
                </Button>
              )}
            </span>
          ) : undefined
        }
      />

      {/* Las conversaciones van ARRIBA de los avisos y de las preferencias:
          es lo único de esta pantalla donde hay una persona esperando del
          otro lado. */}
      <PanelDeSoporte
        abiertoDeEntrada={abrirSoporte}
        solicitudDeApertura={abrirSoporte ? ubicacion.key : undefined}
        abrirFormulario={parametros.get("soporte") === "nuevo"}
        hiloId={parametros.get("hilo") ?? undefined}
      />

      {/* Reemplaza a la línea de "algunos también te llegan por correo": el
          panel dice cuáles, y deja cambiarlo. Es para todos, no solo para el
          Admin: un docente también recibe correos por sus reservas. */}
      <PreferenciasDeCorreo />

      <label className="mb-4 flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={mostrarLeidas}
          disabled={ocupado}
          onChange={(e) => {
            setMostrarLeidas(e.target.checked)
            // Cambiar el filtro cambia la colección: la página actual puede
            // caer más allá del final de la nueva.
            setPagina(1)
          }}
        />
        Mostrar también las leídas
      </label>

      {(error || errorAccion) && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            {error ? getErrorMessage(error) : errorAccion}
            {error && (
              <Button
                variant="outline"
                disabled={isFetching}
                onClick={() => void refetch()}
              >
                Reintentar avisos
              </Button>
            )}
          </AlertDescription>
        </Alert>
      )}
      {resultado && (
        <p role="status" className="mb-4 text-sm">
          {resultado}
        </p>
      )}

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}

      {!isLoading && !error && notificaciones.length === 0 && (
        <p className="text-muted-foreground">
          {mostrarLeidas ? "No tenés ninguna notificación." : "No tenés avisos sin leer."}
        </p>
      )}

      <div className="grid gap-3">
        {!error &&
          notificaciones.map((n) => (
            <Card key={n.id}>
              <CardContent className="grid gap-2 pt-4">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <p
                    className={`min-w-0 flex-1 whitespace-pre-line [overflow-wrap:anywhere] ${n.estado === "NO_LEIDA" ? "font-medium" : ""}`}
                  >
                    {n.mensaje}
                  </p>
                  {n.estado === "NO_LEIDA" ? (
                    <Badge>Sin leer</Badge>
                  ) : (
                    <Badge variant="secondary">Leída</Badge>
                  )}
                </div>

                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-muted-foreground text-sm">
                    {formatearFecha(n.creadaEn)}
                  </span>
                  <div className="flex flex-wrap gap-2">
                    {/* La acción sale del `tipo`, no de leer el mensaje: la
                      redacción de un aviso puede cambiar y el botón tiene que
                      seguir funcionando. */}
                    {ACCION_POR_TIPO[n.tipo] && (
                      <Button asChild variant="outline" size="sm">
                        <Link
                          to={
                            ACCION_POR_TIPO[n.tipo]!.a +
                            ((n.tipo === "SUGERENCIA" ||
                              n.tipo === "SUGERENCIA_RESPONDIDA") &&
                            n.hiloId
                              ? `&hilo=${encodeURIComponent(n.hiloId)}`
                              : "")
                          }
                        >
                          {ACCION_POR_TIPO[n.tipo]!.texto}
                        </Link>
                      </Button>
                    )}
                    {n.estado === "NO_LEIDA" && (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={bloquear}
                        onClick={() => marcarLeida.mutate(n)}
                      >
                        Marcar como leída
                      </Button>
                    )}
                    {/* Borrar aparece sólo en los leídos, que es la misma regla
                      que aplica el servidor: un aviso sin leer todavía es una
                      tarea, y hacerla desaparecer de un clic la pierde. */}
                    {n.estado === "LEIDA" && (
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={bloquear}
                        onClick={() => borrar.mutate(n)}
                      >
                        Borrar
                      </Button>
                    )}
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
      </div>

      {data && !error && (
        <Paginador
          disabled={ocupado}
          meta={data.meta}
          onCambiarPagina={(p) => {
            if (!ocupado) setPagina(p)
          }}
          etiqueta="notificaciones"
        />
      )}
    </div>
  )
}
