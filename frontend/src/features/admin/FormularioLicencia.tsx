import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import type { Licencia, VencimientoDeclarado } from "@/features/inventory/types"

/**
 * Cómo se declara el vencimiento de una licencia.
 *
 * Son cuatro opciones y no un campo de fecha porque el dato llega en formas
 * distintas según dónde esté parado quien carga, y obligarlo a convertir de
 * una a otra en la cabeza es la manera más segura de que entren fechas
 * equivocadas:
 *
 *   - "todavía no sé"  → la licencia queda a verificar, sin avisar nada
 *   - "la renové el…"  → esa fecha + los días de duración
 *   - "quedan N días"  → hoy + N, que es lo que muestra la propia máquina
 *   - "vence el…"      → la fecha, tal cual
 *
 * La primera es el default a propósito. La alternativa —arrancar en "se
 * renovó hoy"— es la que falla en la dirección peligrosa: si en realidad
 * vencía en tres días, el sistema regala treinta de silencio justo cuando
 * tendría que estar avisando.
 */
export type FormaDeVencimiento = "sin-fecha" | "renovada-el" | "quedan-dias" | "vence-el"

export type CamposVencimiento = {
  forma: FormaDeVencimiento
  renovadaEl: string
  quedanDias: string
  venceEl: string
}

export const VENCIMIENTO_VACIO: CamposVencimiento = {
  forma: "sin-fecha",
  renovadaEl: "",
  quedanDias: "",
  venceEl: "",
}

/** El vencimiento tal como está cargado hoy, para abrir la edición. */
export function vencimientoDesdeLicencia(l: Licencia): CamposVencimiento {
  if (!l.fechaVencimiento) return VENCIMIENTO_VACIO
  return {
    forma: "vence-el",
    renovadaEl: l.ultimaRenovacion ?? "",
    quedanDias: "",
    venceEl: l.fechaVencimiento,
  }
}

/**
 * Traduce los campos al cuerpo del request. Manda UNA sola forma: el backend
 * rechaza dos con 400 porque darían fechas distintas.
 */
export function aVencimientoDeclarado(v: CamposVencimiento): VencimientoDeclarado {
  switch (v.forma) {
    case "renovada-el":
      return v.renovadaEl ? { renovadaEl: v.renovadaEl } : {}
    case "quedan-dias":
      return v.quedanDias === "" ? {} : { quedanDias: Number(v.quedanDias) }
    case "vence-el":
      return v.venceEl ? { venceEl: v.venceEl } : {}
    default:
      return {}
  }
}

export function CamposDeVencimiento({
  idPrefijo,
  valor,
  onChange,
}: {
  idPrefijo: string
  valor: CamposVencimiento
  onChange: (v: CamposVencimiento) => void
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="grid gap-1.5">
        <Label htmlFor={`${idPrefijo}-forma`}>¿Cuándo vence?</Label>
        <Select
          id={`${idPrefijo}-forma`}
          value={valor.forma}
          onChange={(e) =>
            onChange({ ...valor, forma: e.target.value as FormaDeVencimiento })
          }
        >
          <option value="sin-fecha">Todavía no sé (la cargo después)</option>
          <option value="quedan-dias">Le quedan N días</option>
          <option value="renovada-el">Se renovó el…</option>
          <option value="vence-el">Vence el…</option>
        </Select>
      </div>

      {valor.forma === "quedan-dias" && (
        <div className="grid gap-1.5">
          <Label htmlFor={`${idPrefijo}-quedan`}>Días que le quedan</Label>
          <Input
            id={`${idPrefijo}-quedan`}
            type="number"
            min={0}
            inputMode="numeric"
            value={valor.quedanDias}
            onChange={(e) => onChange({ ...valor, quedanDias: e.target.value })}
            placeholder="12"
          />
          {/* Es el número que muestra el propio programa en la máquina. */}
          <p className="text-muted-foreground text-xs">
            El número que dice el programa al abrirlo.
          </p>
        </div>
      )}

      {valor.forma === "renovada-el" && (
        <div className="grid gap-1.5">
          <Label htmlFor={`${idPrefijo}-renovada`}>Fecha en que se renovó</Label>
          <Input
            id={`${idPrefijo}-renovada`}
            type="date"
            value={valor.renovadaEl}
            onChange={(e) => onChange({ ...valor, renovadaEl: e.target.value })}
          />
          <p className="text-muted-foreground text-xs">
            Puede ser anterior a hoy: si se renovó el martes y lo cargás el jueves, el
            contador arranca el martes.
          </p>
        </div>
      )}

      {valor.forma === "vence-el" && (
        <div className="grid gap-1.5">
          <Label htmlFor={`${idPrefijo}-vence`}>Fecha de vencimiento</Label>
          <Input
            id={`${idPrefijo}-vence`}
            type="date"
            value={valor.venceEl}
            onChange={(e) => onChange({ ...valor, venceEl: e.target.value })}
          />
        </div>
      )}

      {valor.forma === "sin-fecha" && (
        <p className="text-muted-foreground self-end text-xs">
          Queda “a verificar”: aparece primera en la lista y no dispara avisos hasta que
          cargues la fecha. Es mejor que ponerle una inventada.
        </p>
      )}
    </div>
  )
}

