import { errorDeCicloLectivo } from "./ciclo"
import { useEffect, useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useLocation, useNavigate } from "react-router"

import { etiquetaDeMateriaYLugar } from "@/features/academico/types"
import { SelectorDeHora } from "@/components/SelectorDeHora"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { JORNADA_KEY } from "@/features/disponibilidad/api"
import { dentroDeLaJornada } from "@/features/disponibilidad/types"
import * as reservasApi from "@/features/reservas/api"
import { SelectorDeEquipos } from "@/features/reservas/SelectorDeEquipos"
import { MINIMO_PARA_BUSCAR, sinTildes } from "@/lib/texto"
import { useAuth } from "@/features/auth/AuthContext"
import {
  guardarBorradorReserva,
  hayDatosDeReserva,
  leerBorradorReserva,
} from "@/features/reservas/borrador"
import { borrarBorradorReserva } from "@/lib/borradores"
import { calcularRecurrencia, MAX_OCURRENCIAS } from "@/features/reservas/recurrencias"
import {
  DIAS_SEMANA,
  MAX_HORAS_RESERVA,
  cruzaMedianoche,
  excedeDuracionMaxima,
  hoyISO,
  type DiaSemana,
} from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaLargaCapitalizada } from "@/lib/fechas"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { contar, plural } from "@/lib/plural"
import { useAhora } from "@/lib/useAhora"
import { intervaloDeReserva } from "@/features/reservas/intervalo"

type Modo = "simple" | "recurrente"

/** ["a", "b", "c"] → "a, b y c". Un `join(", ")` suelto se lee a los tirones. */
function enumerar(partes: string[]): string {
  if (partes.length <= 1) return partes.join("")
  return `${partes.slice(0, -1).join(", ")} y ${partes[partes.length - 1]}`
}

/**
 * RF-03.21, con el criterio de RF-03.19: si la reserva desplazó a otros, el
 * que reservó tiene que enterarse de a cuántos. Sin robo no se dice nada —el
 * caso de todos los días— en vez de arrastrar un "se cancelaron 0 reservas".
 *
 * Los docentes se cuentan aparte de las reservas porque no son el mismo
 * número: el backend no cuenta a quien reserva aunque se haya corrido su
 * propia clase entre dos cursos, así que puede haber cancelaciones sin nadie
 * a quien avisarle.
 */
function impactoDelRobo(res: {
  reservasCanceladas?: number
  reservasReubicadas?: number
  docentesAfectados?: number
}): string {
  const canceladas = res.reservasCanceladas ?? 0
  const reubicadas = res.reservasReubicadas ?? 0
  if (canceladas === 0 && reubicadas === 0) return ""

  const avisados = res.docentesAfectados ?? 0
  const aQuienes = avisados > 0 ? ` y se avisó a ${contar(avisados, "docente")}` : ""
  // Los contadores de impacto corresponden a equipos de las reservas, no a
  // clases completas. Una clase puede conservar los equipos no desplazados.
  const reposicion =
    reubicadas > 0
      ? `Se ${plural(reubicadas, "repuso", "repusieron")} ${contar(reubicadas, "equipo")} en las reservas desplazadas`
      : ""
  const faltantes =
    canceladas > 0
      ? `${reposicion ? " y " : ""}${contar(canceladas, "equipo")} ${plural(canceladas, "quedó", "quedaron")} sin reemplazo disponible`
      : ""
  return ` ${reposicion}${faltantes}${aQuienes}.`
}

// RF-04.2 (reserva puntual) y RF-04.5 (recurrente).
export function NuevaReservaPage() {
  const { user } = useAuth()
  return <FormularioDeReserva key={user?.id ?? "sin-cuenta"} usuarioId={user?.id} />
}

