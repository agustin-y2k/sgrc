import { SelectorDeHora } from "@/components/SelectorDeHora"
import { Button } from "@/components/ui/button"
import { DIAS_SEMANA } from "@/features/disponibilidad/types"
import type { DiaSemana } from "@/features/disponibilidad/types"
import { ATAJOS_DE_DIAS, mismosDias, ordenarDias } from "@/features/admin/jornada"

/**
 * El formulario de un tramo de jornada: qué días y entre qué horas.
 *
 * Vive aparte porque lo usan las dos pantallas que declaran una jornada —la
 * de siempre y el asistente del primer arranque— y son literalmente el mismo
 * formulario. Duplicarlo sería duplicar también las reglas de cuándo se puede
 * guardar, que es donde se esconden las diferencias que nadie nota.
 */

/** Los campos de un tramo, compartidos por el alta y la edición. */
export type FormTramo = { dias: DiaSemana[]; horaInicio: string; horaFin: string }

export const TRAMO_VACIO: FormTramo = { dias: [], horaInicio: "08:00", horaFin: "12:00" }

/**
 * Sin días marcados no se arranca: el formulario no adivina "lunes a viernes"
 * porque es exactamente la suposición que esta pantalla vino a sacar del
 * código.
 */
function SelectorDeDias({
  valor,
  onCambio,
}: {
  valor: DiaSemana[]
  onCambio: (dias: DiaSemana[]) => void
}) {
  const alternar = (dia: DiaSemana) =>
    onCambio(
      valor.includes(dia) ? valor.filter((d) => d !== dia) : ordenarDias([...valor, dia])
    )

  return (
    <div className="grid gap-2">
      <span className="text-sm font-medium">Días</span>

      {/* Botones con aria-pressed y no checkboxes: son siete y se marcan y
          desmarcan seguido, así que el objetivo táctil tiene que ser toda la
          etiqueta y no un cuadradito de 16px. */}
      <div className="flex flex-wrap gap-1.5" role="group" aria-label="Días">
        {DIAS_SEMANA.map((d) => {
          const marcado = valor.includes(d.valor)
          return (
            <Button
              key={d.valor}
              type="button"
              size="sm"
              variant={marcado ? "default" : "outline"}
              aria-pressed={marcado}
              onClick={() => alternar(d.valor)}
            >
              {d.etiqueta}
            </Button>
          )
        })}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <span className="text-muted-foreground text-xs">Atajos:</span>
        {ATAJOS_DE_DIAS.map((atajo) => (
          <Button
            key={atajo.etiqueta}
            type="button"
            size="sm"
            variant="ghost"
            className="h-auto px-2 py-1 text-xs"
            aria-pressed={mismosDias(valor, atajo.dias)}
            onClick={() => onCambio([...atajo.dias])}
          >
            {atajo.etiqueta}
          </Button>
        ))}
        {valor.length > 0 && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="text-muted-foreground h-auto px-2 py-1 text-xs"
            onClick={() => onCambio([])}
          >
            Limpiar
          </Button>
        )}
      </div>
    </div>
  )
}

export function CamposDeTramo({
  valor,
  onCambio,
  idPrefijo,
}: {
  valor: FormTramo
  onCambio: (v: FormTramo) => void
  idPrefijo: string
}) {
  return (
    <div className="grid gap-4">
      <SelectorDeDias
        valor={valor.dias}
        onCambio={(dias) => onCambio({ ...valor, dias })}
      />

      <div className="flex flex-wrap items-end gap-4">
        <SelectorDeHora
          id={`${idPrefijo}-inicio`}
          etiqueta="Abre"
          valor={valor.horaInicio}
          onCambio={(v) => onCambio({ ...valor, horaInicio: v })}
        />
        <SelectorDeHora
          id={`${idPrefijo}-fin`}
          etiqueta="Cierra"
          valor={valor.horaFin}
          onCambio={(v) => onCambio({ ...valor, horaFin: v })}
        />
        {cruzaLaMedianoche(valor) && (
          <p className="text-muted-foreground pb-1.5 text-sm">Cierra al día siguiente.</p>
        )}
      </div>
    </div>
  )
}

function cruzaLaMedianoche(v: FormTramo): boolean {
  return v.horaInicio !== "" && v.horaFin !== "" && v.horaFin < v.horaInicio
}

/** Por qué no se puede guardar ese horario todavía, o "" si sí se puede. */
export function motivoDeHoras(horaInicio: string, horaFin: string): string {
  if (horaInicio === "" || horaFin === "") return "Falta completar la hora."
  if (horaFin === horaInicio) {
    return "La hora de cierre no puede ser igual a la de apertura."
  }
  return ""
}

export const SIN_DIAS = "Elegí al menos un día para este tramo."

/** Lo mismo para un tramo entero, que además necesita días. */
export function motivoParaNoGuardar(v: FormTramo): string {
  const horas = motivoDeHoras(v.horaInicio, v.horaFin)
  if (horas !== "") return horas
  if (v.dias.length === 0) return SIN_DIAS
  return ""
}
