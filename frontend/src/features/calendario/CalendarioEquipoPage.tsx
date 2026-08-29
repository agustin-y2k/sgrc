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
const HORA_DESDE_POR_DEFECTO = 7
const HORA_HASTA_POR_DEFECTO = 22
const ALTO_POR_HORA_REM = 3

/**
 * Parte un bloque que cruza la medianoche en los dos pedazos que se dibujan:
 * lo que ocurre en su propio día y lo que se pasa al siguiente.
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

/** De qué hora a qué hora dibujar: se estira hacia afuera para no cortar ningún bloque. */
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
  // sostiene un CHECK en la base— pero el campo es opcional en el tipo porque
  // no existe en las reservas normales, y TypeScript no puede estrecharlo por
  // `esBloqueo`.
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

  // La jornada declarada decide qué días se dibujan.
  const { data: jornada } = useQuery({
    queryKey: JORNADA_KEY,
    queryFn: disponibilidadApi.jornadaDeLaInstitucion,
  })
  const diasCerrados = useMemo(() => {
    const declarados = new Set(diasDeLaJornada(jornada?.data ?? []))
    return new Set(
      semana.filter((fecha) => {
        const dia = diaDeLaFecha(fecha)
        return dia !== null && !declarados.has(dia)
      })
    )
  }, [semana, jornada])

  const { data, isLoading, error } = useQuery({
    queryKey: ["calendario", equipoId, desde, hasta],
    queryFn: () => calendarioApi.calendarioDeEquipo(equipoId, desde, hasta),
  })

  // Se agrupa por fecha una sola vez en vez de filtrar dentro de cada celda
  // (que sería recorrer la lista completa una vez por columna).
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

  // Un día que dejó de estar declarado se sigue dibujando si tiene algo
  // reservado: esa reserva sobrevivió al cambio de jornada, sigue ocupando la
  // máquina y sigue avisando, así que esconderla acá la volvería invisible en
  // vez de resuelta. Los días cerrados y vacíos sí se ocultan, que es para lo
  // que se declara una jornada.
  const dias = useMemo(() => {
    const visibles = semana.filter(
      (fecha) => !diasCerrados.has(fecha) || bloquesPorDia.has(fecha)
    )
    // Una jornada que no cubra ningún día de esta semana dejaría la grilla
    // sin columnas y la pantalla sin sentido.
    return visibles.length > 0 ? visibles : semana
  }, [semana, diasCerrados, bloquesPorDia])

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
      {/* La salida va ARRIBA DE TODO y sola, no al final de la fila de
          controles de semana.
          
          Ahí estaba, en la variante sin borde: el único botón de los cuatro
          que lleva a otra pantalla era también el de menos peso visual, y
          quedaba leyéndose como un control de fecha más. Las únicas flechas
          de esa fila eran las de cambiar de semana, que no van a ningún lado.

          `h-11 sm:h-9` es el mismo blanco táctil que usa la pantalla de "no
          encontrada" para su botón de volver: 44px en el teléfono, que es lo
          que pide WCAG 2.5.5 y verifica e2e/tactil.spec.ts. */}
      <Button asChild variant="outline" className="mb-3 h-11 px-4 sm:h-9">
        <Link to="/inventario">← Volver al inventario</Link>
      </Button>

      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          {/* Con el nombre del equipo: se llega desde un botón por equipo, y
              "Calendario del equipo" a secas hacía que tres pestañas abiertas
              desde tres máquinas distintas se vieran iguales. El servidor lo
              manda con el carro incluido, que es lo que distingue una "PC 7"
              de las otras dos. */}
          <h1 className="text-2xl font-semibold tracking-tight">
            {data?.etiqueta ? `Calendario de ${data.etiqueta}` : "Calendario del equipo"}
          </h1>
          <p className="text-muted-foreground text-sm">
            {/* El rango que se dibuja, no la semana entera: con una escuela
                de lunes a viernes el rótulo prometía dos días que no están. */}
            {formatearRangoVisible(dias)}
          </p>
        </div>
        {/* flex-wrap: los tres botones no entran en una línea en un
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
                    {/* Sin este rótulo la columna miente: un día que la
                        escuela declaró cerrado se vería igual que uno abierto,
                        y lo que quedó adentro parecería normal. */}
                    {diasCerrados.has(fecha) && (
                      <div className="text-muted-foreground text-[0.65rem] leading-tight font-normal">
                        fuera de la jornada
                      </div>
                    )}
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
