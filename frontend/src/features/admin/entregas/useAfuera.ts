import { useQuery } from "@tanstack/react-query"

import {
  PRESTAMOS_KEY,
  REFRESCO_DEL_MOSTRADOR,
} from "@/features/admin/entregas/compartido"
import * as reservasApi from "@/features/reservas/api"

/**
 * Cuántas computadoras están fuera del laboratorio ahora mismo, para el
 * indicador de la barra de navegación.
 *
 * Es lo que sostiene el seguimiento de una máquina que no volvió. El aviso
 * por correo del cierre de jornada sale UNA sola vez, así que si la única
 * huella de una máquina perdida fuera ese correo, alcanzaría con borrarlo —o
 * con no leerlo— para que nadie se enterara nunca más. Acá el número no se va
 * hasta que alguien la recibe de verdad, o hasta que se la da de baja del
 * inventario.
 *
 * Comparte queryKey con la pantalla de Entregas a propósito: recibir una
 * máquina invalida esa clave y el contador baja solo, sin un refresco aparte
 * que podría discrepar de lo que la pantalla muestra.
 */
export function useAfuera(habilitado: boolean) {
  const { data } = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
    enabled: habilitado,
  })

  return data?.data.length ?? 0
}
