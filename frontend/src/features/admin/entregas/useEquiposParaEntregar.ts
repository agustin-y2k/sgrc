import { useMemo } from "react"
import { useQuery } from "@tanstack/react-query"
import * as inventoryApi from "@/features/inventory/api"
import * as reservasApi from "@/features/reservas/api"
import { ETIQUETA_ESTADO_EQUIPO } from "@/features/inventory/types"
import { PRESTAMOS_KEY, REFRESCO_DEL_MOSTRADOR } from "./compartido"
import type { EquipoParaEntregar } from "./SelectorDeEquipos"

export function useEquiposParaEntregar(yaAfuera: Set<string>, reparacion = false) {
  const carros = useQuery({ queryKey: ["carros"], queryFn: inventoryApi.listarCarros })
  const inventario = useQuery({
    queryKey: ["equipos"],
    queryFn: () => inventoryApi.listarEquipos(),
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })
  const prestamos = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })
  const listo = [carros, inventario, prestamos].every(
    (q) => q.isSuccess && !q.error && !q.isFetching
  )
  const equipos: EquipoParaEntregar[] = useMemo(() => {
    const afuera = new Set([
      ...yaAfuera,
      ...(prestamos.data?.data ?? []).map((p) => p.equipoId),
    ])
    const nombres = new Map((carros.data?.data ?? []).map((c) => [c.id, c.nombre]))
    return (inventario.data?.data ?? [])
      .filter(
        (e) =>
          !e.dadoDeBaja &&
          (reparacion ? e.estado !== "DISPONIBLE" : e.estado === "DISPONIBLE") &&
          !afuera.has(e.id)
      )
      .map((e) => ({
        id: e.id,
        etiqueta: e.etiqueta,
        donde: e.carroId ? (nombres.get(e.carroId) ?? "") : e.tipo,
        nota: reparacion ? ETIQUETA_ESTADO_EQUIPO[e.estado] : undefined,
      }))
  }, [yaAfuera, prestamos.data, carros.data, inventario.data, reparacion])
  return {
    equipos,
    listo,
    error: carros.error ?? inventario.error ?? prestamos.error,
    reintentando: carros.isFetching || inventario.isFetching || prestamos.isFetching,
    reintentar: () => {
      void Promise.all([carros.refetch(), inventario.refetch(), prestamos.refetch()])
    },
  }
}
