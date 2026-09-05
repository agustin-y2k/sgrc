import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { MateriasDeCurso } from "@/features/academico/MateriasDeCurso"
import { SelectorDeCurso } from "@/features/academico/SelectorDeCurso"
import type { DatosDeCurso } from "@/features/academico/SelectorDeCurso"
import * as academicoApi from "@/features/academico/api"
import {
  divisionesEnUso,
  etiquetaDeCurso,
  modalidadesEnUso,
} from "@/features/academico/types"
import type { CicloLectivo, Curso } from "@/features/academico/types"
import { getErrorMessage } from "@/lib/api-client"

export function CursosDeCiclo({ ciclo }: { ciclo: CicloLectivo }) {
  const queryClient = useQueryClient()
  const [nuevo, setNuevo] = useState<DatosDeCurso>(CURSO_EN_BLANCO)
  const [editando, setEditando] = useState<{ curso: Curso; datos: DatosDeCurso } | null>(
    null
  )
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
    mutationFn: () => academicoApi.crearCurso(ciclo.id, aDatosDeAPI(nuevo)),
    onSuccess: async () => {
      setNuevo(CURSO_EN_BLANCO)
      await invalidar()
    },
  })

  const editar = useMutation({
    // Los tres datos van juntos: son un solo curso descrito de tres formas, y
    // corregir el nombre sin poder corregir el año que le corresponde dejaría
    // al curso describiéndose mal.
    mutationFn: ({ curso, datos }: { curso: Curso; datos: DatosDeCurso }) =>
      academicoApi.editarCurso(curso.id, aDatosDeAPI(datos)),
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
  const divisiones = divisionesEnUso(cursos)
  const modalidades = modalidadesEnUso(cursos)
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
              valor={nuevo}
              divisionesSugeridas={divisiones}
              modalidadesSugeridas={modalidades}
              onCambio={setNuevo}
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
                <SelectorDeCurso
                  idPrefijo={`editar-curso-${curso.id}`}
                  valor={editando.datos}
                  divisionesSugeridas={divisiones}
                  modalidadesSugeridas={modalidades}
                  onCambio={(datos) => setEditando({ ...editando, datos })}
                />
                <Button type="submit" size="sm" disabled={editar.isPending}>
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
                {/* La modalidad al lado del nombre: dos carreras pueden tener
                    cada una su "1°A" y en la lista serían dos renglones
                    idénticos. */}
                <span className="font-medium">{etiquetaDeCurso(curso)}</span>
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
                        onClick={() =>
                          setEditando({ curso, datos: aDatosDeCurso(curso) })
                        }
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
                  Eliminar «{etiquetaDeCurso(curso)}» borra también todas sus materias.
                  Solo se puede si ninguna tiene reservas.
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

/** Un curso en blanco: primer año, sin división y sin modalidad. */
const CURSO_EN_BLANCO: DatosDeCurso = { anio: 1, division: "", modalidad: "" }

function aDatosDeCurso(curso: Curso): DatosDeCurso {
  return {
    anio: curso.anio,
    division: curso.division ?? "",
    modalidad: curso.modalidad ?? "",
  }
}

/**
 * Los dos opcionales viajan ausentes y no como cadena vacía: en la base son
 * NULL, que es lo que significa "este curso no tiene ese dato".
 */
function aDatosDeAPI(datos: DatosDeCurso) {
  return {
    anio: datos.anio,
    division: datos.division.trim() || undefined,
    modalidad: datos.modalidad.trim() || undefined,
  }
}
