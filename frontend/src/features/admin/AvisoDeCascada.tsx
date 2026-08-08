import { Alert, AlertDescription } from "@/components/ui/alert"
import type { ResultadoCascada } from "@/features/admin/types"

/**
 * Qué se llevó puesto sacar un equipo de circulación (RF-03.8 y RF-03.9).
 *
 * Pasar una máquina a mantenimiento o darla de baja cancela las reservas
 * futuras de otros docentes y les manda un aviso. El backend devuelve la
 * cuenta desde siempre; hasta que esto existió, las dos pantallas la tiraban
 * y quien apretaba el botón no se enteraba de que había cancelado clases
 * ajenas.
 *
 * Devuelve null cuando no se canceló nada, que es el caso normal: un cartel
 * diciendo "se cancelaron 0 reservas" es ruido en la operación de todos los
 * días, y compite por atención con el que sí importa.
 */
export function AvisoDeCascada({ resultado }: { resultado: ResultadoCascada | null }) {
  if (!resultado || resultado.reservasCanceladas === 0) return null

  const { reservasCanceladas: reservas, docentesNotificados: docentes } = resultado

  return (
    <Alert>
      <AlertDescription>
        Se {reservas === 1 ? "canceló" : "cancelaron"} {reservas}{" "}
        {reservas === 1 ? "reserva" : "reservas"} y se avisó a {docentes}{" "}
        {docentes === 1 ? "docente" : "docentes"}.
      </AlertDescription>
    </Alert>
  )
}
