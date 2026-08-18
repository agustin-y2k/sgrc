import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useParams } from "react-router"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import * as calendarioApi from "@/features/calendario/api"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { JORNADA_KEY } from "@/features/disponibilidad/api"
import { diaDeLaFecha, diasDeLaJornada } from "@/features/disponibilidad/types"
import type { BloqueCalendario } from "@/features/calendario/types"
import {
  aFechaISO,
  aMinutos,
  desdeFechaISO,
  etiquetaDeDia,
  fechasDeLaSemana,
  formatearRangoVisible,
  sumarDias,
} from "@/features/calendario/semana"
import { getErrorMessage } from "@/lib/api-client"

// La grilla arranca a las 7 y termina a las 22 mientras no haya nada fuera de
// esa franja: cubre el horario escolar típico sin desperdiciar alto en horas
// en las que nunca hay clase.
//
// Son un piso y un techo por DEFECTO, no límites: una escuela nocturna dicta
// hasta pasada la medianoche y con una grilla fija esas clases se dibujarían
// afuera del recuadro, o sea en ninguna parte. El rango real lo decide
// rangoHorarioVisible() a partir de lo que efectivamente hay esa semana.
const HORA_DESDE_POR_DEFECTO = 7
const HORA_HASTA_POR_DEFECTO = 22
const ALTO_POR_HORA_REM = 3

/**
 * Parte un bloque que cruza la medianoche en los dos pedazos que se dibujan:
 * lo que ocurre en su propio día y lo que se pasa al siguiente.
 *
 * El backend guarda la clase como una sola cosa —una reserva del lunes de
 * 22:00 a 01:00— y así tiene que seguir siendo: es una clase, se cancela
 * entera y se cuenta una vez. Pero un calendario tiene una columna por día,
 * así que para dibujarla hay que cortarla por la medianoche. El corte es
 * puramente visual y no viaja a ningún lado.
 */
function pedazosPorDia(
  b: BloqueCalendario
): { fecha: string; bloque: BloqueCalendario }[] {
  if (aMinutos(b.horaFin) > aMinutos(b.horaInicio)) {
    return [{ fecha: b.fecha, bloque: b }]
  }
  const diaSiguiente = aFechaISO(sumarDias(desdeFechaISO(b.fecha), 1))
  return [
    { fecha: b.fecha, bloque: { ...b, horaFin: "24:00" } },
    { fecha: diaSiguiente, bloque: { ...b, horaInicio: "00:00" } },
  ]
}

/**
 * De qué hora a qué hora dibujar, dado lo que hay que mostrar.
 *
 * Se estira hacia afuera —nunca hacia adentro— para que ningún bloque quede
 * cortado: con una clase de 22:00 a 01:00 en la semana, la grilla llega hasta
 * las 24 y el día siguiente arranca a las 0.
 */
function rangoHorarioVisible(bloques: BloqueCalendario[]): {
  desde: number
  hasta: number
} {
  let desde = HORA_DESDE_POR_DEFECTO
  let hasta = HORA_HASTA_POR_DEFECTO
  for (const b of bloques) {
    desde = Math.min(desde, Math.floor(aMinutos(b.horaInicio) / 60))
    hasta = Math.max(hasta, Math.ceil(aMinutos(b.horaFin) / 60))
  }
  return { desde: Math.max(0, desde), hasta: Math.min(24, hasta) }
}

