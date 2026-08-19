import { useQuery } from "@tanstack/react-query"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import * as adminApi from "@/features/admin/api"
import { PRESTAMOS_KEY, REFRESCO_DEL_MOSTRADOR } from "@/features/admin/entregas/compartido"
import * as reservasApi from "@/features/reservas/api"
import { contar, plural } from "@/lib/plural"

/** Cuántos equipos están físicamente acá en este momento. */
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
  // consultar" llevan a decisiones opuestas, y el mostrador se opera con esto
  // a la vista.
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
          {presentes} de {contar(total, "equipo")}
        </p>
        {/* "en circulación" es vocabulario de depósito: dice si una máquina
            está en condiciones de prestarse, pero hay que saberlo de antes.
            En el mostrador se atiende con alguien esperando enfrente, así que
            la línea tiene que leerse sin traducir nada. */}
        <p className="text-muted-foreground text-sm">
          {afuera} afuera ·{" "}
          {fueraDeCirculacion === 0
            ? "todas las demás se pueden usar"
            : `${fueraDeCirculacion} sin poder usarse`}
        </p>
        {fueraDeCirculacion > 0 && (
          <p className="text-muted-foreground text-xs">
            {plural(fueraDeCirculacion, "La que no se puede usar sigue", "Las que no se pueden usar siguen")}{" "}
            en el laboratorio, pero no se {plural(fueraDeCirculacion, "entrega", "entregan")}.
          </p>
        )}
      </CardContent>
    </Card>
  )
}
