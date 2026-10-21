import { useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { Badge } from "@/components/ui/badge"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import * as adminApi from "@/features/admin/api"
import { Proporcion, Seccion, sumar } from "@/features/admin/ReportesUI"
import {
  formatearDuracion,
  formatearPorcentaje,
  proporcion,
} from "@/features/admin/types"
import type { Ciclo } from "@/features/admin/types"
import { descargarCSV } from "@/lib/csv"

/** RF-06.4 — las estadísticas de los años ya cerrados. */
export function HistoricoPorAnio({ ciclos }: { ciclos: Ciclo[] }) {
  const [anio, setAnio] = useState("")

  /**
   * Los años con histórico son exactamente los de los ciclos archivados: el
   * snapshot se calcula al archivar (RF-02.4) y se guarda bajo el año.
   */
  const aniosArchivados = ciclos
    .filter((c) => c.archivado)
    .map((c) => c.anio)
    .sort((a, b) => b - a)
  const anioElegido =
    (aniosArchivados.includes(Number(anio)) ? Number(anio) : 0) || aniosArchivados[0] || 0

  const historicoEquipos = useQuery({
    queryKey: ["reporte", "historico-equipos", anioElegido],
    queryFn: () => adminApi.historicoUsoEquipos(anioElegido),
    enabled: anioElegido !== 0,
  })

  const historicoDocentes = useQuery({
    queryKey: ["reporte", "historico-docentes", anioElegido],
    queryFn: () => adminApi.historicoUsoDocentes(anioElegido),
    enabled: anioElegido !== 0,
  })

  const equiposListos = historicoEquipos.isSuccess && !historicoEquipos.error
  const docentesListos = historicoDocentes.isSuccess && !historicoDocentes.error

  // Mismo criterio que las tablas del ciclo activo: de mayor a menor y con el
  // total a mano.
  const filasHistoricoEquipos = [...(historicoEquipos.data?.data ?? [])].sort(
    (a, b) => b.minutosReservados - a.minutosReservados
  )
  const totalMinutosHistoricoEquipos = sumar(
    filasHistoricoEquipos,
    (h) => h.minutosReservados
  )

  const filasHistoricoDocentes = [...(historicoDocentes.data?.data ?? [])].sort(
    (a, b) => b.minutosTotales - a.minutosTotales
  )
  const totalMinutosHistoricoDocentes = sumar(
    filasHistoricoDocentes,
    (h) => h.minutosTotales
  )

  return (
    <div className="mt-8 grid gap-4">
      <div className="flex flex-wrap items-end justify-between gap-2">
        {/* RF-06.4 */}
        <h2 className="text-xl font-semibold">Histórico por año</h2>
        {aniosArchivados.length > 0 && (
          <div className="grid gap-1.5">
            <Label htmlFor="anio">Año</Label>
            <Select
              id="anio"
              className="w-auto"
              value={anioElegido}
              onChange={(e) => setAnio(e.target.value)}
            >
              {aniosArchivados.map((a) => (
                <option key={a} value={a}>
                  {a}
                </option>
              ))}
            </Select>
          </div>
        )}
      </div>

      {aniosArchivados.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          Todavía no se archivó ningún ciclo lectivo, así que no hay histórico. Se genera
          solo, al archivar un ciclo desde Académico.
        </p>
      ) : (
        <>
          <p className="text-muted-foreground text-sm">
            El año se elige acá y no usa los filtros de fechas de arriba. Los nombres son
            los que tenían al cerrar el año: un equipo pudo haberse mudado de carro o
            darse de baja desde entonces, y el histórico igual tiene que seguir diciendo
            dónde estaba.
          </p>

          <Seccion
            titulo={`Uso por equipo en ${anioElegido}`}
            resumen={
              equiposListos &&
              filasHistoricoEquipos.length > 0 &&
              `${filasHistoricoEquipos.length} equipos · ${sumar(filasHistoricoEquipos, (h) => h.cantidadReservas)} reservas · ${formatearDuracion(totalMinutosHistoricoEquipos)} en total`
            }
            alDescargar={
              equiposListos &&
              !historicoEquipos.isFetching &&
              filasHistoricoEquipos.length > 0
                ? () =>
                    descargarCSV(`historico-uso-por-equipo_${anioElegido}`, [
                      [
                        "Equipo",
                        "Carro",
                        "Reservas",
                        "Minutos reservados",
                        "% del total",
                      ],
                      ...filasHistoricoEquipos.map((h) => [
                        h.etiquetaSnapshot,
                        h.carroNombreSnapshot,
                        h.cantidadReservas,
                        h.minutosReservados,
                        formatearPorcentaje(
                          proporcion(h.minutosReservados, totalMinutosHistoricoEquipos)
                        ),
                      ]),
                    ])
                : undefined
            }
          >
            {historicoEquipos.error ? (
              <ErrorDeConsulta
                mensaje="No pudimos consultar el histórico de equipos de este año."
                reintentando={historicoEquipos.isFetching}
                onReintentar={() => void historicoEquipos.refetch()}
              />
            ) : !equiposListos ? (
              <p role="status" className="text-muted-foreground text-sm">
                Consultando histórico de equipos…
              </p>
            ) : filasHistoricoEquipos.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                Ese año se archivó sin ninguna reserva registrada.
              </p>
            ) : (
              <Table aria-label={`Histórico de uso por equipo en ${anioElegido}`}>
                <TableHeader>
                  <TableRow>
                    <TableHead>Equipo</TableHead>
                    <TableHead>Carro</TableHead>
                    <TableHead className="text-right">Reservas</TableHead>
                    <TableHead className="text-right">Tiempo reservado</TableHead>
                    <TableHead>Del total</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filasHistoricoEquipos.map((h) => (
                    <TableRow key={h.id}>
                      <TableCell className="font-medium">{h.etiquetaSnapshot}</TableCell>
                      <TableCell>{h.carroNombreSnapshot}</TableCell>
                      <TableCell className="text-right">{h.cantidadReservas}</TableCell>
                      <TableCell className="text-right">
                        {formatearDuracion(h.minutosReservados)}
                      </TableCell>
                      <TableCell>
                        <Proporcion
                          parte={h.minutosReservados}
                          total={totalMinutosHistoricoEquipos}
                        />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Seccion>

          <Seccion
            titulo={`Uso por docente en ${anioElegido}`}
            resumen={
              docentesListos &&
              filasHistoricoDocentes.length > 0 &&
              `${filasHistoricoDocentes.length} docentes · ${sumar(filasHistoricoDocentes, (h) => h.cantidadReservas)} reservas · ${formatearDuracion(totalMinutosHistoricoDocentes)} en total`
            }
            alDescargar={
              docentesListos &&
              !historicoDocentes.isFetching &&
              filasHistoricoDocentes.length > 0
                ? () =>
                    descargarCSV(`historico-uso-por-docente_${anioElegido}`, [
                      ["Docente", "Reservas", "Minutos totales", "% del total"],
                      ...filasHistoricoDocentes.map((h) => [
                        h.nombreDocenteSnapshot,
                        h.cantidadReservas,
                        h.minutosTotales,
                        formatearPorcentaje(
                          proporcion(h.minutosTotales, totalMinutosHistoricoDocentes)
                        ),
                      ]),
                    ])
                : undefined
            }
          >
            {historicoDocentes.error ? (
              <ErrorDeConsulta
                mensaje="No pudimos consultar el histórico de docentes de este año."
                reintentando={historicoDocentes.isFetching}
                onReintentar={() => void historicoDocentes.refetch()}
              />
            ) : !docentesListos ? (
              <p role="status" className="text-muted-foreground text-sm">
                Consultando histórico de docentes…
              </p>
            ) : filasHistoricoDocentes.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                Ese año se archivó sin ninguna reserva registrada.
              </p>
            ) : (
              <Table aria-label={`Histórico de uso por docente en ${anioElegido}`}>
                <TableHeader>
                  <TableRow>
                    <TableHead>Docente</TableHead>
                    <TableHead className="text-right">Reservas</TableHead>
                    <TableHead className="text-right">Tiempo reservado</TableHead>
                    <TableHead>Del total</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {/* La clave es el id del snapshot, no usuarioId: la
                        cuenta pudo eliminarse (RF-01.9) y la columna quedó
                        en NULL. El nombre sobrevive igual. */}
                  {filasHistoricoDocentes.map((h) => (
                    <TableRow key={h.id}>
                      <TableCell className="font-medium">
                        {h.nombreDocenteSnapshot}
                        {!h.usuarioId && (
                          <Badge variant="outline" className="ml-2">
                            Cuenta eliminada
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-right">{h.cantidadReservas}</TableCell>
                      <TableCell className="text-right">
                        {formatearDuracion(h.minutosTotales)}
                      </TableCell>
                      <TableCell>
                        <Proporcion
                          parte={h.minutosTotales}
                          total={totalMinutosHistoricoDocentes}
                        />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Seccion>
        </>
      )}
    </div>
  )
}
