import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useSearchParams } from "react-router"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { Paginador } from "@/components/Paginador"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import * as notificacionesApi from "@/features/notificaciones/api"
import { PreferenciasDeCorreo } from "@/features/notificaciones/PreferenciasDeCorreo"
import { PanelDeSoporte } from "@/features/sugerencias/PanelDeSoporte"
import type { Notificacion } from "@/features/notificaciones/types"
import { NOTIFICACIONES_KEY } from "@/features/notificaciones/useNoLeidas"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaYHora } from "@/lib/fechas"

/** Fecha y hora en la zona del navegador. */
function formatearFecha(iso: string): string {
  return formatearFechaYHora(iso)
}

/** RF-05: notificaciones internas. */
export function NotificacionesPage() {
  const queryClient = useQueryClient()
  // El botón "Pedir ayuda" de la barra de arriba entra por acá: abre la
  // pantalla con el formulario desplegado, en vez de dejar a alguien que
  // necesita algo buscando dónde escribir.
  const [parametros] = useSearchParams()
  const abrirSoporte = parametros.get("soporte") === "nuevo"
  const [mostrarLeidas, setMostrarLeidas] = useState(false)
  const [pagina, setPagina] = useState(1)

  const { data, isLoading, error } = useQuery({
    queryKey: [...NOTIFICACIONES_KEY, mostrarLeidas ? "TODAS" : "NO_LEIDA", pagina],
    queryFn: () =>
      notificacionesApi.listarNotificaciones(
        mostrarLeidas ? undefined : "NO_LEIDA",
        pagina
      ),
  })

  // Invalida toda la familia: el contador de la barra de navegación usa
  // otra clave y también tiene que bajar al marcar algo como leído.
  const invalidar = () => queryClient.invalidateQueries({ queryKey: NOTIFICACIONES_KEY })

  const marcarLeida = useMutation({
    mutationFn: (n: Notificacion) => notificacionesApi.marcarLeida(n.id),
    onSuccess: invalidar,
  })

  const marcarTodas = useMutation({
    mutationFn: notificacionesApi.marcarTodasLeidas,
    onSuccess: invalidar,
  })

  const notificaciones = data?.data ?? []
  const noLeidas = notificaciones.filter((n) => n.estado === "NO_LEIDA")
  const errorAccion = marcarLeida.error ?? marcarTodas.error

  return (
    <div className="mx-auto max-w-3xl">
      <EncabezadoDePagina
        titulo="Notificaciones"
        descripcion="Avisos del sistema: cancelaciones, cuentas pendientes y cambios que te afectan."
        accion={
          noLeidas.length > 0 ? (
            <Button
              variant="outline"
              size="sm"
              disabled={marcarTodas.isPending}
              onClick={() => marcarTodas.mutate()}
            >
              Marcar todas como leídas
            </Button>
          ) : undefined
        }
      />

      {/* Las conversaciones van ARRIBA de los avisos y de las preferencias:
          es lo único de esta pantalla donde hay una persona esperando del
          otro lado. */}
      <PanelDeSoporte abiertoDeEntrada={abrirSoporte} />

      {/* Reemplaza a la línea de "algunos también te llegan por correo": el
          panel dice cuáles, y deja cambiarlo. Es para todos, no solo para el
          Admin: un docente también recibe correos por sus reservas. */}
      <PreferenciasDeCorreo />

      <label className="mb-4 flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={mostrarLeidas}
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
          <AlertDescription>{getErrorMessage(error ?? errorAccion)}</AlertDescription>
        </Alert>
      )}

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}

      {!isLoading && notificaciones.length === 0 && (
        <p className="text-muted-foreground">
          {mostrarLeidas ? "No tenés ninguna notificación." : "No tenés avisos sin leer."}
        </p>
      )}

      <div className="grid gap-3">
        {notificaciones.map((n) => (
          <Card key={n.id}>
            <CardContent className="grid gap-2 pt-4">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <p className={n.estado === "NO_LEIDA" ? "font-medium" : undefined}>
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
                  {n.tipo === "DOCENTE_PENDIENTE" && (
                    <Button asChild variant="outline" size="sm">
                      <Link to="/admin/aprobacion">Ir a aprobar</Link>
                    </Button>
                  )}
                  {n.tipo === "RESERVA_CANCELADA" && (
                    <Button asChild variant="outline" size="sm">
                      <Link to="/reservas">Ver mis reservas</Link>
                    </Button>
                  )}
                  {n.tipo === "LICENCIA_POR_VENCER" && (
                    <Button asChild variant="outline" size="sm">
                      <Link to="/admin/licencias">Ver licencias</Link>
                    </Button>
                  )}
                  {(n.tipo === "RESERVA_POR_COMENZAR" ||
                    n.tipo === "RESERVA_NO_RETIRADA") && (
                    <Button asChild variant="outline" size="sm">
                      <Link to="/reservas">Ver mis reservas</Link>
                    </Button>
                  )}
                  {n.tipo === "EQUIPO_SIN_DEVOLVER" && (
                    <Button asChild variant="outline" size="sm">
                      <Link to="/admin/entregas">Ver entregas</Link>
                    </Button>
                  )}
                  {n.estado === "NO_LEIDA" && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={marcarLeida.isPending}
                      onClick={() => marcarLeida.mutate(n)}
                    >
                      Marcar como leída
                    </Button>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {data && (
        <Paginador
          meta={data.meta}
          onCambiarPagina={setPagina}
          etiqueta="notificaciones"
        />
      )}
    </div>
  )
}
