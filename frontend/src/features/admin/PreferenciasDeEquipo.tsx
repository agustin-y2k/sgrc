import { datosDeMarca, marcaValida, SIN_ATRIBUTO } from "./alcanceDeMarca"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { invalidarInventario } from "./invalidarInventario"
import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  CamposDeMarca,
  MARCA_VACIA,
  type ValoresDeMarca,
} from "@/features/admin/CamposDeMarca"
import * as adminApi from "@/features/admin/api"
import { materiasConLaMismaPrioridad } from "@/features/admin/prioridadEmpatada"
import type { PreferenciaDeEquipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"
import { AyudaDePrioridad } from "@/features/admin/AyudaDePrioridad"

/** RF-03.21 — para qué materias es preferente este equipo. */

export function PreferenciasDeEquipo({ equipoId }: { equipoId: string }) {
  const queryClient = useQueryClient()
  const [valores, setValores] = useState<ValoresDeMarca>(MARCA_VACIA)
  // La marca que se está corrigiendo, o null para dar una nueva de alta. El
  // formulario es el mismo: los campos son los mismos y la materia no se
  // edita, así que un segundo formulario sería el mismo repetido.
  const [editando, setEditando] = useState<PreferenciaDeEquipo | null>(null)

  const preferenciasKey = ["preferencias", "equipo", equipoId]
  const { data, isLoading, isSuccess, isFetching, error, refetch } = useQuery({
    queryKey: preferenciasKey,
    queryFn: () => adminApi.listarPreferenciasDeEquipo(equipoId),
  })

  // El catálogo se comparte entre todos los equipos, así que la clave no
  // lleva el id: se pide una vez y sirve para todas las fichas.
  const consultaMaterias = useQuery({
    queryKey: ["catalogo-prioridades"],
    queryFn: () => adminApi.catalogoDePrioridades(),
  })

  // Invalida el prefijo y no sólo la clave de este equipo: en el inventario el
  // panel del carro puede estar abierto al mismo tiempo, y su aviso de empate
  // se arma con las marcas de todos estos equipos.
  const invalidar = () => invalidarInventario(queryClient)

  const limpiar = () => {
    setEditando(null)
    setValores(MARCA_VACIA)
  }

  /** Carga la marca en el formulario para corregirla. */
  const empezarAEditar = (p: PreferenciaDeEquipo) => {
    setEditando(p)
    setValores({
      materiaNombre: p.materiaNombre,
      modalidad: p.modalidad ?? (p.cursoExacto ? SIN_ATRIBUTO : ""),
      anio: p.anio === undefined ? "" : String(p.anio),
      division: p.division ?? (p.cursoExacto ? SIN_ATRIBUTO : ""),
      prioridad: String(p.prioridad),
      alcance: p.cursoExacto ? "CURSO" : "GENERAL",
    })
  }

  const marcar = useMutation({
    mutationFn: () => {
      if (!listo || !marcaValida(valores, consultaMaterias.data?.data ?? []))
        throw new Error("Revisá la materia y el alcance antes de marcar.")
      return adminApi.marcarPreferencia({
        equipoIds: [equipoId],
        ...datosDeMarca(valores),
      })
    },
    onSuccess: async () => {
      limpiar()
      await invalidar()
    },
  })

  // Corregir el alcance o la prioridad de una marca que ya existe. La materia
  // NO se manda: apuntar a otra es otra marca, no una corrección de ésta, y el
  // backend no la acepta en este PATCH.
  const corregir = useMutation({
    mutationFn: (p: PreferenciaDeEquipo) => {
      if (!listo || !marcaValida(valores, consultaMaterias.data?.data ?? []))
        throw new Error("Revisá la materia y el alcance antes de guardar.")
      const { materiaNombre: _nombre, ...datos } = datosDeMarca(valores)
      return adminApi.editarPreferencia(p.id, {
        ...datos,
      })
    },
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
  if (error || consultaMaterias.error)
    return (
      <ErrorDeConsulta
        mensaje="No pudimos consultar las materias y las prioridades del equipo."
        reintentando={isFetching || consultaMaterias.isFetching}
        onReintentar={() => {
          void refetch()
          void consultaMaterias.refetch()
        }}
      />
    )
  const listo =
    isSuccess && !isFetching && consultaMaterias.isSuccess && !consultaMaterias.isFetching
  if (!listo) return <p role="status">Consultando materias y prioridades…</p>

  const preferencias = data?.data ?? []
  const catalogo = consultaMaterias.data?.data ?? []
  const nombres = [...new Set(catalogo.map((f) => f.materiaNombre))]
  const errorDeAccion = marcar.error ?? corregir.error ?? borrar.error

  // Sólo con una materia elegida: el string vacío del formulario en blanco no
  // coincide con ninguna marca, así que sin este guard toda marca en prioridad 1
  // se leería como empate apenas se abre la ficha.
  //
  // Corrigiendo también avisa: subirle la prioridad a una marca existente hasta
  // la de otra es la forma natural de "hacer que ésta mande", y es exactamente
  // el empate del que hay que avisar. La marca editada no se avisa a sí misma
  // porque se compara por nombre de materia, y el suyo queda fijo.
  const empatadas =
    valores.materiaNombre === ""
      ? []
      : materiasConLaMismaPrioridad(
          preferencias,
          valores.materiaNombre,
          Number(valores.prioridad)
        )

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <div>
        <h3 className="text-sm font-medium">Preferente para</h3>
        <p className="text-muted-foreground text-xs">
          Esta marca ordena la lista de reserva y define quién tiene prioridad sobre este
          dispositivo cuando dos materias lo necesitan en el mismo horario.
        </p>
      </div>
      <AyudaDePrioridad />

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
        <ul aria-label="Marcas de este equipo" className="grid gap-1.5">
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
          className="grid gap-2 sm:grid-cols-2 sm:items-end lg:grid-cols-3"
          onSubmit={(e) => {
            e.preventDefault()
            if (!marcaValida(valores, catalogo) || !listo) return
            if (editando) corregir.mutate(editando)
            else marcar.mutate()
          }}
        >
          {empatadas.length > 0 && (
            <Alert className="sm:col-span-full">
              <AlertDescription>
                Esta máquina ya es preferente de {empatadas.join(", ")} con prioridad{" "}
                {valores.prioridad}. Cuando ambas marcas aplican, con la misma prioridad
                ninguna desplaza a la otra. Revisá sus alcances y elegí un número menor
                para la que deba tener mayor prioridad.
              </AlertDescription>
            </Alert>
          )}
          <CamposDeMarca
            valores={valores}
            onChange={setValores}
            catalogo={catalogo}
            materiaBloqueada={editando !== null}
            idBase={equipoId}
          />
          {editando ? (
            <span className="flex gap-2">
              <Button
                type="submit"
                size="sm"
                disabled={corregir.isPending || !marcaValida(valores, catalogo)}
              >
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
              disabled={!marcaValida(valores, catalogo) || marcar.isPending}
            >
              Marcar
            </Button>
          )}
        </form>
      )}
    </div>
  )
}
