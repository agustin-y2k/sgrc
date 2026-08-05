import { useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
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
import {
  formatearDuracion,
  formatearPorcentaje,
  proporcion,
} from "@/features/admin/types"
import { HistoricoPorAnio } from "@/features/admin/HistoricoPorAnio"
import { Proporcion, Seccion, sumar } from "@/features/admin/ReportesUI"
import { getErrorMessage } from "@/lib/api-client"
import { descargarCSV } from "@/lib/csv"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"

// RF-06: reportes de uso e incidencias del ciclo lectivo elegido. El
// histórico de los años ya cerrados vive en HistoricoPorAnio: se mueve por
// otro eje y no comparte ni estado ni consultas con esto.
export function ReportesPage() {
  const [cicloId, setCicloId] = useState("")
  const [desde, setDesde] = useState("")
  const [hasta, setHasta] = useState("")

  const { data: ciclos } = useQuery({
    queryKey: ["ciclos"],
    queryFn: adminApi.listarCiclos,
  })

  // El ciclo activo es el default razonable: es sobre el que se reporta
  // habitualmente (RF-06.1 habla del "ciclo lectivo activo").
  const listaCiclos = ciclos?.data ?? []
  const cicloElegido =
    cicloId || listaCiclos.find((c) => c.activo)?.id || listaCiclos[0]?.id || ""

  const usoPCs = useQuery({
    queryKey: ["reporte", "uso-pcs", cicloElegido, desde, hasta],
    queryFn: () =>
      adminApi.reporteUsoPCs(cicloElegido, desde || undefined, hasta || undefined),
    enabled: cicloElegido !== "",
  })

  const usoDocentes = useQuery({
    queryKey: ["reporte", "uso-docentes", cicloElegido, desde, hasta],
    queryFn: () =>
      adminApi.reporteUsoDocentes(cicloElegido, desde || undefined, hasta || undefined),
    enabled: cicloElegido !== "",
  })

  const incidenciasPC = useQuery({
    queryKey: ["reporte", "incidencias-pcs", desde, hasta],
    queryFn: () =>
      adminApi.reporteIncidenciasPorPC(desde || undefined, hasta || undefined),
  })

  const incidenciasCarro = useQuery({
    queryKey: ["reporte", "incidencias-carros", desde, hasta],
    queryFn: () =>
      adminApi.reporteIncidenciasPorCarro(desde || undefined, hasta || undefined),
  })

  const errorActivo =
    usoPCs.error ?? usoDocentes.error ?? incidenciasPC.error ?? incidenciasCarro.error

  /**
   * Las cuatro tablas, ordenadas de mayor a menor y con sus totales.
   *
   * El orden es parte del reporte, no una preferencia: lo que se busca acá
   * es "cuál se usa más" y "cuál se rompe más", y con las filas en el orden
   * en que las devolvió la base hay que leerlas todas para contestarlo. De
   * mayor a menor, la respuesta está en la primera fila.
   *
   * Los totales son el denominador que le faltaba a cada número: "18
   * reservas" no dice nada solo, "18 de 214" sí.
   */
  const filasUsoPCs = [...(usoPCs.data?.data ?? [])].sort(
    (a, b) => b.minutosReservados - a.minutosReservados
  )
  const totalMinutosPCs = sumar(filasUsoPCs, (f) => f.minutosReservados)
  const totalReservasPCs = sumar(filasUsoPCs, (f) => f.cantidadReservas)

  const filasUsoDocentes = [...(usoDocentes.data?.data ?? [])].sort(
    (a, b) => b.minutosReservados - a.minutosReservados
  )
  const totalMinutosDocentes = sumar(filasUsoDocentes, (f) => f.minutosReservados)
  const totalReservasDocentes = sumar(filasUsoDocentes, (f) => f.cantidadReservas)

  const filasIncidenciasPC = [...(incidenciasPC.data?.data ?? [])].sort(
    (a, b) => b.total - a.total
  )
  const totalIncidenciasPC = sumar(filasIncidenciasPC, (f) => f.total)
  const totalAbiertasPC = sumar(filasIncidenciasPC, (f) => f.abiertas)
  const totalGravesPC = sumar(filasIncidenciasPC, (f) => f.graves)

  const filasIncidenciasCarro = [...(incidenciasCarro.data?.data ?? [])].sort(
    (a, b) => b.total - a.total
  )
  const totalIncidenciasCarro = sumar(filasIncidenciasCarro, (f) => f.total)

  /**
   * Con qué período se guarda el archivo. Un `reportes.csv` en la carpeta
   * de Descargas, al lado de otros tres, no se distingue de los otros
   * tres: el nombre tiene que decir de qué ciclo y de qué rango es.
   */
  const anioDelCiclo = listaCiclos.find((c) => c.id === cicloElegido)?.anio ?? ""
  const rango = desde || hasta ? `_${desde || "inicio"}_a_${hasta || "hoy"}` : ""
  const sufijo = `${anioDelCiclo}${rango}`

  return (
    <div className="mx-auto max-w-4xl">
      <EncabezadoDePagina
        titulo="Reportes"
        descripcion="Uso de las PCs y de las horas reservadas por docente, por ciclo lectivo."
      />

      <div className="mb-4 grid gap-4 sm:grid-cols-3">
        <div className="grid gap-1.5">
          <Label htmlFor="ciclo">Ciclo lectivo</Label>
          <Select
            id="ciclo"
            value={cicloElegido}
            onChange={(e) => setCicloId(e.target.value)}
          >
            {listaCiclos.map((c) => (
              <option key={c.id} value={c.id}>
                {c.anio}
                {c.activo ? " (activo)" : c.archivado ? " (archivado)" : ""}
              </option>
            ))}
          </Select>
        </div>
        {/* RF-06.1: "filtrable por rango de fechas". */}
        <div className="grid gap-1.5">
          <Label htmlFor="desde">Desde</Label>
          <Input
            id="desde"
            type="date"
            value={desde}
            onChange={(e) => setDesde(e.target.value)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="hasta">Hasta</Label>
          <Input
            id="hasta"
            type="date"
            value={hasta}
            onChange={(e) => setHasta(e.target.value)}
          />
        </div>
      </div>

      {errorActivo && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(errorActivo)}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4">
        {/* RF-06.1 */}
        <Seccion
          titulo="Uso por PC"
          resumen={
            filasUsoPCs.length > 0 &&
            `${filasUsoPCs.length} ${filasUsoPCs.length === 1 ? "PC usada" : "PCs usadas"} · ${totalReservasPCs} reservas · ${formatearDuracion(totalMinutosPCs)} en total`
          }
          alDescargar={
            filasUsoPCs.length > 0
              ? () =>
                  descargarCSV(`uso-por-pc_${sufijo}`, [
                    ["PC", "Carro", "Reservas", "Minutos reservados", "% del total"],
                    ...filasUsoPCs.map((u) => [
                      `PC ${u.identificador}`,
                      u.carroNombre,
                      u.cantidadReservas,
                      u.minutosReservados,
                      formatearPorcentaje(
                        proporcion(u.minutosReservados, totalMinutosPCs)
                      ),
                    ]),
                  ])
              : undefined
          }
        >
          {filasUsoPCs.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No hay reservas en ese ciclo y rango. Si el ciclo ya se archivó, sus
              reservas se borraron: el dato está más abajo, en el histórico por año.
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
                {filasUsoPCs.map((u) => (
                  <TableRow key={u.pcId}>
                    <TableCell className="font-medium">PC {u.identificador}</TableCell>
                    <TableCell>{u.carroNombre}</TableCell>
                    <TableCell className="text-right">{u.cantidadReservas}</TableCell>
                    <TableCell className="text-right">
                      {formatearDuracion(u.minutosReservados)}
                    </TableCell>
                    <TableCell>
                      <Proporcion parte={u.minutosReservados} total={totalMinutosPCs} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Seccion>

        {/* RF-06.2 */}
        <Seccion
          titulo="Horas reservadas por docente"
          resumen={
            filasUsoDocentes.length > 0 &&
            `${filasUsoDocentes.length} ${filasUsoDocentes.length === 1 ? "docente" : "docentes"} · ${totalReservasDocentes} reservas · ${formatearDuracion(totalMinutosDocentes)} en total`
          }
          alDescargar={
            filasUsoDocentes.length > 0
              ? () =>
                  descargarCSV(`uso-por-docente_${sufijo}`, [
                    ["Docente", "Reservas", "Minutos reservados", "% del total"],
                    ...filasUsoDocentes.map((u) => [
                      u.nombreDocente,
                      u.cantidadReservas,
                      u.minutosReservados,
                      formatearPorcentaje(
                        proporcion(u.minutosReservados, totalMinutosDocentes)
                      ),
                    ]),
                  ])
              : undefined
          }
        >
          {filasUsoDocentes.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              Sin datos en ese ciclo y rango.
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
                {filasUsoDocentes.map((u) => (
                  <TableRow key={u.usuarioId}>
                    <TableCell className="font-medium">{u.nombreDocente}</TableCell>
                    <TableCell className="text-right">{u.cantidadReservas}</TableCell>
                    <TableCell className="text-right">
                      {formatearDuracion(u.minutosReservados)}
                    </TableCell>
                    <TableCell>
                      <Proporcion
                        parte={u.minutosReservados}
                        total={totalMinutosDocentes}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Seccion>

        {/* RF-06.3 */}
        <Seccion
          titulo="Incidencias por equipo"
          resumen={
            filasIncidenciasPC.length > 0 && (
              <>
                {totalIncidenciasPC} incidencias en {filasIncidenciasPC.length}{" "}
                {filasIncidenciasPC.length === 1 ? "equipo" : "equipos"} ·{" "}
                {totalAbiertasPC} sin resolver
                {totalGravesPC > 0 && ` · ${totalGravesPC} graves`}
              </>
            )
          }
          alDescargar={
            filasIncidenciasPC.length > 0
              ? () =>
                  descargarCSV(`incidencias-por-equipo${rango}`, [
                    ["PC", "Carro", "Total", "Abiertas", "Graves", "% del total"],
                    ...filasIncidenciasPC.map((x) => [
                      `PC ${x.identificador}`,
                      x.carroNombre,
                      x.total,
                      x.abiertas,
                      x.graves,
                      formatearPorcentaje(proporcion(x.total, totalIncidenciasPC)),
                    ]),
                  ])
              : undefined
          }
        >
          {filasIncidenciasPC.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No se registraron incidencias en ese rango.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>PC</TableHead>
                  <TableHead>Carro</TableHead>
                  <TableHead className="text-right">Total</TableHead>
                  <TableHead className="text-right">Abiertas</TableHead>
                  <TableHead className="text-right">Graves</TableHead>
                  <TableHead>Del total</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filasIncidenciasPC.map((x) => (
                  <TableRow key={x.pcId}>
                    <TableCell className="font-medium">PC {x.identificador}</TableCell>
                    <TableCell>{x.carroNombre}</TableCell>
                    <TableCell className="text-right">{x.total}</TableCell>
                    <TableCell className="text-right">{x.abiertas}</TableCell>
                    <TableCell className="text-right">
                      {x.graves > 0 ? (
                        <Badge variant="destructive">{x.graves}</Badge>
                      ) : (
                        x.graves
                      )}
                    </TableCell>
                    <TableCell>
                      <Proporcion parte={x.total} total={totalIncidenciasPC} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Seccion>

        {/* RF-06.3 */}
        <Seccion
          titulo="Incidencias por carro"
          resumen={
            filasIncidenciasCarro.length > 0 &&
            `${totalIncidenciasCarro} incidencias repartidas en ${filasIncidenciasCarro.length} ${filasIncidenciasCarro.length === 1 ? "carro" : "carros"}`
          }
          alDescargar={
            filasIncidenciasCarro.length > 0
              ? () =>
                  descargarCSV(`incidencias-por-carro${rango}`, [
                    ["Carro", "Total", "Abiertas", "Graves", "% del total"],
                    ...filasIncidenciasCarro.map((x) => [
                      x.carroNombre,
                      x.total,
                      x.abiertas,
                      x.graves,
                      formatearPorcentaje(proporcion(x.total, totalIncidenciasCarro)),
                    ]),
                  ])
              : undefined
          }
        >
          {filasIncidenciasCarro.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No se registraron incidencias en ese rango.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Carro</TableHead>
                  <TableHead className="text-right">Total</TableHead>
                  <TableHead className="text-right">Abiertas</TableHead>
                  <TableHead className="text-right">Graves</TableHead>
                  <TableHead>Del total</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filasIncidenciasCarro.map((x) => (
                  <TableRow key={x.carroId}>
                    <TableCell className="font-medium">{x.carroNombre}</TableCell>
                    <TableCell className="text-right">{x.total}</TableCell>
                    <TableCell className="text-right">{x.abiertas}</TableCell>
                    <TableCell className="text-right">{x.graves}</TableCell>
                    <TableCell>
                      <Proporcion parte={x.total} total={totalIncidenciasCarro} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Seccion>
      </div>
      <HistoricoPorAnio ciclos={listaCiclos} />
    </div>
  )
}
