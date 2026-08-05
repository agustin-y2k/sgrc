import { useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
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
import { getErrorMessage } from "@/lib/api-client"
import { descargarCSV } from "@/lib/csv"

/**
 * RF-06.4 — las estadísticas de los años ya cerrados.
 *
 * Vive aparte del resto del reporte porque se mueve en otro eje: por año
 * archivado y no por ciclo, y sin rango de fechas. Los números ya vienen
 * agregados y las reservas que los produjeron fueron borradas al archivar
 * (RF-02.4), así que no hay nada que filtrar ni recalcular.
 */
export function HistoricoPorAnio({ ciclos }: { ciclos: Ciclo[] }) {
  const [anio, setAnio] = useState("")

  /**
   * Los años con histórico son exactamente los de los ciclos archivados: el
   * snapshot se calcula al archivar (RF-02.4) y se guarda bajo el año. Del
   * más reciente al más viejo, que es el que se suele mirar.
   */
  const aniosArchivados = ciclos
    .filter((c) => c.archivado)
    .map((c) => c.anio)
    .sort((a, b) => b - a)
  const anioElegido = Number(anio) || aniosArchivados[0] || 0

  const historicoPCs = useQuery({
    queryKey: ["reporte", "historico-pcs", anioElegido],
    queryFn: () => adminApi.historicoUsoPCs(anioElegido),
    enabled: anioElegido !== 0,
  })

  const historicoDocentes = useQuery({
    queryKey: ["reporte", "historico-docentes", anioElegido],
    queryFn: () => adminApi.historicoUsoDocentes(anioElegido),
    enabled: anioElegido !== 0,
  })

  const errorHistorico = historicoPCs.error ?? historicoDocentes.error

  // Mismo criterio que las tablas del ciclo activo: de mayor a menor y con
  // el total a mano. Ojo con el nombre del campo de tiempo — acá es
  // `minutosTotales`, no `minutosReservados` (ver types.ts).
  const filasHistoricoPCs = [...(historicoPCs.data?.data ?? [])].sort(
    (a, b) => b.minutosReservados - a.minutosReservados
  )
  const totalMinutosHistoricoPCs = sumar(filasHistoricoPCs, (h) => h.minutosReservados)

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
            Los nombres son los que tenían al cerrar el año: una PC pudo haberse mudado de
            carro o darse de baja desde entonces, y el histórico igual tiene que seguir
            diciendo dónde estaba.
          </p>

          {errorHistorico && (
            <Alert variant="destructive">
              <AlertDescription>{getErrorMessage(errorHistorico)}</AlertDescription>
            </Alert>
          )}

          <Seccion
            titulo={`Uso por PC en ${anioElegido}`}
            resumen={
              filasHistoricoPCs.length > 0 &&
              `${filasHistoricoPCs.length} PCs · ${sumar(filasHistoricoPCs, (h) => h.cantidadReservas)} reservas · ${formatearDuracion(totalMinutosHistoricoPCs)} en total`
            }
            alDescargar={
              filasHistoricoPCs.length > 0
                ? () =>
                    descargarCSV(`historico-uso-por-pc_${anioElegido}`, [
                      ["PC", "Carro", "Reservas", "Minutos reservados", "% del total"],
                      ...filasHistoricoPCs.map((h) => [
                        `PC ${h.identificadorSnapshot}`,
                        h.carroNombreSnapshot,
                        h.cantidadReservas,
                        h.minutosReservados,
                        formatearPorcentaje(
                          proporcion(h.minutosReservados, totalMinutosHistoricoPCs)
                        ),
                      ]),
                    ])
                : undefined
            }
          >
            {filasHistoricoPCs.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                Ese año se archivó sin ninguna reserva registrada.
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>PC</TableHead>
                    <TableHead>Carro</TableHead>
                    <TableHead className="text-right">Reservas</TableHead>
                    <TableHead className="text-right">Tiempo reservado</TableHead>
                    <TableHead>Del total</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filasHistoricoPCs.map((h) => (
                    <TableRow key={h.id}>
                      <TableCell className="font-medium">
                        PC {h.identificadorSnapshot}
                      </TableCell>
                      <TableCell>{h.carroNombreSnapshot}</TableCell>
                      <TableCell className="text-right">{h.cantidadReservas}</TableCell>
                      <TableCell className="text-right">
                        {formatearDuracion(h.minutosReservados)}
                      </TableCell>
                      <TableCell>
                        <Proporcion
                          parte={h.minutosReservados}
                          total={totalMinutosHistoricoPCs}
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
              filasHistoricoDocentes.length > 0 &&
              `${filasHistoricoDocentes.length} docentes · ${sumar(filasHistoricoDocentes, (h) => h.cantidadReservas)} reservas · ${formatearDuracion(totalMinutosHistoricoDocentes)} en total`
            }
            alDescargar={
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
            {filasHistoricoDocentes.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                Ese año se archivó sin ninguna reserva registrada.
              </p>
            ) : (
              <Table>
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
