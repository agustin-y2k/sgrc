import { useQuery } from "@tanstack/react-query"
import { descargarCSV } from "@/lib/csv"
import { Seccion } from "./ReportesUI"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

import { consultarActividad, type Metricas } from "./actividad-api"
export function ActividadDelCiclo({
  cicloId,
  desde,
  hasta,
  archivado,
  pausa,
}: {
  cicloId: string
  desde: string
  hasta: string
  archivado: boolean
  pausa?: string
}) {
  const consulta = useQuery({
    queryKey: [
      "reporte",
      "actividad",
      cicloId,
      archivado ? "" : desde,
      archivado ? "" : hasta,
    ],
    queryFn: () =>
      consultarActividad(
        cicloId,
        archivado ? undefined : desde || undefined,
        archivado ? undefined : hasta || undefined
      ),
    enabled: !!cicloId && !pausa,
  })
  const a = consulta.data
  const m = a?.totales
  const cabecera = [
    "Nombre",
    "Reservas solicitadas",
    "Reservas atendidas",
    "No retiradas",
    "Canceladas",
    "Préstamos con reserva",
    "Préstamos sin reserva",
    "Origen no determinado",
    "Abiertos",
    "Devueltos",
  ]
  const valores = (m: Metricas) => [
    m.reservasSolicitadas,
    m.reservasAtendidas,
    m.reservasNoRetiradas,
    m.reservasCanceladas,
    m.prestamosConReserva,
    m.prestamosSinReserva,
    m.prestamosOrigenNoDeterminado,
    m.prestamosAbiertos,
    m.prestamosDevueltos,
  ]
  return (
    <Seccion
      titulo="Reservas y entregas efectivas"
      consulta={consulta}
      pausa={pausa}
      alcance={
        archivado
          ? "Resumen sellado del año completo. Los filtros de fechas no modifican el histórico."
          : "Reservas del ciclo y préstamos registrados. Cada movimiento corresponde a un equipo."
      }
      alDescargar={
        a?.disponible
          ? () =>
              descargarCSV(`actividad_${a.anio}`, [
                ["Desglose", ...cabecera],
                ["Total", "Institución", ...valores(a.totales)],
                ...a.equipos.map((e) => ["Equipo", e.etiqueta, ...valores(e)]),
                ...a.responsables.map((r) => ["Responsable", r.nombre, ...valores(r)]),
              ])
          : undefined
      }
    >
      {a && !a.disponible ? (
        <p>
          Este cierre es anterior al resumen de entregas efectivas. Sus cifras originales
          se conservan; no se reconstruyen automáticamente.
        </p>
      ) : (
        m && (
          <>
            <dl className="grid gap-3 sm:grid-cols-2">
              {[
                ["Reservas solicitadas", m.reservasSolicitadas],
                ["Reservas atendidas", m.reservasAtendidas],
                ["Reservas no retiradas", m.reservasNoRetiradas],
                ["Reservas canceladas", m.reservasCanceladas],
                ["Préstamos con reserva", m.prestamosConReserva],
                ["Préstamos sin reserva", m.prestamosSinReserva],
                ["Préstamos de origen no determinado", m.prestamosOrigenNoDeterminado],
                [
                  a.historico ? "Abiertos al cierre" : "Préstamos abiertos",
                  m.prestamosAbiertos,
                ],
                ["Préstamos devueltos", m.prestamosDevueltos],
              ].map(([label, valor]) => (
                <div key={label} className="rounded-md border p-3">
                  <dt className="text-muted-foreground text-sm">{label}</dt>
                  <dd className="text-xl font-semibold">{valor}</dd>
                </div>
              ))}
            </dl>
            <p className="text-muted-foreground my-3 text-sm">
              Una reserva atendida se cuenta una vez aunque tenga varios retiros. Cada
              préstamo conserva su movimiento; los préstamos y las reservas son medidas
              distintas y no se suman como usos adicionales. Un préstamo sin reserva al
              mismo docente no convierte una reserva no retirada en atendida.
            </p>
            <p className="text-muted-foreground mb-3 text-sm">
              Las entregas con contexto pertenecen a su ciclo; las independientes se
              ubican por el año local del retiro. Los préstamos antiguos que perdieron su
              vínculo se muestran con origen no determinado. Las personas sin cuenta se
              conservan por movimiento y no se fusionan por nombre.
            </p>
            {[
              {
                titulo: "Entregas por equipo",
                filas: a.equipos.map((e) => ({
                  id: e.equipoId,
                  nombre: e.etiqueta,
                  m: e,
                })),
              },
              {
                titulo: "Entregas por responsable",
                filas: a.responsables.map((r) => ({
                  id: r.identidad,
                  nombre: r.nombre,
                  m: r,
                })),
              },
            ].map((tabla) => (
              <div key={tabla.titulo} className="mt-4">
                <h3 className="mb-2 font-medium">{tabla.titulo}</h3>
                <Table aria-label={tabla.titulo}>
                  <TableHeader>
                    <TableRow>
                      <TableHead>
                        {tabla.titulo === "Entregas por equipo"
                          ? "Equipo"
                          : "Responsable"}
                      </TableHead>
                      <TableHead>Reservas atendidas</TableHead>
                      <TableHead>Con reserva</TableHead>
                      <TableHead>Sin reserva</TableHead>
                      <TableHead>Origen no determinado</TableHead>
                      <TableHead>Abiertos</TableHead>
                      <TableHead>Devueltos</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tabla.filas.length ? (
                      tabla.filas.map((f) => (
                        <TableRow key={f.id}>
                          <TableCell>{f.nombre}</TableCell>
                          {[
                            f.m.reservasAtendidas,
                            f.m.prestamosConReserva,
                            f.m.prestamosSinReserva,
                            f.m.prestamosOrigenNoDeterminado,
                            f.m.prestamosAbiertos,
                            f.m.prestamosDevueltos,
                          ].map((v, i) => (
                            <TableCell key={i}>{v}</TableCell>
                          ))}
                        </TableRow>
                      ))
                    ) : (
                      <TableRow>
                        <TableCell colSpan={7}>
                          Sin actividad registrada en este período.
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            ))}
          </>
        )
      )}
    </Seccion>
  )
}
