import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import * as academicoApi from "@/features/academico/api"
import type { CicloLectivo, Espacio } from "@/features/academico/types"
import { getErrorMessage } from "@/lib/api-client"
import { claveDeNombre } from "@/lib/texto"

/**
 * RF-02.13 — los lugares de la institución que no son cursos: Dirección,
 * Biblioteca, Preceptoría, un laboratorio.
 *
 * Van en su propia sección y no mezclados con los cursos porque son otra cosa:
 * un curso se nombra por su año y su división, y un espacio es sólo un nombre.
 * Tienen materias igual —«Apoyo escolar» en la Biblioteca— y se reserva para
 * ellas exactamente como para Matemática.
 */
export function EspaciosDeCiclo({ ciclo }: { ciclo: CicloLectivo }) {
  const queryClient = useQueryClient()
  const [nombre, setNombre] = useState("")
  const [editando, setEditando] = useState<{ espacio: Espacio; nombre: string } | null>(
    null
  )
  const [eliminando, setEliminando] = useState<Espacio | null>(null)

  const soloLectura = ciclo.archivado
  const espaciosKey = ["espacios", ciclo.id]

  const { data, isLoading } = useQuery({
    queryKey: espaciosKey,
    queryFn: () => academicoApi.listarEspacios(ciclo.id),
  })

  const invalidar = () => queryClient.invalidateQueries({ queryKey: espaciosKey })

  const crear = useMutation({
    mutationFn: () => academicoApi.crearEspacio(ciclo.id, nombre.trim()),
    onSuccess: async () => {
      setNombre("")
      await invalidar()
    },
  })

  const editar = useMutation({
    mutationFn: ({ espacio, nombre }: { espacio: Espacio; nombre: string }) =>
      academicoApi.editarEspacio(espacio.id, nombre.trim()),
    onSuccess: async () => {
      setEditando(null)
      await invalidar()
    },
  })

  const eliminar = useMutation({
    mutationFn: (espacio: Espacio) => academicoApi.eliminarEspacio(espacio.id),
    onSuccess: async () => {
      setEliminando(null)
      await invalidar()
    },
  })

  if (isLoading) {
    return <p className="text-muted-foreground text-sm">Cargando espacios…</p>
  }

  const espacios = data?.data ?? []
  const error = crear.error ?? editar.error ?? eliminar.error

  // La misma regla que el índice único de la base —sin tildes, sin mayúsculas,
  // sin espacios de más— dicha ANTES de apretar el botón. El 409 llega igual si
  // dos pestañas cargan a la vez, pero enterarse al escribir convierte un error
  // evitable en algo que parece una falla del sistema. Mismo criterio que las
  // materias de un curso.
  const yaExiste = espacios.some(
    (e) => claveDeNombre(e.nombre) === claveDeNombre(nombre)
  )

  return (
    <div className="grid gap-3">
      <div>
        <h3 className="text-sm font-medium">Otros lugares de la escuela</h3>
        <p className="text-muted-foreground text-xs">
          Dirección, Biblioteca, Preceptoría, un laboratorio. No llevan año ni
          división, y <b>no tienen materias</b>: se reserva directamente para el
          lugar. Sirven para que quien trabaja ahí pueda pedir equipos, y para que al
          registrarse pueda decir dónde está.
        </p>
      </div>

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
            <Label htmlFor={`espacio-${ciclo.id}`}>Nuevo lugar</Label>
            <Input
              id={`espacio-${ciclo.id}`}
              value={nombre}
              maxLength={80}
              placeholder="Ej.: Biblioteca"
              aria-invalid={yaExiste}
              aria-describedby={yaExiste ? `espacio-repetido-${ciclo.id}` : undefined}
              onChange={(e) => setNombre(e.target.value)}
            />
            {yaExiste && (
              <p id={`espacio-repetido-${ciclo.id}`} className="text-destructive text-xs">
                Ya hay un lugar con ese nombre.
              </p>
            )}
          </div>
          {/* «Agregar lugar» y no «Agregar» a secas: en esta pantalla conviven
              el alta de cursos y la de lugares, y dos botones con el mismo
              rótulo obligan a mirar cuál está más cerca para saber cuál es. */}
          <Button
            type="submit"
            size="sm"
            disabled={nombre.trim() === "" || yaExiste || crear.isPending}
          >
            Agregar lugar
          </Button>
        </form>
      )}

      {espacios.length === 0 && (
        <p className="text-muted-foreground text-sm">
          Todavía no hay ninguno. Es opcional: si en tu escuela sólo se reserva para
          cursos, no hace falta cargar nada acá.
        </p>
      )}

      {espacios.map((espacio) => {
        const editandoEste = editando?.espacio.id === espacio.id
        const eliminandoEste = eliminando?.id === espacio.id

        return (
          <div key={espacio.id} className="grid gap-2 rounded-md border p-3">
            {editandoEste && editando ? (
              <form
                className="grid gap-2 sm:grid-cols-[1fr_auto_auto] sm:items-end"
                onSubmit={(e) => {
                  e.preventDefault()
                  editar.mutate(editando)
                }}
              >
                <div className="grid gap-1.5">
                  <Label htmlFor={`editar-espacio-${espacio.id}`}>Nombre</Label>
                  <Input
                    id={`editar-espacio-${espacio.id}`}
                    value={editando.nombre}
                    maxLength={80}
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
                <span className="min-w-0 font-medium break-words">{espacio.nombre}</span>
                <div className="flex flex-wrap gap-2">
                  {!soloLectura && !eliminandoEste && (
                    <>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setEditando({ espacio, nombre: espacio.nombre })}
                      >
                        Renombrar
                      </Button>
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => setEliminando(espacio)}
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
                {/* Mismo criterio que un curso (RF-02.11): lo que tiene clases
                    dadas no se borra. El backend responde 409 y su mensaje se
                    muestra tal cual. */}
                <p className="text-destructive text-sm">
                  Eliminar «{espacio.nombre}» solo se puede si no tiene ninguna
                  reserva hecha.
                </p>
                <div className="flex gap-2">
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={eliminar.isPending}
                    onClick={() => eliminar.mutate(espacio)}
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
