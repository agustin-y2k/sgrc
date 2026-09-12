import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import {
  MAX_ANIO_CURSO,
  MAX_LARGO_DIVISION,
  MAX_LARGO_MODALIDAD,
  MIN_ANIO_CURSO,
  divisionesEnUso,
  modalidadesEnUso,
} from "@/features/academico/types"
import { useCursosDelCicloActivo } from "@/features/academico/useCursosDelCicloActivo"
import * as adminApi from "@/features/admin/api"
import type { PreferenciaDeEquipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

/** RF-03.21 — para qué materias es preferente este equipo. */

/** Nueve escalones son muchos más de los que una escuela va a usar. */
const PRIORIDADES = [1, 2, 3, 4, 5, 6, 7, 8, 9]

export function PreferenciasDeEquipo({ equipoId }: { equipoId: string }) {
  const queryClient = useQueryClient()
  const [materiaNombre, setMateriaNombre] = useState("")
  const [modalidad, setModalidad] = useState("")
  const [anio, setAnio] = useState("")
  const [division, setDivision] = useState("")
  const [prioridad, setPrioridad] = useState("1")
  // La marca que se está corrigiendo, o null para dar una nueva de alta. El
  // formulario es el mismo: los campos son los mismos y la materia no se
  // edita, así que un segundo formulario sería el mismo repetido.
  const [editando, setEditando] = useState<PreferenciaDeEquipo | null>(null)

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

  // Los cursos y las modalidades que la escuela usa de verdad: acotar una
  // marca a algo que no existe no marca nada, y los nombres tienen que estar
  // escritos igual que los del curso para que el cruce los encuentre.
  const cursos = useCursosDelCicloActivo()
  const divisiones = divisionesEnUso(cursos)
  const modalidades = modalidadesEnUso(cursos)

  const invalidar = () => queryClient.invalidateQueries({ queryKey: preferenciasKey })

  const limpiar = () => {
    setEditando(null)
    setMateriaNombre("")
    setModalidad("")
    setAnio("")
    setDivision("")
    setPrioridad("1")
  }

  /** Carga la marca en el formulario para corregirla. */
  const empezarAEditar = (p: PreferenciaDeEquipo) => {
    setEditando(p)
    setMateriaNombre(p.materiaNombre)
    setModalidad(p.modalidad ?? "")
    setAnio(p.anio === undefined ? "" : String(p.anio))
    setDivision(p.division ?? "")
    setPrioridad(String(p.prioridad))
  }

  const marcar = useMutation({
    mutationFn: () =>
      adminApi.marcarPreferencia({
        equipoIds: [equipoId],
        materiaNombre,
        // Vacío se manda como ausente: "toda materia con ese nombre" es un
        // alcance, no un dato faltante.
        modalidad: modalidad.trim() || undefined,
        anio: anio ? Number(anio) : undefined,
        division: division.trim() || undefined,
        prioridad: Number(prioridad),
      }),
    onSuccess: async () => {
      limpiar()
      await invalidar()
    },
  })

  // Corregir el alcance o la prioridad de una marca que ya existe. La materia
  // NO se manda: apuntar a otra es otra marca, no una corrección de ésta, y el
  // backend no la acepta en este PATCH.
  const corregir = useMutation({
    mutationFn: (p: PreferenciaDeEquipo) =>
      adminApi.editarPreferencia(p.id, {
        modalidad: modalidad.trim() || undefined,
        anio: anio ? Number(anio) : undefined,
        division: division.trim() || undefined,
        prioridad: Number(prioridad),
      }),
    onSuccess: async () => {
      limpiar()
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
  const errorDeAccion = marcar.error ?? corregir.error ?? borrar.error

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
              <span className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={corregir.isPending}
                  onClick={() => empezarAEditar(p)}
                >
                  Editar
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={borrar.isPending}
                  onClick={() => borrar.mutate(p.id)}
                >
                  Quitar
                </Button>
              </span>
            </li>
          ))}
        </ul>
      )}

      {/* Sin materias cargadas no hay nada que marcar — pero sí puede haber
          marcas viejas que corregir, así que editar abre el formulario igual. */}
      {nombres.length === 0 && !editando ? (
        <p className="text-muted-foreground text-sm">
          Todavía no hay materias cargadas. Se crean desde Académico.
        </p>
      ) : (
        <form
          className="grid gap-2 sm:grid-cols-[1fr_auto_auto_auto_auto] sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            if (editando) corregir.mutate(editando)
            else marcar.mutate()
          }}
        >
          <div className="grid gap-1.5">
            <Label htmlFor={`materia-pref-${equipoId}`}>Materia</Label>
            {/* Corregir una marca no cambia de materia: para eso está quitarla
                y hacer otra. El selector se deshabilita en vez de esconderse
                para que el formulario siga diciendo de qué marca habla. */}
            <Select
              id={`materia-pref-${equipoId}`}
              value={materiaNombre}
              disabled={editando !== null}
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
          {/* Los tres ejes son independientes y todos opcionales: cada uno
              vacío significa "no acota por esto". Se combinan —"Matemática de
              4°2, Electromecánica"— y a más ejes puestos, más específica es la
              marca y antes gana. */}
          <div className="grid gap-1.5">
            <Label htmlFor={`modalidad-pref-${equipoId}`}>Modalidad</Label>
            <Input
              id={`modalidad-pref-${equipoId}`}
              className="w-44"
              value={modalidad}
              maxLength={MAX_LARGO_MODALIDAD}
              list={modalidades.length > 0 ? `modalidades-pref-${equipoId}` : undefined}
              placeholder="Todas"
              onChange={(e) => setModalidad(e.target.value)}
            />
            {modalidades.length > 0 && (
              <datalist id={`modalidades-pref-${equipoId}`}>
                {modalidades.map((m) => (
                  <option key={m} value={m} />
                ))}
              </datalist>
            )}
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={`anio-pref-${equipoId}`}>Año</Label>
            <Input
              id={`anio-pref-${equipoId}`}
              className="w-24"
              type="number"
              inputMode="numeric"
              min={MIN_ANIO_CURSO}
              max={MAX_ANIO_CURSO}
              value={anio}
              placeholder="Todos"
              onChange={(e) => setAnio(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={`division-pref-${equipoId}`}>División</Label>
            <Input
              id={`division-pref-${equipoId}`}
              className="w-28"
              value={division}
              maxLength={MAX_LARGO_DIVISION}
              list={divisiones.length > 0 ? `divisiones-pref-${equipoId}` : undefined}
              placeholder="Todas"
              onChange={(e) => setDivision(e.target.value)}
            />
            {divisiones.length > 0 && (
              <datalist id={`divisiones-pref-${equipoId}`}>
                {divisiones.map((d) => (
                  <option key={d} value={d} />
                ))}
              </datalist>
            )}
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
          {editando ? (
            <span className="flex gap-2">
              <Button type="submit" size="sm" disabled={corregir.isPending}>
                Guardar
              </Button>
              <Button type="button" variant="outline" size="sm" onClick={limpiar}>
                Cancelar
              </Button>
            </span>
          ) : (
            <Button
              type="submit"
              size="sm"
              disabled={materiaNombre === "" || marcar.isPending}
            >
              Marcar
            </Button>
          )}
        </form>
      )}
    </div>
  )
}
