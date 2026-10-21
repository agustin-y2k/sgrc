import { useState } from "react"
import { useMutation, useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import * as reservasApi from "@/features/reservas/api"
import type {
  EquipoDisponible,
  EquipoOcupado,
  TramoPreferencia,
} from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"
import { contar, plural } from "@/lib/plural"
import { MINIMO_PARA_BUSCAR, sinTildes } from "@/lib/texto"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"

/** Título del grupo de lo que no cuelga de ningún carro. */
const SIN_CARRO = "Otros equipos"

type Props = {
  fecha: string
  horaInicio: string
  horaFin: string
  seleccionadas: string[]
  /**
   * `aTomar` es el subconjunto de `equipoIds` que estaba OCUPADO: lo que el
   * backend necesita para autorizar el desplazamiento (RF-03.21). Va por acá y
   * no lo deriva el padre porque quién está ocupado lo sabe esta pantalla, que
   * es la que tiene la lista.
   */
  onCambio: (equipoIds: string[], aTomar: string[]) => void
  /** RF-03.21: para qué materia se ordena la lista. */
  materiaId?: string
}

/**
 * RF-04.2: "selecciona equipos de una lista (como tildar casillas) hasta
 * juntar la cantidad que necesita — la lista no está restringida a un solo
 * carro".
 */
export function SelectorDeEquipos({
  fecha,
  horaInicio,
  horaFin,
  seleccionadas,
  onCambio,
  materiaId,
}: Props) {
  const [busqueda, setBusqueda] = useState("")
  const franjaCompleta = Boolean(fecha && horaInicio && horaFin && horaFin !== horaInicio)

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    // La materia entra en la clave: cambiarla reordena la lista entera
    // (RF-03.21), así que la respuesta cacheada de otra materia no sirve.
    queryKey: ["equipos-disponibles", fecha, horaInicio, horaFin, materiaId ?? ""],
    queryFn: () =>
      reservasApi.equiposDisponibles({ fecha, horaInicio, horaFin, materiaId }),
    enabled: franjaCompleta,
  })

  if (!franjaCompleta) {
    return (
      <p className="text-muted-foreground text-sm">
        Elegí la fecha y el horario para ver qué computadoras están libres.
      </p>
    )
  }
  if (isLoading)
    return <p className="text-muted-foreground text-sm">Buscando equipos libres…</p>
  if (error) {
    return (
      <ErrorDeConsulta
        mensaje="No pudimos consultar la disponibilidad de los equipos."
        reintentando={isFetching}
        onReintentar={() => {
          void refetch()
        }}
      />
    )
  }

  const todos = data?.data ?? []
  const ocupados = data?.ocupados ?? []
  // Los dos lados de la lista de ocupados, que son dos acciones distintas:
  // los que esta materia puede quedarse tildándolos (RF-03.21) y los que sólo
  // se le pueden pedir al dueño (RF-04.12). El servidor ya garantiza que son
  // excluyentes.
  const tomables = ocupados.filter((eq) => eq.puedeTomarse)
  const idsTomables = new Set(tomables.map((eq) => eq.equipoId))
  const soloPedibles = ocupados.filter((eq) => !eq.puedeTomarse)

  // El texto busca en el nombre, el carro y el software: en el mostrador se
  // busca "PC 12", "EDUTEC" o "AutoCAD" según lo que se tenga en la cabeza.
  const filtro = sinTildes(busqueda.trim())
  const equipos = filtro
    ? todos.filter((eq) =>
        sinTildes(
          `${eq.etiqueta} ${eq.carroNombre ?? ""} ${eq.softwareInstalado ?? ""}`
        ).includes(filtro)
      )
    : todos

  if (todos.length === 0 && ocupados.length === 0) {
    return (
      <div className="grid gap-2">
        <p className="text-muted-foreground text-sm">
          No hay ningún equipo libre en esa franja.
        </p>
        {seleccionadas.length > 0 && (
          <Button
            type="button"
            variant="outline"
            className="h-11 justify-self-start"
            onClick={() => onCambio([], [])}
          >
            Limpiar la selección
          </Button>
        )}
      </div>
    )
  }

  /** Un solo lugar que parte la selección en "libres" y "hay que tomarlos". */
  function avisar(equipoIds: string[]) {
    const unicos = [...new Set(equipoIds)]
    onCambio(
      unicos,
      unicos.filter((id) => idsTomables.has(id))
    )
  }

  function alternar(equipoId: string, tildada: boolean) {
    avisar(
      tildada
        ? [...seleccionadas, equipoId]
        : seleccionadas.filter((id) => id !== equipoId)
    )
  }

  /**
   * Marcar o desmarcar un carro entero. Una clase de treinta se arma con las
   * treinta de un carro, y tildarlas de a una son treinta clics.
   *
   * Opera sobre los que se están VIENDO: con una búsqueda puesta, marca lo que
   * quedó a la vista y no lo que el filtro escondió, que es lo que espera
   * quien acaba de escribir "EDUTEC".
   */
  function alternarCarro(delCarro: EquipoDisponible[], marcar: boolean) {
    const ids = new Set(delCarro.map((eq) => eq.equipoId))
    avisar(
      marcar
        ? [
            ...seleccionadas,
            ...delCarro
              .map((eq) => eq.equipoId)
              .filter((id) => !seleccionadas.includes(id)),
          ]
        : seleccionadas.filter((id) => !ids.has(id))
    )
  }

  // La lista ya viene ordenada por tramo del servidor; acá sólo se corta en
  // bloques respetando ese orden.
  const tramos = agruparPorTramo(equipos)
  const seleccionUnica = new Set(seleccionadas)
  const libresSeleccionados = todos.filter((e) => seleccionUnica.has(e.equipoId)).length
  const porPrioridad = [...idsTomables].filter((id) => seleccionUnica.has(id)).length
  const fueraDeLista = [...seleccionUnica].filter(
    (id) => !todos.some((e) => e.equipoId === id) && !idsTomables.has(id)
  ).length

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <p role="status" className="text-muted-foreground text-sm">
          {seleccionadas.length === 0
            ? `${contar(todos.length, "equipo")} ${plural(todos.length, "libre")} en esa franja.`
            : `${contar(seleccionUnica.size, "equipo")} ${plural(seleccionUnica.size, "seleccionado")}: ${libresSeleccionados} ${plural(libresSeleccionados, "libre")} y ${porPrioridad} por prioridad.${fueraDeLista ? ` ${contar(fueraDeLista, "equipo")} ya no ${plural(fueraDeLista, "está", "están")} disponible para esta selección. Limpiá la selección y volvé a elegir.` : ""}`}
        </p>
        {seleccionadas.length > 0 && (
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground text-xs underline"
            onClick={() => avisar([])}
          >
            Limpiar la selección ({seleccionadas.length})
          </button>
        )}
      </div>

      {/* El buscador aparece recién cuando la lista es larga. Con tres
          carros cargados son más de ochenta casillas, y encontrar "la 12 del
          Carro 2" scrolleando es justo lo que no se puede hacer con alguien
          esperando. */}
      {todos.length >= MINIMO_PARA_BUSCAR && (
        <Input
          value={busqueda}
          onChange={(e) => setBusqueda(e.target.value)}
          aria-label="Buscar una computadora"
          placeholder="Buscar — ej.: PC 12, Carro 2, AutoCAD"
        />
      )}

      {todos.length > 0 && equipos.length === 0 && (
        <p className="text-muted-foreground text-sm">
          Ninguna computadora coincide con «{busqueda.trim()}».
        </p>
      )}

      {todos.length === 0 && (
        <p className="text-sm">
          No queda ninguno libre en esa franja, pero abajo está quién tiene cada uno.
        </p>
      )}

      {/* Con un solo tramo no hay nada que distinguir, así que la lista sale
          como siempre. Es el caso normal mientras el inventario no tenga
          ninguna marca cargada, y ponerle un título a un único bloque sería
          agregar vocabulario nuevo sin decir nada. */}
      {tramos.length <= 1 ? (
        <EquiposPorCarro
          equipos={equipos}
          seleccionadas={seleccionadas}
          alternar={alternar}
          alternarCarro={alternarCarro}
        />
      ) : (
        tramos.map(([tramo, delTramo]) => (
          <section key={tramo} className="grid gap-2">
            <div>
              <h3 className="text-sm font-medium">{TITULO_TRAMO[tramo]}</h3>
              {AYUDA_TRAMO[tramo] && (
                <p className="text-muted-foreground text-xs">{AYUDA_TRAMO[tramo]}</p>
              )}
            </div>
            <EquiposPorCarro
              equipos={delTramo}
              seleccionadas={seleccionadas}
              alternar={alternar}
              alternarCarro={alternarCarro}
            />
          </section>
        ))
      )}

      {tomables.length > 0 && (
        <EquiposQueSePuedenTomar
          equipos={tomables}
          seleccionadas={seleccionadas}
          alternar={alternar}
        />
      )}
      {soloPedibles.length > 0 && <EquiposTomados ocupados={soloPedibles} />}
    </div>
  )
}

