import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

/**
 * El curso que un docente declara al registrarse (RF-01.3).
 *
 * Es un campo de texto y no una lista, por dos razones distintas que apuntan
 * al mismo lado:
 *
 * - **Esta pantalla se ve sin haber iniciado sesión**, así que los cursos de
 *   la institución no se le pueden mostrar a cualquiera que abra el registro.
 * - **No hay una forma única de nombrar un curso** (RF-02.2): "4°2" en una
 *   escuela técnica, "2°" en un terciario que no divide sus cursos.
 *
 * Y de todos modos lo que se escribe acá **no es una referencia**: es lo que
 * la persona dice que va a dictar, para que el Admin sepa a qué asignarla —o
 * que tiene que crear el curso antes de poder hacerlo.
 */
export function SelectorDeCursoSolicitado({
  idPrefijo,
  value,
  onChange,
  deshabilitado,
}: {
  idPrefijo: string
  value: string
  onChange: (curso: string) => void
  deshabilitado?: boolean
}) {
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={`${idPrefijo}-curso`}>Curso</Label>
      <Input
        id={`${idPrefijo}-curso`}
        value={value}
        disabled={deshabilitado}
        maxLength={40}
        placeholder="Ej.: 4°2"
        onChange={(e) => onChange(e.target.value)}
      />
      <p className="text-muted-foreground text-xs">
        Como lo nombra tu escuela. Si todavía no lo sabés, dejalo vacío.
      </p>
    </div>
  )
}
