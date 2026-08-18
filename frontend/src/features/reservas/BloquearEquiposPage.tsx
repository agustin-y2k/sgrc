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
import type { Carro, Equipo } from "@/features/inventory/types"
import * as reservasApi from "@/features/reservas/api"
import { cruzaMedianoche, hoyISO, type ResultadoBloqueo } from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"

const ETIQUETA_ESTADO: Record<string, string> = {
  EN_MANTENIMIENTO: "En mantenimiento",
  FUERA_DE_SERVICIO: "Fuera de servicio",
}

type EquipoDelCarro = { carro: Carro; equipos: Equipo[] }

/** Un equipo se puede bloquear solo si el backend la aceptaría (RF-04.7). */
function esBloqueable(equipo: Equipo): boolean {
  return equipo.estado === "DISPONIBLE" && !equipo.dadoDeBaja
}

/**
 * RF-04.7 — un Admin toma equipos en una fecha y un rango horario conocidos
 * de antemano, por el motivo que sea.
 *
 * El motivo es texto libre y obligatorio: el laboratorio se toma por una
 * evaluación, una jornada docente, una capacitación o una obra en el aula, y
 * el sistema no puede prever la lista. Ese texto es el que lee el docente al
 * que le cancelaron la clase.
 *
 * Es la operación más destructiva que puede hacer un Admin sin darse cuenta:
 * cancela las reservas ajenas que se solapen, no se restauran solas, y el
 * docente se entera por una notificación. Por eso la pantalla insiste en
 * mostrar qué se va a llevar puesto ANTES de confirmar, que es lo que la API
 * por sí sola no ofrece.
 */