/**
 * RF-03.21: equipos que están reservados por otra materia, pero cuya marca
 * preferente es de la materia que está reservando. Se tildan como cualquier
 * otro y al confirmar desplazan al que los tenía.
 *
 * Se muestran separados de los libres para explicar el desplazamiento y la
 * reposición antes de seleccionarlos.
 */
function EquiposQueSePuedenTomar({
  equipos,
  seleccionadas,
  alternar,
}: {
  equipos: EquipoOcupado[]
  seleccionadas: string[]
  alternar: (equipoId: string, tildada: boolean) => void
}) {
  const porEquipo = new Map<string, EquipoOcupado[]>()
  for (const equipo of equipos) {
    const reservas = porEquipo.get(equipo.equipoId) ?? []
    reservas.push(equipo)
    porEquipo.set(equipo.equipoId, reservas)
  }
  return (
    <div className="grid gap-2 border-t pt-4">
      <h3 className="text-sm font-medium">Reservados, pero preferentes de tu materia</h3>
      <p className="text-muted-foreground text-sm">
        Tu materia tiene mayor prioridad sobre estos dispositivos. Los que tildes pasan a
        tu reserva al confirmar. El sistema intentará reponerlos en las reservas
        desplazadas: primero con equipos libres y después, si corresponde, desplazando
        otras reservas con menor prioridad sobre cada sustituto. Si no alcanza, pueden
        quedar faltantes. Se avisa a los docentes afectados.
      </p>

      <div className="grid gap-2 sm:grid-cols-2">
        {[...porEquipo.values()].map((reservas) => {
          const equipo = reservas[0]
          return (
            <label
              key={equipo.equipoId}
              className="flex items-start gap-2 rounded-md border border-dashed p-2"
            >
              <Checkbox
                className="mt-0.5"
                checked={seleccionadas.includes(equipo.equipoId)}
                onCheckedChange={(v) => alternar(equipo.equipoId, v === true)}
                // Explícito y corto, como en la lista de libres: sin esto el
                // nombre accesible se lleva todo el renglón —de quién es la
                // reserva y en qué horario— y "PC 12" hay uno por carro.
                aria-label={
                  equipo.carroNombre
                    ? `${equipo.etiqueta} (${equipo.carroNombre})`
                    : equipo.etiqueta
                }
              />
              <span className="grid gap-0.5">
                <span className="text-sm font-medium">
                  {equipo.etiqueta}
                  {equipo.carroNombre && (
                    <span className="text-muted-foreground font-normal">
                      {" "}
                      · {equipo.carroNombre}
                    </span>
                  )}
                </span>
                {/* Quién pierde la clase y cuál: es el dato que hace que la
                  decisión sea informada, y el único que el desplazado no
                  puede negociar después. "Es de" y no "la tiene" porque el
                  equipo puede ser un proyector. */}
                {reservas.map((reserva) => (
                  <span
                    key={reserva.reservaId}
                    className="text-muted-foreground grid gap-0.5 text-xs"
                  >
                    <span>
                      Ahora es de {reserva.docenteNombre}
                      {reserva.materiaNombre ? ` · ${reserva.materiaNombre}` : ""}
                    </span>
                    <span>
                      {reserva.horaInicio} a {reserva.horaFin}
                    </span>
                  </span>
                ))}
              </span>
            </label>
          )
        })}
      </div>
    </div>
  )
}

