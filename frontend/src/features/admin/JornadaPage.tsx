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
import { etiquetaDia } from "@/features/disponibilidad/types"
import { impactoDelError } from "@/features/disponibilidad/types"
import type {
  BloqueHorario,
  ImpactoDeJornada,
  TramoDeJornada,
} from "@/features/disponibilidad/types"
import { ImpactoDeLaJornada } from "@/features/admin/ImpactoDeLaJornada"
import {
  agruparTramos,
  aTramos,
  etiquetaDeDias,
  expandirDias,
  sinLosBloques,
} from "@/features/admin/jornada"
import type { TramoAgrupado } from "@/features/admin/jornada"
import {
  CamposDeTramo,
  motivoDeHoras,
  motivoParaNoGuardar,
  SIN_DIAS,
  TRAMO_VACIO,
} from "@/features/admin/CamposDeTramo"
import type { FormTramo } from "@/features/admin/CamposDeTramo"
import { getErrorMessage } from "@/lib/api-client"

/** La jornada de la institución: qué días y en qué horas abre la escuela. */

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

export function JornadaPage() {
  const queryClient = useQueryClient()
  const [nuevo, setNuevo] = useState<FormTramo>(TRAMO_VACIO)
  const [editando, setEditando] = useState<string | null>(null)
  const [edicion, setEdicion] = useState<FormTramo>(TRAMO_VACIO)
  const [desplegados, setDesplegados] = useState<string[]>([])
  const [falloAlGuardar, setFalloAlGuardar] = useState<string | null>(null)
  // La jornada que espera confirmación, junto con lo que dejaría afuera.
  const [porConfirmar, setPorConfirmar] = useState<{
    tramos: TramoDeJornada[]
    impacto: ImpactoDeJornada
  } | null>(null)
  const [canceladas, setCanceladas] = useState(0)

  const { data, isPending, error } = useQuery({
    queryKey: JORNADA_KEY,
    queryFn: disponibilidadApi.jornadaDeLaInstitucion,
  })

  const invalidar = () => queryClient.invalidateQueries({ queryKey: JORNADA_KEY })

  // Una sola mutación para las cuatro operaciones de la pantalla —agregar,
  // editar, quitar, corregir un día suelto— porque desde el backend las
  // cuatro son la misma: acá está la jornada completa, dejala así.
  //
  // Antes cada una se traducía a una serie de altas, PATCH y bajas que se
  // mandaban en orden, y de ahí salía el guardado parcial: podía entrar el
  // lunes y rechazarse el martes, dejando la jornada en un estado que nadie
  // pidió. Ahora entra entera o no entra.
  const guardar = useMutation({
    mutationFn: ({
      tramos,
      confirmado,
    }: {
      tramos: TramoDeJornada[]
      confirmado: boolean
    }) => disponibilidadApi.reemplazarJornada(tramos, confirmado),
    onSuccess: async (respuesta) => {
      setFalloAlGuardar(null)
      setPorConfirmar(null)
      setEditando(null)
      setCanceladas(respuesta.reservasCanceladas ?? 0)
      await invalidar()
    },
    // Un 409 con impacto no es un error que mostrar y olvidar: es la pregunta
    // que el backend devuelve en vez de aplicar el cambio. Se guarda la
    // jornada propuesta para poder reintentarla con la confirmación puesta.
    onError: async (e, variables) => {
      const impacto = impactoDelError(e)
      if (impacto !== null) {
        setPorConfirmar({ tramos: variables.tramos, impacto })
        setFalloAlGuardar(null)
        return
      }
      setFalloAlGuardar(getErrorMessage(e))
      // Un error puede haber dejado la jornada cambiada igual: es el caso de
      // la cascada a medias, donde el horario nuevo ya rige y lo que falló fue
      // cancelar lo que quedó afuera. Se relee para que la pantalla muestre lo
      // que hay de verdad, en vez de la jornada vieja al lado de un error que
      // habla de otra cosa.
      setPorConfirmar(null)
      await invalidar()
    },
  })

  /** Manda la jornada sin confirmar: si algo queda afuera, vuelve la pregunta. */
  function proponer(tramos: TramoDeJornada[]) {
    setCanceladas(0)
    guardar.mutate({ tramos, confirmado: false })
  }

  const bloques = data?.data ?? []
  const tramos = agruparTramos(bloques)
  const motivoNuevo = motivoParaNoGuardar(nuevo)
  const motivoEdicion = motivoParaNoGuardar(edicion)
  const trabajando = guardar.isPending

  function empezarAEditar(grupo: TramoAgrupado) {
    setFalloAlGuardar(null)
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

      {/* Un solo mensaje y no una lista por día: el guardado ya no es
          parcial, así que no existe el caso de "entró el lunes y falló el
          martes" que obligaba a decir cuál rehacer. */}
      {falloAlGuardar !== null && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{falloAlGuardar}</AlertDescription>
        </Alert>
      )}

      {canceladas > 0 && (
        <Alert className="mb-4">
          <AlertDescription>
            La jornada se guardó. Se cancelaron {canceladas}{" "}
            {canceladas === 1 ? "reserva" : "reservas"} que quedaban fuera del horario y
            se avisó por correo a sus docentes.
          </AlertDescription>
        </Alert>
      )}

      {porConfirmar !== null && (
        <ImpactoDeLaJornada
          impacto={porConfirmar.impacto}
          guardando={guardar.isPending}
          onConfirmar={() =>
            guardar.mutate({ tramos: porConfirmar.tramos, confirmado: true })
          }
          onCancelar={() => setPorConfirmar(null)}
        />
      )}

      <Card className="mb-6">
        <CardContent className="grid gap-4 pt-6">
          <p className="font-medium">Agregar un tramo</p>
          <CamposDeTramo valor={nuevo} onCambio={setNuevo} idPrefijo="nuevo" />
          <div className="flex flex-wrap items-center gap-3">
            <Button
              disabled={motivoNuevo !== "" || trabajando}
              onClick={() => {
                proponer([
                  ...aTramos(bloques),
                  ...expandirDias(nuevo.dias, nuevo.horaInicio, nuevo.horaFin),
                ])
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
                            proponer([
                              ...sinLosBloques(bloques, t.bloques),
                              ...expandirDias(
                                edicion.dias,
                                edicion.horaInicio,
                                edicion.horaFin
                              ),
                            ])
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
                        proponer(sinLosBloques(bloques, t.bloques))
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
                          proponer([
                            ...sinLosBloques(bloques, [b]),
                            { diaSemana: b.diaSemana, horaInicio, horaFin },
                          ])
                        }}
                        onQuitar={() => {
                          proponer(sinLosBloques(bloques, [b]))
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
