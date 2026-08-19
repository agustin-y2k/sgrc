import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { ChevronDown, ChevronRight } from "lucide-react"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { SelectorDeHora } from "@/components/SelectorDeHora"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { JORNADA_KEY } from "@/features/disponibilidad/api"
import { DIAS_SEMANA, etiquetaDia } from "@/features/disponibilidad/types"
import type { BloqueHorario, DiaSemana } from "@/features/disponibilidad/types"
import {
  agruparTramos,
  ATAJOS_DE_DIAS,
  etiquetaDeDias,
  mismosDias,
  ordenarDias,
} from "@/features/admin/jornada"
import type { TramoAgrupado } from "@/features/admin/jornada"
import { getErrorMessage } from "@/lib/api-client"

/** La jornada de la institución: qué días y en qué horas abre la escuela. */

/** Los campos de un tramo, compartidos por el alta y la edición. */
type FormTramo = { dias: DiaSemana[]; horaInicio: string; horaFin: string }

const TRAMO_VACIO: FormTramo = { dias: [], horaInicio: "08:00", horaFin: "12:00" }

/**
 * Sin días marcados no se arranca: el formulario no adivina "lunes a viernes"
 * porque es exactamente la suposición que esta pantalla vino a sacar del
 * código.
 */
function SelectorDeDias({
  valor,
  onCambio,
}: {
  valor: DiaSemana[]
  onCambio: (dias: DiaSemana[]) => void
}) {
  const alternar = (dia: DiaSemana) =>
    onCambio(
      valor.includes(dia) ? valor.filter((d) => d !== dia) : ordenarDias([...valor, dia])
    )

  return (
    <div className="grid gap-2">
      <span className="text-sm font-medium">Días</span>

      {/* Botones con aria-pressed y no checkboxes: son siete y se marcan y
          desmarcan seguido, así que el objetivo táctil tiene que ser toda la
          etiqueta y no un cuadradito de 16px. */}
      <div className="flex flex-wrap gap-1.5" role="group" aria-label="Días">
        {DIAS_SEMANA.map((d) => {
          const marcado = valor.includes(d.valor)
          return (
            <Button
              key={d.valor}
              type="button"
              size="sm"
              variant={marcado ? "default" : "outline"}
              aria-pressed={marcado}
              onClick={() => alternar(d.valor)}
            >
              {d.etiqueta}
            </Button>
          )
        })}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <span className="text-muted-foreground text-xs">Atajos:</span>
        {ATAJOS_DE_DIAS.map((atajo) => (
          <Button
            key={atajo.etiqueta}
            type="button"
            size="sm"
            variant="ghost"
            className="h-auto px-2 py-1 text-xs"
            aria-pressed={mismosDias(valor, atajo.dias)}
            onClick={() => onCambio([...atajo.dias])}
          >
            {atajo.etiqueta}
          </Button>
        ))}
        {valor.length > 0 && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="text-muted-foreground h-auto px-2 py-1 text-xs"
            onClick={() => onCambio([])}
          >
            Limpiar
          </Button>
        )}
      </div>
    </div>
  )
}

function CamposDeTramo({
  valor,
  onCambio,
  idPrefijo,
}: {
  valor: FormTramo
  onCambio: (v: FormTramo) => void
  idPrefijo: string
}) {
  return (
    <div className="grid gap-4">
      <SelectorDeDias
        valor={valor.dias}
        onCambio={(dias) => onCambio({ ...valor, dias })}
      />

      <div className="flex flex-wrap items-end gap-4">
        <SelectorDeHora
          id={`${idPrefijo}-inicio`}
          etiqueta="Abre"
          valor={valor.horaInicio}
          onCambio={(v) => onCambio({ ...valor, horaInicio: v })}
        />
        <SelectorDeHora
          id={`${idPrefijo}-fin`}
          etiqueta="Cierra"
          valor={valor.horaFin}
          onCambio={(v) => onCambio({ ...valor, horaFin: v })}
        />
        {cruzaLaMedianoche(valor) && (
          <p className="text-muted-foreground pb-1.5 text-sm">Cierra al día siguiente.</p>
        )}
      </div>
    </div>
  )
}