function BloqueOcupado({
  bloque,
  horaDesde,
}: {
  bloque: BloqueCalendario
  horaDesde: number
}) {
  const inicio = aMinutos(bloque.horaInicio)
  const fin = aMinutos(bloque.horaFin)
  const minutosBase = horaDesde * 60

  const esBloqueo = bloque.tipo === "BLOQUEO"
  // En un bloqueo el motivo viene siempre —es obligatorio al crearlo y lo
  // sostiene un CHECK en la base— pero el campo es opcional en el tipo
  // porque no existe en las reservas normales, y TypeScript no puede
  // estrecharlo por `esBloqueo`. El respaldo cubre lo que el tipo permite:
  // sin él, un `undefined` se leería tal cual dentro del title.
  const motivo = bloque.motivoBloqueo || "Bloqueado"
  const alto = ((fin - inicio) / 60) * ALTO_POR_HORA_REM
  const desplazamiento = ((inicio - minutosBase) / 60) * ALTO_POR_HORA_REM

  return (
    <div
      className={
        "absolute inset-x-0.5 overflow-hidden rounded-md border px-1.5 py-1 text-xs leading-tight " +
        (esBloqueo
          ? "border-destructive/40 bg-destructive/10 text-destructive"
          : "border-primary/30 bg-primary/10")
      }
      style={{ top: `${desplazamiento}rem`, height: `${alto}rem` }}
      title={
        esBloqueo
          ? `${motivo} · ${bloque.horaInicio}–${bloque.horaFin}`
          : `${bloque.materiaNombre} (${bloque.cursoNombre}) · ${bloque.docente} · ${bloque.horaInicio}–${bloque.horaFin}`
      }
    >
      {/* En un bloqueo, el motivo ocupa el lugar que en una clase tiene la
          materia: es lo que responde "¿por qué no puedo reservar acá?", que
          es exactamente lo que trae a alguien a mirar el calendario. */}
      <p className="truncate font-medium">{esBloqueo ? motivo : bloque.materiaNombre}</p>
      {!esBloqueo && (
        <>
          <p className="truncate opacity-80">{bloque.cursoNombre}</p>
          <p className="truncate opacity-80">{bloque.docente}</p>
        </>
      )}
      <p className="opacity-70">
        {bloque.horaInicio}–{bloque.horaFin}
      </p>
    </div>
  )
}

