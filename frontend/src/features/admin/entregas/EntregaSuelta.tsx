import { useMemo, useState } from "react"
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { PRESTAMOS_KEY } from "@/features/admin/entregas/compartido"
import * as inventoryApi from "@/features/inventory/api"
import * as reservasApi from "@/features/reservas/api"
import { getErrorMessage } from "@/lib/api-client"

/**
 * Entregar una computadora sin reserva detrás: "necesito una compu para
 * hacer un trámite".
 *
 * Vive suelto porque se usa desde dos lados — el panel del laboratorio y la
 * pantalla de entregas—: es de las cosas que más se hacen en el mostrador y
 * tenerla a un clic desde donde el Admin ya está parado es la diferencia
 * entre usarlo y volver al papel.
 */
export function EntregaSuelta({
  yaAfuera,
  onCerrar,
}: {
  yaAfuera: Set<string>
  onCerrar: () => void
}) {
  const queryClient = useQueryClient()
  const [nombre, setNombre] = useState("")
  const [motivo, setMotivo] = useState("")
  const [devolucion, setDevolucion] = useState("")
  const [seleccionadas, setSeleccionadas] = useState<Set<string>>(new Set())
  const [resumen, setResumen] = useState<string | null>(null)

  const { data: carros } = useQuery({
    queryKey: ["carros"],
    queryFn: inventoryApi.listarCarros,
  })

  const consultasDeEquipos = useQueries({
    queries: (carros?.data ?? []).map((c) => ({
      queryKey: ["equipos", c.id],
      queryFn: () => inventoryApi.listarEquiposDeCarro(c.id),
    })),
  })

  // Los equipos que no están en ningún carro: el proyector, los cargadores,
  // las notebooks sueltas. La mayoría de los préstamos espontáneos son
  // justamente de estas cosas, así que van primero.
  const { data: sueltos } = useQuery({
    queryKey: ["equipos-sueltos"],
    queryFn: inventoryApi.listarEquiposSueltos,
  })

  // Se ofrece todo lo que esté en el inventario y no esté ya afuera. No se
  // filtra por estado a propósito: llevarle al técnico un equipo en
  // mantenimiento es justamente un préstamo, y prohibirlo obligaría a
  // sacarla del sistema para poder anotarlo. Tampoco por `reservable`: un
  // cargador no se reserva pero sí se presta — es el caso principal.
  const equipos = useMemo(() => {
    const lista: { id: string; etiqueta: string; donde: string }[] = []

    ;(sueltos?.data ?? [])
      .filter((eq) => !eq.dadoDeBaja && !yaAfuera.has(eq.id))
      .forEach((eq) => lista.push({ id: eq.id, etiqueta: eq.etiqueta, donde: eq.tipo }))

    ;(carros?.data ?? []).forEach((carro, i) => {
      const equiposDelCarro = consultasDeEquipos[i]?.data?.data ?? []
      equiposDelCarro
        .filter((equipo) => !equipo.dadoDeBaja && !yaAfuera.has(equipo.id))
        .forEach((equipo) => lista.push({ id: equipo.id, etiqueta: equipo.etiqueta, donde: carro.nombre }))
    })
    return lista
  }, [carros, consultasDeEquipos, sueltos, yaAfuera])

  const entregar = useMutation({
    mutationFn: () =>
      reservasApi.entregarSuelta({
        equipoIds: [...seleccionadas],
        nombre: nombre.trim(),
        motivo: motivo.trim() || undefined,
        devolucionEstimada: devolucion ? new Date(devolucion).toISOString() : undefined,
      }),
    onSuccess: async (respuesta) => {
      const avisos = respuesta.avisos ?? []
      const noSalieron = respuesta.noEntregadas ?? []
      const partes = [`Salieron ${respuesta.entregadas.length} computadora(s).`]
      if (noSalieron.length > 0) {
        partes.push(`No salieron ${noSalieron.length}: ${noSalieron.map((n) => n.detalle).join("; ")}`)
      }
      // El aviso no impidió nada: el sistema no sabe cuánto dura un trámite,
      // así que la decisión es del Admin.
      for (const a of avisos) {
        partes.push(
          `Ojo: esa máquina tiene reserva ${a.fecha} de ${a.horaInicio} a ${a.horaFin}${a.docente ? ` (${a.docente})` : ""}.`
        )
      }
      setResumen(partes.join(" "))
      setSeleccionadas(new Set())
      setNombre("")
      setMotivo("")
      setDevolucion("")
      await queryClient.invalidateQueries({ queryKey: PRESTAMOS_KEY })
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Entregar sin reserva</CardTitle>
        <CardDescription>
          Para cuando piden una computadora en el momento — un trámite, algo puntual. No
          hace falta que la persona tenga cuenta en el sistema.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            setResumen(null)
            entregar.mutate()
          }}
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="entrega-nombre">¿A quién?</Label>
              <Input
                id="entrega-nombre"
                value={nombre}
                onChange={(e) => setNombre(e.target.value)}
                placeholder="Ej.: Marta (secretaría)"
                required
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="entrega-motivo">¿Para qué? (opcional)</Label>
              <Input
                id="entrega-motivo"
                value={motivo}
                onChange={(e) => setMotivo(e.target.value)}
                placeholder="Ej.: trámite"
              />
            </div>
          </div>

          <div className="grid gap-1.5 sm:max-w-xs">
            <Label htmlFor="entrega-devolucion">¿Cuándo la devuelve? (opcional)</Label>
            <Input
              id="entrega-devolucion"
              type="datetime-local"
              value={devolucion}
              onChange={(e) => setDevolucion(e.target.value)}
            />
            {/* Sin hora pactada no se le reclama nada: "vengo en un rato" es
                una respuesta válida, y una hora inventada solo generaría
                reclamos falsos. */}
            <p className="text-muted-foreground text-xs">
              Si no la sabés, dejalo vacío: no se le va a reclamar la devolución.
            </p>
          </div>

          <div className="grid gap-2">
            <Label>¿Qué computadoras?</Label>
            <div className="grid max-h-56 gap-1 overflow-y-auto rounded-md border p-2 sm:grid-cols-2">
              {equipos.length === 0 && (
                <p className="text-muted-foreground text-sm">
                  No hay computadoras disponibles para entregar.
                </p>
              )}
              {equipos.map((equipo) => (
                <label key={equipo.id} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={seleccionadas.has(equipo.id)}
                    onChange={() => {
                      const nueva = new Set(seleccionadas)
                      if (nueva.has(equipo.id)) nueva.delete(equipo.id)
                      else nueva.add(equipo.id)
                      setSeleccionadas(nueva)
                    }}
                  />
                  {equipo.etiqueta}
                  <span className="text-muted-foreground">({equipo.donde})</span>
                </label>
              ))}
            </div>
          </div>

          {entregar.error && (
            <Alert variant="destructive">
              <AlertDescription>{getErrorMessage(entregar.error)}</AlertDescription>
            </Alert>
          )}
          {resumen && (
            <Alert>
              <AlertDescription>{resumen}</AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={entregar.isPending || seleccionadas.size === 0}>
              Entregar {seleccionadas.size} computadora(s)
            </Button>
            <Button type="button" variant="outline" onClick={onCerrar}>
              Cerrar
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