export function BloquearEquiposPage() {
  const queryClient = useQueryClient()
  const [fecha, setFecha] = useState("")
  const [horaInicio, setHoraInicio] = useState("")
  const [horaFin, setHoraFin] = useState("")
  const [motivo, setMotivo] = useState("")
  const [seleccionadas, setSeleccionadas] = useState<string[]>([])
  const [confirmando, setConfirmando] = useState(false)
  const [resultado, setResultado] = useState<ResultadoBloqueo | null>(null)

  const franjaCompleta = Boolean(fecha && horaInicio && horaFin && horaFin !== horaInicio)

  const carrosQuery = useQuery({
    queryKey: ["carros"],
    queryFn: () => inventoryApi.listarCarros(),
  })
  const carros = carrosQuery.data?.data ?? []

  // Todo el inventario de una, no carro por carro como en /inventario: acá
  // hace falta el total para poder decir cuántas de los equipos elegidas están
  // ocupadas, y la selección cruza carros.
  const equiposQueries = useQueries({
    queries: carros.map((c) => ({
      queryKey: ["equipos", c.id],
      queryFn: () => inventoryApi.listarEquiposDeCarro(c.id),
    })),
  })

  /**
   * Los equipos libres en la franja. La diferencia contra el inventario es lo
   * que permite avisar "esta tiene una reserva que se va a cancelar" sin
   * que el backend tenga un endpoint de simulación.
   *
   * `equipos-disponibles` ya excluye las que no son reservables, así que se
   * cruza solo contra las bloqueables.
   */
  const libresQuery = useQuery({
    queryKey: ["equipos-disponibles", fecha, horaInicio, horaFin],
    queryFn: () => reservasApi.equiposDisponibles({ fecha, horaInicio, horaFin }),
    enabled: franjaCompleta,
  })

  const bloquear = useMutation({
    mutationFn: (req: Parameters<typeof reservasApi.bloquearEquipos>[0]) =>
      reservasApi.bloquearEquipos(req),
    onSuccess: async (res) => {
      setResultado(res)
      setConfirmando(false)
      setSeleccionadas([])
      // El bloqueo ocupa los equipos y cancela reservas: lo que quedó cacheado
      // de disponibilidad y de listados de reservas ya no vale.
      await queryClient.invalidateQueries({ queryKey: ["equipos-disponibles"] })
      await queryClient.invalidateQueries({ queryKey: ["reservas"] })
    },
  })

  const inventario: EquipoDelCarro[] = carros.map((carro, i) => ({
    carro,
    equipos: equiposQueries[i]?.data?.data ?? [],
  }))
  const inventarioCargando =
    carrosQuery.isLoading || equiposQueries.some((q) => q.isLoading)

  // Mientras la consulta de libres no haya vuelto no se sabe nada: sin este
  // recaudo un `Set` vacío haría figurar TODAS los equipos como ocupadas.
  const hayDatosDeOcupacion = franjaCompleta && libresQuery.isSuccess
  const idsLibres = new Set((libresQuery.data?.data ?? []).map((p) => p.equipoId))

  function estaOcupada(equipo: Equipo): boolean {
    return hayDatosDeOcupacion && esBloqueable(equipo) && !idsLibres.has(equipo.id)
  }

  const todasLasEquipos = inventario.flatMap((g) => g.equipos)
  const elegidasOcupadas = todasLasEquipos.filter(
    (equipo) => seleccionadas.includes(equipo.id) && estaOcupada(equipo)
  )

  // El motivo lo exige también el dominio, así que mandarlo vacío da un 400
  // y no un bloqueo mudo. Se pide igual acá para que el botón diga por qué no
  // se puede apretar, en vez de fallar después.
  const puedeBloquear = franjaCompleta && motivo.trim() !== "" && seleccionadas.length > 0

  function alternar(equipoId: string, tildada: boolean) {
    setResultado(null)
    setConfirmando(false)
    setSeleccionadas((antes) =>
      tildada ? [...antes, equipoId] : antes.filter((id) => id !== equipoId)
    )
  }

  function confirmar() {
    bloquear.mutate({
      equipoIds: seleccionadas,
      fecha,
      horaInicio,
      horaFin,
      motivo: motivo.trim(),
    })
  }

  return (
    <div className="mx-auto max-w-3xl">
      <EncabezadoDePagina
        titulo="Bloquear equipos"
        descripcion="Toma los equipos para otra cosa —una evaluación, una jornada, una capacitación— y cancela las reservas de docentes que se superpongan. Los docentes afectados reciben un aviso con el motivo."
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
            Bloqueo creado sobre {resultado.bloqueos.length} equipo
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
              {/* Sin aviso de jornada a propósito: RF-04.7 está exceptuada —
                  un bloqueo es excepcional por naturaleza y lo carga el Admin,
                  que es quien declara la jornada. Impedirle bloquear un día
                  que él mismo marcó cerrado sería discutirle un dato suyo. */}
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

          {horaInicio && horaFin && horaFin === horaInicio && (
            <p className="text-destructive text-sm">
              La hora de fin no puede ser igual a la de inicio.
            </p>
          )}

          {/* Un bloqueo también puede cruzar la medianoche — la obra en el
              aula o la jornada docente no se detienen a las 00:00. Se avisa
              por la misma razón que en la reserva: alguien que puso 01:00 sin
              querer tiene que poder verlo antes de cancelar clases ajenas. */}
          {cruzaMedianoche(horaInicio, horaFin) && (
            <p className="text-muted-foreground text-sm">
              Este bloqueo termina al día siguiente, a las {horaFin}.
            </p>
          )}

          <div className="grid gap-2">
            <Label htmlFor="motivo">¿Por qué se bloquean?</Label>
            <Input
              id="motivo"
              value={motivo}
              onChange={(e) => setMotivo(e.target.value)}
              placeholder="Ej.: evaluación Aprender, jornada docente, obra en el aula"
            />
            {/* Va tal cual: al calendario, a "Mis reservas" y al aviso de
                cancelación. No se lo envuelve en ninguna categoría, porque el
                sistema no sabe de qué clase de cosa se trata. */}
            <p className="text-muted-foreground text-sm">
              Queda guardado en el bloqueo y es lo que van a leer los docentes en el aviso
              de cancelación. Aparece también en el calendario del equipo, así que sirve
              aunque no se cancele ninguna reserva.
            </p>
          </div>
        </CardContent>
      </Card>

      <h2 className="mb-2 text-xl font-semibold">Qué equipos se bloquean</h2>

      {!franjaCompleta && (
        <p className="text-muted-foreground mb-2 text-sm">
          Completá la fecha y el horario para ver cuáles ya tienen reserva.
        </p>
      )}
      {inventarioCargando && <p className="text-muted-foreground">Cargando equipos…</p>}
      {!inventarioCargando && todasLasEquipos.length === 0 && (
        <p className="text-muted-foreground">
          No hay ningún equipo cargado en el inventario.
        </p>
      )}

      <div className="grid gap-4">
        {inventario.map(({ carro, equipos }) => {
          const activas = equipos.filter((equipo) => !equipo.dadoDeBaja)
          if (activas.length === 0) return null

          return (
            <fieldset key={carro.id} className="grid gap-2">
              <legend className="mb-1 text-sm font-medium">{carro.nombre}</legend>
              <div className="grid gap-2 sm:grid-cols-2">
                {activas.map((equipo) => {
                  const id = `equipo-${equipo.id}`
                  const bloqueable = esBloqueable(equipo)
                  const ocupada = estaOcupada(equipo)

                  return (
                    <div
                      key={equipo.id}
                      className="flex items-start gap-2 rounded-md border p-2"
                    >
                      {/* Las que no están DISPONIBLE no se ofrecen: el
                          backend rechaza el bloqueo ENTERO si viene una
                          sola así (ErrEquipoNoDisponible), no la saltea. */}
                      <Checkbox
                        id={id}
                        disabled={!bloqueable}
                        checked={seleccionadas.includes(equipo.id)}
                        onCheckedChange={(v) => alternar(equipo.id, v === true)}
                      />
                      <div className="grid gap-0.5">
                        <Label htmlFor={id} className="cursor-pointer">
                          {equipo.etiqueta}
                        </Label>
                        {!bloqueable && (
                          <Badge variant="destructive" className="w-fit">
                            {ETIQUETA_ESTADO[equipo.estado] ?? equipo.estado}
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
            {seleccionadas.length} equipo{seleccionadas.length === 1 ? "" : "s"}{" "}
            seleccionado
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
              Se van a bloquear {seleccionadas.length} equipo
              {seleccionadas.length === 1 ? "" : "s"} el {fecha} de {horaInicio} a{" "}
              {horaFin}.
            </p>
            {elegidasOcupadas.length > 0 ? (
              <p className="text-destructive text-sm">
                {elegidasOcupadas.length} de esos equipos tienen una reserva en esa
                franja: se va a cancelar y el docente va a recibir un aviso. Las reservas
                canceladas no se recuperan aunque después se saque el bloqueo.
              </p>
            ) : (
              <p className="text-muted-foreground text-sm">
                Ninguna de los equipos elegidas tiene reservas en esa franja.
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
