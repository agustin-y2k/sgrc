import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { MateriasDeCurso } from "@/features/academico/MateriasDeCurso"
import { SelectorDeCurso } from "@/features/academico/SelectorDeCurso"
import * as academicoApi from "@/features/academico/api"
import {
  componerNombreDeCurso,
  esNombreDeCursoValido,
  separarNombreDeCurso,
} from "@/features/academico/types"
import type { CicloLectivo, Curso } from "@/features/academico/types"
import { getErrorMessage } from "@/lib/api-client"

export function CursosDeCiclo({ ciclo }: { ciclo: CicloLectivo }) {
  const queryClient = useQueryClient()
  const [anioNuevo, setAnioNuevo] = useState("1")
  const [divisionNueva, setDivisionNueva] = useState("A")
  const [editando, setEditando] = useState<{ curso: Curso; nombre: string } | null>(null)
  const [eliminando, setEliminando] = useState<Curso | null>(null)
  const [cursoAbierto, setCursoAbierto] = useState<string | null>(null)

  const soloLectura = ciclo.archivado
  const cursosKey = ["cursos", ciclo.id]

  const { data, isLoading } = useQuery({
    queryKey: cursosKey,
    queryFn: () => academicoApi.listarCursos(ciclo.id),
  })

  const invalidar = () => queryClient.invalidateQueries({ queryKey: cursosKey })

  const crear = useMutation({
    mutationFn: () =>
      academicoApi.crearCurso(ciclo.id, componerNombreDeCurso(anioNuevo, divisionNueva)),
    onSuccess: async () => {
      setAnioNuevo("1")
      setDivisionNueva("A")
      await invalidar()
    },
  })

  const editar = useMutation({
    mutationFn: ({ curso, nombre }: { curso: Curso; nombre: string }) =>
      academicoApi.editarCurso(curso.id, nombre.trim().toUpperCase()),
    onSuccess: async () => {
      setEditando(null)
      await invalidar()
    },
  })

  const eliminar = useMutation({
    mutationFn: (curso: Curso) => academicoApi.eliminarCurso(curso.id),
    onSuccess: async () => {
      setEliminando(null)
      await invalidar()
    },
  })

  if (isLoading) return <p className="text-muted-foreground text-sm">Cargando cursos…</p>

  const cursos = data?.data ?? []
  const error = crear.error ?? editar.error ?? eliminar.error

  return (
    <div className="grid gap-3">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {soloLectura && (
        <p className="text-muted-foreground text-sm">
          Este ciclo está archivado: sus cursos y materias se conservan como referencia,
          pero no se pueden modificar ni reservar sobre ellos.
        </p>
      )}

      {!soloLectura && (
        <form
          className="grid gap-2 sm:grid-cols-[1fr_auto] sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            crear.mutate()
          }}
        >
          <div className="grid gap-1.5">
            <p className="text-sm font-medium">Nuevo curso</p>
            <SelectorDeCurso
              idPrefijo={`curso-${ciclo.id}`}
              anio={anioNuevo}
              division={divisionNueva}
              onCambio={(a, d) => {
                setAnioNuevo(a)
                setDivisionNueva(d)
              }}
            />
          </div>
          <Button type="submit" size="sm" disabled={crear.isPending}>
            Agregar
          </Button>
        </form>
      )}

      {cursos.length === 0 && (
        <p className="text-muted-foreground text-sm">
          Este ciclo todavía no tiene cursos.
        </p>
      )}

      {cursos.map((curso) => {
        const editandoEste = editando?.curso.id === curso.id
        const eliminandoEste = eliminando?.id === curso.id
        const abierto = cursoAbierto === curso.id

        return (
          <div key={curso.id} className="grid gap-2 rounded-md border p-3">
            {editandoEste && editando ? (
              <form
                className="grid gap-2 sm:grid-cols-[1fr_auto_auto] sm:items-end"
                onSubmit={(e) => {
                  e.preventDefault()
                  editar.mutate(editando)
                }}
              >
                <div className="grid gap-1.5">
                  {/* Un nombre que no matchee el patrón —una fila vieja o
                      cargada por API— cae al campo de texto, para no quedarse
                      sin poder editarla. */}
                  {separarNombreDeCurso(editando.nombre) ? (
                    <SelectorDeCurso
                      idPrefijo={`editar-curso-${curso.id}`}
                      anio={separarNombreDeCurso(editando.nombre)!.anio}
                      division={separarNombreDeCurso(editando.nombre)!.division}
                      onCambio={(a, d) =>
                        setEditando({ ...editando, nombre: componerNombreDeCurso(a, d) })
                      }
                    />
                  ) : (
                    <>
                      <Label htmlFor={`editar-curso-${curso.id}`}>Nombre</Label>
                      <Input
                        id={`editar-curso-${curso.id}`}
                        value={editando.nombre}
                        onChange={(e) =>
                          setEditando({ ...editando, nombre: e.target.value })
                        }
                      />
                      {!esNombreDeCursoValido(editando.nombre.trim().toUpperCase()) && (
                        <p className="text-destructive text-sm">
                          Formato inválido. Por ejemplo: 1°A, 6°Z.
                        </p>
                      )}
                    </>
                  )}
                </div>
                <Button
                  type="submit"
                  size="sm"
                  disabled={
                    !esNombreDeCursoValido(editando.nombre.trim().toUpperCase()) ||
                    editar.isPending
                  }
                >
                  Guardar
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setEditando(null)}
                >
                  Cancelar
                </Button>
              </form>
            ) : (
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="font-medium">{curso.nombre}</span>
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    aria-expanded={abierto}
                    onClick={() => setCursoAbierto(abierto ? null : curso.id)}
                  >
                    {abierto ? "Ocultar materias" : "Materias"}
                  </Button>
                  {!soloLectura && !eliminandoEste && (
                    <>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setEditando({ curso, nombre: curso.nombre })}
                      >
                        Renombrar
                      </Button>
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => setEliminando(curso)}
                      >
                        Eliminar
                      </Button>
                    </>
                  )}
                </div>
              </div>
            )}

            {eliminandoEste && (
              <div className="grid gap-2 rounded-md border p-3">
                {/* RF-02.11: eliminar un curso arrastra sus materias. El
                    backend lo rechaza si alguna tiene reservas. */}
                <p className="text-destructive text-sm">
                  Eliminar «{curso.nombre}» borra también todas sus materias. Solo se
                  puede si ninguna tiene reservas.
                </p>
                <div className="flex gap-2">
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={eliminar.isPending}
                    onClick={() => eliminar.mutate(curso)}
                  >
                    Confirmar
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setEliminando(null)}>
                    Volver
                  </Button>
                </div>
              </div>
            )}

            {abierto && (
              <div className="border-t pt-3">
                <MateriasDeCurso curso={curso} soloLectura={soloLectura} />
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
