import { useState } from "react"
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query"

import { SelectorDeHora } from "@/components/SelectorDeHora"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import * as inventoryApi from "@/features/inventory/api"
import type { Carro, PC } from "@/features/inventory/types"
import * as reservasApi from "@/features/reservas/api"
import { hoyISO, type ResultadoBloqueoEvaluacion } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"

const ETIQUETA_ESTADO: Record<string, string> = {
  EN_MANTENIMIENTO: "En mantenimiento",
  FUERA_DE_SERVICIO: "Fuera de servicio",
}

type PCDelCarro = { carro: Carro; pcs: PC[] }

/** Una PC se puede bloquear solo si el backend la aceptaría (RF-04.7). */
function esBloqueable(pc: PC): boolean {
  return pc.estado === "DISPONIBLE" && !pc.dadaDeBaja
}

/**
 * RF-04.7 — un Admin bloquea PCs para una evaluación estatal en una fecha y
 * un rango horario conocidos de antemano.
 *
 * El endpoint existía desde el principio sin ninguna pantalla que lo
 * llamara. Es la operación más destructiva que puede hacer un Admin sin
 * darse cuenta: cancela las reservas ajenas que se solapen, no se
 * restauran solas, y el docente se entera por una notificación. Por eso la
 * pantalla insiste en mostrar qué se va a llevar puesto ANTES de confirmar,
 * que es lo que la API por sí sola no ofrece.
 */
