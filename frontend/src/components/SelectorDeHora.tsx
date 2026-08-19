import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"

/** Hora y minutos en dos campos separados, en formato de 24 horas. */

const HORAS = Array.from({ length: 24 }, (_, h) => String(h).padStart(2, "0"))

/**
 * Minutos de 5 en 5. El backend acepta cualquier minuto —los horarios son
 * libres a propósito, la escuela no tiene módulos fijos— pero una lista de 60
 * entradas es incómoda de recorrer y no describe ningún horario real de
 * clase.
 */
const PASO_MINUTOS = 5

function opcionesDeMinutos(minutoActual: string): string[] {
  const base: string[] = []
  for (let m = 0; m < 60; m += PASO_MINUTOS) {
    base.push(String(m).padStart(2, "0"))
  }
  if (minutoActual !== "" && !base.includes(minutoActual)) {
    base.push(minutoActual)
    base.sort()
  }
  return base
}

/** "08:30" → ["08", "30"]. Un valor vacío o raro da ["", ""]. */
function separar(valor: string): [string, string] {
  const m = /^(\d{2}):(\d{2})$/.exec(valor)
  return m ? [m[1], m[2]] : ["", ""]
}

export function SelectorDeHora({
  id,
  etiqueta,
  valor,
  onCambio,
}: {
  id: string
  etiqueta: string
  /** "HH:MM", o "" si todavía no se eligió. */
  valor: string
  onCambio: (valor: string) => void
}) {
  const [hora, minuto] = separar(valor)

  // Elegir una sola de las dos partes ya arma una hora válida: la otra cae en
  // "00".
  const cambiar = (nuevaHora: string, nuevoMinuto: string) => {
    if (nuevaHora === "" && nuevoMinuto === "") {
      onCambio("")
      return
    }
    onCambio(`${nuevaHora || "00"}:${nuevoMinuto || "00"}`)
  }

  // `w-auto` contra el `w-full` que trae Select: son dos campos de dos
  // dígitos, uno al lado del otro.
  const claseSelect = "w-auto tabular-nums"

  return (
    <div className="grid gap-1.5">
      <Label htmlFor={`${id}-hora`}>{etiqueta}</Label>
      <div className="flex items-center gap-1.5">
        <Select
          id={`${id}-hora`}
          className={claseSelect}
          aria-label={`${etiqueta}: hora`}
          value={hora}
          onChange={(e) => cambiar(e.target.value, minuto)}
        >
          <option value="">--</option>
          {HORAS.map((h) => (
            <option key={h} value={h}>
              {h}
            </option>
          ))}
        </Select>
        <span aria-hidden="true" className="text-muted-foreground">
          :
        </span>
        <Select
          id={`${id}-minutos`}
          className={claseSelect}
          aria-label={`${etiqueta}: minutos`}
          value={minuto}
          onChange={(e) => cambiar(hora, e.target.value)}
        >
          <option value="">--</option>
          {opcionesDeMinutos(minuto).map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </Select>
        <span className="text-muted-foreground ml-1 text-xs">24 h</span>
      </div>
    </div>
  )
}
