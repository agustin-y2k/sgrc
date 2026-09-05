import { useId } from "react"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import {
  MAX_ANIO_CURSO,
  MAX_LARGO_DIVISION,
  MAX_LARGO_MODALIDAD,
  MIN_ANIO_CURSO,
  componerNombreDeCurso,
} from "@/features/academico/types"

/** Los tres datos de un curso, tal como se cargan (RF-02.2). */
export type DatosDeCurso = {
  anio: number
  division: string
  modalidad: string
}

/** 1° a 15°: cubre primaria, secundaria y una carrera de grado. */
const ANIOS = Array.from(
  { length: MAX_ANIO_CURSO - MIN_ANIO_CURSO + 1 },
  (_, i) => MIN_ANIO_CURSO + i
)

/**
 * El formulario de un curso: el año —que tienen todos los ámbitos— más la
 * división y la modalidad, que no.
 *
 * El nombre no se escribe: sale del año y la división, y el eco de abajo
 * muestra cómo va a quedar antes de guardarlo.
 */
export function SelectorDeCurso({
  idPrefijo,
  valor,
  onCambio,
  deshabilitado,
  divisionesSugeridas = [],
  modalidadesSugeridas = [],
}: {
  idPrefijo: string
  valor: DatosDeCurso
  onCambio: (datos: DatosDeCurso) => void
  deshabilitado?: boolean
  /** Lo que la institución ya usa, para no tener que recordarlo. */
  divisionesSugeridas?: string[]
  modalidadesSugeridas?: string[]
}) {
  // Los ids se generan acá: dos formularios en la misma pantalla —el de
  // "nuevo curso" y el de uno que se está editando— tendrían el mismo
  // <datalist> y el navegador se queda con uno solo.
  const listaDivisiones = useId()
  const listaModalidades = useId()

  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-end gap-2">
        <div className="grid gap-1.5">
          {/* "Año del curso" y no "Año" a secas: en esta misma pantalla
              está el año del ciclo lectivo, y dos campos con el mismo rótulo
              no se distinguen ni leyendo ni con un lector de pantalla. */}
          <Label htmlFor={`${idPrefijo}-anio`}>Año del curso</Label>
          <Select
            id={`${idPrefijo}-anio`}
            className="w-auto"
            value={valor.anio}
            disabled={deshabilitado}
            onChange={(e) => onCambio({ ...valor, anio: Number(e.target.value) })}
          >
            {ANIOS.map((a) => (
              <option key={a} value={a}>
                {a}°
              </option>
            ))}
          </Select>
        </div>

        <div className="grid gap-1.5">
          <Label htmlFor={`${idPrefijo}-division`}>División (opcional)</Label>
          <Input
            id={`${idPrefijo}-division`}
            className="w-28"
            value={valor.division}
            disabled={deshabilitado}
            maxLength={MAX_LARGO_DIVISION}
            list={divisionesSugeridas.length > 0 ? listaDivisiones : undefined}
            placeholder="A, 2, 1ra"
            onChange={(e) => onCambio({ ...valor, division: e.target.value })}
          />
          {divisionesSugeridas.length > 0 && (
            <datalist id={listaDivisiones}>
              {divisionesSugeridas.map((d) => (
                <option key={d} value={d} />
              ))}
            </datalist>
          )}
        </div>

        {/* El eco es lo que reemplaza a escribir el nombre: se ve cómo va a
            quedar, con el `°` que el sistema pone por su cuenta. */}
        <p className="text-muted-foreground pb-1.5 text-sm">
          Queda como <strong>{componerNombreDeCurso(valor.anio, valor.division)}</strong>
        </p>
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor={`${idPrefijo}-modalidad`}>Modalidad o carrera (opcional)</Label>
        <Input
          id={`${idPrefijo}-modalidad`}
          className="w-72"
          value={valor.modalidad}
          disabled={deshabilitado}
          maxLength={MAX_LARGO_MODALIDAD}
          list={modalidadesSugeridas.length > 0 ? listaModalidades : undefined}
          placeholder="Ej.: Electromecánica"
          onChange={(e) => onCambio({ ...valor, modalidad: e.target.value })}
        />
        {modalidadesSugeridas.length > 0 && (
          <datalist id={listaModalidades}>
            {modalidadesSugeridas.map((m) => (
              <option key={m} value={m} />
            ))}
          </datalist>
        )}
        {/* Dicho una vez y no en cada campo: el sistema no exige un concepto
            que la institución no tenga. */}
        <p className="text-muted-foreground text-xs">
          Una universidad puede no tener división; una primaria, no tener modalidad. Lo
          que tu escuela no use, dejalo vacío.
        </p>
      </div>
    </div>
  )
}
