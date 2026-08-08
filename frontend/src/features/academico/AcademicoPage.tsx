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

/** Estado del formulario de archivado; `clonarA` vacío = archivar sin clonar. */
type Archivado = { ciclo: CicloLectivo; clonarA: string }

/**
 * RF-02: ciclos lectivos, cursos y materias.
 *
 * Es la puerta de entrada de todo el sistema: sin un ciclo no hay cursos,
 * sin cursos no hay materias, y sin materias nadie puede reservar. Por eso
 * el estado vacío explica el camino en vez de limitarse a decir "no hay
 * datos".
 */

const CICLOS_KEY = ["ciclos"]

export function AcademicoPage() {
  const queryClient = useQueryClient()
  const [anioNuevo, setAnioNuevo] = useState(String(new Date().getFullYear()))
  const [cicloAbierto, setCicloAbierto] = useState<string | null>(null)
  const [archivando, setArchivando] = useState<Archivado | null>(null)
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
              ? ` Se creó el ciclo siguiente con ${resultadoArchivado.cursosClonados} curso(s) y ${resultadoArchivado.materiasClonadas} materia(s) — falta asignarles docentes.`
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
              ? "Ya hay un ciclo activo. Para abrir el siguiente hay que archivar el actual primero."
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
                    {!ciclo.archivado && archivando?.ciclo.id !== ciclo.id && (
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() =>
                          setArchivando({ ciclo, clonarA: String(ciclo.anio + 1) })
                        }
                      >
                        Cerrar el año
                      </Button>
                    )}
                  </span>
                </CardTitle>
              </CardHeader>

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
                        Equipo y por docente), que quedan disponibles para siempre.
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
                        Crear el ciclo siguiente copiando cursos y materias (opcional)
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
