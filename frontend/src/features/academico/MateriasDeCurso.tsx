import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { CopiarMateriasACursos } from "@/features/academico/CopiarMateriasACursos"
import { DocentesDeMateria } from "@/features/academico/DocentesDeMateria"
import * as academicoApi from "@/features/academico/api"
import type { AsignacionDocente, Curso, Materia } from "@/features/academico/types"
import { useAsignacionesDelCiclo } from "@/features/academico/useAsignaciones"
import { getErrorMessage } from "@/lib/api-client"
import { claveDeNombre } from "@/lib/texto"

/** "Ana Gómez (titular), Juan Pérez (suplente)", o "" si no hay ninguno. */
function docentesDe(asignaciones: AsignacionDocente[], materiaId: string): string {
  return asignaciones
    .filter((a) => a.materiaId === materiaId)
    .sort((a, b) => (a.rol === b.rol ? 0 : a.rol === "TITULAR" ? -1 : 1))
    .map((a) => `${a.docenteNombre} (${a.rol === "TITULAR" ? "titular" : "suplente"})`)
    .join(", ")
}

export function MateriasDeCurso({
  curso,
  soloLectura,
  cursosDelCiclo = [],
}: {
  curso: Curso
  soloLectura: boolean
  /** Los demás cursos del ciclo, para poder copiarles estas materias. */
  cursosDelCiclo?: Curso[]
}) {
  const queryClient = useQueryClient()
  const [nombreNueva, setNombreNueva] = useState("")
  const [editando, setEditando] = useState<{ materia: Materia; nombre: string } | null>(
    null
  )
  const [eliminando, setEliminando] = useState<Materia | null>(null)
  // Las marcas que el último renombre dejó huérfanas. `null` es "todavía no se
  // renombró nada"; 0 no se guarda porque no hay nada que contar.
  const [marcasHuerfanas, setMarcasHuerfanas] = useState<{
    nombre: string
    cantidad: number
  } | null>(null)
  const [docentesAbiertos, setDocentesAbiertos] = useState<string | null>(null)
  const [copiando, setCopiando] = useState(false)

  const materiasKey = ["materias", curso.id]
  const { data, isLoading } = useQuery({
    queryKey: materiasKey,
    queryFn: () => academicoApi.listarMaterias(curso.id),
  })

  // Quién dicta cada materia, para poder decirlo en la fila. Antes había que
  // desplegar materia por materia para enterarse, así que la pregunta más
  // frecuente —"¿esta materia tiene docente?"— costaba un clic por materia.
  const asignaciones = useAsignacionesDelCiclo(curso.cicloLectivoId)

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
    onSuccess: async (resultado, { nombre }) => {
      setEditando(null)
      setMarcasHuerfanas(
        resultado.marcasDeEquipoAfectadas > 0
          ? { nombre: nombre.trim(), cantidad: resultado.marcasDeEquipoAfectadas }
          : null
      )
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

  // La misma materia no entra dos veces en el mismo curso, y la comparación
  // ignora tildes y mayúsculas — igual que el índice único de la base, que es
  // quien lo garantiza de verdad. Esto sólo lo dice ANTES: el 409 llega igual
  // si dos pestañas cargan a la vez, pero enterarse al apretar el botón
  // convierte un error evitable en algo que parece una falla del sistema.
  const yaEstaCargada = materias.some(
    (m) => claveDeNombre(m.nombre) === claveDeNombre(nombreNueva)
  )

  return (
    <div className="grid gap-3">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {/* El renombre se hizo: esto no es un error, es la consecuencia que no se
          ve desde acá. Las marcas siguen existiendo apuntando al nombre viejo,
          así que esas máquinas dejaron de aparecer primero para esta materia y
          nadie lo pidió. Se resuelven en Inventario, que es donde viven. */}
      {marcasHuerfanas && (
        <Alert>
          <AlertDescription>
            Se renombró la materia.{" "}
            {marcasHuerfanas.cantidad === 1
              ? "Una marca de equipo quedó"
              : `${marcasHuerfanas.cantidad} marcas de equipo quedaron`}{" "}
            apuntando al nombre anterior, así que esos equipos ya no aparecen primero al
            reservar «{marcasHuerfanas.nombre}». Se revisan en Gestión del inventario, en
            «Marcas que quedaron sin materia».
          </AlertDescription>
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
              aria-invalid={yaEstaCargada}
              aria-describedby={
                yaEstaCargada ? `materia-repetida-${curso.id}` : undefined
              }
            />
            {yaEstaCargada && (
              <p id={`materia-repetida-${curso.id}`} className="text-destructive text-xs">
                Este curso ya tiene esa materia.
              </p>
            )}
          </div>
          <Button
            type="submit"
            size="sm"
            disabled={nombreNueva.trim() === "" || yaEstaCargada || crear.isPending}
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

      {/* RF-02.12 — las mismas materias en varias divisiones del año se cargan
          una vez y se copian, no diez veces a mano. Sólo con algo que copiar y
          con algún curso al que copiarlo. */}
      {!soloLectura &&
        materias.length > 0 &&
        cursosDelCiclo.length > 1 &&
        (copiando ? (
          <CopiarMateriasACursos
            origen={curso}
            materias={materias}
            cursosDelCiclo={cursosDelCiclo}
            onCerrar={() => setCopiando(false)}
          />
        ) : (
          <div>
            <Button variant="outline" size="sm" onClick={() => setCopiando(true)}>
              Copiar estas materias a otros cursos
            </Button>
          </div>
        ))}

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
                <div className="min-w-0">
                  <span className="font-medium">{materia.nombre}</span>
                  {/* Los titulares primero: es quien responde por la materia.
                      «Sin docentes» se dice con todas las letras, porque una
                      materia sin nadie asignado es algo para resolver y no un
                      dato que falta. */}
                  <p className="text-muted-foreground text-sm">
                    {docentesDe(asignaciones, materia.id) || "Sin docentes asignados"}
                  </p>
                </div>
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
