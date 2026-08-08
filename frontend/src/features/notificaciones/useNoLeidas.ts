import { useQuery } from "@tanstack/react-query"

import * as notificacionesApi from "@/features/notificaciones/api"

export const NOTIFICACIONES_KEY = ["notificaciones"]

/**
 * Cantidad de notificaciones sin leer, para el indicador de la barra de
 * navegación (RF-05.7: "visibles al ingresar al sistema").
 *
 * Se repregunta cada dos minutos porque las notificaciones no las genera
 * quien está mirando la pantalla: aparecen cuando un Admin cancela una
 * reserva o saca un equipo de servicio, en cualquier momento. Sin refresco
 * periódico, alguien con la pestaña abierta toda la mañana no se entera de
 * que le cancelaron la clase hasta que recarga.
 *
 * Dos minutos es un intervalo cómodo para decenas de usuarios contra un
 * único servidor; no hace falta nada más fino para un aviso interno.
 */
export function useNoLeidas() {
  const { data } = useQuery({
    queryKey: [...NOTIFICACIONES_KEY, "NO_LEIDA"],
    queryFn: () => notificacionesApi.listarNotificaciones("NO_LEIDA"),
    refetchInterval: 2 * 60 * 1000,
  })

  // El total del `meta`, no la cantidad de filas: con el listado paginado
  // `data.length` se queda en el tamaño de página (50) y el indicador
  // mentiría justo cuando hay muchas sin leer, que es cuando importa.
  return data?.meta.total ?? 0
}
