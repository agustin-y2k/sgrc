import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { DocentesDeMateria } from "@/features/academico/DocentesDeMateria"
import * as academicoApi from "@/features/academico/api"
import type { Curso, Materia } from "@/features/academico/types"
import { getErrorMessage } from "@/lib/api-client"

export function MateriasDeCurso({
  curso,
  soloLectura,
}: {
  curso: Curso
  soloLectura: boolean
}) {
  const queryClient = useQueryClient()
  const [nombreNueva, setNombreNueva] = useState("")
  const [editando, setEditando] = useState<{ materia: Materia; nombre: string } | null>(
    null
  )
  const [eliminando, setEliminando] = useState<Materia | null>(null)
  const [docentesAbiertos, setDocentesAbiertos] = useState<string | null>(null)

  const materiasKey = ["materias", curso.id]
  const { data, isLoading } = useQuery({
    queryKey: materiasKey,
    queryFn: () => academicoApi.listarMaterias(curso.id),
  })

  const invalidar = () => queryClient.invalidateQueries({ queryKey: materiasKey })

  const crear = useMutation({
    mutationFn: () => academicoApi.crearMateria(curso.id, nombreNueva.trim()),
    onSuccess: async () => {
      setNombreNueva("")
      await invalidar()
    },
  })

  const editar = useMutation({
    mutationFn: ({ materia, nombre }: { materia: Materia; nombre: string }) =>
      academicoApi.editarMateria(materia.id, nombre.trim()),
    onSuccess: async () => {
      setEditando(null)
      await invalidar()
    },
  })

  const eliminar = useMutation({
    mutationFn: (materia: Materia) => academicoApi.eliminarMateria(materia.id),
    onSuccess: async () => {
      setEliminando(null)
      await invalidar()
    },
  })

  if (isLoading)
    return <p className="text-muted-foreground text-sm">Cargando materias…</p>

  const materias = data?.data ?? []
  const error = crear.error ?? editar.error ?? eliminar.error

  return (
    <div className="grid gap-3">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
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
            <Label htmlFor={`materia-${curso.id}`}>Nueva materia</Label>
            <Input
              id={`materia-${curso.id}`}
              value={nombreNueva}
              onChange={(e) => setNombreNueva(e.target.value)}
              placeholder="Ej: Matemáticas"
            />
          </div>
          <Button
            type="submit"
            size="sm"
            disabled={nombreNueva.trim() === "" || crear.isPending}
          >
            Agregar
          </Button>
        </form>
      )}

      {materias.length === 0 && (
        <p className="text-muted-foreground text-sm">
          Este curso todavía no tiene materias.
        </p>
      )}

      {materias.map((materia) => {
        const editandoEsta = editando?.materia.id === materia.id
        const eliminandoEsta = eliminando?.id === materia.id

        return (
          <div key={materia.id} className="grid gap-2 rounded-md border p-3">
            {editandoEsta && editando ? (
              <form
                className="grid gap-2 sm:grid-cols-[1fr_auto_auto] sm:items-end"
                onSubmit={(e) => {
                  e.preventDefault()
                  editar.mutate(editando)
                }}
              >
                <div className="grid gap-1.5">
                  <Label htmlFor={`editar-materia-${materia.id}`}>Nombre</Label>
                  <Input
                    id={`editar-materia-${materia.id}`}
                    value={editando.nombre}
                    onChange={(e) => setEditando({ ...editando, nombre: e.target.value })}
                  />
                </div>
                <Button
                  type="submit"
                  size="sm"
                  disabled={editando.nombre.trim() === "" || editar.isPending}
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
                <span className="font-medium">{materia.nombre}</span>
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    aria-expanded={docentesAbiertos === materia.id}
                    onClick={() =>
                      setDocentesAbiertos(
                        docentesAbiertos === materia.id ? null : materia.id
                      )
                    }
                  >
                    {docentesAbiertos === materia.id ? "Ocultar docentes" : "Docentes"}
                  </Button>
                  {!soloLectura && !eliminandoEsta && (
                    <>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setEditando({ materia, nombre: materia.nombre })}
                      >
                        Renombrar
                      </Button>
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => setEliminando(materia)}
                      >
                        Eliminar
                      </Button>
                    </>
                  )}
                </div>
              </div>
            )}

            {docentesAbiertos === materia.id && (
              <div className="border-t pt-2">
                <DocentesDeMateria materia={materia} soloLectura={soloLectura} />
              </div>
            )}

            {eliminandoEsta && (
              <div className="grid gap-2 rounded-md border p-3">
                {/* RF-02.11: solo se puede borrar si no tiene reservas; si
                    las tiene, el backend responde 409 y se muestra su
                    mensaje tal cual. */}
                <p className="text-destructive text-sm">
                  Eliminar «{materia.nombre}» es definitivo. Solo se puede si todavía no
                  tiene ninguna reserva.
                </p>
                <div className="flex gap-2">
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={eliminar.isPending}
                    onClick={() => eliminar.mutate(materia)}
                  >
                    Confirmar
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setEliminando(null)}>
                    Volver
                  </Button>
                </div>
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

/**
 * RF-02.2 — el nombre de un curso es año + división ("5°A"), así que se elige
 * de dos listas en vez de escribirse.
 */
