import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useParams } from "react-router"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import * as calendarioApi from "@/features/calendario/api"
import type { BloqueCalendario } from "@/features/calendario/types"
import {
  DIAS_HABILES,
  aFechaISO,
  aMinutos,
  desdeFechaISO,
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

  const esEvaluacion = bloque.tipo === "EVALUACION_ESTATAL"
  const alto = ((fin - inicio) / 60) * ALTO_POR_HORA_REM
  const desplazamiento = ((inicio - minutosBase) / 60) * ALTO_POR_HORA_REM

  return (
    <div
      className={
        "absolute inset-x-0.5 overflow-hidden rounded-md border px-1.5 py-1 text-xs leading-tight " +
        (esEvaluacion
          ? "border-destructive/40 bg-destructive/10 text-destructive"
          : "border-primary/30 bg-primary/10")
      }
      style={{ top: `${desplazamiento}rem`, height: `${alto}rem` }}
      title={
        esEvaluacion
          ? `Evaluación estatal · ${bloque.horaInicio}–${bloque.horaFin}`
          : `${bloque.materiaNombre} (${bloque.cursoNombre}) · ${bloque.docente} · ${bloque.horaInicio}–${bloque.horaFin}`
      }
    >
      <p className="truncate font-medium">
        {esEvaluacion ? "Evaluación estatal" : bloque.materiaNombre}
      </p>
      {!esEvaluacion && (
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

// RF-04.4: calendario completo de una PC, con nombre del docente, materia
// y horario. Lo puede ver cualquier usuario autenticado.
export function CalendarioPCPage() {
  const { pcId = "" } = useParams()
  const [referencia, setReferencia] = useState(() => new Date())

  const dias = useMemo(() => fechasDeLaSemana(referencia), [referencia])
  const desde = dias[0]
  const hasta = dias[dias.length - 1]

  const { data, isLoading, error } = useQuery({
    queryKey: ["calendario", pcId, desde, hasta],
    queryFn: () => calendarioApi.calendarioDePC(pcId, desde, hasta),
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
          <h1 className="text-2xl font-semibold tracking-tight">Calendario de la PC</h1>
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
          <div className="grid grid-cols-[3.5rem_repeat(6,1fr)] border-b">
            <div />
            {DIAS_HABILES.map((dia, i) => (
              <div
                key={dia}
                className={
                  "border-l px-2 py-1 text-center text-sm font-medium " +
                  (dias[i] === hoy ? "text-primary" : "")
                }
              >
                <div>{dia}</div>
                <div className="text-muted-foreground text-xs">
                  {desdeFechaISO(dias[i]).getDate()}
                </div>
              </div>
            ))}
          </div>

          <div className="grid grid-cols-[3.5rem_repeat(6,1fr)]">
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
          Bloqueo por evaluación estatal
        </span>
        <Badge variant="outline">Los huecos en blanco están libres</Badge>
      </div>
    </div>
  )
}
