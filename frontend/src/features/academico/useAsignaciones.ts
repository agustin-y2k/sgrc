import { useQuery, useQueryClient } from "@tanstack/react-query"

import * as academicoApi from "@/features/academico/api"
import type { AsignacionDocente } from "@/features/academico/types"

/** La clave de caché, para poder invalidarla desde donde se asigna. */
export function claveDeAsignaciones(cicloId: string) {
  return ["asignaciones", cicloId]
}

/**
 * Quién dicta qué en un ciclo (RF-02.6).
 *
 * Se pide una vez por ciclo y no una por materia: la relación se mira desde
 * las dos puntas —las materias de un curso muestran sus docentes, el listado
 * de usuarios muestra qué dicta cada quien— y consultarla de a una materia es
 * un N+1 en cada carga de pantalla.
 *
 * Devuelve una lista vacía mientras carga o si no hay ciclo: es información
 * que acompaña, y ninguna pantalla se bloquea por no tenerla todavía.
 */
export function useAsignacionesDelCiclo(
  cicloId: string | undefined
): AsignacionDocente[] {
  const { data } = useQuery({
    queryKey: claveDeAsignaciones(cicloId ?? ""),
    queryFn: () => academicoApi.listarAsignaciones(cicloId!),
    enabled: !!cicloId,
  })
  return data?.data ?? []
}

/**
 * Lo mismo, para las pantallas que no saben de qué ciclo hablan: el activo.
 * Las materias de años archivados no cuentan — quién dicta qué se recrea
 * entero cada año (RF-02.5), y lo que interesa saber de una persona es lo que
 * está dando ahora.
 */
export function useAsignacionesDelCicloActivo(): AsignacionDocente[] {
  const { data: ciclos } = useQuery({
    queryKey: ["ciclos"],
    queryFn: academicoApi.listarCiclos,
  })
  return useAsignacionesDelCiclo(ciclos?.data.find((c) => c.activo)?.id)
}

/**
 * Para usar después de asignar, cambiar de rol o quitar un docente: sin esto
 * la lista de arriba sigue mostrando lo de antes hasta recargar la página.
 */
export function useInvalidarAsignaciones() {
  const queryClient = useQueryClient()
  return () => queryClient.invalidateQueries({ queryKey: ["asignaciones"] })
}