export function BloqueoEvaluacionPage() {
  const queryClient = useQueryClient()
  const [fecha, setFecha] = useState("")
  const [horaInicio, setHoraInicio] = useState("")
  const [horaFin, setHoraFin] = useState("")
  const [motivo, setMotivo] = useState("")
  const [seleccionadas, setSeleccionadas] = useState<string[]>([])
  const [confirmando, setConfirmando] = useState(false)
  const [resultado, setResultado] = useState<ResultadoBloqueoEvaluacion | null>(null)

  const franjaCompleta = Boolean(fecha && horaInicio && horaFin && horaFin > horaInicio)

  const carrosQuery = useQuery({
    queryKey: ["carros"],
    queryFn: () => inventoryApi.listarCarros(),
  })
  const carros = carrosQuery.data?.data ?? []

  // Todo el inventario de una, no carro por carro como en /inventario: acá
  // hace falta el total para poder decir cuántas de las PCs elegidas están
  // ocupadas, y la selección cruza carros.
  const pcsQueries = useQueries({
    queries: carros.map((c) => ({
      queryKey: ["pcs", c.id],
      queryFn: () => inventoryApi.listarPCsDeCarro(c.id),
    })),
  })

  /**
   * Las PCs libres en la franja. La diferencia contra el inventario es lo
   * que permite avisar "esta tiene una reserva que se va a cancelar" sin
   * que el backend tenga un endpoint de simulación.
   *
   * `pcs-disponibles` ya excluye las que no son reservables, así que se
   * cruza solo contra las bloqueables.
   */
  const libresQuery = useQuery({
    queryKey: ["pcs-disponibles", fecha, horaInicio, horaFin],
    queryFn: () => reservasApi.pcsDisponibles(fecha, horaInicio, horaFin),
    enabled: franjaCompleta,
  })

  const bloquear = useMutation({
    mutationFn: (req: Parameters<typeof reservasApi.bloquearParaEvaluacion>[0]) =>
      reservasApi.bloquearParaEvaluacion(req),
    onSuccess: async (res) => {
      setResultado(res)
      setConfirmando(false)
      setSeleccionadas([])
      // El bloqueo ocupa las PCs y cancela reservas: lo que quedó cacheado
      // de disponibilidad y de listados de reservas ya no vale.
      await queryClient.invalidateQueries({ queryKey: ["pcs-disponibles"] })
      await queryClient.invalidateQueries({ queryKey: ["reservas"] })
    },
  })

  const inventario: PCDelCarro[] = carros.map((carro, i) => ({
    carro,
    pcs: pcsQueries[i]?.data?.data ?? [],
  }))
  const inventarioCargando = carrosQuery.isLoading || pcsQueries.some((q) => q.isLoading)

  // Mientras la consulta de libres no haya vuelto no se sabe nada: sin este
  // recaudo un `Set` vacío haría figurar TODAS las PCs como ocupadas.
  const hayDatosDeOcupacion = franjaCompleta && libresQuery.isSuccess
  const idsLibres = new Set((libresQuery.data?.data ?? []).map((p) => p.pcId))

  function estaOcupada(pc: PC): boolean {
    return hayDatosDeOcupacion && esBloqueable(pc) && !idsLibres.has(pc.id)
  }

  const todasLasPCs = inventario.flatMap((g) => g.pcs)
  const elegidasOcupadas = todasLasPCs.filter(
    (pc) => seleccionadas.includes(pc.id) && estaOcupada(pc)
  )

  // El backend no valida el motivo: lo intercala tal cual en el aviso a cada
  // docente ("…bloqueo por evaluación estatal (%s)"), así que vacío deja un
  // paréntesis hueco en la notificación. Se exige acá.
  const puedeBloquear = franjaCompleta && motivo.trim() !== "" && seleccionadas.length > 0

  function alternar(pcId: string, tildada: boolean) {
    setResultado(null)
    setConfirmando(false)
    setSeleccionadas((antes) =>
      tildada ? [...antes, pcId] : antes.filter((id) => id !== pcId)
    )
  }

  function confirmar() {
    bloquear.mutate({
      pcIds: seleccionadas,
      fecha,
      horaInicio,
      horaFin,
      motivo: motivo.trim(),
    })
  }

  return (
    <div className="mx-auto max-w-3xl">
      <EncabezadoDePagina
        titulo="Bloqueo por evaluación estatal"
        descripcion="Reserva las PCs para una evaluación y cancela las reservas de docentes que se superpongan. Los docentes afectados reciben un aviso con el motivo."
      />

      {carrosQuery.error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(carrosQuery.error)}</AlertDescription>
        </Alert>
      )}
      {bloquear.error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(bloquear.error)}</AlertDescription>
        </Alert>
      )}

      {resultado && (
        <Alert className="mb-4">
          <AlertDescription>
            Bloqueo creado sobre {resultado.bloqueos.length} PC
            {resultado.bloqueos.length === 1 ? "" : "s"}.{" "}
            {resultado.reservasCanceladas === 0
              ? "No había ninguna reserva en esa franja."
              : `Se cancelaron ${resultado.reservasCanceladas} reserva(s) y se notificó a ${resultado.docentesNotificados} docente(s).`}
          </AlertDescription>
        </Alert>
      )}

      <Card className="mb-4">
        <CardContent className="grid gap-3 pt-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <div className="grid gap-2">
              <Label htmlFor="fecha">Fecha</Label>
              {/* Sin aviso de fin de semana a propósito: RF-04.7 está
                  exceptuada de la semana lectiva — una evaluación estatal es
                  excepcional por naturaleza y la fecha la pone el Admin. */}
              <Input
                id="fecha"
                type="date"
                min={hoyISO()}
                value={fecha}
                onChange={(e) => setFecha(e.target.value)}
              />
            </div>
            <SelectorDeHora
              id="horaInicio"
              etiqueta="Hora de inicio"
              valor={horaInicio}
              onCambio={setHoraInicio}
            />
            <SelectorDeHora
              id="horaFin"
              etiqueta="Hora de fin"
              valor={horaFin}
              onCambio={setHoraFin}
            />
          </div>

          {horaInicio && horaFin && horaFin <= horaInicio && (
            <p className="text-destructive text-sm">
              La hora de fin tiene que ser posterior a la de inicio.
            </p>
          )}

          <div className="grid gap-2">
            <Label htmlFor="motivo">Motivo</Label>
            <Input
              id="motivo"
              value={motivo}
              onChange={(e) => setMotivo(e.target.value)}
              placeholder="Evaluación Aprender 2026"
            />
            <p className="text-muted-foreground text-sm">
              Es lo que van a leer los docentes en el aviso de cancelación.
            </p>
          </div>
        </CardContent>
      </Card>

      <h2 className="mb-2 text-xl font-semibold">Qué PCs se bloquean</h2>

      {!franjaCompleta && (
        <p className="text-muted-foreground mb-2 text-sm">
          Completá la fecha y el horario para ver cuáles ya tienen reserva.
        </p>
      )}
      {inventarioCargando && <p className="text-muted-foreground">Cargando PCs…</p>}
      {!inventarioCargando && todasLasPCs.length === 0 && (
        <p className="text-muted-foreground">
          No hay ninguna PC cargada en el inventario.
        </p>
      )}

      <div className="grid gap-4">
        {inventario.map(({ carro, pcs }) => {
          const activas = pcs.filter((pc) => !pc.dadaDeBaja)
          if (activas.length === 0) return null

          return (
            <fieldset key={carro.id} className="grid gap-2">
              <legend className="mb-1 text-sm font-medium">{carro.nombre}</legend>
              <div className="grid gap-2 sm:grid-cols-2">
                {activas.map((pc) => {
                  const id = `pc-${pc.id}`
                  const bloqueable = esBloqueable(pc)
                  const ocupada = estaOcupada(pc)

                  return (
                    <div
                      key={pc.id}
                      className="flex items-start gap-2 rounded-md border p-2"
                    >
                      {/* Las que no están DISPONIBLE no se ofrecen: el
                          backend rechaza el bloqueo ENTERO si viene una
                          sola así (ErrPCNoDisponible), no la saltea. */}
                      <Checkbox
                        id={id}
                        disabled={!bloqueable}
                        checked={seleccionadas.includes(pc.id)}
                        onCheckedChange={(v) => alternar(pc.id, v === true)}
                      />
                      <div className="grid gap-0.5">
                        <Label htmlFor={id} className="cursor-pointer">
                          PC {pc.identificador}
                        </Label>
                        {!bloqueable && (
                          <Badge variant="destructive" className="w-fit">
                            {ETIQUETA_ESTADO[pc.estado] ?? pc.estado}
                          </Badge>
                        )}
                        {ocupada && (
                          <Badge variant="outline" className="w-fit">
                            Con reserva en esa franja
                          </Badge>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            </fieldset>
          )
        })}
      </div>

      <div className="mt-4 grid gap-3">
        {seleccionadas.length > 0 && (
          <p className="text-sm">
            {seleccionadas.length} PC{seleccionadas.length === 1 ? "" : "s"} seleccionada
            {seleccionadas.length === 1 ? "" : "s"}.
          </p>
        )}

        {!confirmando && (
          <div>
            <Button disabled={!puedeBloquear} onClick={() => setConfirmando(true)}>
              Revisar bloqueo
            </Button>
          </div>
        )}

        {/* El paso de confirmación existe por lo que dice adentro: la
            cascada no se deshace, ni siquiera borrando el bloqueo. */}
        {confirmando && (
          <div className="grid gap-2 rounded-md border p-3">
            <p className="text-sm">
              Se van a bloquear {seleccionadas.length} PC
              {seleccionadas.length === 1 ? "" : "s"} el {fecha} de {horaInicio} a{" "}
              {horaFin}.
            </p>
            {elegidasOcupadas.length > 0 ? (
              <p className="text-destructive text-sm">
                {elegidasOcupadas.length} de esas PCs tienen una reserva en esa franja: se
                va a cancelar y el docente va a recibir un aviso. Las reservas canceladas
                no se recuperan aunque después se saque el bloqueo.
              </p>
            ) : (
              <p className="text-muted-foreground text-sm">
                Ninguna de las PCs elegidas tiene reservas en esa franja.
              </p>
            )}
            <div className="flex gap-2">
              <Button
                variant="destructive"
                size="sm"
                disabled={bloquear.isPending}
                onClick={confirmar}
              >
                Confirmar bloqueo
              </Button>
              <Button variant="outline" size="sm" onClick={() => setConfirmando(false)}>
                Volver
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
