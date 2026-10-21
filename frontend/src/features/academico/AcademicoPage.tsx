import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
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
import { EspaciosDeCiclo } from "@/features/academico/EspaciosDeCiclo"
import * as academicoApi from "@/features/academico/api"
import type { CicloLectivo, ResultadoArchivado } from "@/features/academico/types"
import { getErrorMessage } from "@/lib/api-client"
import { contar } from "@/lib/plural"
import { parsearAnioDeCiclo } from "./anioCiclo"

type Archivado = { ciclo: CicloLectivo; clonarA: string; crearSiguiente: boolean }

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
  const [corrigiendo, setCorrigiendo] = useState<{
    ciclo: CicloLectivo
    anio: string
  } | null>(null)
  const [resultadoArchivado, setResultadoArchivado] = useState<ResultadoArchivado | null>(
    null
  )

  const { data, isLoading, isFetching, isSuccess, error, refetch } = useQuery({
    queryKey: CICLOS_KEY,
    queryFn: academicoApi.listarCiclos,
  })

  const crearCiclo = useMutation({
    mutationFn: () => academicoApi.crearCiclo(Number(anioNuevo)),
    onSuccess: async (nuevo) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: CICLOS_KEY }),
        queryClient.invalidateQueries({ queryKey: ["catalogo-prioridades"] }),
      ])
      setCicloAbierto(nuevo.id)
    },
  })

  const corregir = useMutation({
    mutationFn: ({ ciclo, anio }: { ciclo: CicloLectivo; anio: string }) =>
      academicoApi.corregirCiclo(ciclo.id, Number(anio)),
    onSuccess: async () => {
      setCorrigiendo(null)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: CICLOS_KEY }),
        queryClient.invalidateQueries({ queryKey: ["catalogo-prioridades"] }),
      ])
    },
  })

  const eliminar = useMutation({
    mutationFn: (ciclo: CicloLectivo) => academicoApi.eliminarCiclo(ciclo.id),
    onSuccess: async () => {
      setCorrigiendo(null)
      // Eliminarlo libera el único lugar de ciclo activo, así que el formulario
      // de arriba vuelve a habilitarse solo.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: CICLOS_KEY }),
        queryClient.invalidateQueries({ queryKey: ["catalogo-prioridades"] }),
      ])
    },
  })

  const archivar = useMutation({
    mutationFn: ({ ciclo, clonarA, crearSiguiente }: Archivado) => {
      const destino = parsearAnioDeCiclo(clonarA)
      if (crearSiguiente && destino === undefined) {
        throw new Error("Ingresá un año válido para crear el ciclo siguiente.")
      }
      return academicoApi.archivarCiclo(ciclo.id, crearSiguiente ? destino : undefined)
    },
    onSuccess: async (res, { ciclo }) => {
      setArchivando(null)
      setResultadoArchivado(res)
      setCicloAbierto(res.nuevoCicloId ?? null)
      if (!res.nuevoCicloId) setAnioNuevo(String(ciclo.anio + 1))
      // Cambia el ciclo activo y, si se clonó, aparece uno nuevo con sus
      // cursos: se invalida todo el árbol, no solo la lista de ciclos.
      await queryClient.invalidateQueries()
    },
  })

  const ciclos = data?.data ?? []
  const consultaLista = isSuccess && !error && !isFetching
  const cicloParaCerrar = ciclos.find((c) => c.id === archivando?.ciclo.id)
  const cierreVigente =
    !!cicloParaCerrar?.activo &&
    !cicloParaCerrar.archivado &&
    cicloParaCerrar.anio === archivando?.ciclo.anio
  const hayActivo = ciclos.some((c) => c.activo)
  const anioValido = parsearAnioDeCiclo(anioNuevo) !== undefined
  const errorAnioNuevo = !anioValido
    ? "Ingresá un año entero entre 2000 y 2100."
    : ciclos.some((c) => c.anio === Number(anioNuevo))
      ? `Ya existe un ciclo para ${Number(anioNuevo)}. Elegí otro año.`
      : null
  const destinoCierre = archivando ? parsearAnioDeCiclo(archivando.clonarA) : undefined
  const errorDestino = !archivando?.crearSiguiente
    ? null
    : destinoCierre === undefined
      ? "Ingresá un año entero entre 2000 y 2100 para el ciclo siguiente."
      : ciclos.some((c) => c.anio === destinoCierre)
        ? `Ya existe un ciclo para ${destinoCierre}. Elegí otro año.`
        : null

  return (
    <div className="mx-auto min-w-0 max-w-4xl [&_button]:h-auto [&_button]:min-h-11 sm:[&_button]:min-h-8 [&_button]:min-w-0 [&_button]:max-w-full [&_button]:whitespace-normal">
      <EncabezadoDePagina
        titulo="Ciclos, cursos y materias"
        descripcion="Todo empieza acá: un ciclo lectivo contiene cursos, cada curso sus materias, y sobre las materias es que los docentes reservan."
      />

      {error && (
        <ErrorDeConsulta
          mensaje="No pudimos consultar los ciclos lectivos."
          reintentando={isFetching}
          onReintentar={() => void refetch()}
        />
      )}
      {crearCiclo.error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(crearCiclo.error)}</AlertDescription>
        </Alert>
      )}

      {resultadoArchivado && (
        <Alert className="mb-4">
          <AlertDescription>
            Ciclo cerrado. Las estadísticas del año quedaron guardadas.
            {resultadoArchivado.nuevoCicloId
              ? ` Se creó el ciclo siguiente con ${contar(resultadoArchivado.cursosClonados, "curso")} y ${contar(resultadoArchivado.materiasClonadas, "materia")}. También se copiaron los espacios y sus materias. Falta asignar docentes.`
              : " No se creó un ciclo nuevo."}{" "}
            Los préstamos pendientes siguen abiertos y se reciben desde Entregas.
          </AlertDescription>
        </Alert>
      )}

      {consultaLista && ciclos.length === 0 && (
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
            className="grid min-w-0 gap-3 sm:flex sm:flex-wrap sm:items-end"
            onSubmit={(e) => {
              e.preventDefault()
              if (consultaLista && !errorAnioNuevo && !hayActivo && !crearCiclo.isPending)
                crearCiclo.mutate()
            }}
          >
            <div className="grid min-w-0 gap-1.5">
              <Label htmlFor="anioCiclo">Año</Label>
              <Input
                id="anioCiclo"
                inputMode="numeric"
                className="w-32"
                value={anioNuevo}
                aria-invalid={consultaLista && !hayActivo && !!errorAnioNuevo}
                aria-describedby={
                  consultaLista && !hayActivo && errorAnioNuevo
                    ? "error-anio-nuevo"
                    : undefined
                }
                onChange={(e) => setAnioNuevo(e.target.value)}
              />
              {consultaLista && !hayActivo && errorAnioNuevo && (
                <p
                  id="error-anio-nuevo"
                  className="text-destructive text-sm"
                  aria-live="polite"
                >
                  {errorAnioNuevo}
                </p>
              )}
            </div>
            <Button
              type="submit"
              disabled={
                !consultaLista || !!errorAnioNuevo || hayActivo || crearCiclo.isPending
              }
            >
              Crear ciclo
            </Button>
          </form>
        </CardContent>
      </Card>

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}

      <div className="grid min-w-0 gap-3">
        {ciclos.map((ciclo) => {
          const abierto = cicloAbierto === ciclo.id
          return (
            <Card key={ciclo.id}>
              <CardHeader>
                <CardTitle className="flex min-w-0 flex-wrap items-center justify-between gap-2">
                  <span className="flex min-w-0 items-center gap-2">
                    {ciclo.anio}
                    {ciclo.activo && <Badge>Activo</Badge>}
                    {ciclo.archivado && <Badge variant="secondary">Archivado</Badge>}
                  </span>
                  <span className="flex min-w-0 flex-wrap gap-2">
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
                          disabled={!consultaLista || archivar.isPending}
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
                          disabled={!consultaLista || archivar.isPending}
                          onClick={() => {
                            archivar.reset()
                            setArchivando({
                              ciclo,
                              clonarA: String(ciclo.anio + 1),
                              crearSiguiente: true,
                            })
                          }}
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
                  <div className="grid min-w-0 gap-3 rounded-md border p-3">
                    <p className="text-sm font-medium">Corregir el ciclo {ciclo.anio}</p>
                    <p className="text-muted-foreground text-sm">
                      Cambiarle el año sólo se puede mientras no tenga reservas: una
                      reserva lleva su propia fecha y no se mueve con el ciclo.
                    </p>

                    <form
                      className="grid min-w-0 gap-3 sm:grid-cols-[auto_auto_1fr] sm:items-end"
                      onSubmit={(e) => {
                        e.preventDefault()
                        corregir.mutate(corrigiendo)
                      }}
                    >
                      <div className="grid min-w-0 gap-1.5">
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
                        Sólo si todavía no tiene ningún curso cargado. Libera el año y el
                        lugar de ciclo activo, para poder crear el correcto.
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
                  <div className="grid min-w-0 gap-3 rounded-md border p-3">
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
                        · También se eliminan los bloqueos administrativos de ese año.
                      </li>
                      <li>
                        · Los cursos, espacios y materias se conservan archivados, con sus
                        docentes asignados.
                      </li>
                      <li>· Las incidencias del inventario no se ven afectadas.</li>
                      <li>
                        · Las prioridades de los dispositivos se mantienen y siguen
                        aplicando a las materias que coincidan con su nombre y alcance.
                      </li>
                      <li>
                        · Los préstamos pendientes siguen abiertos. Cerrar el año no
                        registra su devolución.
                      </li>
                      <li>· No se puede deshacer.</li>
                    </ul>

                    <fieldset
                      className="grid min-w-0 gap-2"
                      disabled={archivar.isPending}
                    >
                      <legend className="mb-2 text-sm font-medium">
                        ¿Cómo querés cerrar el año?
                      </legend>
                      <label className="flex min-w-0 items-center gap-2 text-sm">
                        <input
                          type="radio"
                          name={`cierre-${ciclo.id}`}
                          checked={archivando.crearSiguiente}
                          onChange={() =>
                            setArchivando({ ...archivando, crearSiguiente: true })
                          }
                        />
                        Cerrar y crear el siguiente ciclo
                      </label>
                      <label className="flex min-w-0 items-center gap-2 text-sm">
                        <input
                          type="radio"
                          name={`cierre-${ciclo.id}`}
                          checked={!archivando.crearSiguiente}
                          onChange={() =>
                            setArchivando({ ...archivando, crearSiguiente: false })
                          }
                        />
                        Cerrar sin crear otro ciclo
                      </label>
                    </fieldset>

                    {archivando.crearSiguiente ? (
                      <div className="grid min-w-0 gap-1.5">
                        <Label htmlFor={`clonar-${ciclo.id}`}>
                          Año del ciclo siguiente — se crea copiando todos los cursos y
                          materias de {ciclo.anio}
                        </Label>
                        <Input
                          id={`clonar-${ciclo.id}`}
                          inputMode="numeric"
                          className="w-32"
                          disabled={archivar.isPending}
                          value={archivando.clonarA}
                          onChange={(e) =>
                            setArchivando({ ...archivando, clonarA: e.target.value })
                          }
                          aria-invalid={!!errorDestino}
                          aria-describedby={
                            errorDestino ? `error-clonar-${ciclo.id}` : undefined
                          }
                        />
                        {errorDestino && (
                          <p
                            id={`error-clonar-${ciclo.id}`}
                            className="text-destructive text-sm"
                            aria-live="polite"
                          >
                            {errorDestino}
                          </p>
                        )}
                        <p className="text-muted-foreground text-xs">
                          {/* RF-02.5: el clonado copia la estructura, no las
                            asignaciones — hay que reasignar docentes. */}
                          Se copian los cursos, espacios y sus materias, pero no los
                          docentes asignados: hay que volver a asignarlos en el ciclo
                          nuevo.
                        </p>
                      </div>
                    ) : (
                      <p className="text-sm">
                        No se abrirá un ciclo nuevo. Nadie podrá crear nuevas reservas
                        hasta que administración cree uno.
                      </p>
                    )}

                    {archivar.error && (
                      <Alert variant="destructive">
                        <AlertDescription>
                          {getErrorMessage(archivar.error)}
                        </AlertDescription>
                      </Alert>
                    )}
                    {!cierreVigente && (
                      <p role="status" className="text-sm">
                        Este ciclo cambió o ya no está activo. Volvé y revisá la lista
                        actualizada antes de realizar otra operación.
                      </p>
                    )}

                    <div className="flex min-w-0 flex-wrap gap-2">
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={
                          !consultaLista ||
                          !cierreVigente ||
                          !!errorDestino ||
                          archivar.isPending
                        }
                        className="h-auto min-h-9 whitespace-normal"
                        onClick={() => {
                          if (
                            consultaLista &&
                            cierreVigente &&
                            !errorDestino &&
                            !archivar.isPending
                          )
                            archivar.mutate(archivando)
                        }}
                      >
                        {archivar.isPending
                          ? "Cerrando…"
                          : archivando.crearSiguiente
                            ? `Cerrar ${ciclo.anio} y crear ${destinoCierre ?? "ciclo"}`
                            : `Cerrar ${ciclo.anio} sin abrir otro ciclo`}
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
                <CardContent className="grid min-w-0 gap-6">
                  <CursosDeCiclo ciclo={ciclo} />
                  {/* Los lugares que no son cursos van DEBAJO y separados: la
                      enorme mayoría de las escuelas sólo usa cursos, y esto es
                      opcional. Arriba competiría con lo que casi todos vienen a
                      hacer acá. */}
                  <div className="border-t pt-4">
                    <EspaciosDeCiclo ciclo={ciclo} />
                  </div>
                </CardContent>
              )}
            </Card>
          )
        })}
      </div>
    </div>
  )
}
