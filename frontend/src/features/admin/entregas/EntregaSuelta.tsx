import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

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
import { contar } from "@/lib/plural"

/**
 * Entregar algo sin reserva detrás: "necesito una compu para hacer un
 * trámite", "me llevo el proyector".
 *
 * Ofrece TODO el inventario que no esté afuera, no solo las computadoras de
 * los carros: lo que más se presta en el momento son justamente los equipos
 * sueltos —un proyector, un cargador—, así que van primero en la lista.
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

  // Todo el inventario en UNA consulta: el endpoint sin filtro ya devuelve
  // los de carro y los sueltos juntos, así que no hace falta pedir carro por
  // carro y unir las respuestas. Los carros se siguen pidiendo, pero solo
  // para poder decir de dónde sale cada equipo.
  const { data: todos } = useQuery({
    queryKey: ["equipos"],
    queryFn: () => inventoryApi.listarEquipos(),
  })

  const nombreDeCarro = useMemo(
    () => new Map((carros?.data ?? []).map((c) => [c.id, c.nombre])),
    [carros]
  )

  // Se ofrece todo lo que esté en el inventario y no esté ya afuera. No se
  // filtra por estado a propósito: llevarle al técnico un equipo en
  // mantenimiento es justamente un préstamo, y prohibirlo obligaría a
  // sacarla del sistema para poder anotarlo. Tampoco por `reservable`: un
  // cargador no se reserva pero sí se presta — es el caso principal.
  const equipos = useMemo(
    () =>
      (todos?.data ?? [])
        .filter((eq) => !eq.dadoDeBaja && !yaAfuera.has(eq.id))
        .map((eq) => ({
          id: eq.id,
          etiqueta: eq.etiqueta,
          // De dónde sale: el carro si pertenece a uno, y si no su tipo
          // ("PROYECTOR"), que es lo único que ubica a un equipo suelto.
          donde: eq.carroId ? (nombreDeCarro.get(eq.carroId) ?? "") : eq.tipo,
        })),
    [todos, nombreDeCarro, yaAfuera]
  )

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
      const partes = [`Salieron ${contar(respuesta.entregadas.length, "equipo")}.`]
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
          Para cuando piden algo en el momento — una computadora para un trámite, el
          proyector para una charla. No hace falta que la persona tenga cuenta en el
          sistema.
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
            <Label>¿Qué equipos?</Label>
            <div className="grid max-h-56 gap-1 overflow-y-auto rounded-md border p-2 sm:grid-cols-2">
              {equipos.length === 0 && (
                <p className="text-muted-foreground text-sm">
                  No hay equipos disponibles para entregar.
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
              Entregar {contar(seleccionadas.size, "equipo")}
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
