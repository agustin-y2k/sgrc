import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import type { MateriaParaPreferencia } from "@/features/inventory/types"
import { coincidenEjes, marcaValida, mismoTexto, SIN_ATRIBUTO } from "./alcanceDeMarca"
import type { ValoresDeMarca } from "./alcanceDeMarca"
export { MARCA_VACIA, type ValoresDeMarca } from "./alcanceDeMarca"

function unicos(valores: string[]) {
  return [...new Set(valores)].sort((a, b) => a.localeCompare(b, "es", { numeric: true }))
}

/** El catálogo conserva cada materia junto a su curso; no se cruzan listas independientes. */
export function CamposDeMarca({
  valores,
  onChange,
  catalogo,
  materiaBloqueada = false,
  idBase,
}: {
  valores: ValoresDeMarca
  onChange: (v: ValoresDeMarca) => void
  catalogo: MateriaParaPreferencia[]
  materiaBloqueada?: boolean
  idBase: string
}) {
  const concreto = valores.alcance === "CURSO"
  const filas = catalogo.filter(
    (f) =>
      (!concreto || f.tipo === "CURSO") &&
      (!materiaBloqueada || mismoTexto(f.materiaNombre, valores.materiaNombre))
  )
  const modalidades = unicos(filas.map((f) => f.modalidad || SIN_ATRIBUTO)).filter(
    (m) => concreto || m !== SIN_ATRIBUTO
  )
  const conModalidad = filas.filter((f) =>
    coincidenEjes(f, { ...valores, anio: "", division: "" })
  )
  const anios = unicos(
    conModalidad.flatMap((f) => (f.anio === undefined ? [] : [String(f.anio)]))
  )
  const conAnio = conModalidad.filter(
    (f) => !valores.anio || f.anio === Number(valores.anio)
  )
  const divisiones = unicos(conAnio.map((f) => f.division || SIN_ATRIBUTO)).filter(
    (d) => concreto || d !== SIN_ATRIBUTO
  )
  const coincidentes = filas.filter((f) => coincidenEjes(f, valores))
  const materias = unicos(coincidentes.map((f) => f.materiaNombre))
  const cursoListo =
    !concreto || (!!valores.modalidad && !!valores.anio && !!valores.division)
  function cambiar(campo: "alcance" | "modalidad" | "anio" | "division", valor: string) {
    const siguiente = { ...valores, [campo]: valor }
    if (campo === "alcance") siguiente.modalidad = ""
    if (campo === "alcance" || campo === "modalidad") siguiente.anio = ""
    if (campo !== "division") siguiente.division = ""
    if (!materiaBloqueada) siguiente.materiaNombre = ""
    onChange(siguiente as ValoresDeMarca)
  }
  const elegida = coincidentes.find((f) =>
    mismoTexto(f.materiaNombre, valores.materiaNombre)
  )
  return (
    <>
      <div className="grid min-w-0 gap-1.5">
        <Label htmlFor={`alcance-pref-${idBase}`}>Alcance</Label>
        <Select
          id={`alcance-pref-${idBase}`}
          value={valores.alcance}
          onChange={(e) => cambiar("alcance", e.target.value)}
        >
          <option value="CURSO">Una materia de un curso concreto</option>
          <option value="GENERAL">Regla general para varios cursos o espacios</option>
        </Select>
      </div>
      <div className="grid min-w-0 gap-1.5">
        <Label htmlFor={`modalidad-pref-${idBase}`}>Modalidad</Label>
        <Select
          id={`modalidad-pref-${idBase}`}
          value={valores.modalidad}
          onChange={(e) => cambiar("modalidad", e.target.value)}
        >
          <option value="">
            {concreto ? "Elegí la modalidad…" : "Todas las modalidades"}
          </option>
          {modalidades.map((m) => (
            <option key={m} value={m}>
              {m === SIN_ATRIBUTO ? "Sin modalidad" : m}
            </option>
          ))}
        </Select>
      </div>
      <div className="grid min-w-0 gap-1.5">
        <Label htmlFor={`anio-pref-${idBase}`}>Año</Label>
        <Select
          id={`anio-pref-${idBase}`}
          value={valores.anio}
          disabled={concreto && !valores.modalidad}
          onChange={(e) => cambiar("anio", e.target.value)}
        >
          <option value="">{concreto ? "Elegí el año…" : "Todos los años"}</option>
          {anios.map((a) => (
            <option key={a} value={a}>
              {a}°
            </option>
          ))}
        </Select>
      </div>
      <div className="grid min-w-0 gap-1.5">
        <Label htmlFor={`division-pref-${idBase}`}>División</Label>
        <Select
          id={`division-pref-${idBase}`}
          value={valores.division}
          disabled={concreto && (!valores.modalidad || !valores.anio)}
          onChange={(e) => cambiar("division", e.target.value)}
        >
          <option value="">
            {concreto ? "Elegí la división…" : "Todas las divisiones"}
          </option>
          {divisiones.map((d) => (
            <option key={d} value={d}>
              {d === SIN_ATRIBUTO ? "Sin división" : d}
            </option>
          ))}
        </Select>
      </div>
      <div className="grid min-w-0 gap-1.5">
        <Label htmlFor={`materia-pref-${idBase}`}>Materia</Label>
        <Select
          id={`materia-pref-${idBase}`}
          value={valores.materiaNombre}
          disabled={materiaBloqueada || !cursoListo}
          onChange={(e) => onChange({ ...valores, materiaNombre: e.target.value })}
        >
          <option value="">
            {cursoListo ? "Elegí una materia…" : "Primero elegí el curso…"}
          </option>
          {materiaBloqueada && !materias.includes(valores.materiaNombre) && (
            <option value={valores.materiaNombre}>{valores.materiaNombre}</option>
          )}
          {materias.map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
        </Select>
      </div>
      <div className="grid min-w-0 gap-1.5">
        <Label htmlFor={`prioridad-pref-${idBase}`}>Prioridad</Label>
        <Select
          id={`prioridad-pref-${idBase}`}
          value={valores.prioridad}
          aria-describedby={`ayuda-prioridad-${idBase}`}
          onChange={(e) => onChange({ ...valores, prioridad: e.target.value })}
        >
          {[1, 2, 3, 4, 5, 6, 7, 8, 9].map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </Select>
        <p id={`ayuda-prioridad-${idBase}`} className="text-muted-foreground text-xs">
          1 = mayor prioridad; 9 = menor prioridad.
        </p>
      </div>
      <p className="text-muted-foreground text-xs sm:col-span-full" role="status">
        {marcaValida(valores, catalogo)
          ? concreto
            ? `Sólo ${valores.materiaNombre} de ${elegida!.contenedorNombre}${elegida!.modalidad ? `, ${elegida!.modalidad}` : ", sin modalidad"}.`
            : `Regla general: todas las materias con este nombre que cumplan los filtros. Coincide con ${coincidentes.filter((f) => mismoTexto(f.materiaNombre, valores.materiaNombre)).length} materias del ciclo activo y también podrá aplicar en ciclos siguientes.`
          : valores.materiaNombre
            ? "Esta combinación no existe en el ciclo activo. Revisá el alcance antes de guardar."
            : concreto && filas.length === 0
              ? "No hay materias de cursos disponibles. Para una materia de un espacio, elegí la regla general; los cursos y materias se cargan desde Académico."
              : concreto
                ? "Elegí modalidad, año y división para consultar las materias reales de ese curso."
                : "Cada opción «Todas» amplía el alcance de esta regla. Elegí la materia para ver a qué coincide."}
      </p>
    </>
  )
}
