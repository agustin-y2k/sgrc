import { useQuery } from "@tanstack/react-query"

import * as notificacionesApi from "@/features/notificaciones/api"

export const NOTIFICACIONES_KEY = ["notificaciones"]

/**
 * Cantidad de notificaciones sin leer, para el indicador de la barra de
 * navegación (RF-05.7: "visibles al ingresar al sistema").
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
