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
  formatearRangoSemana,
  sumarDias,
} from "@/features/calendario/semana"
import { getErrorMessage } from "@/lib/api-client"

// La grilla arranca a las 7 y termina a las 22: cubre el horario escolar
// sin desperdiciar alto en horas en las que nunca hay clase.
const HORA_DESDE = 7
const HORA_HASTA = 22
const ALTO_POR_HORA_REM = 3

function BloqueOcupado({ bloque }: { bloque: BloqueCalendario }) {
  const inicio = aMinutos(bloque.horaInicio)
  const fin = aMinutos(bloque.horaFin)
  const minutosBase = HORA_DESDE * 60

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
      <p className="truncate font-medium">
        {esBloqueo ? motivo : bloque.materiaNombre}
      </p>
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
  // celda (que sería recorrer la lista completa seis veces).
  const bloquesPorDia = useMemo(() => {
    const mapa = new Map<string, BloqueCalendario[]>()
    for (const b of data?.bloques ?? []) {
      const delDia = mapa.get(b.fecha)
      if (delDia) delDia.push(b)
      else mapa.set(b.fecha, [b])
    }
    return mapa
  }, [data])

  const horas = Array.from({ length: HORA_HASTA - HORA_DESDE }, (_, i) => HORA_DESDE + i)
  const hoy = aFechaISO(new Date())

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold tracking-tight">Calendario del equipo</h1>
          <p className="text-muted-foreground text-sm">
            {formatearRangoSemana(referencia)}
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

      {/* La grilla scrollea horizontalmente en pantallas angostas en vez de
          desbordar la página (RNF-07). */}
      <div className="overflow-x-auto">
        <div className="min-w-[44rem]">
          <div className="grid border-b"
            style={{ gridTemplateColumns: `3.5rem repeat(${dias.length}, 1fr)` }}>
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

          <div className="grid"
            style={{ gridTemplateColumns: `3.5rem repeat(${dias.length}, 1fr)` }}>
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
                  <BloqueOcupado key={b.reservaId} bloque={b} />
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
    </div>
  )
}
