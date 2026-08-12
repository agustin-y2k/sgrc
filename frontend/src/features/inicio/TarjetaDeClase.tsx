import { useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { CambiarEquipoDeReserva } from "@/features/reservas/CambiarEquipoDeReserva"
import { CancelarReserva } from "@/features/reservas/CancelarReserva"
import type { GrupoDeReservas } from "@/features/reservas/types"
import { etiquetaDeDia } from "@/lib/fechas"

/** Cuántos equipos se nombran antes de resumir el resto en "y N más". */
const EQUIPOS_A_LA_VISTA = 6

/**
 * Una clase del docente en la pantalla de inicio, con lo que puede hacerle.
 *
 * Las dos acciones se resuelven acá y no mandan a otra pantalla, que es la
 * diferencia con el listado que había antes: las dos cosas que le pasan a
 * una reserva ya hecha —me cambiaron el aula y necesito otra máquina, o
 * directamente no la doy— son urgentes y las hace alguien que no sabe en qué
 * sección del sistema vive cada una.
 *
 * Los paneles son los mismos componentes que usa la pantalla de reservas, no
 * una segunda versión: `CambiarEquipoDeReserva` y `CancelarReserva`. Una
 * copia propia acá terminaría, con el tiempo, cancelando distinto de como
 * cancela la otra pantalla.
 *
 * Solo se abre uno por vez: los dos hablan de la misma reserva y abiertos
 * juntos se leen como un solo formulario largo con dos botones de confirmar.
 */
export function TarjetaDeClase({
  grupo,
  hoy,
  destacada,
}: {
  grupo: GrupoDeReservas
  hoy: string
  /** La próxima: se marca para que sea lo primero que se mira. */
  destacada?: boolean
}) {
  const [panel, setPanel] = useState<"cambiar" | "cancelar" | null>(null)

  const equipos = grupo.reservas.filter((r) => r.estado === "CONFIRMADA")
  const deMas = equipos.length - EQUIPOS_A_LA_VISTA

  return (
    <div
      className={[
        "grid gap-3 rounded-xl border p-4",
        destacada ? "border-primary/40 bg-primary/5" : "bg-superficie",
      ].join(" ")}
    >
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <p className="text-lg font-semibold">
          {etiquetaDeDia(grupo.fecha, hoy)}{" "}
          <span className="tabular-nums">
            de {grupo.horaInicio} a {grupo.horaFin}
          </span>
        </p>
        {grupo.esRecurrente && (
          <span className="text-muted-foreground text-sm">
            Se repite todas las semanas
          </span>
        )}
      </div>

      <div>
        <p className="font-medium">
          {grupo.materiaNombre ?? "Reserva"}
          {grupo.cursoNombre && (
            <span className="text-muted-foreground font-normal">
              {" "}
              · {grupo.cursoNombre}
            </span>
          )}
        </p>
        <p className="text-muted-foreground text-sm">
          {equipos.length}{" "}
          {equipos.length === 1 ? "computadora reservada" : "computadoras reservadas"}
        </p>
      </div>

      {/* Cuáles le tocan, por su número de zócalo: es lo que va a pedir en el
          laboratorio. Se cortan en unas pocas porque una reserva de un curso
          entero llenaría la pantalla de etiquetas y taparía las acciones. */}
      <div className="flex flex-wrap gap-1.5">
        {equipos.slice(0, EQUIPOS_A_LA_VISTA).map((r) => (
          <Badge key={r.id} variant="outline">
            {r.etiqueta}
            {r.carroNombre && ` · ${r.carroNombre}`}
          </Badge>
        ))}
        {deMas > 0 && <Badge variant="outline">y {deMas} más</Badge>}
      </div>

      {/* `h-11` y no el alto que traen los tamaños del botón: son 44px, el
          mínimo que WCAG 2.5.5 pide para un blanco táctil, y estos dos se
          aprietan desde un teléfono. Con el `sm` del sistema quedaban en 28px
          — la mitad del ancho de un dedo— en la pantalla que justamente tiene
          que poder usar alguien que no maneja bien el teléfono. */}
      {panel === null && (
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            className="h-11 px-4 sm:h-9"
            onClick={() => setPanel("cambiar")}
          >
            Cambiar una computadora
          </Button>
          <Button
            variant="outline"
            className="h-11 px-4 sm:h-9"
            onClick={() => setPanel("cancelar")}
          >
            Cancelar esta clase
          </Button>
        </div>
      )}

      {panel === "cambiar" && (
        <CambiarEquipoDeReserva grupo={grupo} onListo={() => setPanel(null)} />
      )}
      {panel === "cancelar" && (
        <CancelarReserva grupo={grupo} onListo={() => setPanel(null)} />
      )}
    </div>
  )
}
