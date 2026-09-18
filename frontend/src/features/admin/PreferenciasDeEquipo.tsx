import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { CamposDeMarca, MARCA_VACIA, type ValoresDeMarca } from "@/features/admin/CamposDeMarca"
import * as adminApi from "@/features/admin/api"
import { materiasConLaMismaPrioridad } from "@/features/admin/prioridadEmpatada"
import type { PreferenciaDeEquipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

/** RF-03.21 — para qué materias es preferente este equipo. */

export function PreferenciasDeEquipo({ equipoId }: { equipoId: string }) {
  const queryClient = useQueryClient()
  const [valores, setValores] = useState<ValoresDeMarca>(MARCA_VACIA)
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

  const invalidar = () => queryClient.invalidateQueries({ queryKey: preferenciasKey })

  const limpiar = () => {
    setEditando(null)
    setValores(MARCA_VACIA)
  }

  /** Carga la marca en el formulario para corregirla. */
  const empezarAEditar = (p: PreferenciaDeEquipo) => {
    setEditando(p)
    setValores({
      materiaNombre: p.materiaNombre,
      modalidad: p.modalidad ?? "",
      anio: p.anio === undefined ? "" : String(p.anio),
      division: p.division ?? "",
      prioridad: String(p.prioridad),
    })
  }

  const marcar = useMutation({
    mutationFn: () =>
      adminApi.marcarPreferencia({
        equipoIds: [equipoId],
        materiaNombre: valores.materiaNombre,
        // Vacío se manda como ausente: "toda materia con ese nombre" es un
        // alcance, no un dato faltante.
        modalidad: valores.modalidad.trim() || undefined,
        anio: valores.anio ? Number(valores.anio) : undefined,
        division: valores.division.trim() || undefined,
        prioridad: Number(valores.prioridad),
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
        modalidad: valores.modalidad.trim() || undefined,
        anio: valores.anio ? Number(valores.anio) : undefined,
        division: valores.division.trim() || undefined,
        prioridad: Number(valores.prioridad),
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

  // Sólo al dar de alta y con una materia elegida: corrigiendo una marca que ya
  // existe, ella misma está en la lista y se avisaría a sí misma; y sin materia
  // todavía no hay nada que comparar, sólo el string vacío del formulario en blanco.
  const empatadas =
    editando || valores.materiaNombre === ""
      ? []
      : materiasConLaMismaPrioridad(
          preferencias,
          valores.materiaNombre,
          Number(valores.prioridad),
        )

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
          {empatadas.length > 0 && (
            <Alert className="sm:col-span-full">
              <AlertDescription>
                Esta máquina ya es preferente de {empatadas.join(", ")} con prioridad{" "}
                {valores.prioridad}. Con la misma prioridad ninguna le gana a la otra: si
                querés que una mande, ponele un número distinto.
              </AlertDescription>
            </Alert>
          )}
          <CamposDeMarca
            valores={valores}
            onChange={setValores}
            materias={nombres}
            materiaBloqueada={editando !== null}
            idBase={equipoId}
          />
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
              disabled={valores.materiaNombre === "" || marcar.isPending}
            >
              Marcar
            </Button>
          )}
        </form>
      )}
    </div>
  )
}
