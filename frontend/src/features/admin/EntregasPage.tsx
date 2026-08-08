import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { Button } from "@/components/ui/button"
import {
  PRESTAMOS_KEY,
  REFRESCO_DEL_MOSTRADOR,
} from "@/features/admin/entregas/compartido"
import { EntregarDeUnaReserva } from "@/features/admin/entregas/EntregarDeUnaReserva"
import { EntregaSuelta } from "@/features/admin/entregas/EntregaSuelta"
import { LoQueEstaAfuera } from "@/features/admin/entregas/LoQueEstaAfuera"
import * as reservasApi from "@/features/reservas/api"

/**
 * RF-08 — el mostrador completo: qué computadoras están afuera, quién se las
 * llevó y cuáles volvieron.
 *
 * Es la vista extendida de lo que el Admin ya tiene en su pantalla de inicio.
 * Sigue existiendo aparte por dos razones: es a donde llevan los avisos de
 * "una máquina no volvió", y es donde se entrega una reserva del día con
 * calma, sin el resto del panel alrededor.
 *
 * Lo que se ve acá NO es "el estado de la PC": no hay ninguna columna que
 * diga "prestada". Se deriva de si existe un préstamo sin devolver, y por eso
 * no puede quedar desincronizado — que es exactamente lo que le pasa al papel
 * cuando alguien devuelve una máquina y nadie tacha el renglón.
 */
export function EntregasPage() {
  const [entregandoSuelta, setEntregandoSuelta] = useState(false)

  const { data } = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  // Qué máquinas están afuera, para no ofrecerlas de nuevo. Se calcula acá y
  // se pasa a los dos formularios: es el mismo dato y una sola consulta.
  const yaAfuera = useMemo(() => new Set((data?.data ?? []).map((p) => p.pcId)), [data])

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