function cruzaLaMedianoche(v: FormTramo): boolean {
  return v.horaInicio !== "" && v.horaFin !== "" && v.horaFin < v.horaInicio
}

/** Por qué no se puede guardar ese horario todavía, o "" si sí se puede. */
function motivoDeHoras(horaInicio: string, horaFin: string): string {
  if (horaInicio === "" || horaFin === "") return "Falta completar la hora."
  if (horaFin === horaInicio) {
    return "La hora de cierre no puede ser igual a la de apertura."
  }
  return ""
}

const SIN_DIAS = "Elegí al menos un día para este tramo."

/** Lo mismo para un tramo entero, que además necesita días. */
function motivoParaNoGuardar(v: FormTramo): string {
  const horas = motivoDeHoras(v.horaInicio, v.horaFin)
  if (horas !== "") return horas
  if (v.dias.length === 0) return SIN_DIAS
  return ""
}

/** Un día suelto de un tramo, con su propio horario editable. */
function FilaDeDia({
  bloque,
  deshabilitado,
  onGuardar,
  onQuitar,
}: {
  bloque: BloqueHorario
  deshabilitado: boolean
  onGuardar: (horaInicio: string, horaFin: string) => void
  onQuitar: () => void
}) {
  const [editando, setEditando] = useState(false)
  const [horaInicio, setHoraInicio] = useState(bloque.horaInicio)
  const [horaFin, setHoraFin] = useState(bloque.horaFin)

  const dia = etiquetaDia(bloque.diaSemana)
  const motivo = motivoDeHoras(horaInicio, horaFin)

  function empezar() {
    setHoraInicio(bloque.horaInicio)
    setHoraFin(bloque.horaFin)
    setEditando(true)
  }

  if (!editando) {
    return (
      <li className="flex flex-wrap items-center justify-between gap-2 py-1 text-sm">
        <span>
          <span className="font-medium">{dia}</span>{" "}
          <span className="text-muted-foreground">
            de {bloque.horaInicio} a {bloque.horaFin}
          </span>
        </span>
        <span className="flex gap-1">
          <Button
            variant="ghost"
            size="sm"
            disabled={deshabilitado}
            onClick={empezar}
            aria-label={`Editar solo ${dia}`}
          >
            Editar
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={deshabilitado}
            onClick={onQuitar}
            aria-label={`Quitar solo ${dia}`}
          >
            Quitar
          </Button>
        </span>
      </li>
    )
  }

  return (
    <li className="grid gap-3 py-2">
      <p className="text-sm font-medium">{dia}</p>
      <div className="flex flex-wrap items-end gap-4">
        <SelectorDeHora
          id={`dia-${bloque.id}-inicio`}
          etiqueta="Abre"
          valor={horaInicio}
          onCambio={setHoraInicio}
        />
        <SelectorDeHora
          id={`dia-${bloque.id}-fin`}
          etiqueta="Cierra"
          valor={horaFin}
          onCambio={setHoraFin}
        />
        {horaInicio !== "" && horaFin !== "" && horaFin < horaInicio && (
          <p className="text-muted-foreground pb-1.5 text-sm">Cierra al día siguiente.</p>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Button
          size="sm"
          disabled={motivo !== "" || deshabilitado}
          onClick={() => onGuardar(horaInicio, horaFin)}
        >
          Guardar {dia.toLocaleLowerCase("es")}
        </Button>
        <Button variant="outline" size="sm" onClick={() => setEditando(false)}>
          Cancelar
        </Button>
        {motivo !== "" && <p className="text-muted-foreground text-sm">{motivo}</p>}
      </div>
    </li>
  )
}

/** Un paso del guardado, atado al día que lo motivó. */
type Paso = { dia: DiaSemana; ejecutar: () => Promise<unknown> }

async function ejecutarEnOrden(pasos: Paso[]): Promise<string[]> {
  const fallos: string[] = []
  for (const paso of pasos) {
    try {
      await paso.ejecutar()
    } catch (e) {
      fallos.push(`${etiquetaDia(paso.dia)}: ${getErrorMessage(e)}`)
    }
  }
  return fallos
}

/** Los pasos para dejar un tramo como dice el formulario. */
function pasosDeEdicion(grupo: TramoAgrupado, valor: FormTramo): Paso[] {
  const quitados = grupo.bloques.filter((b) => !valor.dias.includes(b.diaSemana))
  const conservados = grupo.bloques.filter((b) => valor.dias.includes(b.diaSemana))
  const agregados = valor.dias.filter((d) => !grupo.dias.includes(d))

  const horarioCambio =
    valor.horaInicio !== grupo.horaInicio || valor.horaFin !== grupo.horaFin

  return [
    ...quitados.map((b) => ({
      dia: b.diaSemana,
      ejecutar: () => disponibilidadApi.eliminarBloqueDeJornada(b.id),
    })),
    // Sin cambio de horario no hay nada que editar en los días que siguen: un
    // PATCH que manda lo mismo que ya está solo agrega un request que puede
    // fallar por un solape consigo mismo mal resuelto.
    ...(horarioCambio
      ? conservados.map((b) => ({
          dia: b.diaSemana,
          ejecutar: () =>
            disponibilidadApi.editarBloqueDeJornada(b.id, {
              horaInicio: valor.horaInicio,
              horaFin: valor.horaFin,
            }),
        }))
      : []),
    ...agregados.map((d) => ({
      dia: d,
      ejecutar: () =>
        disponibilidadApi.agregarBloqueDeJornada(d, valor.horaInicio, valor.horaFin),
    })),
  ]
}

export function JornadaPage() {
  const queryClient = useQueryClient()
  const [nuevo, setNuevo] = useState<FormTramo>(TRAMO_VACIO)
  const [editando, setEditando] = useState<string | null>(null)
  const [edicion, setEdicion] = useState<FormTramo>(TRAMO_VACIO)
  const [desplegados, setDesplegados] = useState<string[]>([])
  const [fallos, setFallos] = useState<string[]>([])

  const { data, isPending, error } = useQuery({
    queryKey: JORNADA_KEY,
    queryFn: disponibilidadApi.jornadaDeLaInstitucion,
  })

  const invalidar = () => queryClient.invalidateQueries({ queryKey: JORNADA_KEY })

  const agregar = useMutation({
    mutationFn: (v: FormTramo) =>
      ejecutarEnOrden(
        v.dias.map((d) => ({
          dia: d,
          ejecutar: () =>
            disponibilidadApi.agregarBloqueDeJornada(d, v.horaInicio, v.horaFin),
        }))
      ),
    // Siempre se invalida, también con fallos parciales: los días que sí
    // entraron ya están guardados y la lista tiene que mostrarlos.
    onSuccess: async (fallidos, v) => {
      setFallos(fallidos)
      setNuevo({ ...v, dias: v.dias.filter((d) => fallidos.some(nombra(d))) })
      await invalidar()
    },
  })

  const guardarEdicion = useMutation({
    mutationFn: ({ grupo, valor }: { grupo: TramoAgrupado; valor: FormTramo }) =>
      ejecutarEnOrden(pasosDeEdicion(grupo, valor)),
    onSuccess: async (fallidos) => {
      setFallos(fallidos)
      if (fallidos.length === 0) setEditando(null)
      await invalidar()
    },
  })

  const quitar = useMutation({
    mutationFn: (bloques: BloqueHorario[]) =>
      ejecutarEnOrden(
        bloques.map((b) => ({
          dia: b.diaSemana,
          ejecutar: () => disponibilidadApi.eliminarBloqueDeJornada(b.id),
        }))
      ),
    onSuccess: async (fallidos) => {
      setFallos(fallidos)
      await invalidar()
    },
  })

  // Un día suelto que se desprende del grupo: es un PATCH sobre el bloque que
  // ya existe para ese día, así que el resto del tramo no se entera.
  const guardarDia = useMutation({
    mutationFn: ({
      bloque,
      horaInicio,
      horaFin,
    }: {
      bloque: BloqueHorario
      horaInicio: string
      horaFin: string
    }) =>
      ejecutarEnOrden([
        {
          dia: bloque.diaSemana,
          ejecutar: () =>
            disponibilidadApi.editarBloqueDeJornada(bloque.id, { horaInicio, horaFin }),
        },
      ]),
    onSuccess: async (fallidos) => {
      setFallos(fallidos)
      await invalidar()
    },
  })

  const tramos = agruparTramos(data?.data ?? [])
  const motivoNuevo = motivoParaNoGuardar(nuevo)
  const motivoEdicion = motivoParaNoGuardar(edicion)
  const trabajando =
    agregar.isPending ||
    guardarEdicion.isPending ||
    quitar.isPending ||
    guardarDia.isPending

  function empezarAEditar(grupo: TramoAgrupado) {
    setFallos([])
    setEditando(claveDe(grupo))
    setEdicion({
      dias: grupo.dias,
      horaInicio: grupo.horaInicio,
      horaFin: grupo.horaFin,
    })
  }

  function alternarDespliegue(clave: string) {
    setDesplegados((abiertos) =>
      abiertos.includes(clave)
        ? abiertos.filter((c) => c !== clave)
        : [...abiertos, clave]
    )
  }

  return (
    <div className="mx-auto max-w-3xl">
      <EncabezadoDePagina
        titulo="Jornada de la escuela"
        descripcion="Los días y horas en que la escuela está abierta. Las reservas fuera de este horario se rechazan."
      />

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {/* Los fallos se listan por día porque el guardado es parcial: puede
          haber entrado el lunes y haberse rechazado el martes, y sin el
          detalle no hay forma de saber cuál hay que rehacer. */}
      {fallos.length > 0 && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            <p>Estos días quedaron sin guardar:</p>
            <ul className="mt-1 list-disc pl-5">
              {fallos.map((f) => (
                <li key={f}>{f}</li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      <Card className="mb-6">
        <CardContent className="grid gap-4 pt-6">
          <p className="font-medium">Agregar un tramo</p>
          <CamposDeTramo valor={nuevo} onCambio={setNuevo} idPrefijo="nuevo" />
          <div className="flex flex-wrap items-center gap-3">
            <Button
              disabled={motivoNuevo !== "" || trabajando}
              onClick={() => {
                setFallos([])
                agregar.mutate(nuevo)
              }}
            >
              Agregar tramo
            </Button>
            {motivoNuevo !== "" && (
              <p className="text-muted-foreground text-sm">{motivoNuevo}</p>
            )}
          </div>
        </CardContent>
      </Card>

      {isPending ? (
        <p className="text-muted-foreground text-sm">Cargando…</p>
      ) : tramos.length === 0 ? (
        // El texto importa: sin jornada cargada el sistema NO bloquea nada, y
        // quien mira esta pantalla vacía tiene que entender que eso es una
        // decisión y no una configuración a medias.
        <Alert>
          <AlertDescription>
            Todavía no se declaró la jornada, así que se puede reservar cualquier día y a
            cualquier hora. Cargá los tramos en que la escuela abre para que el sistema
            empiece a rechazar lo que queda afuera.
          </AlertDescription>
        </Alert>
      ) : (
        <ul className="grid gap-2">
          {tramos.map((t) => {
            const clave = claveDe(t)
            const nombre = `${etiquetaDeDias(t.dias)} de ${t.horaInicio} a ${t.horaFin}`
            const desplegado = desplegados.includes(clave)

            if (editando === clave) {
              return (
                <li key={clave}>
                  <Card>
                    <CardContent className="grid gap-4 pt-6">
                      <CamposDeTramo
                        valor={edicion}
                        onCambio={setEdicion}
                        idPrefijo={`editar-${clave}`}
                      />
                      {/* Dicho acá y no en un cartel aparte: es el momento en
                          que alguien podría creer que está corrigiendo un día
                          y estar corrigiendo cinco. */}
                      {t.dias.length > 1 && (
                        <p className="text-muted-foreground text-sm">
                          La hora que pongas vale para todos los días del tramo. Para que
                          uno solo difiera, cerrá esto y usá “Día por día”.
                        </p>
                      )}
                      <div className="flex flex-wrap items-center gap-3">
                        <Button
                          size="sm"
                          disabled={motivoEdicion !== "" || trabajando}
                          onClick={() => {
                            setFallos([])
                            guardarEdicion.mutate({ grupo: t, valor: edicion })
                          }}
                        >
                          Guardar
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setEditando(null)}
                        >
                          Cancelar
                        </Button>
                        {motivoEdicion !== "" && (
                          <p className="text-muted-foreground text-sm">
                            {motivoEdicion === SIN_DIAS
                              ? "Elegí al menos un día, o quitá el tramo entero."
                              : motivoEdicion}
                          </p>
                        )}
                      </div>
                    </CardContent>
                  </Card>
                </li>
              )
            }

            return (
              <li key={clave} className="rounded-md border px-4 py-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span>
                    <span className="font-medium">{etiquetaDeDias(t.dias)}</span>{" "}
                    <span className="text-muted-foreground">
                      de {t.horaInicio} a {t.horaFin}
                    </span>
                  </span>
                  <span className="flex flex-wrap gap-1">
                    {/* Con un solo día el grupo YA es el día: desplegarlo
                        mostraría dos veces la misma línea. */}
                    {t.bloques.length > 1 && (
                      <Button
                        variant="ghost"
                        size="sm"
                        aria-expanded={desplegado}
                        onClick={() => alternarDespliegue(clave)}
                        aria-label={`Día por día: ${nombre}`}
                      >
                        {desplegado ? (
                          <ChevronDown className="size-3.5" />
                        ) : (
                          <ChevronRight className="size-3.5" />
                        )}
                        Día por día
                      </Button>
                    )}
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={trabajando}
                      onClick={() => empezarAEditar(t)}
                      aria-label={`Editar ${nombre}`}
                    >
                      Editar
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={trabajando}
                      onClick={() => {
                        setFallos([])
                        quitar.mutate(t.bloques)
                      }}
                      aria-label={`Quitar ${nombre}`}
                    >
                      Quitar
                    </Button>
                  </span>
                </div>

                {desplegado && (
                  <ul className="mt-2 border-t pt-2">
                    {t.bloques.map((b) => (
                      <FilaDeDia
                        key={b.id}
                        bloque={b}
                        deshabilitado={trabajando}
                        onGuardar={(horaInicio, horaFin) => {
                          setFallos([])
                          guardarDia.mutate({ bloque: b, horaInicio, horaFin })
                        }}
                        onQuitar={() => {
                          setFallos([])
                          quitar.mutate([b])
                        }}
                      />
                    ))}
                  </ul>
                )}
              </li>
            )
          })}
        </ul>
      )}

      <p className="text-muted-foreground mt-6 text-sm">
        Una escuela nocturna declara, por ejemplo, 20:00–01:00: si la hora de cierre es
        menor que la de apertura, el tramo termina al día siguiente. Además, se pueden
        cargar varios tramos para el mismo día: una escuela con turno mañana y turno noche
        declara, por ejemplo, 07:00–12:00 y 18:00–23:00, y el mediodía queda cerrado. Los
        días sin ningún tramo son días en que la escuela no abre.
      </p>
    </div>
  )
}

/** Identifica al tramo en la pantalla. */
function claveDe(t: TramoAgrupado): string {
  return `${t.horaInicio}-${t.horaFin}`
}

/** Si el mensaje de fallo habla de ese día, para dejarlo marcado y reintentar. */
function nombra(dia: DiaSemana): (fallo: string) => boolean {
  return (fallo) => fallo.startsWith(`${etiquetaDia(dia)}:`)
}