function FormularioDeReserva({ usuarioId }: { usuarioId: string | undefined }) {
  const ahora = useAhora()
  const [inicial] = useState(() => leerBorradorReserva(usuarioId))
  const navigate = useNavigate()
  const { state } = useLocation()
  const destino =
    state && typeof state === "object" && "volverA" in state
      ? state.volverA
      : inicial?.volverA
  const volverA =
    typeof destino === "string" && /^\/reservas(?:\?|$)/.test(destino)
      ? destino
      : "/reservas"
  const queryClient = useQueryClient()

  const [recuperado, setRecuperado] = useState(!!inicial)
  const [errorGuardado, setErrorGuardado] = useState(false)
  const [modo, setModo] = useState<Modo>(inicial?.modo ?? "simple")
  const [materiaId, setMateriaId] = useState(inicial?.materiaId ?? "")
  const [busquedaMateria, setBusquedaMateria] = useState(inicial?.busquedaMateria ?? "")
  const [fecha, setFecha] = useState(inicial?.fecha ?? "")
  const [horaInicio, setHoraInicio] = useState(inicial?.horaInicio ?? "")
  const [horaFin, setHoraFin] = useState(inicial?.horaFin ?? "")
  const [diaSemana, setDiaSemana] = useState<DiaSemana>(inicial?.diaSemana ?? "LUNES")
  const [fechaInicio, setFechaInicio] = useState(inicial?.fechaInicio ?? "")
  const [fechaFin, setFechaFin] = useState(inicial?.fechaFin ?? "")
  const claveDeSeleccion = JSON.stringify([
    modo,
    materiaId,
    fecha,
    horaInicio,
    horaFin,
    diaSemana,
    fechaInicio,
    fechaFin,
  ])
  const [seleccion, setSeleccion] = useState<{
    clave: string
    ids: string[]
    aTomar: string[]
  }>(inicial?.seleccion ?? { clave: "", ids: [], aTomar: [] })
  const borrador = useMemo(
    () => ({
      modo,
      materiaId,
      busquedaMateria,
      volverA,
      fecha,
      horaInicio,
      horaFin,
      diaSemana,
      fechaInicio,
      fechaFin,
      seleccion,
    }),
    [
      modo,
      materiaId,
      busquedaMateria,
      volverA,
      fecha,
      horaInicio,
      horaFin,
      diaSemana,
      fechaInicio,
      fechaFin,
      seleccion,
    ]
  )
  useEffect(() => {
    setErrorGuardado(!guardarBorradorReserva(usuarioId, borrador))
  }, [usuarioId, borrador])
  const descartar = () => {
    borrarBorradorReserva()
    setModo("simple")
    setMateriaId("")
    setBusquedaMateria("")
    setFecha("")
    setHoraInicio("")
    setHoraFin("")
    setDiaSemana("LUNES")
    setFechaInicio("")
    setFechaFin("")
    setSeleccion({ clave: "", ids: [], aTomar: [] })
    setRecuperado(false)
  }
  const equipoIds = seleccion.clave === claveDeSeleccion ? seleccion.ids : []
  // RF-03.21: de los tildados, cuáles estaban ocupados. Es lo único que
  // autoriza a desplazar al que los tenía, y lo decide el selector, que es
  // quien tiene la lista de ocupados.
  const equiposATomar = seleccion.clave === claveDeSeleccion ? seleccion.aTomar : []
  const claveSerie = JSON.stringify([claveDeSeleccion, equipoIds, equiposATomar])
  const [consentimientoSerie, setConsentimientoSerie] = useState("")
  const comprobarSerie = useMutation({
    mutationFn: async () => ({
      clave: claveSerie,
      resultado: await reservasApi.previsualizarSerie({
        materiaId,
        diaSemana,
        horaInicio,
        horaFin,
        fechaInicio,
        fechaFin,
        equipoIds,
      }),
    }),
  })
  const serieActual =
    comprobarSerie.data?.clave === claveSerie ? comprobarSerie.data.resultado : undefined
  const serieAutorizada =
    !!serieActual?.puedeCrear &&
    !comprobarSerie.isPending &&
    !comprobarSerie.error &&
    (serieActual.equiposATomar.length === 0 || consentimientoSerie === claveSerie)
  const reiniciarComprobacion = comprobarSerie.reset
  useEffect(() => {
    reiniciarComprobacion()
    setConsentimientoSerie("")
  }, [claveSerie, reiniciarComprobacion])

  const {
    data: materias,
    isLoading: cargandoMaterias,
    error: errorMaterias,
  } = useQuery({
    queryKey: ["mis-materias"],
    queryFn: reservasApi.misMaterias,
  })

  const crear = useMutation({
    /**
     * Devuelve la frase que va a leer el usuario en el listado, no el cuerpo
     * de la respuesta: los dos endpoints responden cosas distintas y ninguna
     * de las dos se usa acá.
     */
    mutationFn: async (): Promise<string> => {
      const cuantosEquipos = `${contar(equipoIds.length, "computadora")}`

      if (modo === "simple") {
        const res = await reservasApi.crearReserva({
          materiaId,
          fecha,
          horaInicio,
          horaFin,
          equipoIds,
          equiposATomar,
        })
        return (
          `Reserva confirmada para el ${formatearFechaLargaCapitalizada(fecha)}, de ${horaInicio} a ${horaFin}, con ${cuantosEquipos}.` +
          impactoDelRobo(res)
        )
      }

      const res = await reservasApi.crearReservaRecurrente({
        materiaId,
        diaSemana,
        horaInicio,
        horaFin,
        fechaInicio,
        fechaFin,
        equipoIds,
        equiposATomar: serieActual?.equiposATomar ?? [],
      })
      const dia = DIAS_SEMANA.find((d) => d.valor === diaSemana)?.etiqueta ?? diaSemana
      return (
        `Reserva recurrente confirmada: todos los ${dia.toLowerCase()} de ${horaInicio} a ${horaFin}, con ${cuantosEquipos}, hasta el ${formatearFechaLargaCapitalizada(fechaFin)}.` +
        impactoDelRobo(res)
      )
    },
    onError: () => {
      reiniciarComprobacion()
      setConsentimientoSerie("")
    },
    // El listado es la confirmación: la reserva recién creada aparece ahí.
    onSuccess: async (confirmacion) => {
      borrarBorradorReserva()
      await queryClient.invalidateQueries({ queryKey: ["reservas"] })
      navigate("/reservas", { state: { confirmacion } })
    },
  })

  // La franja de la que depende el selector de equipos: en modo recurrente se
  // usa la primera fecha del rango como muestra, porque el backend valida la
  // disponibilidad de TODAS las ocurrencias al crear (RF-04.5) — acá solo se
  // necesita una lista razonable para tildar.
  const recurrencia = calcularRecurrencia(fechaInicio, fechaFin, diaSemana)
  const fechaParaDisponibilidad =
    modo === "simple" ? fecha : (recurrencia?.primeraFecha ?? "")
  const errorDeSerie =
    modo !== "recurrente" || !recurrencia
      ? null
      : recurrencia.cantidad === 0
        ? "El rango no contiene ninguna clase del día elegido."
        : recurrencia.cantidad > MAX_OCURRENCIAS
          ? `Una serie admite hasta ${MAX_OCURRENCIAS} clases. El rango elegido contiene ${recurrencia.cantidad}.`
          : null

  const materiasDisponibles = materias?.data ?? []
  const materiaElegida = materiasDisponibles.find((m) => m.materiaId === materiaId)
  const materiaValida = !!materiaElegida
  const inicioCiclo = materiaElegida ? `${materiaElegida.cicloAnio}-01-01` : undefined
  const finCiclo = materiaElegida ? `${materiaElegida.cicloAnio}-12-31` : undefined
  const minFecha = inicioCiclo && inicioCiclo > hoyISO() ? inicioCiclo : hoyISO()
  const errorDeCiclo = errorDeCicloLectivo(
    materiaElegida?.cicloAnio,
    modo === "simple" ? fecha : fechaInicio,
    modo === "simple" ? fecha : fechaFin,
    horaInicio,
    horaFin,
    modo === "recurrente" ? diaSemana : undefined
  )
  const materiasFiltradas = materiasDisponibles.filter((m) =>
    sinTildes(`${etiquetaDeMateriaYLugar(m)} ${m.cicloAnio}`).includes(
      sinTildes(busquedaMateria.trim())
    )
  )
  const elegidaFueraDelFiltro = materiasDisponibles.find(
    (m) =>
      m.materiaId === materiaId &&
      !materiasFiltradas.some((f) => f.materiaId === materiaId)
  )
  // El `min` de los inputs cubre la fecha; esto cubre el horario, que el
  // navegador no puede limitar solo.
  const duracionExcesiva = excedeDuracionMaxima(horaInicio, horaFin)

  // La jornada declarada por la institución.
  const consultaJornada = useQuery({
    queryKey: JORNADA_KEY,
    queryFn: disponibilidadApi.jornadaDeLaInstitucion,
  })
  const jornadaLista =
    consultaJornada.isSuccess && !consultaJornada.isFetching && !consultaJornada.error
  const bloquesDeJornada = consultaJornada.data?.data ?? []
  const fueraDeLaJornada =
    jornadaLista &&
    fechaParaDisponibilidad !== "" &&
    horaInicio !== "" &&
    horaFin !== "" &&
    horaFin !== horaInicio &&
    !dentroDeLaJornada(bloquesDeJornada, fechaParaDisponibilidad, horaInicio, horaFin)
  const fechaPasada = !!fechaParaDisponibilidad && fechaParaDisponibilidad < hoyISO()
  const horarioTerminado =
    !!fechaParaDisponibilidad &&
    !!horaInicio &&
    !!horaFin &&
    horaInicio !== horaFin &&
    intervaloDeReserva({ fecha: fechaParaDisponibilidad, horaInicio, horaFin }).fin <=
      ahora.getTime()
  const franjaConsultable =
    jornadaLista &&
    materiaValida &&
    !errorDeCiclo &&
    !fechaPasada &&
    !horarioTerminado &&
    !!fechaParaDisponibilidad &&
    !!horaInicio &&
    !!horaFin &&
    horaInicio !== horaFin
  const disponibilidad = useQuery({
    queryKey: [
      "equipos-disponibles",
      fechaParaDisponibilidad,
      horaInicio,
      horaFin,
      materiaId ?? "",
    ],
    queryFn: () =>
      reservasApi.equiposDisponibles({
        fecha: fechaParaDisponibilidad,
        horaInicio,
        horaFin,
        materiaId,
      }),
    enabled: franjaConsultable,
  })
  const libresActuales = new Set(disponibilidad.data?.data.map((e) => e.equipoId) ?? [])
  const tomablesActuales = new Set(
    disponibilidad.data?.ocupados?.filter((e) => e.puedeTomarse).map((e) => e.equipoId) ??
      []
  )
  const seleccionValida =
    disponibilidad.isSuccess &&
    !disponibilidad.isFetching &&
    !disponibilidad.error &&
    equipoIds.every(
      (id) =>
        libresActuales.has(id) || (tomablesActuales.has(id) && equiposATomar.includes(id))
    )

  /** Lo que todavía falta para poder confirmar, dicho en palabras. */
  const faltantes: string[] = []
  if (materiaId === "") faltantes.push("elegir la materia")
  if (modo === "simple") {
    if (fecha === "") faltantes.push("elegir la fecha")
  } else {
    if (fechaInicio === "") faltantes.push("indicar desde qué fecha se repite")
    if (fechaFin === "") faltantes.push("indicar hasta qué fecha se repite")
  }
  if (horaInicio === "") faltantes.push("elegir la hora de inicio")
  if (horaFin === "") faltantes.push("elegir la hora de fin")
  if (equipoIds.length === 0) faltantes.push("tildar al menos una computadora")

  const listoParaComprobar =
    faltantes.length === 0 &&
    materiaValida &&
    seleccionValida &&
    jornadaLista &&
    !horarioTerminado &&
    !fechaPasada &&
    (modo === "simple" || !!recurrencia) &&
    horaFin !== horaInicio &&
    !duracionExcesiva &&
    !fueraDeLaJornada &&
    !errorDeSerie &&
    !errorDeCiclo &&
    !errorMaterias
  const listoParaEnviar = listoParaComprobar && (modo === "simple" || serieAutorizada)

  return (
    <div className="mx-auto max-w-3xl">
      <EncabezadoDePagina
        titulo="Nueva reserva"
        descripcion="Elegí el día, desde qué hora hasta qué hora, y qué computadoras necesitás. Los horarios van en formato de 24 horas."
      />
      {recuperado && (
        <Alert className="mb-4">
          <AlertDescription>
            Borrador recuperado. Revisá la disponibilidad y los datos antes de confirmar.
          </AlertDescription>
        </Alert>
      )}
      {errorGuardado && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            No pudimos guardar el borrador en este navegador. Mantené el formulario
            abierto hasta terminar la reserva.
          </AlertDescription>
        </Alert>
      )}
      {fechaPasada && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            La fecha del borrador ya pasó. Elegí una nueva fecha para consultar
            disponibilidad.
          </AlertDescription>
        </Alert>
      )}
      {!fechaPasada && horarioTerminado && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            El horario elegido ya terminó. Elegí una franja vigente para reservar.
          </AlertDescription>
        </Alert>
      )}
      {consultaJornada.error && (
        <div className="mb-4">
          <ErrorDeConsulta
            mensaje="No pudimos consultar la jornada de la escuela. La reserva se habilitará cuando podamos verificar el horario."
            reintentando={consultaJornada.isFetching}
            onReintentar={() => void consultaJornada.refetch()}
          />
        </div>
      )}
      {materiaId && materias && !errorMaterias && !materiaValida && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            La materia elegida ya no está disponible para tu cuenta. Elegí otra o consultá
            con administración.
          </AlertDescription>
        </Alert>
      )}
      {disponibilidad.isSuccess &&
        equipoIds.length > 0 &&
        !seleccionValida &&
        !disponibilidad.isFetching && (
          <Alert className="mb-4">
            <AlertDescription>
              Cambió la disponibilidad de los equipos elegidos. Limpiá la selección y
              volvé a elegir, revisando cuáles requieren prioridad.
            </AlertDescription>
          </Alert>
        )}

      {!cargandoMaterias && !errorMaterias && materiasDisponibles.length === 0 && (
        <Alert className="mb-4">
          <AlertDescription>
            No estás asignado a ninguna materia de un ciclo activo, así que todavía no
            podés reservar.{" "}
            <Link to="/perfil" className="font-medium underline">
              Solicitá una materia desde Mi perfil
            </Link>{" "}
            y volvé cuando administración la asigne.
          </AlertDescription>
        </Alert>
      )}

      {(crear.error || errorMaterias) && (
        <Alert variant="destructive" className="mb-4">
          {/* RF-04.3: el mensaje del backend nombra qué equipo choca, en qué
              fecha y con quién. Se muestra tal cual: uno genérico obligaría a
              destildar de a uno para encontrar el ocupado. */}
          <AlertDescription>
            {getErrorMessage(crear.error ?? errorMaterias)}
          </AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Datos de la reserva</CardTitle>
          <CardDescription>
            Podés combinar computadoras de distintos carros en la misma reserva.
          </CardDescription>
        </CardHeader>
        <CardContent className="min-w-0">
          <form
            className="grid min-w-0 gap-5 [&_button]:min-h-11 [&_button]:min-w-11 sm:pointer-fine:[&_button]:min-h-0 sm:pointer-fine:[&_button]:min-w-0"
            onSubmit={(e) => {
              e.preventDefault()
              if (listoParaEnviar && !crear.isPending) crear.mutate()
            }}
          >
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant={modo === "simple" ? "default" : "outline"}
                aria-pressed={modo === "simple"}
                size="sm"
                onClick={() => setModo("simple")}
              >
                Una sola fecha
              </Button>
              <Button
                type="button"
                variant={modo === "recurrente" ? "default" : "outline"}
                aria-pressed={modo === "recurrente"}
                size="sm"
                onClick={() => setModo("recurrente")}
              >
                Se repite todas las semanas
              </Button>
            </div>

            <div className="grid gap-2">
              {materiasDisponibles.length >= MINIMO_PARA_BUSCAR && (
                <>
                  <Label htmlFor="buscar-materia-reserva">Buscar materia o curso</Label>
                  <Input
                    id="buscar-materia-reserva"
                    value={busquedaMateria}
                    maxLength={200}
                    onChange={(e) => setBusquedaMateria(e.target.value)}
                    placeholder="Nombre de materia, curso o modalidad"
                  />
                  {materiasFiltradas.length === 0 && (
                    <p className="text-muted-foreground text-sm">
                      Ninguna materia coincide con la búsqueda. La materia ya elegida se
                      conserva hasta que selecciones otra.
                    </p>
                  )}
                </>
              )}
              <Label htmlFor="materia">Materia</Label>
              <Select
                id="materia"
                value={materiaId}
                onChange={(e) => setMateriaId(e.target.value)}
              >
                <option value="">Elegí una materia…</option>
                {elegidaFueraDelFiltro && (
                  <optgroup label="Materia ya elegida">
                    <option value={elegidaFueraDelFiltro.materiaId}>
                      {etiquetaDeMateriaYLugar(elegidaFueraDelFiltro)}
                    </option>
                  </optgroup>
                )}
                {materiasFiltradas.map((m) => (
                  <option key={m.materiaId} value={m.materiaId}>
                    {etiquetaDeMateriaYLugar(m)} ({m.cicloAnio})
                  </option>
                ))}
              </Select>
            </div>

            {modo === "simple" ? (
              <div className="grid gap-2">
                <Label htmlFor="fecha">Fecha</Label>
                <Input
                  id="fecha"
                  type="date"
                  min={minFecha}
                  max={finCiclo}
                  value={fecha}
                  onChange={(e) => setFecha(e.target.value)}
                />
              </div>
            ) : (
              <div className="grid gap-4 sm:grid-cols-3">
                <div className="grid gap-2">
                  <Label htmlFor="diaSemana">Día de la semana</Label>
                  <Select
                    id="diaSemana"
                    value={diaSemana}
                    onChange={(e) => setDiaSemana(e.target.value as DiaSemana)}
                  >
                    {DIAS_SEMANA.map((d) => (
                      <option key={d.valor} value={d.valor}>
                        {d.etiqueta}
                      </option>
                    ))}
                  </Select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="fechaInicio">Desde</Label>
                  <Input
                    id="fechaInicio"
                    type="date"
                    min={minFecha}
                    max={finCiclo}
                    value={fechaInicio}
                    onChange={(e) => setFechaInicio(e.target.value)}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="fechaFin">Hasta</Label>
                  <Input
                    id="fechaFin"
                    type="date"
                    min={fechaInicio && fechaInicio > minFecha ? fechaInicio : minFecha}
                    max={finCiclo}
                    value={fechaFin}
                    onChange={(e) => setFechaFin(e.target.value)}
                  />
                </div>
              </div>
            )}
            {errorDeCiclo && (
              <Alert variant="destructive">
                <AlertDescription>{errorDeCiclo}</AlertDescription>
              </Alert>
            )}
            {errorDeSerie && (
              <Alert variant="destructive">
                <AlertDescription>{errorDeSerie}</AlertDescription>
              </Alert>
            )}

            <div className="grid gap-4 sm:grid-cols-2">
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

            {horaInicio !== "" && horaFin !== "" && horaFin === horaInicio && (
              <p className="text-destructive text-sm">
                La hora de fin no puede ser igual a la de inicio.
              </p>
            )}

            {/* Que la clase termine al día siguiente es válido y es lo normal
                en una escuela nocturna, pero conviene decirlo: alguien que
                puso 01:00 por error tiene que poder darse cuenta antes de
                confirmar. */}
            {cruzaMedianoche(horaInicio, horaFin) && (
              <p className="text-muted-foreground text-sm">
                Esta reserva termina al día siguiente, a las {horaFin}.
              </p>
            )}

            {duracionExcesiva && (
              <p className="text-destructive text-sm">
                Una reserva no puede durar más de {MAX_HORAS_RESERVA} horas.
              </p>
            )}

            {fueraDeLaJornada && (
              <p className="text-destructive text-sm">
                Ese día y horario quedan fuera de la jornada de la escuela. Consultá con
                un Admin si el horario de la escuela cambió.
              </p>
            )}

            <div className="grid gap-2">
              <Label>Qué computadoras necesitás</Label>
              {modo === "recurrente" && (
                <p className="text-muted-foreground text-xs">
                  Se muestra la disponibilidad de la primera clase. Al confirmar se
                  validan todas las fechas solicitadas: los desplazamientos y la
                  reposición se resuelven para las reservas que se solapan con esas fechas
                  y horarios. Las demás fechas de las series ajenas se conservan. Si algún
                  conflicto no se puede resolver, no se crea ninguna clase.
                </p>
              )}
              {jornadaLista ? (
                <SelectorDeEquipos
                  fecha={franjaConsultable ? fechaParaDisponibilidad : ""}
                  horaInicio={horaInicio}
                  horaFin={horaFin}
                  seleccionadas={equipoIds}
                  onCambio={(ids, aTomar) => {
                    setSeleccion({ clave: claveDeSeleccion, ids, aTomar })
                  }}
                  materiaId={materiaId}
                />
              ) : (
                !consultaJornada.error && (
                  <p className="text-muted-foreground text-sm" role="status">
                    Consultando la jornada de la escuela…
                  </p>
                )
              )}
            </div>

            {modo === "recurrente" && (
              <section
                aria-label="Comprobación de la serie"
                className="space-y-3 rounded-md border p-3"
              >
                <p className="text-sm">
                  Comprobá todas las fechas antes de confirmar. La disponibilidad puede
                  cambiar y se validará de nuevo al reservar.
                </p>
                <Button
                  type="button"
                  variant="outline"
                  disabled={
                    !listoParaComprobar || comprobarSerie.isPending || crear.isPending
                  }
                  onClick={() => {
                    setConsentimientoSerie("")
                    comprobarSerie.mutate()
                  }}
                >
                  {comprobarSerie.isPending ? "Comprobando fechas…" : "Comprobar serie"}
                </Button>
                {comprobarSerie.error && (
                  <Alert variant="destructive">
                    <AlertDescription>
                      {getErrorMessage(comprobarSerie.error)}
                    </AlertDescription>
                  </Alert>
                )}
                {comprobarSerie.data && !serieActual && (
                  <p className="text-sm" role="status">
                    Los datos cambiaron. Volvé a comprobar la serie.
                  </p>
                )}
                {serieActual && (
                  <div className="space-y-3" aria-live="polite">
                    <p className="font-medium">
                      {contar(serieActual.cantidadClases, "clase")} comprobadas.{" "}
                      {serieActual.puedeCrear
                        ? "Todos los equipos pueden reservarse."
                        : "Hay equipos no disponibles. Cambiá las fechas o la selección y comprobá de nuevo."}
                    </p>
                    <ul className="space-y-2">
                      {serieActual.fechas.map((f) => (
                        <li key={f.fecha} className="rounded border p-2 text-sm">
                          <details
                            open={f.equipos.some((e) => e.estado === "NO_DISPONIBLE")}
                          >
                            <summary className="min-h-11 cursor-pointer">
                              <span className="font-medium">
                                {formatearFechaLargaCapitalizada(f.fecha)}
                              </span>
                              <span className="mt-1 block">
                                Libres:{" "}
                                {f.equipos.filter((e) => e.estado === "LIBRE").length} ·
                                Por prioridad:{" "}
                                {
                                  f.equipos.filter((e) => e.estado === "POR_PRIORIDAD")
                                    .length
                                }{" "}
                                · No disponibles:{" "}
                                {
                                  f.equipos.filter((e) => e.estado === "NO_DISPONIBLE")
                                    .length
                                }
                              </span>
                            </summary>
                            <ul>
                              {f.equipos.map((e) => (
                                <li key={e.equipoId}>
                                  {e.etiqueta}:{" "}
                                  {e.estado === "LIBRE"
                                    ? "libre"
                                    : e.estado === "POR_PRIORIDAD"
                                      ? "por prioridad"
                                      : "no disponible"}
                                  {e.motivo ? `. ${e.motivo}` : ""}
                                </li>
                              ))}
                            </ul>
                          </details>
                        </li>
                      ))}
                    </ul>
                    {serieActual.equiposATomar.length > 0 && (
                      <label className="flex min-h-11 items-start gap-2 text-sm">
                        <input
                          type="checkbox"
                          className="mt-1 size-4 shrink-0"
                          checked={consentimientoSerie === claveSerie}
                          onChange={(e) =>
                            setConsentimientoSerie(e.target.checked ? claveSerie : "")
                          }
                        />
                        Autorizo tomar los equipos por prioridad en las fechas indicadas.
                        Se intentará reponer las reservas desplazadas en cascada; pueden
                        quedar faltantes.
                      </label>
                    )}
                  </div>
                )}
              </section>
            )}

            {equiposATomar.length > 0 && (
              <Alert>
                <AlertDescription>
                  Vas a reservar {contar(equipoIds.length, "equipo")}:{" "}
                  {equipoIds.length - equiposATomar.length} libres y{" "}
                  {equiposATomar.length} por prioridad. Se intentará reponer las reservas
                  desplazadas en cascada; si no hay reemplazos, pueden quedar faltantes.
                  {modo === "recurrente" &&
                    " Estos conteos corresponden a la primera fecha; revisá las demás en la comprobación de la serie."}
                  {modo === "simple"
                    ? " El alcance es esta fecha y este horario."
                    : " El alcance son las fechas y horarios solicitados de esta serie; el impacto puede variar en cada clase."}
                </AlertDescription>
              </Alert>
            )}

            {/* `aria-live`: la lista cambia sola a medida que se completa el
                formulario, sin que nada reciba el foco. Sin esto, alguien
                que usa lector de pantalla no se entera de que ya está. */}
            {faltantes.length > 0 && (
              <p className="text-muted-foreground text-sm" aria-live="polite">
                {faltantes.length === 1
                  ? "Para confirmar falta"
                  : "Para confirmar faltan"}{" "}
                {enumerar(faltantes)}.
              </p>
            )}

            <div className="flex flex-wrap gap-2">
              <Button type="submit" disabled={!listoParaEnviar || crear.isPending}>
                {crear.isPending ? "Reservando…" : "Confirmar reserva"}
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  borrarBorradorReserva()
                  navigate(volverA)
                }}
              >
                Cancelar
              </Button>
              {hayDatosDeReserva(borrador) && (
                <Button
                  type="button"
                  variant="outline"
                  className="h-11"
                  onClick={descartar}
                >
                  Descartar borrador
                </Button>
              )}
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
