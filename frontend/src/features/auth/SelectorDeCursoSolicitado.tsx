import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import {
  ANIOS_DE_CURSO,
  DIVISIONES_DE_CURSO,
  componerNombreDeCurso,
  separarNombreDeCurso,
} from "@/features/academico/types"

/**
 * El curso que un docente declara al registrarse, con los mismos dos
 * desplegables que usa el Admin para crear un curso (SelectorDeCurso).
 *
 * Antes era un campo de texto libre con el placeholder "Ej.: 5°A", y eso
 * dejaba que cada docente escribiera el mismo curso de una forma distinta:
 * "5A", "5º A" (con el ordinal masculino, que NO es el `°` que valida el
 * sistema), "quinto A". El Admin recibía un texto que después tenía que
 * traducir a mano al curso real. Con los desplegables, lo que llega es
 * exactamente el nombre canónico que ya existe del otro lado.
 *
 * La diferencia con el del Admin es que acá el curso es OPCIONAL, y esa
 * diferencia es la razón de que sea un componente aparte en vez de una
 * prop del otro. Un desplegable siempre tiene algo elegido, así que sin una
 * opción vacía todos los registros llegarían diciendo "1°A" —incluido el de
 * quien no sabía qué poner— y el Admin no podría distinguir un dato
 * declarado de un valor por defecto. Eso sería peor que el texto libre: un
 * dato equivocado se ve igual que uno correcto.
 */
export function SelectorDeCursoSolicitado({
  idPrefijo,
  value,
  onChange,
  deshabilitado,
}: {
  idPrefijo: string
  /** El nombre canónico ya compuesto ("5°A"), o "" si no eligió nada. */
  value: string
  onChange: (nombreDeCurso: string) => void
  deshabilitado?: boolean
}) {
  // El valor que viaja en el formulario es el nombre compuesto, no el par
  // año/división: es lo que espera el backend y lo que ya guardan las filas
  // existentes. Se separa solo para pintar los dos selects.
  const partes = separarNombreDeCurso(value)
  const anio = partes?.anio ?? ""
  const division = partes?.division ?? ""

  const claseSelect = "w-auto"

  return (
    <div className="grid gap-1.5">
      <div className="flex flex-wrap items-end gap-2">
        <div className="grid gap-1.5">
          <Label htmlFor={`${idPrefijo}-anio`}>Año</Label>
          <Select
            id={`${idPrefijo}-anio`}
            className={claseSelect}
            value={anio}
            disabled={deshabilitado}
            onChange={(e) => {
              const nuevoAnio = e.target.value
              // Volver a "Sin especificar" limpia el campo entero: dejar la
              // división colgada guardaría una mitad que no significa nada.
              if (!nuevoAnio) return onChange("")
              // Elegir el año alcanza para tener un curso válido. La "A"
              // existe en todos los años, y el desplegable de al lado queda
              // a la vista para corregirla — es el mismo default que usa el
              // Admin al crear un curso.
              onChange(componerNombreDeCurso(nuevoAnio, division || "A"))
            }}
          >
            <option value="">Sin especificar</option>
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
            // Sin año elegido no hay nada que dividir, y una división suelta
            // no se puede componer en un nombre.
            disabled={deshabilitado || !anio}
            onChange={(e) => onChange(componerNombreDeCurso(anio, e.target.value))}
          >
            {DIVISIONES_DE_CURSO.map((d) => (
              <option key={d} value={d}>
                {d}
              </option>
            ))}
          </Select>
        </div>

        {/* Mismo eco que en el panel del Admin: confirma cómo va a quedar
            guardado, con el `°` que el sistema pone por su cuenta. */}
        {value && (
          <p className="text-muted-foreground pb-1.5 text-sm">
            Queda como <strong>{value}</strong>
          </p>
        )}
      </div>
    </div>
  )
}
