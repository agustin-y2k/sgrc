import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import {
  MAX_ANIO_CURSO,
  MAX_LARGO_DIVISION,
  MAX_LARGO_MODALIDAD,
  MIN_ANIO_CURSO,
  divisionesEnUso,
  modalidadesEnUso,
} from "@/features/academico/types"
import { useCursosDelCicloActivo } from "@/features/academico/useCursosDelCicloActivo"

/** Nueve escalones son muchos más de los que una escuela va a usar. */
const PRIORIDADES = [1, 2, 3, 4, 5, 6, 7, 8, 9]

/**
 * Los cinco campos de una marca de preferencia (RF-03.21), como texto: son
 * valores de inputs y los convierte quien guarda.
 */
export type ValoresDeMarca = {
  materiaNombre: string
  modalidad: string
  anio: string
  division: string
  prioridad: string
}

export const MARCA_VACIA: ValoresDeMarca = {
  materiaNombre: "",
  modalidad: "",
  anio: "",
  division: "",
  prioridad: "1",
}

/**
 * Los campos de la marca, sin lógica de guardado: los comparten el panel de un
 * equipo y el alta masiva de un carro. Viven juntos para que no se puedan
 * desincronizar — el día que el alcance gane un eje, lo gana en los dos lados.
 *
 * `materiaBloqueada` es para corregir una marca que ya existe: apuntar a otra
 * materia es otra marca, no una corrección de ésta. El selector se deshabilita
 * en vez de esconderse para que el formulario siga diciendo de qué marca habla.
 */
export function CamposDeMarca({
  valores,
  onChange,
  materias,
  materiaBloqueada = false,
  idBase,
}: {
  valores: ValoresDeMarca
  onChange: (v: ValoresDeMarca) => void
  materias: string[]
  materiaBloqueada?: boolean
  idBase: string
}) {
  const cursos = useCursosDelCicloActivo()
  const divisiones = divisionesEnUso(cursos)
  const modalidades = modalidadesEnUso(cursos)

  const cambiar = (campo: keyof ValoresDeMarca, valor: string) =>
    onChange({ ...valores, [campo]: valor })

  return (
    <>
      <div className="grid gap-1.5">
        <Label htmlFor={`materia-pref-${idBase}`}>Materia</Label>
        <Select
          id={`materia-pref-${idBase}`}
          value={valores.materiaNombre}
          disabled={materiaBloqueada}
          onChange={(e) => cambiar("materiaNombre", e.target.value)}
        >
          <option value="">Elegí una materia…</option>
          {materias.map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
        </Select>
      </div>
      {/* Los tres ejes son independientes y todos opcionales: cada uno vacío
          significa "no acota por esto". Se combinan —"Matemática de 4°2,
          Electromecánica"— y a más ejes puestos, más específica es la marca y
          antes gana. */}
      <div className="grid gap-1.5">
        <Label htmlFor={`modalidad-pref-${idBase}`}>Modalidad</Label>
        <Input
          id={`modalidad-pref-${idBase}`}
          className="w-44"
          value={valores.modalidad}
          maxLength={MAX_LARGO_MODALIDAD}
          list={modalidades.length > 0 ? `modalidades-pref-${idBase}` : undefined}
          placeholder="Todas"
          onChange={(e) => cambiar("modalidad", e.target.value)}
        />
        {modalidades.length > 0 && (
          <datalist id={`modalidades-pref-${idBase}`}>
            {modalidades.map((m) => (
              <option key={m} value={m} />
            ))}
          </datalist>
        )}
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor={`anio-pref-${idBase}`}>Año</Label>
        <Input
          id={`anio-pref-${idBase}`}
          className="w-24"
          type="number"
          inputMode="numeric"
          min={MIN_ANIO_CURSO}
          max={MAX_ANIO_CURSO}
          value={valores.anio}
          placeholder="Todos"
          onChange={(e) => cambiar("anio", e.target.value)}
        />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor={`division-pref-${idBase}`}>División</Label>
        <Input
          id={`division-pref-${idBase}`}
          className="w-28"
          value={valores.division}
          maxLength={MAX_LARGO_DIVISION}
          list={divisiones.length > 0 ? `divisiones-pref-${idBase}` : undefined}
          placeholder="Todas"
          onChange={(e) => cambiar("division", e.target.value)}
        />
        {divisiones.length > 0 && (
          <datalist id={`divisiones-pref-${idBase}`}>
            {divisiones.map((d) => (
              <option key={d} value={d} />
            ))}
          </datalist>
        )}
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor={`prioridad-pref-${idBase}`}>Prioridad</Label>
        <Select
          id={`prioridad-pref-${idBase}`}
          className="w-auto"
          value={valores.prioridad}
          onChange={(e) => cambiar("prioridad", e.target.value)}
        >
          {PRIORIDADES.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </Select>
      </div>
    </>
  )
}