const TITULO_TRAMO: Record<TramoPreferencia, string> = {
  PREFERENTE: "Recomendados para esta materia",
  NEUTRAL: "Los demás equipos",
  DE_OTRA_MATERIA: "Preferentes de otra materia",
}

/**
 * La aclaración del último tramo no es decorativa: sin ella, "preferentes de
 * otra materia" se lee como una prohibición, y no lo es.
 */
const AYUDA_TRAMO: Record<TramoPreferencia, string> = {
  PREFERENTE: "",
  NEUTRAL: "",
  DE_OTRA_MATERIA:
    "Se pueden reservar igual mientras estén libres. Otra materia con mayor prioridad sobre ese dispositivo podría desplazarte; el sistema intentará reponerlo.",
}

/** El orden en que se muestran los bloques. */
const ORDEN_TRAMOS: TramoPreferencia[] = ["PREFERENTE", "NEUTRAL", "DE_OTRA_MATERIA"]

function agruparPorTramo(
  equipos: EquipoDisponible[]
): [TramoPreferencia, EquipoDisponible[]][] {
  const porTramo = new Map<TramoPreferencia, EquipoDisponible[]>()
  for (const equipo of equipos) {
    const delTramo = porTramo.get(equipo.tramo)
    if (delTramo) delTramo.push(equipo)
    else porTramo.set(equipo.tramo, [equipo])
  }
  return ORDEN_TRAMOS.filter((t) => porTramo.has(t)).map((t) => [t, porTramo.get(t)!])
}

