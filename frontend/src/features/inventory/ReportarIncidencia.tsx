import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as inventoryApi from "@/features/inventory/api"
import type { GravedadIncidencia, PC } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-03.5 — reportar una falla. Lo puede hacer cualquier usuario
 * autenticado, no solo un Admin: el docente sentado frente a la PC es el
 * que ve que no anda.
 *
 * Reportar NO cambia el estado de la PC: sigue siendo reservable hasta que
 * un Admin decida sacarla de servicio (RF-03.8). Se dice en la pantalla
 * para que nadie asuma que reportar ya la bloqueó.
 */

const GRAVEDADES: { valor: GravedadIncidencia; etiqueta: string; ayuda: string }[] = [
  { valor: "LEVE", etiqueta: "Leve", ayuda: "Molesta pero se puede usar" },
  { valor: "MODERADA", etiqueta: "Moderada", ayuda: "Se puede usar a medias" },
  { valor: "GRAVE", etiqueta: "Grave", ayuda: "No se puede usar" },
]

export function ReportarIncidencia({ pc, onListo }: { pc: PC; onListo: () => void }) {
  const queryClient = useQueryClient()
  const [descripcion, setDescripcion] = useState("")
  const [gravedad, setGravedad] = useState<GravedadIncidencia>("MODERADA")
  const [reportada, setReportada] = useState(false)

  const reportar = useMutation({
    mutationFn: () =>
      inventoryApi.reportarIncidencia({ pcId: pc.id, descripcion, gravedad }),
    onSuccess: async () => {
      setReportada(true)
      await queryClient.invalidateQueries({ queryKey: ["incidencias", pc.id] })
    },
  })

  if (reportada) {
    return (
      <div className="grid gap-2 rounded-md border p-3">
        <p className="text-sm">
          Listo: la falla de la PC {pc.identificador} quedó registrada. Un Admin la va a
          ver en el historial del equipo.
        </p>
        <div>
          <Button variant="outline" size="sm" onClick={onListo}>
            Cerrar
          </Button>
        </div>
      </div>
    )
  }

  return (
    <form
      className="grid gap-3 rounded-md border p-3"
      onSubmit={(e) => {
        e.preventDefault()
        reportar.mutate()
      }}
    >
      <p className="font-medium">Reportar un problema en la PC {pc.identificador}</p>

      {reportar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(reportar.error)}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-1.5">
        <Label htmlFor={`descripcion-${pc.id}`}>¿Qué le pasa?</Label>
        <Input
          id={`descripcion-${pc.id}`}
          value={descripcion}
          onChange={(e) => setDescripcion(e.target.value)}
          placeholder="Ej.: no arranca, la pantalla parpadea, no tiene teclado"
        />
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor={`gravedad-${pc.id}`}>Gravedad</Label>
        <Select
          id={`gravedad-${pc.id}`}
          value={gravedad}
          onChange={(e) => setGravedad(e.target.value as GravedadIncidencia)}
        >
          {GRAVEDADES.map((g) => (
            <option key={g.valor} value={g.valor}>
              {g.etiqueta} — {g.ayuda}
            </option>
          ))}
        </Select>
      </div>

      <p className="text-muted-foreground text-xs">
        Reportarla no saca la PC de circulación: se sigue pudiendo reservar hasta que un
        Admin la ponga en mantenimiento o fuera de servicio.
      </p>

      <div className="flex gap-2">
        <Button
          type="submit"
          size="sm"
          disabled={descripcion.trim() === "" || reportar.isPending}
        >
          {reportar.isPending ? "Enviando…" : "Reportar"}
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onListo}>
          Cancelar
        </Button>
      </div>
    </form>
  )
}
