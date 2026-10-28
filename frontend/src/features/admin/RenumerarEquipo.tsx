import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { renumerarEquipo } from "./api"
import { invalidarInventario } from "./invalidarInventario"
import type { Equipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

export function RenumerarEquipo({
  equipo,
  onListo,
}: {
  equipo: Equipo
  onListo: () => void
}) {
  const [abierto, setAbierto] = useState(false)
  const [numero, setNumero] = useState("")
  const [motivo, setMotivo] = useState("")
  const [confirmado, setConfirmado] = useState(false)
  const cliente = useQueryClient()
  const identificadorActual = equipo.identificador ?? 0
  const cambio = useMutation({
    mutationFn: () =>
      renumerarEquipo(equipo.id, identificadorActual, Number(numero), motivo.trim()),
    onSuccess: async () => {
      await invalidarInventario(cliente)
      onListo()
    },
  })
  if (equipo.dadoDeBaja || identificadorActual <= 0) return null
  if (!abierto)
    return (
      <Button type="button" variant="outline" onClick={() => setAbierto(true)}>
        Cambiar número de PC
      </Button>
    )
  const valido =
    /^\d+$/.test(numero) &&
    Number(numero) > 0 &&
    Number(numero) <= 2147483647 &&
    Number(numero) !== equipo.identificador
  const prefijo = `rotulo-${equipo.id}`
  return (
    <form
      className="grid gap-3 rounded-xl border p-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (valido && confirmado && motivo.trim().length >= 5) cambio.mutate()
      }}
    >
      <h3 className="font-semibold">Cambiar el rótulo de {equipo.etiqueta}</h3>
      <p className="text-sm">
        Serie: <strong>{equipo.numeroSerie}</strong>. Se conserva la misma computadora,
        con sus reservas, préstamos, cuentas y prioridades. Las consultas mostrarán el
        nuevo número. Los registros de auditoría y los reportes de ciclos cerrados
        conservan sus datos históricos.
      </p>
      {cambio.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(cambio.error)}</AlertDescription>
        </Alert>
      )}
      <div className="grid gap-1.5">
        <Label htmlFor={`${prefijo}-numero`}>Nuevo número de PC</Label>
        <Input
          id={`${prefijo}-numero`}
          type="number"
          min={1}
          max={2147483647}
          step={1}
          required
          value={numero}
          onChange={(e) => {
            setNumero(e.target.value)
            setConfirmado(false)
          }}
          disabled={cambio.isPending}
          aria-describedby={`${prefijo}-ayuda`}
        />
        <p id={`${prefijo}-ayuda`} className="text-muted-foreground text-sm">
          Elegí un número libre dentro de este carro. No se modifica ninguna otra PC.
        </p>
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor={`${prefijo}-motivo`}>Motivo del cambio de rótulo</Label>
        <Input
          id={`${prefijo}-motivo`}
          minLength={5}
          maxLength={500}
          required
          value={motivo}
          onChange={(e) => setMotivo(e.target.value)}
          disabled={cambio.isPending}
          placeholder="Ej.: reemplazo del rótulo deteriorado"
        />
      </div>
      {valido && (
        <p className="rounded-lg bg-muted p-3 font-medium">
          {equipo.etiqueta} → PC {Number(numero)} · misma serie {equipo.numeroSerie}
        </p>
      )}
      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          className="mt-1"
          checked={confirmado}
          onChange={(e) => setConfirmado(e.target.checked)}
          disabled={cambio.isPending}
        />
        Confirmo que es la misma computadora y que actualizaré su rótulo físico.
      </label>
      <div className="flex flex-wrap gap-2">
        <Button
          type="submit"
          disabled={
            !valido || !confirmado || motivo.trim().length < 5 || cambio.isPending
          }
        >
          {cambio.isPending ? "Cambiando rótulo…" : "Confirmar nuevo rótulo"}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={cambio.isPending}
          onClick={() => {
            setAbierto(false)
            cambio.reset()
          }}
        >
          Cancelar cambio de rótulo
        </Button>
      </div>
    </form>
  )
}