/**
 * Agrupadas por carro solo para que sea legible; la selección cruza carros.
 */
function EquiposPorCarro({
  equipos,
  seleccionadas,
  alternar,
  alternarCarro,
}: {
  equipos: EquipoDisponible[]
  seleccionadas: string[]
  alternar: (equipoId: string, tildada: boolean) => void
  alternarCarro: (delCarro: EquipoDisponible[], marcar: boolean) => void
}) {
  const porCarro = new Map<string, EquipoDisponible[]>()
  for (const equipo of equipos) {
    const grupo = equipo.carroNombre || SIN_CARRO
    const delCarro = porCarro.get(grupo)
    if (delCarro) delCarro.push(equipo)
    else porCarro.set(grupo, [equipo])
  }

  return (
    <>
      {[...porCarro.entries()].map(([carro, delCarro]) => (
        <fieldset key={carro} className="grid gap-2">
          <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
            <legend className="text-sm font-medium">{carro}</legend>
            {delCarro.length > 1 && (
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground text-xs underline"
                onClick={() =>
                  alternarCarro(
                    delCarro,
                    !delCarro.every((eq) => seleccionadas.includes(eq.equipoId))
                  )
                }
              >
                {delCarro.every((eq) => seleccionadas.includes(eq.equipoId))
                  ? "Desmarcar"
                  : "Marcar"}{" "}
                {contar(delCarro.length, "equipo")}
              </button>
            )}
          </div>
          <div className="grid gap-2 sm:grid-cols-2">
            {delCarro.map((equipo) => {
              const id = `equipo-${equipo.equipoId}`
              return (
                <div
                  key={equipo.equipoId}
                  className="flex items-start gap-2 rounded-md border p-2"
                >
                  <Checkbox
                    id={id}
                    checked={seleccionadas.includes(equipo.equipoId)}
                    onCheckedChange={(v) => alternar(equipo.equipoId, v === true)}
                    // Quien navega con lector de pantalla no tiene el título
                    // del carro a la vista, y "PC 12" hay uno por carro.
                    aria-label={`${equipo.etiqueta} (${carro})`}
                  />
                  <div className="grid gap-0.5">
                    <Label htmlFor={id} className="cursor-pointer">
                      {equipo.etiqueta}
                      {equipo.freezado && (
                        <Badge variant="outline" className="ml-1">
                          Freezada
                        </Badge>
                      )}
                    </Label>
                    {/* Por qué está en este bloque, en palabras. Sin el
                        motivo, el orden es un misterio: se ve que la lista
                        cambió pero no qué la ordenó. */}
                    {equipo.motivo && (
                      <span className="text-muted-foreground text-xs">
                        {equipo.motivo}
                      </span>
                    )}
                    {/* RF-03.7: el software es lo que define la elección. */}
                    {equipo.softwareInstalado && (
                      <span className="text-muted-foreground text-xs">
                        {equipo.softwareInstalado}
                      </span>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        </fieldset>
      ))}
    </>
  )
}

/** RF-04.11 + RF-04.12: los equipos que ya tiene alguien en esa franja. */
function EquiposTomados({ ocupados }: { ocupados: EquipoOcupado[] }) {
  return (
    <div className="grid gap-2 border-t pt-4">
      <h3 className="text-sm font-medium">Ya reservados en esa franja</h3>
      <p className="text-muted-foreground text-sm">
        No se pueden tildar. Si necesitás alguno, podés pedírselo a quien lo tiene: le
        llega un aviso en el sistema y decide él. El pedido no cambia su reserva.
      </p>

      <div className="grid gap-2 sm:grid-cols-2">
        {ocupados.map((equipo) => (
          <EquipoTomado key={`${equipo.equipoId}:${equipo.reservaId}`} equipo={equipo} />
        ))}
      </div>
    </div>
  )
}

function EquipoTomado({ equipo }: { equipo: EquipoOcupado }) {
  const [pidiendo, setPidiendo] = useState(false)
  const [mensaje, setMensaje] = useState("")

  const pedir = useMutation({
    mutationFn: () => reservasApi.pedirLiberacion(equipo.reservaId!, mensaje),
  })

  return (
    <div className="grid gap-2 rounded-md border border-dashed p-2">
      <div className="grid gap-0.5">
        <span className="text-sm font-medium">
          {equipo.etiqueta}
          {equipo.carroNombre && (
            <span className="text-muted-foreground font-normal">
              {" "}
              · {equipo.carroNombre}
            </span>
          )}
        </span>
        {/* Un bloqueo administrativo no tiene docente: lo que explica la
            franja es el motivo que escribió el Admin. */}
        <span className="text-muted-foreground text-xs">
          {equipo.docenteNombre
            ? `${equipo.docenteNombre}${equipo.materiaNombre ? ` · ${equipo.materiaNombre}` : ""}`
            : equipo.motivo || "Tomado por administración"}
        </span>
        <span className="text-muted-foreground text-xs">
          {equipo.horaInicio} a {equipo.horaFin}
        </span>
      </div>

      {pedir.isSuccess ? (
        <p className="text-xs">
          Le avisamos. La reserva sigue siendo suya hasta que decida.
        </p>
      ) : equipo.puedePedirse ? (
        pidiendo ? (
          <div className="grid gap-2">
            {/* "lo" y no "la": el referente es el equipo, que puede ser un
                proyector o un cargador y no solo una computadora. Con el
                femenino, la pantalla le decía "Pedírsela" a un proyector —
                y a dos renglones de la línea que ya lo dice bien ("podés
                pedírselo a quien lo tiene"). */}
            <Label htmlFor={`pedido-${equipo.equipoId}`} className="text-xs">
              ¿Para qué lo necesitás? (opcional)
            </Label>
            <Input
              id={`pedido-${equipo.equipoId}`}
              value={mensaje}
              maxLength={500}
              placeholder="Tengo una evaluación a esa hora"
              onChange={(e) => setMensaje(e.target.value)}
            />
            {pedir.isError && (
              <Alert variant="destructive">
                <AlertDescription>{getErrorMessage(pedir.error)}</AlertDescription>
              </Alert>
            )}
            <div className="flex gap-2">
              <Button
                type="button"
                size="sm"
                disabled={pedir.isPending}
                onClick={() => pedir.mutate()}
              >
                {pedir.isPending ? "Enviando…" : "Enviar pedido"}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => setPidiendo(false)}
              >
                Cancelar
              </Button>
            </div>
          </div>
        ) : (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setPidiendo(true)}
          >
            Pedírselo
          </Button>
        )
      ) : null}
    </div>
  )
}
