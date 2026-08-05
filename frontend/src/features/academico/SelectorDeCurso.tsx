import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import {
  ANIOS_DE_CURSO,
  DIVISIONES_DE_CURSO,
  componerNombreDeCurso,
} from "@/features/academico/types"

export function SelectorDeCurso({
  idPrefijo,
  anio,
  division,
  onCambio,
  deshabilitado,
}: {
  idPrefijo: string
  anio: string
  division: string
  onCambio: (anio: string, division: string) => void
  deshabilitado?: boolean
}) {
  // Dos campos de un carácter —"1°" y "A"— en una fila junto al texto que
  // muestra cómo queda el nombre: a ancho completo ocuparían la fila entera
  // cada uno y el conjunto dejaría de leerse como un solo control.
  const claseSelect = "w-auto"
  return (
    <div className="flex flex-wrap items-end gap-2">
      <div className="grid gap-1.5">
        <Label htmlFor={`${idPrefijo}-anio`}>Año del curso</Label>
        <Select
          id={`${idPrefijo}-anio`}
          className={claseSelect}
          value={anio}
          disabled={deshabilitado}
          onChange={(e) => onCambio(e.target.value, division)}
        >
          {ANIOS_DE_CURSO.map((a) => (
            <option key={a} value={a}>
              {a}°
            </option>
          ))}
        </Select>
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor={`${idPrefijo}-division`}>División</Label>
        <Select
          id={`${idPrefijo}-division`}
          className={claseSelect}
          value={division}
          disabled={deshabilitado}
          onChange={(e) => onCambio(anio, e.target.value)}
        >
          {DIVISIONES_DE_CURSO.map((d) => (
            <option key={d} value={d}>
              {d}
            </option>
          ))}
        </Select>
      </div>
      <p className="text-muted-foreground pb-1.5 text-sm">
        Queda como <strong>{componerNombreDeCurso(anio, division)}</strong>
      </p>
    </div>
  )
}

/** Cursos de un ciclo (RF-02.2), con el formato de nombre validado. */
