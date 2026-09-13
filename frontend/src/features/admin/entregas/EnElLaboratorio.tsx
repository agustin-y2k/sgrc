import { useQuery } from "@tanstack/react-query"

import * as adminApi from "@/features/admin/api"
import {
  PRESTAMOS_KEY,
  REFRESCO_DEL_MOSTRADOR,
} from "@/features/admin/entregas/compartido"
import { Indicador } from "@/features/inicio/Indicador"
import * as reservasApi from "@/features/reservas/api"
import { contar } from "@/lib/plural"

/**
 * Cuántos equipos están físicamente acá en este momento.
 *
 * Es una celda de la tira de estado y ya no una tarjeta propia: era un número
 * de un vistazo dibujado como panel, arriba de la mitad de la pantalla y
 * separado de los otros cuatro números por toda la portada. Lo que se mira de
 * reojo va junto y arriba.
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

  const fallo = errorInventario || errorPrestamos

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

  // Un fallo no puede convertirse en un cero: "0 afuera" y "no se pudo
  // consultar" llevan a decisiones opuestas, y el mostrador se opera con esto
  // a la vista. El Indicador dibuja un guion cuando el valor es null.
  return (
    <Indicador
      valor={fallo ? null : total - afuera}
      sufijo={fallo ? undefined : `de ${total}`}
      rotulo="acá ahora"
      detalle={
        fallo
          ? "No se pudo consultar"
          : // "en circulación" es vocabulario de depósito: dice si una máquina
            // está en condiciones de prestarse, pero hay que saberlo de antes.
            // En el mostrador se atiende con alguien esperando enfrente.
            fueraDeCirculacion === 0
            ? `${contar(total, "equipo")}, todos usables`
            : `${fueraDeCirculacion} no se pueden usar`
      }
      a="/inventario"
    />
  )
}
