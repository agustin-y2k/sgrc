import { Alert, AlertDescription } from "@/components/ui/alert"
import type { ResultadoCascada } from "@/features/admin/types"

/** Qué se llevó puesto sacar un equipo de circulación (RF-03.8 y RF-03.9). */
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
