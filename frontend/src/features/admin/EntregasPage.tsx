import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  PRESTAMOS_KEY,
  REFRESCO_DEL_MOSTRADOR,
} from "@/features/admin/entregas/compartido"
import { EntregarDeUnaReserva } from "@/features/admin/entregas/EntregarDeUnaReserva"
import { EntregaSuelta } from "@/features/admin/entregas/EntregaSuelta"
import { LoQueEstaAfuera } from "@/features/admin/entregas/LoQueEstaAfuera"
import * as reservasApi from "@/features/reservas/api"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-08 — el mostrador completo: qué computadoras están afuera, quién se las
 * llevó y cuáles volvieron.
 */
export function EntregasPage() {
  const [entregandoSuelta, setEntregandoSuelta] = useState(false)

  const { data, error } = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  // Qué máquinas están afuera, para no ofrecerlas de nuevo.
  const yaAfuera = useMemo(() => new Set((data?.data ?? []).map((p) => p.equipoId)), [data])

  return (
    <div>
      <EncabezadoDePagina
        titulo="Entregas y devoluciones"
        descripcion="Qué computadoras están afuera del laboratorio, quién se las llevó y cuándo tienen que volver. Reemplaza el registro en papel."
        accion={
          !entregandoSuelta && (
            <Button variant="outline" onClick={() => setEntregandoSuelta(true)}>
              Entregar sin reserva
            </Button>
          )
        }
      />

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            No se pudo consultar qué hay afuera del laboratorio, así que los
            formularios pueden ofrecer equipos que ya están prestados. Probá recargar.
            ({getErrorMessage(error)})
          </AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4">
        {/* El formulario va en el cuerpo y no en el slot de acción del
            encabezado: ese slot es `shrink-0` y está pensado para botones,
            así que una tarjeta entera adentro queda apretada contra el borde
            en un teléfono. */}
        {entregandoSuelta && (
          <EntregaSuelta yaAfuera={yaAfuera} onCerrar={() => setEntregandoSuelta(false)} />
        )}
        <LoQueEstaAfuera />
        <EntregarDeUnaReserva yaAfuera={yaAfuera} />
      </div>
    </div>
  )
}
