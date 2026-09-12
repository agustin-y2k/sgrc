import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { CursosDeCiclo } from "@/features/academico/CursosDeCiclo"
import * as academicoApi from "@/features/academico/api"
import type { CicloLectivo, ResultadoArchivado } from "@/features/academico/types"
import { getErrorMessage } from "@/lib/api-client"
import { contar } from "@/lib/plural"

/** Estado del formulario de archivado; `clonarA` vacío = archivar sin clonar. */
type Archivado = { ciclo: CicloLectivo; clonarA: string }

/** RF-02: ciclos lectivos, cursos y materias. */

const CICLOS_KEY = ["ciclos"]

export function AcademicoPage() {
  const queryClient = useQueryClient()
  const [anioNuevo, setAnioNuevo] = useState(String(new Date().getFullYear()))
  const [cicloAbierto, setCicloAbierto] = useState<string | null>(null)
  const [archivando, setArchivando] = useState<Archivado | null>(null)
  // Corregir el año y eliminar el ciclo son las dos formas de deshacer un
  // ciclo creado mal. Van juntas en el mismo panel plegable porque son la misma
  // situación vista de dos maneras: el año está equivocado, o el ciclo entero
  // sobra.
  const [corrigiendo, setCorrigiendo] = useState<{ ciclo: CicloLectivo; anio: string } | null>(
    null
  )
  const [resultadoArchivado, setResultadoArchivado] = useState<ResultadoArchivado | null>(
    null
  )

  const { data, isLoading, error } = useQuery({
    queryKey: CICLOS_KEY,
    queryFn: academicoApi.listarCiclos,
  })

  const crearCiclo = useMutation({
    mutationFn: () => academicoApi.crearCiclo(Number(anioNuevo)),
    onSuccess: async (nuevo) => {
      await queryClient.invalidateQueries({ queryKey: CICLOS_KEY })
      setCicloAbierto(nuevo.id)
    },
  })

  const corregir = useMutation({
    mutationFn: ({ ciclo, anio }: { ciclo: CicloLectivo; anio: string }) =>
      academicoApi.corregirCiclo(ciclo.id, Number(anio)),
    onSuccess: async () => {
      setCorrigiendo(null)
      await queryClient.invalidateQueries({ queryKey: CICLOS_KEY })
    },
  })

  const eliminar = useMutation({
    mutationFn: (ciclo: CicloLectivo) => academicoApi.eliminarCiclo(ciclo.id),
    onSuccess: async () => {
      setCorrigiendo(null)
      // Eliminarlo libera el único lugar de ciclo activo, así que el formulario
      // de arriba vuelve a habilitarse solo.
      await queryClient.invalidateQueries({ queryKey: CICLOS_KEY })
    },
  })

  const archivar = useMutation({
    mutationFn: ({ ciclo, clonarA }: Archivado) =>
      academicoApi.archivarCiclo(ciclo.id, clonarA ? Number(clonarA) : undefined),
    onSuccess: async (res) => {
      setArchivando(null)
      setResultadoArchivado(res)
      // Cambia el ciclo activo y, si se clonó, aparece uno nuevo con sus
      // cursos: se invalida todo el árbol, no solo la lista de ciclos.
      await queryClient.invalidateQueries()
    },
  })

  const ciclos = data?.data ?? []
  const hayActivo = ciclos.some((c) => c.activo)
  const anioValido = /^\d{4}$/.test(anioNuevo.trim())

  return (
    <div className="mx-auto max-w-4xl">
      <EncabezadoDePagina
        titulo="Ciclos, cursos y materias"
        descripcion="Todo empieza acá: un ciclo lectivo contiene cursos, cada curso sus materias, y sobre las materias es que los docentes reservan."
      />

      {(error || crearCiclo.error || archivar.error) && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            {getErrorMessage(error ?? crearCiclo.error ?? archivar.error)}
          </AlertDescription>
        </Alert>
      )}

      {resultadoArchivado && (
        <Alert className="mb-4">
          <AlertDescription>
            Ciclo cerrado. Las estadísticas del año quedaron guardadas.
            {resultadoArchivado.nuevoCicloId
              ? ` Se creó el ciclo siguiente con ${contar(resultadoArchivado.cursosClonados, "curso")} y ${contar(resultadoArchivado.materiasClonadas, "materia")} — falta asignarles docentes.`
              : " No se creó un ciclo nuevo."}
          </AlertDescription>
        </Alert>
      )}

      {!isLoading && ciclos.length === 0 && (
        <Alert className="mb-4">
          <AlertDescription>
            Todavía no hay ningún ciclo lectivo. Creá el del año en curso para poder
            cargar los cursos y sus materias — hasta entonces nadie va a poder reservar.
          </AlertDescription>
        </Alert>
      )}

      <Card className="mb-4">
        <CardHeader>
          <CardTitle>Nuevo ciclo lectivo</CardTitle>
          <CardDescription>
            {/* RF-02.1: el índice único de Postgres garantiza un solo ciclo
                activo; se avisa antes de que el backend responda 409. */}
            {hayActivo
              ? "Ya hay un ciclo activo. El siguiente se abre desde «Cerrar el año y abrir el siguiente», que además copia los cursos y las materias de este."
              : "Solo puede haber un ciclo activo a la vez."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="grid gap-3 sm:flex sm:flex-wrap sm:items-end"
            onSubmit={(e) => {
              e.preventDefault()
              crearCiclo.mutate()
            }}
          >
            <div className="grid gap-1.5">
              <Label htmlFor="anioCiclo">Año</Label>
              <Input
                id="anioCiclo"
                inputMode="numeric"
                className="w-32"
                value={anioNuevo}
                onChange={(e) => setAnioNuevo(e.target.value)}
              />
            </div>
            <Button
              type="submit"
              disabled={!anioValido || hayActivo || crearCiclo.isPending}
            >
              Crear ciclo
            </Button>
          </form>
        </CardContent>
      </Card>

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}

      <div className="grid gap-3">
        {ciclos.map((ciclo) => {
          const abierto = cicloAbierto === ciclo.id
          return (
            <Card key={ciclo.id}>
              <CardHeader>
                <CardTitle className="flex flex-wrap items-center justify-between gap-2">
                  <span className="flex items-center gap-2">
                    {ciclo.anio}
                    {ciclo.activo && <Badge>Activo</Badge>}
                    {ciclo.archivado && <Badge variant="secondary">Archivado</Badge>}
                  </span>
                  <span className="flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      aria-expanded={abierto}
                      onClick={() => setCicloAbierto(abierto ? null : ciclo.id)}
                    >
                      {abierto ? "Ocultar cursos" : "Cursos"}
                    </Button>
                    {!ciclo.archivado &&
                      archivando?.ciclo.id !== ciclo.id &&
                      corrigiendo?.ciclo.id !== ciclo.id && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            setCorrigiendo({ ciclo, anio: String(ciclo.anio) })
                          }
                        >
                          Corregir
                        </Button>
                      )}
                    {!ciclo.archivado &&
                      archivando?.ciclo.id !== ciclo.id &&
                      corrigiendo?.ciclo.id !== ciclo.id && (
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() =>
                          setArchivando({ ciclo, clonarA: String(ciclo.anio + 1) })
                        }
                      >
                        {/* El botón dice las DOS cosas que hace. Decía sólo
                            «Cerrar el año», y como pasar los cursos y materias
                            al año siguiente vive adentro de este mismo paso
                            (RF-02.5), quien buscaba renovar el ciclo no tenía
                            cómo saber que estaba acá: el único botón a la vista
                            anunciaba lo que iba a borrar. */}
                        Cerrar el año y abrir el siguiente
                      </Button>
                    )}
                  </span>
                </CardTitle>
              </CardHeader>

              {corrigiendo?.ciclo.id === ciclo.id && (
                <CardContent>
                  <div className="grid gap-3 rounded-md border p-3">
                    <p className="text-sm font-medium">
                      Corregir el ciclo {ciclo.anio}
                    </p>
                    <p className="text-muted-foreground text-sm">
                      Cambiarle el año sólo se puede mientras no tenga reservas: una
                      reserva lleva su propia fecha y no se mueve con el ciclo.
                    </p>

                    <form
                      className="grid gap-3 sm:grid-cols-[auto_auto_1fr] sm:items-end"
                      onSubmit={(e) => {
                        e.preventDefault()
                        corregir.mutate(corrigiendo)
                      }}
                    >
                      <div className="grid gap-1.5">
                        {/* No dice sólo «Año»: el formulario de crear un ciclo,
                            más arriba en la misma página, ya tiene un campo con
                            esa etiqueta, y dos controles con el mismo nombre son
                            ambiguos para quien navega con lector de pantalla. */}
                        <Label htmlFor={`corregir-${ciclo.id}`}>Nuevo año</Label>
                        <Input
                          id={`corregir-${ciclo.id}`}
                          type="number"
                          className="w-28"
                          value={corrigiendo.anio}
                          onChange={(e) =>
                            setCorrigiendo({ ciclo, anio: e.target.value })
                          }
                        />
                      </div>
                      <Button type="submit" size="sm" disabled={corregir.isPending}>
                        Guardar el año
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="justify-self-start"
                        onClick={() => setCorrigiendo(null)}
                      >
                        Volver
                      </Button>
                    </form>

                    {/* Eliminarlo es la otra mitad de la misma situación: no es
                        que el año esté mal, es que el ciclo sobra. Sólo sirve
                        con el ciclo vacío — uno con cursos cargados se cierra,
                        no se borra. */}
                    <div className="border-t pt-3">
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={eliminar.isPending}
                        onClick={() => eliminar.mutate(ciclo)}
                      >
                        Eliminar el ciclo {ciclo.anio}
                      </Button>
                      <p className="text-muted-foreground mt-2 text-sm">
                        Sólo si todavía no tiene ningún curso cargado. Libera el año y
                        el lugar de ciclo activo, para poder crear el correcto.
                      </p>
                    </div>

                    {(corregir.error || eliminar.error) && (
                      <Alert variant="destructive">
                        <AlertDescription>
                          {getErrorMessage(corregir.error ?? eliminar.error)}
                        </AlertDescription>
                      </Alert>
                    )}
                  </div>
                </CardContent>
              )}

              {archivando?.ciclo.id === ciclo.id && (
                <CardContent>
                  <div className="grid gap-3 rounded-md border p-3">
                    {/* RF-02.4: el archivado calcula el snapshot histórico y
                        recién después borra las reservas del año. No hay
                        vuelta atrás, así que se dice con todas las letras
                        qué se conserva y qué no. */}
                    <p className="text-destructive text-sm font-medium">
                      Cerrar {ciclo.anio} elimina definitivamente todas sus reservas.
                    </p>
                    <ul className="text-muted-foreground grid gap-1 text-sm">
                      <li>
                        · Antes de borrarlas se guardan las estadísticas del año (uso por
                        equipo y por docente), que quedan disponibles para siempre.
                      </li>
                      <li>
                        · Los cursos y materias se conservan archivados, con sus docentes
                        asignados.
                      </li>
                      <li>· Las incidencias del inventario no se ven afectadas.</li>
                      <li>· No se puede deshacer.</li>
                    </ul>

                    <div className="grid gap-1.5">
                      <Label htmlFor={`clonar-${ciclo.id}`}>
                        Año del ciclo siguiente — se crea copiando todos los cursos y
                        materias de {ciclo.anio} (dejar vacío para no crearlo)
                      </Label>
                      <Input
                        id={`clonar-${ciclo.id}`}
                        inputMode="numeric"
                        className="w-32"
                        value={archivando.clonarA}
                        onChange={(e) =>
                          setArchivando({ ...archivando, clonarA: e.target.value })
                        }
                        placeholder="Dejar vacío"
                      />
                      <p className="text-muted-foreground text-xs">
                        {/* RF-02.5: el clonado copia la estructura, no las
                            asignaciones — hay que reasignar docentes. */}
                        Se copian los cursos y sus materias, pero no los docentes
                        asignados: hay que volver a asignarlos en el ciclo nuevo.
                      </p>
                    </div>

                    <div className="flex gap-2">
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={archivar.isPending}
                        onClick={() => archivar.mutate(archivando)}
                      >
                        {archivar.isPending ? "Cerrando…" : `Cerrar ${ciclo.anio}`}
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={archivar.isPending}
                        onClick={() => setArchivando(null)}
                      >
                        Volver
                      </Button>
                    </div>
                  </div>
                </CardContent>
              )}

              {abierto && (
                <CardContent>
                  <CursosDeCiclo ciclo={ciclo} />
                </CardContent>
              )}
            </Card>
          )
        })}
      </div>
    </div>
  )
}
