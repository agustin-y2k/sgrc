import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as adminApi from "@/features/admin/api"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-03.21 — para qué materias es preferente este equipo.
 *
 * La marca **sólo ordena** la lista al reservar: la materia marcada ve esta
 * máquina primero y las demás la ven al final. Nadie queda excluido, así que
 * poner o sacar una marca no cancela nada ni pide confirmación — a
 * diferencia de casi todo lo demás en esta pantalla, acá no hay ninguna
 * consecuencia que avisar.
 *
 * El alta de a un equipo vive acá porque es la corrección puntual ("esta
 * máquina también"). Marcar un carro entero se hace desde la lista, con
 * selección múltiple.
 */

/** Los años que admite un curso, igual que el CHECK de `curso.nombre`. */
const ANIOS = [1, 2, 3, 4, 5, 6]
const DIVISIONES = "ABCDEFGHIJKLMNOPQRSTUVWXYZ".split("")
/** Nueve escalones son muchos más de los que una escuela va a usar. */
const PRIORIDADES = [1, 2, 3, 4, 5, 6, 7, 8, 9]

export function PreferenciasDeEquipo({ equipoId }: { equipoId: string }) {
  const queryClient = useQueryClient()
  const [materiaNombre, setMateriaNombre] = useState("")
  const [anio, setAnio] = useState("")
  const [division, setDivision] = useState("")
  const [prioridad, setPrioridad] = useState("1")

  const preferenciasKey = ["preferencias", "equipo", equipoId]
  const { data, isLoading, error } = useQuery({
    queryKey: preferenciasKey,
    queryFn: () => adminApi.listarPreferenciasDeEquipo(equipoId),
  })

  // Los nombres se comparten entre todos los equipos, así que la clave no
  // lleva el id: se pide una vez y sirve para todas las fichas.
  const { data: materias } = useQuery({
    queryKey: ["materias-en-uso"],
    queryFn: () => adminApi.materiasEnUso(),
  })

  const invalidar = () => queryClient.invalidateQueries({ queryKey: preferenciasKey })

  const marcar = useMutation({
    mutationFn: () =>
      adminApi.marcarPreferencia({
        equipoIds: [equipoId],
        materiaNombre,
        // Vacío se manda como ausente: "toda materia con ese nombre" es un
        // alcance, no un dato faltante.
        anio: anio ? Number(anio) : undefined,
        division: division || undefined,
        prioridad: Number(prioridad),
      }),
    onSuccess: async () => {
      setMateriaNombre("")
      setAnio("")
      setDivision("")
      setPrioridad("1")
      await invalidar()
    },
  })

  const borrar = useMutation({
    mutationFn: (id: string) => adminApi.borrarPreferencia(id),
    onSuccess: invalidar,
  })

  if (isLoading) {
    return <p className="text-muted-foreground text-sm">Cargando preferencias…</p>
  }
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{getErrorMessage(error)}</AlertDescription>
      </Alert>
    )
  }

  const preferencias = data?.data ?? []
  const nombres = materias?.data ?? []
  const errorDeAccion = marcar.error ?? borrar.error

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <div>
        <h3 className="text-sm font-medium">Preferente para</h3>
        <p className="text-muted-foreground text-xs">
          Al reservar, esa materia ve este equipo primero y las demás lo ven al final.
          Nadie queda excluido: cualquiera lo puede reservar igual.
        </p>
      </div>

      {errorDeAccion && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(errorDeAccion)}</AlertDescription>
        </Alert>
      )}

      {preferencias.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          Sin marcas: este equipo aparece en el orden de siempre para todas las materias.
        </p>
      ) : (
        <ul className="grid gap-1.5">
          {preferencias.map((p) => (
            <li key={p.id} className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-sm">
                {p.alcance} <Badge variant="outline">Prioridad {p.prioridad}</Badge>
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={borrar.isPending}
                onClick={() => borrar.mutate(p.id)}
              >
                Quitar
              </Button>
            </li>
          ))}
        </ul>
      )}

      {nombres.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          Todavía no hay materias cargadas. Se crean desde Académico.
        </p>
      ) : (
        <form
          className="grid gap-2 sm:grid-cols-[1fr_auto_auto_auto_auto] sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            marcar.mutate()
          }}
        >
          <div className="grid gap-1.5">
            <Label htmlFor={`materia-pref-${equipoId}`}>Materia</Label>
            <Select
              id={`materia-pref-${equipoId}`}
              value={materiaNombre}
              onChange={(e) => setMateriaNombre(e.target.value)}
            >
              <option value="">Elegí una materia…</option>
              {nombres.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={`anio-pref-${equipoId}`}>Año</Label>
            <Select
              id={`anio-pref-${equipoId}`}
              className="w-auto"
              value={anio}
              onChange={(e) => {
                setAnio(e.target.value)
                // Sin año, una división no significa nada: no existen "todas
                // las B". El backend lo rechaza; acá directamente no se puede
                // llegar a ese estado.
                if (e.target.value === "") setDivision("")
              }}
            >
              <option value="">Todos</option>
              {ANIOS.map((a) => (
                <option key={a} value={a}>
                  {a}°
                </option>
              ))}
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={`division-pref-${equipoId}`}>División</Label>
            <Select
              id={`division-pref-${equipoId}`}
              className="w-auto"
              value={division}
              disabled={anio === ""}
              onChange={(e) => setDivision(e.target.value)}
            >
              <option value="">Todas</option>
              {DIVISIONES.map((d) => (
                <option key={d} value={d}>
                  {d}
                </option>
              ))}
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={`prioridad-pref-${equipoId}`}>Prioridad</Label>
            <Select
              id={`prioridad-pref-${equipoId}`}
              className="w-auto"
              value={prioridad}
              onChange={(e) => setPrioridad(e.target.value)}
            >
              {PRIORIDADES.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </Select>
          </div>
          <Button
            type="submit"
            size="sm"
            disabled={materiaNombre === "" || marcar.isPending}
          >
            Marcar
          </Button>
        </form>
      )}
    </div>
  )
}
