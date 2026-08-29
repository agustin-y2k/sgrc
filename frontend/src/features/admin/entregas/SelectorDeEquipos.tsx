import { useMemo, useState } from "react"

import { Input } from "@/components/ui/input"
import { MINIMO_PARA_BUSCAR, sinTildes } from "@/lib/texto"
import { contar } from "@/lib/plural"

/**
 * Elegir qué equipos salen del laboratorio. Lo usan las dos pantallas que
 * sacan algo sin reserva detrás: la entrega del día a día y la salida a
 * reparación.
 */

export type EquipoParaEntregar = {
  id: string
  etiqueta: string
  /**
   * Dónde está: el carro al que pertenece, o su tipo si no está en ninguno
   * ("PROYECTOR"), que es lo único que ubica a un equipo suelto.
   */
  donde: string
  /** Un dato más al costado del nombre. Hoy lo usa el estado del equipo. */
  nota?: string
}

export function SelectorDeEquipos({
  equipos,
  seleccionados,
  onSeleccionar,
  titulo,
  vacio,
}: {
  equipos: EquipoParaEntregar[]
  seleccionados: Set<string>
  onSeleccionar: (ids: Set<string>) => void
  titulo: string
  /** Qué decir cuando no hay ninguno para ofrecer. */
  vacio: string
}) {
  const [busqueda, setBusqueda] = useState("")

  const visibles = useMemo(() => {
    const texto = sinTildes(busqueda.trim())
    if (!texto) return equipos
    return equipos.filter((eq) => sinTildes(`${eq.etiqueta} ${eq.donde}`).includes(texto))
  }, [equipos, busqueda])

  /**
   * Agrupados por dónde están, y no en una lista corrida: en el mostrador se
   * busca "la 3 del Carro 2", y "PC 3" existe en cada carro — sin el título
   * encima, dos renglones idénticos no se distinguen.
   */
  const grupos = useMemo(() => {
    // La clave va normalizada y el rótulo es el primero que se vio. El tipo
    // de un equipo suelto es texto libre, así que un "PROYECTOR" y un
    // "Proyector" cargados en momentos distintos son el mismo lugar — y como
    // el encabezado se dibuja en mayúsculas, agrupar por el texto crudo
    // mostraba dos títulos idénticos con un equipo cada uno.
    const porDonde = new Map<string, { rotulo: string; equipos: EquipoParaEntregar[] }>()
    for (const eq of visibles) {
      const clave = sinTildes(eq.donde)
      const grupo = porDonde.get(clave) ?? { rotulo: eq.donde, equipos: [] }
      grupo.equipos.push(eq)
      porDonde.set(clave, grupo)
    }
    return [...porDonde.values()]
  }, [visibles])

  const alternar = (id: string) => {
    const nueva = new Set(seleccionados)
    if (nueva.has(id)) nueva.delete(id)
    else nueva.add(id)
    onSeleccionar(nueva)
  }

  const alternarGrupo = (delGrupo: EquipoParaEntregar[]) => {
    const nueva = new Set(seleccionados)
    const todos = delGrupo.every((eq) => nueva.has(eq.id))
    for (const eq of delGrupo) {
      if (todos) nueva.delete(eq.id)
      else nueva.add(eq.id)
    }
    onSeleccionar(nueva)
  }

  return (
    <div className="grid content-start gap-2">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <p className="text-sm font-medium">{titulo}</p>
        {seleccionados.size > 0 && (
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground text-xs underline"
            onClick={() => onSeleccionar(new Set())}
          >
            Limpiar la selección ({seleccionados.size})
          </button>
        )}
      </div>

      {equipos.length >= MINIMO_PARA_BUSCAR && (
        <Input
          value={busqueda}
          onChange={(e) => setBusqueda(e.target.value)}
          aria-label="Buscar un equipo"
          placeholder="Buscar — ej.: PC 3, proyector, Carro 2"
        />
      )}

      <div className="max-h-[26rem] overflow-y-auto rounded-md border p-3">
        {equipos.length === 0 && <p className="text-muted-foreground text-sm">{vacio}</p>}
        {equipos.length > 0 && visibles.length === 0 && (
          <p className="text-muted-foreground text-sm">
            Ningún equipo coincide con «{busqueda.trim()}».
          </p>
        )}

        <div className="grid gap-4">
          {grupos.map(({ rotulo, equipos: delGrupo }) => {
            const todosMarcados = delGrupo.every((eq) => seleccionados.has(eq.id))
            return (
              <div key={rotulo} className="grid gap-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
                    {rotulo}
                  </p>
                  {delGrupo.length > 1 && (
                    <button
                      type="button"
                      className="text-muted-foreground hover:text-foreground text-xs underline"
                      onClick={() => alternarGrupo(delGrupo)}
                    >
                      {todosMarcados ? "Desmarcar" : "Marcar"}{" "}
                      {contar(delGrupo.length, "equipo")}
                    </button>
                  )}
                </div>
                {/* Tarjetas y no renglones a secas: se toca con el dedo en un
                    mostrador, y con una casilla de 16 píxeles por equipo hay
                    que apuntar. Toda la tarjeta es el área sensible. */}
                <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                  {delGrupo.map((eq) => (
                    <label
                      key={eq.id}
                      className="hover:bg-accent/50 has-[:checked]:border-primary has-[:checked]:bg-primary/5 flex cursor-pointer items-center gap-2.5 rounded-md border p-2.5 text-sm transition-colors"
                    >
                      <input
                        type="checkbox"
                        className="size-4 shrink-0"
                        checked={seleccionados.has(eq.id)}
                        onChange={() => alternar(eq.id)}
                        // El nombre accesible lleva el carro: "PC 3" a secas
                        // se repite en cada uno, y quien navega con lector de
                        // pantalla no tiene el título de arriba a la vista.
                        aria-label={`${eq.etiqueta} (${eq.donde})`}
                      />
                      <span className="min-w-0 flex-1 truncate font-medium">
                        {eq.etiqueta}
                      </span>
                      {eq.nota && (
                        <span className="text-muted-foreground shrink-0 text-xs">
                          {eq.nota}
                        </span>
                      )}
                    </label>
                  ))}
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
