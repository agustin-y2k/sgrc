import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import * as academicoApi from "@/features/academico/api"
import * as perfilApi from "@/features/perfil/api"
import { getErrorMessage } from "@/lib/api-client"
import { etiquetaDeCurso } from "@/features/academico/types"
import { sinTildes } from "@/lib/texto"

/** Pedir dictar una materia más. */
export function PedirMateria({
  onListo,
  activo = true,
}: {
  onListo?: () => void
  activo?: boolean
}) {
  const qc = useQueryClient()
  const [modo, setModo] = useState<"lista" | "nueva">("lista")
  const [materiaId, setMateriaId] = useState("")
  const [materia, setMateria] = useState("")
  const [curso, setCurso] = useState("")
  const [motivo, setMotivo] = useState("")
  const [error, setError] = useState("")
  const [listo, setListo] = useState(false)
  const [cursoFiltro, setCursoFiltro] = useState("")
  const [busqueda, setBusqueda] = useState("")

  // Las materias de todo el ciclo activo, no solo las propias: justamente se
  // está pidiendo una que no se dicta.
  const ciclos = useQuery({
    queryKey: ["ciclos"],
    queryFn: academicoApi.listarCiclos,
    enabled: activo && modo === "lista",
  })
  const ciclosListos = ciclos.isSuccess && !ciclos.isFetching && !ciclos.error
  const cicloActivo = ciclos.data?.data.find((c) => c.activo)
  const cursos = useQuery({
    queryKey: ["cursos", cicloActivo?.id],
    queryFn: () => academicoApi.listarCursos(cicloActivo!.id),
    enabled: activo && modo === "lista" && ciclosListos && !!cicloActivo,
  })
  const cursosListos =
    ciclosListos &&
    !!cicloActivo &&
    cursos.isSuccess &&
    !cursos.isFetching &&
    !cursos.error
  const listaCursos = cursos.data?.data ?? []
  const materias = useQuery({
    queryKey: ["materias-de-todos-los-cursos", listaCursos.map((c) => c.id)],
    queryFn: () =>
      Promise.all(
        listaCursos.map(async (c) => ({
          curso: c,
          materias: (await academicoApi.listarMaterias(c.id)).data,
        }))
      ),
    enabled: activo && modo === "lista" && cursosListos && listaCursos.length > 0,
  })
  const catalogoListo =
    cursosListos &&
    listaCursos.length > 0 &&
    materias.isSuccess &&
    !materias.isFetching &&
    !materias.error
  const materiaValida =
    catalogoListo &&
    materias.data?.some(({ materias }) =>
      materias.some((m) => m.id === materiaId && !m.archivado)
    )
  const consultaFallida = ciclos.error
    ? ciclos
    : ciclosListos && cicloActivo && cursos.error
      ? cursos
      : cursosListos && listaCursos.length > 0 && materias.error
        ? materias
        : null
  const mensajeError = ciclos.error
    ? "No pudimos consultar el ciclo lectivo."
    : cursos.error
      ? "No pudimos consultar los cursos."
      : "No pudimos consultar las materias."
  const sinMaterias =
    catalogoListo &&
    !materias.data?.some(({ materias }) => materias.some((m) => !m.archivado))
  const filtro = sinTildes(busqueda.trim())
  const cursoAplicado = listaCursos.some((c) => c.id === cursoFiltro) ? cursoFiltro : ""
  const porCursoFiltradas = (catalogoListo ? (materias.data ?? []) : [])
    .filter(({ curso }) => !cursoAplicado || curso.id === cursoAplicado)
    .map(({ curso, materias }) => ({
      curso,
      materias: materias.filter(
        (m) =>
          !m.archivado &&
          sinTildes(`${m.nombre} ${etiquetaDeCurso(curso)}`).includes(filtro)
      ),
    }))
    .filter(({ materias }) => materias.length > 0)

  const pedir = useMutation({
    mutationFn: () =>
      modo === "lista"
        ? perfilApi.pedirMateriaExistente(materiaId, motivo)
        : perfilApi.pedirMateriaNueva(materia, curso, motivo),
    onSuccess: () => {
      setError("")
      setListo(true)
      setMateriaId("")
      setMateria("")
      setCurso("")
      setMotivo("")
      qc.invalidateQueries({ queryKey: ["perfil", "mis-pedidos"] })
      onListo?.()
    },
    onError: (e) => setError(getErrorMessage(e)),
  })

  const faltante =
    modo === "lista"
      ? !materiaValida
        ? "elegir la materia"
        : ""
      : materia.trim() === ""
        ? "escribir cómo se llama la materia"
        : ""
  const faltaMotivo = motivo.trim() === "" ? "explicar por qué la pedís" : ""
  const pendientes = [faltante, faltaMotivo].filter(Boolean)
  const puedePedir = pendientes.length === 0 && !pedir.isPending

  return (
    <div className="bg-superficie grid gap-3 rounded-xl border p-4">
      <div>
        <p className="font-medium">Pedir otra materia</p>
        <p className="text-muted-foreground text-sm">
          Lo resuelve el equipo de administración, no es automático. Si la materia ya la
          da otro docente, también le avisamos a esa persona.
        </p>
      </div>

      {listo && (
        <Alert>
          <AlertDescription>
            Listo, tu pedido quedó registrado. Revisá la resolución en Avisos y tus
            materias en Mi cuenta.
          </AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant={modo === "lista" ? "default" : "outline"}
          size="sm"
          className="h-11 px-4 sm:h-9"
          onClick={() => setModo("lista")}
        >
          Está en la lista
        </Button>
        <Button
          type="button"
          variant={modo === "nueva" ? "default" : "outline"}
          size="sm"
          className="h-11 px-4 sm:h-9"
          onClick={() => setModo("nueva")}
        >
          Todavía no está cargada
        </Button>
      </div>

      {modo === "lista" ? (
        <div className="grid gap-2">
          {consultaFallida ? (
            <ErrorDeConsulta
              mensaje={mensajeError}
              reintentando={consultaFallida.isFetching}
              onReintentar={() => {
                void consultaFallida.refetch()
              }}
            />
          ) : !ciclosListos ||
            (cicloActivo && !cursosListos) ||
            (cursosListos && listaCursos.length > 0 && !catalogoListo) ? (
            <p role="status" className="text-muted-foreground text-sm">
              Cargando materias…
            </p>
          ) : !cicloActivo ? (
            <p className="text-muted-foreground text-sm">
              No hay un ciclo lectivo activo. Consultá con administración.
            </p>
          ) : listaCursos.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              El ciclo activo todavía no tiene cursos cargados.
            </p>
          ) : sinMaterias ? (
            <p className="text-muted-foreground text-sm">
              Todavía no hay materias cargadas en los cursos del ciclo activo.
            </p>
          ) : null}
          <div className="grid gap-1.5">
            <Label htmlFor="curso-pedido-materia">Filtrar por curso</Label>
            <Select
              id="curso-pedido-materia"
              value={cursoAplicado}
              disabled={!catalogoListo}
              onChange={(e) => {
                setCursoFiltro(e.target.value)
                setMateriaId("")
              }}
            >
              <option value="">Todos los cursos</option>
              {listaCursos.map((c) => (
                <option key={c.id} value={c.id}>
                  {etiquetaDeCurso(c)}
                </option>
              ))}
            </Select>
            <Label htmlFor="buscar-materia-pedido">Buscar materia</Label>
            <Input
              id="buscar-materia-pedido"
              value={busqueda}
              disabled={!catalogoListo}
              onChange={(e) => {
                setBusqueda(e.target.value)
                setMateriaId("")
              }}
              placeholder="Nombre de materia, curso o modalidad"
            />
            {catalogoListo && !sinMaterias && porCursoFiltradas.length === 0 && (
              <p className="text-muted-foreground text-sm">
                Ninguna materia coincide con los filtros. Cambiá el curso o la búsqueda.
              </p>
            )}
            <Label htmlFor="materia-existente">¿Cuál materia?</Label>
            <Select
              id="materia-existente"
              value={materiaValida ? materiaId : ""}
              disabled={!catalogoListo || sinMaterias}
              onChange={(e) => setMateriaId(e.target.value)}
            >
              <option value="">Elegí una materia…</option>
              {porCursoFiltradas.map(({ curso, materias }) => (
                <optgroup key={curso.id} label={etiquetaDeCurso(curso)}>
                  {materias
                    .filter((m) => !m.archivado)
                    .map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.nombre}
                      </option>
                    ))}
                </optgroup>
              ))}
            </Select>
          </div>
        </div>
      ) : (
        <div className="grid gap-2 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="materia-nueva">¿Cómo se llama la materia?</Label>
            <Input
              id="materia-nueva"
              value={materia}
              onChange={(e) => setMateria(e.target.value)}
              placeholder="Ej.: Robótica"
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="curso-nuevo">¿De qué curso?</Label>
            <Input
              id="curso-nuevo"
              value={curso}
              onChange={(e) => setCurso(e.target.value)}
              placeholder="Ej.: 5°B"
            />
          </div>
        </div>
      )}

      <div className="grid gap-1.5">
        <Label htmlFor="motivo-pedido">¿Por qué la pedís?</Label>
        <Input
          id="motivo-pedido"
          value={motivo}
          onChange={(e) => setMotivo(e.target.value)}
          placeholder="Ej.: Me asignaron el segundo turno desde mayo"
        />
        <p className="text-muted-foreground text-sm">
          Es lo que va a leer quien lo resuelva antes de hablar con vos.
        </p>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {pendientes.length > 0 && (
        <p className="text-muted-foreground text-sm">
          Para mandarlo falta {pendientes.join(" y ")}.
        </p>
      )}

      <div>
        <Button
          type="button"
          size="sm"
          className="h-11 px-4 sm:h-9"
          disabled={!puedePedir}
          onClick={() => {
            if (puedePedir) pedir.mutate()
          }}
        >
          {pedir.isPending ? "Mandando…" : "Mandar el pedido"}
        </Button>
      </div>
    </div>
  )
}
