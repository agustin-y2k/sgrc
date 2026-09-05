import { useQuery } from "@tanstack/react-query"

import * as academicoApi from "@/features/academico/api"
import type { Curso } from "@/features/academico/types"

/**
 * Los cursos del ciclo que está abierto.
 *
 * Existe porque desde que la división la escribe la institución (007) no
 * alcanza con una lista de la A a la Z: lo único que sabe si acá se dice "4°"
 * o "4ta" son los cursos que ya están cargados. Lo usan las pantallas que
 * ofrecen un curso sin ser la de cursos —marcar un equipo como preferente,
 * anotar a dónde va lo que se entrega—, así que la consulta vive en un solo
 * lugar y las dos comparten la caché.
 *
 * Devuelve una lista vacía mientras carga o si no hay ciclo activo: son
 * sugerencias, y ninguna pantalla se bloquea por no tenerlas.
 */
export function useCursosDelCicloActivo(): Curso[] {
  const { data: ciclos } = useQuery({
    queryKey: ["ciclos"],
    queryFn: academicoApi.listarCiclos,
  })
  const cicloActivo = ciclos?.data.find((c) => c.activo)

  const { data: cursos } = useQuery({
    queryKey: ["cursos", cicloActivo?.id],
    queryFn: () => academicoApi.listarCursos(cicloActivo!.id),
    enabled: !!cicloActivo,
  })

  return cursos?.data ?? []
}