// RF-04.4: calendario completo de un equipo, con nombre del docente, materia
// y horario. Lo puede ver cualquier usuario autenticado.
export function CalendarioEquipoPage() {
  const { equipoId = "" } = useParams()
  const [referencia, setReferencia] = useState(() => new Date())

  const semana = useMemo(() => fechasDeLaSemana(referencia), [referencia])
  // El rango que se le pide al backend es la semana COMPLETA, aunque después
  // se dibujen menos columnas: si la escuela cambia su jornada, lo que ya
  // estaba reservado un día que dejó de estar declarado sigue existiendo, y
  // esconderlo de la consulta lo volvería invisible en vez de resuelto.
  const desde = semana[0]
  const hasta = semana[semana.length - 1]

  // La jornada declarada decide qué días se dibujan. Una escuela de lunes a
  // viernes ve cinco columnas; una albergue, siete. Sin jornada declarada se
  // muestran los siete, porque no hay restricción que refleje.
  const { data: jornada } = useQuery({
    queryKey: JORNADA_KEY,
    queryFn: disponibilidadApi.jornadaDeLaInstitucion,
  })
  const dias = useMemo(() => {
    const declarados = new Set(diasDeLaJornada(jornada?.data ?? []))
    const visibles = semana.filter((fecha) => {
      const dia = diaDeLaFecha(fecha)
      return dia !== null && declarados.has(dia)
    })
    // Una jornada que no cubra ningún día de esta semana dejaría la grilla
    // sin columnas y la pantalla sin sentido. Es un caso de borde raro pero
    // posible mientras alguien está cargando la jornada a medias.
    return visibles.length > 0 ? visibles : semana
  }, [semana, jornada])

  const { data, isLoading, error } = useQuery({
    queryKey: ["calendario", equipoId, desde, hasta],
    queryFn: () => calendarioApi.calendarioDeEquipo(equipoId, desde, hasta),
  })

  // Se agrupa por fecha una sola vez en vez de filtrar dentro de cada
  // celda (que sería recorrer la lista completa una vez por columna).
  //
  // Un bloque que cruza la medianoche entra en DOS días, partido: la clase
  // del lunes a las 22:00 ocupa el final del lunes y el principio del martes.
  const bloquesPorDia = useMemo(() => {
    const mapa = new Map<string, BloqueCalendario[]>()
    for (const b of data?.bloques ?? []) {
      for (const { fecha, bloque } of pedazosPorDia(b)) {
        const delDia = mapa.get(fecha)
        if (delDia) delDia.push(bloque)
        else mapa.set(fecha, [bloque])
      }
    }
    return mapa
  }, [data])

  const rango = useMemo(
    () => rangoHorarioVisible([...bloquesPorDia.values()].flat()),
    [bloquesPorDia]
  )
  const horas = Array.from(
    { length: rango.hasta - rango.desde },
    (_, i) => rango.desde + i
  )
  const hoy = aFechaISO(new Date())

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold tracking-tight">Calendario del equipo</h1>
          <p className="text-muted-foreground text-sm">
            {/* El rango que se dibuja, no la semana entera: con una escuela
                de lunes a viernes el rótulo prometía dos días que no están. */}
            {formatearRangoVisible(dias)}
          </p>
        </div>
        {/* flex-wrap: los cuatro botones no entran en una línea en un
            teléfono y sin esto empujaban el ancho de la página (RNF-07). */}
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setReferencia(sumarDias(referencia, -7))}
          >
            ← Semana anterior
          </Button>
          <Button variant="outline" size="sm" onClick={() => setReferencia(new Date())}>
            Hoy
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setReferencia(sumarDias(referencia, 7))}
          >
            Semana siguiente →
          </Button>
          <Button asChild variant="ghost" size="sm">
            <Link to="/inventario">Volver al inventario</Link>
          </Button>
        </div>
      </div>

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}
      {isLoading && <p className="text-muted-foreground">Cargando calendario…</p>}

      {/* Con un error no se dibuja la grilla. Antes se dibujaba igual, así que
          un equipo que no existe mostraba el cartel Y una semana entera vacía
          debajo: la pantalla se contradecía a sí misma. */}
      {!error && (
        <>
          {/* La grilla scrollea horizontalmente en pantallas angostas en vez de
          desbordar la página (RNF-07). */}
          <div className="overflow-x-auto">
            <div className="min-w-[44rem]">
              <div
                className="grid border-b"
                style={{ gridTemplateColumns: `3.5rem repeat(${dias.length}, 1fr)` }}
              >
                <div />
                {dias.map((fecha) => (
                  <div
                    key={fecha}
                    className={
                      "border-l px-2 py-1 text-center text-sm font-medium " +
                      (fecha === hoy ? "text-primary" : "")
                    }
                  >
                    <div>{etiquetaDeDia(fecha)}</div>
                    <div className="text-muted-foreground text-xs">
                      {desdeFechaISO(fecha).getDate()}
                    </div>
                  </div>
                ))}
              </div>

              <div
                className="grid"
                style={{ gridTemplateColumns: `3.5rem repeat(${dias.length}, 1fr)` }}
              >
                {/* Columna de horas */}
                <div>
                  {horas.map((h) => (
                    <div
                      key={h}
                      className="text-muted-foreground border-b pr-2 text-right text-xs"
                      style={{ height: `${ALTO_POR_HORA_REM}rem` }}
                    >
                      {String(h).padStart(2, "0")}:00
                    </div>
                  ))}
                </div>

                {dias.map((fecha) => (
                  <div key={fecha} className="relative border-l">
                    {horas.map((h) => (
                      <div
                        key={h}
                        className="border-b"
                        style={{ height: `${ALTO_POR_HORA_REM}rem` }}
                      />
                    ))}
                    {(bloquesPorDia.get(fecha) ?? []).map((b) => (
                      <BloqueOcupado
                        key={`${b.reservaId}-${b.horaInicio}`}
                        bloque={b}
                        horaDesde={rango.desde}
                      />
                    ))}
                  </div>
                ))}
              </div>
            </div>
          </div>

          <div className="text-muted-foreground mt-4 flex flex-wrap items-center gap-3 text-xs">
            <span className="flex items-center gap-1.5">
              <span className="border-primary/30 bg-primary/10 inline-block size-3 rounded border" />
              Reserva de clase
            </span>
            <span className="flex items-center gap-1.5">
              <span className="border-destructive/40 bg-destructive/10 inline-block size-3 rounded border" />
              Bloqueado por un Admin
            </span>
            <Badge variant="outline">Los huecos en blanco están libres</Badge>
          </div>
        </>
      )}
    </div>
  )
}
