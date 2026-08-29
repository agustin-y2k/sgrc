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
import {
  SelectorDeEquipos,
  type EquipoParaEntregar,
} from "@/features/admin/entregas/SelectorDeEquipos"
import * as inventoryApi from "@/features/inventory/api"
import * as reservasApi from "@/features/reservas/api"
import { getErrorMessage } from "@/lib/api-client"
import { contar } from "@/lib/plural"

/**
 * Entregar algo sin reserva detrás: "necesito una compu para hacer un
 * trámite", "me llevo el proyector".
 *
 * Es la entrega que más se usa, así que ocupa todo el ancho que le den y no
 * media columna: quien la completa tiene a alguien esperando enfrente, y los
 * equipos se eligen de una grilla que se ve entera, no de una lista de tres
 * renglones con barra de desplazamiento.
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
  // carro y unir las respuestas.
  const { data: todos } = useQuery({
    queryKey: ["equipos"],
    queryFn: () => inventoryApi.listarEquipos(),
  })

  const nombreDeCarro = useMemo(
    () => new Map((carros?.data ?? []).map((c) => [c.id, c.nombre])),
    [carros]
  )

  // Se ofrece lo que está en el inventario, en condiciones de prestarse y no
  // está ya afuera. Un equipo en mantenimiento o fuera de servicio está acá y
  // no se le da a nadie (RF-08.17): para sacarlo del laboratorio está la
  // salida a reparación, que es otra pantalla a propósito.
  const equipos: EquipoParaEntregar[] = useMemo(
    () =>
      (todos?.data ?? [])
        .filter(
          (eq) => !eq.dadoDeBaja && eq.estado === "DISPONIBLE" && !yaAfuera.has(eq.id)
        )
        .map((eq) => ({
          id: eq.id,
          etiqueta: eq.etiqueta,
          donde: eq.carroId ? (nombreDeCarro.get(eq.carroId) ?? "") : eq.tipo,
        })),
    [todos, nombreDeCarro, yaAfuera]
  )

  // Qué se está por entregar, escrito con los nombres que se leen en la
  // etiqueta de la máquina: la grilla puede quedar desplazada y la selección
  // fuera de la vista justo cuando se aprieta el botón.
  const nombresElegidos = useMemo(
    () => equipos.filter((eq) => seleccionadas.has(eq.id)).map((eq) => eq.etiqueta),
    [equipos, seleccionadas]
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
        partes.push(
          `No salieron ${noSalieron.length}: ${noSalieron.map((n) => n.detalle).join("; ")}`
        )
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
          className="grid gap-6"
          onSubmit={(e) => {
            e.preventDefault()
            setResumen(null)
            entregar.mutate()
          }}
        >
          {/* Los datos de la persona a la izquierda y los equipos a la
              derecha, que es el orden en que se pregunta en el mostrador. En
              un teléfono se apilan en ese mismo orden. */}
          <div className="grid gap-6 lg:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]">
            <div className="grid content-start gap-4">
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

              <div className="grid gap-1.5">
                <Label htmlFor="entrega-devolucion">
                  ¿Cuándo la devuelve? (opcional)
                </Label>
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
            </div>

            <SelectorDeEquipos
              titulo="¿Qué equipos?"
              equipos={equipos}
              seleccionados={seleccionadas}
              onSeleccionar={setSeleccionadas}
              vacio="No hay equipos disponibles para entregar. Los que están en mantenimiento o fuera de servicio no se prestan, y los que ya salieron figuran en «Afuera del laboratorio»."
            />
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

          <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t pt-4">
            <Button
              type="submit"
              size="lg"
              disabled={entregar.isPending || seleccionadas.size === 0}
            >
              {seleccionadas.size === 0
                ? "Entregar"
                : `Entregar ${contar(seleccionadas.size, "equipo")}`}
            </Button>
            <Button type="button" variant="outline" onClick={onCerrar}>
              Cerrar
            </Button>
            {nombresElegidos.length > 0 && (
              <p className="text-muted-foreground min-w-0 flex-1 text-sm">
                Salen: {nombresElegidos.join(", ")}
              </p>
            )}
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
