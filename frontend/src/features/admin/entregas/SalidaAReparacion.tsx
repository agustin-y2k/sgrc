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
import { ETIQUETA_ESTADO_EQUIPO } from "@/features/inventory/types"
import * as reservasApi from "@/features/reservas/api"
import { getErrorMessage } from "@/lib/api-client"
import { contar } from "@/lib/plural"

/**
 * Sacar del laboratorio un equipo que NO está en condiciones de prestarse:
 * se lo lleva el técnico, va al service, vuelve arreglado o no vuelve.
 *
 * Vive en un panel aparte del mostrador a propósito. Es el único camino para
 * que salga algo en mantenimiento o fuera de servicio —la entrega del día a
 * día ni siquiera los ofrece (RF-08.17)—, y mezclarlos haría que la lista de
 * todos los días tenga máquinas rotas adentro. Lo que sí comparten es el
 * registro: una vez afuera, figura y se recibe como cualquier otra.
 */
export function SalidaAReparacion({
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

  const { data: todos } = useQuery({
    queryKey: ["equipos"],
    queryFn: () => inventoryApi.listarEquipos(),
  })

  const nombreDeCarro = useMemo(
    () => new Map((carros?.data ?? []).map((c) => [c.id, c.nombre])),
    [carros]
  )

  // Justo lo contrario que la entrega del día a día: acá se ofrece solo lo
  // que no está disponible. Lo dado de baja no aparece ni acá — ya no es
  // parte del parque, y lo que sale del laboratorio es lo que después hay que
  // esperar de vuelta.
  const equipos: EquipoParaEntregar[] = useMemo(
    () =>
      (todos?.data ?? [])
        .filter(
          (eq) => !eq.dadoDeBaja && eq.estado !== "DISPONIBLE" && !yaAfuera.has(eq.id)
        )
        .map((eq) => ({
          id: eq.id,
          etiqueta: eq.etiqueta,
          donde: eq.carroId ? (nombreDeCarro.get(eq.carroId) ?? "") : eq.tipo,
          nota: ETIQUETA_ESTADO_EQUIPO[eq.estado],
        })),
    [todos, nombreDeCarro, yaAfuera]
  )

  const enviar = useMutation({
    mutationFn: () =>
      reservasApi.entregarSuelta({
        equipoIds: [...seleccionadas],
        nombre: nombre.trim(),
        motivo: motivo.trim(),
        devolucionEstimada: devolucion ? new Date(devolucion).toISOString() : undefined,
        salidaAReparacion: true,
      }),
    onSuccess: async (respuesta) => {
      const noSalieron = respuesta.noEntregadas ?? []
      const partes = [`Salieron ${contar(respuesta.entregadas.length, "equipo")}.`]
      if (noSalieron.length > 0) {
        partes.push(
          `No salieron ${noSalieron.length}: ${noSalieron.map((n) => n.detalle).join("; ")}`
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
        <CardTitle>Salida a reparación</CardTitle>
        <CardDescription>
          Para lo que está en mantenimiento o fuera de servicio y se va del laboratorio:
          al técnico, al service, a que lo miren. No se presta a nadie — queda registrado
          que salió, quién se lo llevó y por qué, y se recibe como cualquier otro equipo
          cuando vuelve.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-6"
          onSubmit={(e) => {
            e.preventDefault()
            setResumen(null)
            enviar.mutate()
          }}
        >
          <div className="grid gap-6 lg:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]">
            <div className="grid content-start gap-4">
              <div className="grid gap-1.5">
                <Label htmlFor="reparacion-nombre">¿Quién se lo lleva?</Label>
                <Input
                  id="reparacion-nombre"
                  value={nombre}
                  onChange={(e) => setNombre(e.target.value)}
                  placeholder="Ej.: Service Rossi"
                  required
                />
              </div>

              {/* Obligatorio, al revés que en la entrega común: sin esto la
                  salida a reparación es un préstamo mal cargado, y el registro
                  de que la máquina se fue no dice a dónde. */}
              <div className="grid gap-1.5">
                <Label htmlFor="reparacion-motivo">¿A dónde va y por qué?</Label>
                <Input
                  id="reparacion-motivo"
                  value={motivo}
                  onChange={(e) => setMotivo(e.target.value)}
                  placeholder="Ej.: al service, no enciende"
                  required
                />
                <p className="text-muted-foreground text-xs">
                  Es la constancia: dentro de dos meses es lo único que va a explicar por
                  qué el equipo no está.
                </p>
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="reparacion-devolucion">¿Cuándo vuelve? (opcional)</Label>
                <Input
                  id="reparacion-devolucion"
                  type="datetime-local"
                  value={devolucion}
                  onChange={(e) => setDevolucion(e.target.value)}
                />
                <p className="text-muted-foreground text-xs">
                  Una reparación casi nunca tiene fecha. Sin ella no se reclama nada.
                </p>
              </div>
            </div>

            <SelectorDeEquipos
              titulo="¿Qué equipos salen?"
              equipos={equipos}
              seleccionados={seleccionadas}
              onSeleccionar={setSeleccionadas}
              vacio="No hay ningún equipo en mantenimiento ni fuera de servicio esperando salir. Se marca el estado desde Inventario."
            />
          </div>

          {enviar.error && (
            <Alert variant="destructive">
              <AlertDescription>{getErrorMessage(enviar.error)}</AlertDescription>
            </Alert>
          )}
          {resumen && (
            <Alert>
              <AlertDescription>{resumen}</AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t pt-4">
            <Button type="submit" disabled={enviar.isPending || seleccionadas.size === 0}>
              {seleccionadas.size === 0
                ? "Registrar la salida"
                : `Registrar la salida de ${contar(seleccionadas.size, "equipo")}`}
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