export type CamposLicencia = {
  nombre: string
  diasDuracion: string
  diasAviso: string
  vencimiento: CamposVencimiento
}

export const LICENCIA_VACIA: CamposLicencia = {
  nombre: "",
  // 30 días es el caso que motivó todo esto (AutoCAD), y el más común en
  // licencias educativas que hay que renovar a mano.
  diasDuracion: "30",
  diasAviso: "1",
  vencimiento: VENCIMIENTO_VACIO,
}

export function desdeLicencia(l: Licencia): CamposLicencia {
  return {
    nombre: l.nombre,
    diasDuracion: String(l.diasDuracion),
    diasAviso: String(l.diasAviso),
    vencimiento: vencimientoDesdeLicencia(l),
  }
}

export function CamposComunesDeLicencia({
  idPrefijo,
  valor,
  onChange,
  sugerencias,
}: {
  idPrefijo: string
  valor: CamposLicencia
  onChange: (v: CamposLicencia) => void
  /** Nombres ya usados, para no cargar "AutoCAD 2027" y "Autocad 2027". */
  sugerencias?: string[]
}) {
  const listaId = `${idPrefijo}-nombres`

  return (
    <>
      <div className="grid gap-1.5">
        <Label htmlFor={`${idPrefijo}-nombre`}>Software</Label>
        <Input
          id={`${idPrefijo}-nombre`}
          value={valor.nombre}
          onChange={(e) => onChange({ ...valor, nombre: e.target.value })}
          placeholder="Ej.: AutoCAD 2027"
          list={sugerencias && sugerencias.length > 0 ? listaId : undefined}
          required
        />
        {/* El datalist ofrece los nombres que ya existen. La unicidad por Equipo
            ignora mayúsculas, pero nada impide cargar "AutoCAD 2027" en una
            máquina y "Autocad 2027" en otra: ahí serían dos programas
            distintos en la lista, con dos contadores que nadie relaciona. */}
        {sugerencias && sugerencias.length > 0 && (
          <datalist id={listaId}>
            {sugerencias.map((s) => (
              <option key={s} value={s} />
            ))}
          </datalist>
        )}
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor={`${idPrefijo}-duracion`}>Días que dura cada renovación</Label>
          <Input
            id={`${idPrefijo}-duracion`}
            type="number"
            min={1}
            inputMode="numeric"
            value={valor.diasDuracion}
            onChange={(e) => onChange({ ...valor, diasDuracion: e.target.value })}
            required
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor={`${idPrefijo}-aviso`}>Avisar con cuántos días de anticipación</Label>
          <Input
            id={`${idPrefijo}-aviso`}
            type="number"
            min={0}
            inputMode="numeric"
            value={valor.diasAviso}
            onChange={(e) => onChange({ ...valor, diasAviso: e.target.value })}
            required
          />
          <p className="text-muted-foreground text-xs">
            Llega un mail a todos los administradores ese día, y otro el día que vence.
          </p>
        </div>
      </div>
    </>
  )
}

/** Selección de los equipos sobre las que se va a dar de alta la licencia. */
export function SelectorDeEquipos({
  equipos,
  seleccionadas,
  onChange,
}: {
  equipos: { id: string; etiqueta: string; carroNombre: string }[]
  seleccionadas: Set<string>
  onChange: (s: Set<string>) => void
}) {
  const alternar = (id: string) => {
    const nueva = new Set(seleccionadas)
    if (nueva.has(id)) nueva.delete(id)
    else nueva.add(id)
    onChange(nueva)
  }

  const todasMarcadas = equipos.length > 0 && equipos.every((equipo) => seleccionadas.has(equipo.id))

  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between">
        <Label>Equipos donde está instalado</Label>
        {/* "Todas" es el caso normal: el software se instala por carro
            entero. Marcar ocho casillas a mano para eso sería absurdo. */}
        <button
          type="button"
          className="text-muted-foreground hover:text-foreground text-xs underline"
          onClick={() => onChange(todasMarcadas ? new Set() : new Set(equipos.map((p) => p.id)))}
        >
          {todasMarcadas ? "Desmarcar todas" : "Marcar todas"}
        </button>
      </div>
      <div className="grid max-h-56 gap-1 overflow-y-auto rounded-md border p-2 sm:grid-cols-2">
        {equipos.length === 0 && (
          <p className="text-muted-foreground text-sm">
            No hay equipos cargados. Cargá el inventario antes de las licencias.
          </p>
        )}
        {equipos.map((equipo) => (
          <label key={equipo.id} className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={seleccionadas.has(equipo.id)}
              onChange={() => alternar(equipo.id)}
            />
            {equipo.etiqueta}
            <span className="text-muted-foreground">({equipo.carroNombre})</span>
          </label>
        ))}
      </div>
    </div>
  )
}
