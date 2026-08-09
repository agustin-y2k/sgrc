import { useQuery } from "@tanstack/react-query"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import * as adminApi from "@/features/admin/api"
import { PRESTAMOS_KEY, REFRESCO_DEL_MOSTRADOR } from "@/features/admin/entregas/compartido"
import * as reservasApi from "@/features/reservas/api"

/**
 * Cuántos equipos están físicamente acá en este momento.
 *
 * Es el reverso de LoQueEstaAfuera, y la pregunta que faltaba: esa tarjeta
 * dice qué salió, esta dice con qué se cuenta si alguien golpea la puerta
 * pidiendo una máquina. Se responde con lo que ya existe —el inventario y
 * los préstamos abiertos—, sin endpoint nuevo.
 *
 * "Acá" no es lo mismo que "se puede entregar": una máquina en
 * mantenimiento está en el laboratorio y no se le da a nadie. Por eso el
 * número grande cuenta presencia y el renglón de abajo descuenta lo que no
 * está en circulación, en vez de mezclar las dos cosas en un solo total que
 * después nadie sabe qué significa.
 */
export function EnElLaboratorio() {
  const { data: inventario, error: errorInventario } = useQuery({
    queryKey: ["reporte", "inventario", "estado"],
    queryFn: adminApi.reporteEstadoDelInventario,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  const { data: prestamos, error: errorPrestamos } = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  // Un fallo no puede convertirse en un cero: "0 afuera" y "no se pudo
  // consultar" llevan a decisiones opuestas, y el mostrador se opera con
  // esto a la vista. Mismo criterio que los indicadores de la pantalla.
  if (errorInventario || errorPrestamos) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>En el laboratorio ahora</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-muted-foreground text-sm">
            No se pudo consultar cuántos equipos hay acá.
          </p>
        </CardContent>
      </Card>
    )
  }

  const filas = inventario?.data ?? []
  // El total del inventario ya viene sin los dados de baja: no son parte del
  // parque y nadie los espera de vuelta.
  const total = filas.reduce((suma, f) => suma + f.total, 0)
  const fueraDeCirculacion = filas.reduce(
    (suma, f) => suma + f.enMantenimiento + f.fueraDeServicio,
    0
  )

  // Un préstamo no cambia el estado del equipo, así que "afuera" se cuenta
  // aparte y puede incluir una máquina que salió camino al técnico.
  const afuera = prestamos?.data.length ?? 0
  const presentes = total - afuera

  return (
    <Card>
      <CardHeader>
        <CardTitle>En el laboratorio ahora</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-1">
        <p className="text-2xl font-semibold tabular-nums">
          {presentes} de {total} {total === 1 ? "equipo" : "equipos"}
        </p>
        <p className="text-muted-foreground text-sm">
          {afuera} afuera ·{" "}
          {fueraDeCirculacion === 0
            ? "todos en circulación"
            : `${fueraDeCirculacion} fuera de circulación`}
        </p>
        {fueraDeCirculacion > 0 && (
          <p className="text-muted-foreground text-xs">
            Los que están fuera de circulación siguen acá, pero no se entregan.
          </p>
        )}
      </CardContent>
    </Card>
  )
}
