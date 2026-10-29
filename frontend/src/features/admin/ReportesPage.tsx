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
import { ActividadDelCiclo } from "./ActividadDelCiclo"
import { HistoricoPorAnio } from "@/features/admin/HistoricoPorAnio"
import { Proporcion, Seccion, sumar } from "@/features/admin/ReportesUI"
import { ETIQUETA_ESTADO_EQUIPO } from "@/features/inventory/types"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { errorRangoFechas } from "@/lib/rango-fechas"
import { descargarCSV } from "@/lib/csv"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"

// RF-06: reportes de uso e incidencias del ciclo lectivo elegido.
export function ReportesPage() {
  const [cicloId, setCicloId] = useState("")
  const [desde, setDesde] = useState("")
  const [hasta, setHasta] = useState("")

  const consultaCiclos = useQuery({
    queryKey: ["ciclos"],
    queryFn: adminApi.listarCiclos,
  })

  // El ciclo activo es el default razonable: es sobre el que se reporta
  // habitualmente (RF-06.1 habla del "ciclo lectivo activo").
  const listaCiclos = consultaCiclos.data?.data ?? []
  const ciclosListos = consultaCiclos.isSuccess && !consultaCiclos.error
  const errorFechas = errorRangoFechas(desde, hasta)
  const cicloElegido =
    (listaCiclos.some((c) => c.id === cicloId) ? cicloId : "") ||
    listaCiclos.find((c) => c.activo)?.id ||
    listaCiclos[0]?.id ||
    ""

  const usoEquipos = useQuery({
    queryKey: ["reporte", "uso-equipos", cicloElegido, desde, hasta],
    queryFn: () =>
      adminApi.reporteUsoEquipos(cicloElegido, desde || undefined, hasta || undefined),
    enabled: ciclosListos && cicloElegido !== "" && !errorFechas,
  })

  const usoDocentes = useQuery({
    queryKey: ["reporte", "uso-docentes", cicloElegido, desde, hasta],
    queryFn: () =>
      adminApi.reporteUsoDocentes(cicloElegido, desde || undefined, hasta || undefined),
    enabled: ciclosListos && cicloElegido !== "" && !errorFechas,
  })

  const incidenciasEquipo = useQuery({
    queryKey: ["reporte", "incidencias-equipos", desde, hasta],
    queryFn: () =>
      adminApi.reporteIncidenciasPorEquipo(desde || undefined, hasta || undefined),
    enabled: !errorFechas,
  })

  const incidenciasCarro = useQuery({
    queryKey: ["reporte", "incidencias-carros", desde, hasta],
    queryFn: () =>
      adminApi.reporteIncidenciasPorCarro(desde || undefined, hasta || undefined),
    enabled: !errorFechas,
  })

  // Los dos del parque NO llevan rango de fechas: describen la situación de
  // hoy.
  const estadoInventario = useQuery({
    queryKey: ["reporte", "estado-inventario"],
    queryFn: adminApi.reporteEstadoDelInventario,
  })

  const fueraDeCirculacion = useQuery({
    queryKey: ["reporte", "fuera-de-circulacion"],
    queryFn: adminApi.reporteEquiposFueraDeCirculacion,
  })

  const porCategoria = useQuery({
    queryKey: ["reporte", "incidencias-categorias", desde, hasta],
    queryFn: () =>
      adminApi.reporteIncidenciasPorCategoria(desde || undefined, hasta || undefined),
    enabled: !errorFechas,
  })

  const pausaUso =
    errorFechas ||
    (!ciclosListos
      ? "Esperando la consulta de ciclos lectivos."
      : !cicloElegido
        ? "Creá un ciclo lectivo para consultar su uso."
        : undefined)

  /** Las cuatro tablas, ordenadas de mayor a menor y con sus totales. */
  const filasUsoEquipos = [...(usoEquipos.data?.data ?? [])].sort(
    (a, b) => b.minutosReservados - a.minutosReservados
  )
  const totalMinutosEquipos = sumar(filasUsoEquipos, (f) => f.minutosReservados)
  const totalReservasEquipos = sumar(filasUsoEquipos, (f) => f.cantidadReservas)

  const filasUsoDocentes = [...(usoDocentes.data?.data ?? [])].sort(
    (a, b) => b.minutosReservados - a.minutosReservados
  )
  const totalMinutosDocentes = sumar(filasUsoDocentes, (f) => f.minutosReservados)
  const totalReservasDocentes = sumar(filasUsoDocentes, (f) => f.cantidadReservas)

  const filasIncidenciasEquipo = [...(incidenciasEquipo.data?.data ?? [])].sort(
    (a, b) => b.total - a.total
  )
  const totalIncidenciasEquipo = sumar(filasIncidenciasEquipo, (f) => f.total)
  const totalAbiertasEquipo = sumar(
    filasIncidenciasEquipo,
    (f) => f.abiertas + f.enReparacion + f.enviadasASoporte
  )
  const totalGravesEquipo = sumar(filasIncidenciasEquipo, (f) => f.graves)

  const filasIncidenciasCarro = [...(incidenciasCarro.data?.data ?? [])].sort(
    (a, b) => b.total - a.total
  )
  const totalIncidenciasCarro = sumar(filasIncidenciasCarro, (f) => f.total)

  // ── El parque de equipos (RF-06.5) ────────────────────────────────────
  const filasInventario = estadoInventario.data?.data ?? []
  const parqueTotal = sumar(filasInventario, (f) => f.total)
  const parqueDisponible = sumar(filasInventario, (f) => f.disponibles)
  const parqueMantenimiento = sumar(filasInventario, (f) => f.enMantenimiento)
  const parqueFueraDeServicio = sumar(filasInventario, (f) => f.fueraDeServicio)
  const parqueAfuera = parqueMantenimiento + parqueFueraDeServicio

  const filasFuera = fueraDeCirculacion.data?.data ?? []
  const sinDiagnosticar = filasFuera.filter((f) => !f.categoria).length

  const filasCategoria = porCategoria.data?.data ?? []
  const totalPorCategoria = sumar(filasCategoria, (f) => f.total)

  /** Con qué período se guarda el archivo. */
  const anioDelCiclo = listaCiclos.find((c) => c.id === cicloElegido)?.anio ?? ""
  const rango = desde || hasta ? `_${desde || "inicio"}_a_${hasta || "hoy"}` : ""
  const sufijo = `${anioDelCiclo}${rango}`

  return (
    <div className="mx-auto max-w-4xl">
      <EncabezadoDePagina
        titulo="Reportes"
        descripcion="Uso de los equipos y de las horas reservadas por docente, por ciclo lectivo."
      />

      <div className="mb-4 grid gap-4 sm:grid-cols-3">
        <div className="grid gap-1.5">
          <Label htmlFor="ciclo">Ciclo lectivo</Label>
          <Select
            id="ciclo"
            value={cicloElegido}
            disabled={!ciclosListos || listaCiclos.length === 0}
            onChange={(e) => setCicloId(e.target.value)}
          >
            {!ciclosListos || listaCiclos.length === 0 ? (
              <option value="">
                {consultaCiclos.error
                  ? "No se pudieron consultar los ciclos"
                  : consultaCiclos.isPending
                    ? "Cargando ciclos…"
                    : "Sin ciclos lectivos"}
              </option>
            ) : null}
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
            max="9999-12-31"
            aria-invalid={!!errorFechas}
            aria-describedby={errorFechas ? "error-fechas-reportes" : undefined}
            value={desde}
            onChange={(e) => setDesde(e.target.value)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="hasta">Hasta</Label>
          <Input
            id="hasta"
            type="date"
            max="9999-12-31"
            aria-invalid={!!errorFechas}
            aria-describedby={errorFechas ? "error-fechas-reportes" : undefined}
            value={hasta}
            onChange={(e) => setHasta(e.target.value)}
          />
        </div>
      </div>

      {consultaCiclos.error && (
        <div className="mb-4">
          <ErrorDeConsulta
            mensaje="No pudimos consultar los ciclos lectivos."
            reintentando={consultaCiclos.isFetching}
            onReintentar={() => void consultaCiclos.refetch()}
          />
        </div>
      )}
      {errorFechas && (
        <Alert variant="destructive" className="mb-4" id="error-fechas-reportes">
          <AlertDescription>{errorFechas}</AlertDescription>
        </Alert>
      )}
      <p className="text-muted-foreground mb-4 text-sm">
        El ciclo filtra el uso por equipo y docente. Las fechas también filtran las
        incidencias, de todos los ciclos. El estado del inventario muestra la situación
        actual. El histórico tiene su propio selector de año y no usa estas fechas.
      </p>
      <p className="text-muted-foreground mb-4 text-sm">
        Se cuenta cada equipo reservado: una clase de 10 equipos durante una hora suma 10
        reservas y 10 horas de equipo, no 10 horas de clase. Se incluyen reservas vigentes
        y finalizadas, además de las no retiradas que tuvieron una entrega vinculada. Se
        excluyen canceladas y no retiradas sin entrega. El uso por equipo incluye bloqueos
        institucionales.
      </p>

      <div className="grid gap-4">
        <ActividadDelCiclo
          cicloId={cicloElegido}
          desde={desde}
          hasta={hasta}
          archivado={!!listaCiclos.find((c) => c.id === cicloElegido)?.archivado}
          pausa={pausaUso}
        />
        {/* RF-06.1 */}
        <Seccion
          titulo="Uso por equipo"
          consulta={usoEquipos}
          alcance="Ciclo y rango seleccionados. Tiempo reservado, no medición de uso efectivo."
          pausa={pausaUso}
          resumen={
            filasUsoEquipos.length > 0 &&
            `${filasUsoEquipos.length} ${filasUsoEquipos.length === 1 ? "equipo usado" : "equipos usados"} · ${totalReservasEquipos} reservas · ${formatearDuracion(totalMinutosEquipos)} en total`
          }
          alDescargar={
            filasUsoEquipos.length > 0
              ? () =>
                  descargarCSV(`uso-por-equipo_${sufijo}`, [
                    ["Equipo", "Carro", "Reservas", "Minutos reservados", "% del total"],
                    ...filasUsoEquipos.map((u) => [
                      u.etiqueta,
                      u.carroNombre,
                      u.cantidadReservas,
                      u.minutosReservados,
                      formatearPorcentaje(
                        proporcion(u.minutosReservados, totalMinutosEquipos)
                      ),
                    ]),
                  ])
              : undefined
          }
        >
          {filasUsoEquipos.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No hay reservas en ese ciclo y rango. Si el ciclo ya se archivó, sus
              reservas se borraron: el dato está más abajo, en el histórico por año.
            </p>
          ) : (
            <Table aria-label="Uso por equipo">
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
                {filasUsoEquipos.map((u) => (
                  <TableRow key={u.equipoId}>
                    <TableCell className="font-medium">{u.etiqueta}</TableCell>
                    <TableCell>{u.carroNombre}</TableCell>
                    <TableCell className="text-right">{u.cantidadReservas}</TableCell>
                    <TableCell className="text-right">
                      {formatearDuracion(u.minutosReservados)}
                    </TableCell>
                    <TableCell>
                      <Proporcion
                        parte={u.minutosReservados}
                        total={totalMinutosEquipos}
                      />
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
          consulta={usoDocentes}
          alcance="Tiempo total de equipos reservados por cada docente."
          pausa={pausaUso}
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
            <Table aria-label="Horas reservadas por docente">
              <TableHeader>
                <TableRow>
                  <TableHead>Docente</TableHead>
                  <TableHead className="text-right">Reservas</TableHead>
                  <TableHead className="text-right">Tiempo reservado</TableHead>
                  <TableHead>Del total</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filasUsoDocentes.map((u, index) => (
                  <TableRow key={u.usuarioId ?? `eliminado-${index}`}>
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
          consulta={incidenciasEquipo}
          alcance="Todos los ciclos. Fecha de apertura de la incidencia."
          pausa={errorFechas}
          resumen={
            filasIncidenciasEquipo.length > 0 && (
              <>
                {totalIncidenciasEquipo} incidencias en {filasIncidenciasEquipo.length}{" "}
                {filasIncidenciasEquipo.length === 1 ? "equipo" : "equipos"} ·{" "}
                {totalAbiertasEquipo} sin resolver
                {totalGravesEquipo > 0 && ` · ${totalGravesEquipo} graves`}
              </>
            )
          }
          alDescargar={
            filasIncidenciasEquipo.length > 0
              ? () =>
                  descargarCSV(`incidencias-por-equipo${rango}`, [
                    [
                      "Equipo",
                      "Carro",
                      "Total",
                      "Abiertas",
                      "En reparación",
                      "En soporte",
                      "Resueltas",
                      "Graves",
                      "% del total",
                    ],
                    ...filasIncidenciasEquipo.map((x) => [
                      x.etiqueta,
                      x.carroNombre,
                      x.total,
                      x.abiertas,
                      x.enReparacion,
                      x.enviadasASoporte,
                      x.resueltas,
                      x.graves,
                      formatearPorcentaje(proporcion(x.total, totalIncidenciasEquipo)),
                    ]),
                  ])
              : undefined
          }
        >
          {filasIncidenciasEquipo.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No se registraron incidencias en ese rango.
            </p>
          ) : (
            <Table aria-label="Incidencias por equipo">
              <TableHeader>
                <TableRow>
                  <TableHead>Equipo</TableHead>
                  <TableHead>Carro</TableHead>
                  <TableHead className="text-right">Total</TableHead>
                  <TableHead className="text-right">Abiertas</TableHead>
                  {/* El desglose por estado lo devuelve el backend desde
                      siempre y esta tabla lo tiraba. Es la diferencia entre
                      "12 incidencias" y "12, de las cuales 3 sin tocar, 2 en
                      el taller y 7 resueltas": lo segundo dice si el problema
                      está atendido o esperando. */}
                  <TableHead className="text-right">En reparación</TableHead>
                  <TableHead className="text-right">En soporte</TableHead>
                  <TableHead className="text-right">Resueltas</TableHead>
                  <TableHead className="text-right">Graves</TableHead>
                  <TableHead>Del total</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filasIncidenciasEquipo.map((x) => (
                  <TableRow key={x.equipoId}>
                    <TableCell className="font-medium">{x.etiqueta}</TableCell>
                    <TableCell>{x.carroNombre}</TableCell>
                    <TableCell className="text-right">{x.total}</TableCell>
                    <TableCell className="text-right">{x.abiertas}</TableCell>
                    <TableCell className="text-right">{x.enReparacion}</TableCell>
                    <TableCell className="text-right">{x.enviadasASoporte}</TableCell>
                    <TableCell className="text-right">{x.resueltas}</TableCell>
                    <TableCell className="text-right">
                      {x.graves > 0 ? (
                        <Badge variant="destructive">{x.graves}</Badge>
                      ) : (
                        x.graves
                      )}
                    </TableCell>
                    <TableCell>
                      <Proporcion parte={x.total} total={totalIncidenciasEquipo} />
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
          consulta={incidenciasCarro}
          alcance="Todos los ciclos. Fecha de apertura de la incidencia; carro actual del equipo."
          pausa={errorFechas}
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
            <Table aria-label="Incidencias por carro">
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

        {/* RF-06.5 — el estado del parque. Va después de las cuatro de uso
            porque responde otra pregunta: aquellas miran un período, estas
            tres miran cómo está el inventario ahora. */}
        <Seccion
          titulo="Estado del parque de equipos"
          consulta={estadoInventario}
          alcance="Situación actual; no cambia con los filtros de ciclo o fechas. Disponible indica el estado del inventario: un equipo puede estar reservado o prestado."
          resumen={
            parqueTotal > 0 &&
            `${parqueDisponible} de ${parqueTotal} disponibles · ${parqueAfuera} fuera de circulación`
          }
          alDescargar={
            filasInventario.length > 0
              ? () =>
                  descargarCSV(`estado-inventario_actual.csv`, [
                    [
                      "Carro",
                      "Disponibles",
                      "En mantenimiento",
                      "Fuera de servicio",
                      "Total",
                    ],
                    ...filasInventario.map((f) => [
                      f.carroNombre || "Sin carro",
                      f.disponibles,
                      f.enMantenimiento,
                      f.fueraDeServicio,
                      f.total,
                    ]),
                  ])
              : undefined
          }
        >
          {filasInventario.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No hay ningún equipo cargado todavía.
            </p>
          ) : (
            <Table aria-label="Estado del parque de equipos">
              <TableHeader>
                <TableRow>
                  <TableHead>Carro</TableHead>
                  <TableHead className="text-right">Disponibles</TableHead>
                  <TableHead className="text-right">En mantenimiento</TableHead>
                  <TableHead className="text-right">Fuera de servicio</TableHead>
                  <TableHead className="text-right">Total</TableHead>
                  <TableHead className="w-40">Disponibilidad</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filasInventario.map((f) => (
                  <TableRow key={f.carroId || "sueltos"}>
                    <TableCell className="font-medium">
                      {f.carroNombre || "Sin carro"}
                    </TableCell>
                    <TableCell className="text-right">{f.disponibles}</TableCell>
                    <TableCell className="text-right">{f.enMantenimiento}</TableCell>
                    <TableCell className="text-right">{f.fueraDeServicio}</TableCell>
                    <TableCell className="text-right">{f.total}</TableCell>
                    <TableCell>
                      <Proporcion parte={f.disponibles} total={f.total} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Seccion>

        <Seccion
          titulo="Equipos fuera de circulación"
          consulta={fueraDeCirculacion}
          alcance="Situación actual; no cambia con los filtros de ciclo o fechas."
          resumen={
            filasFuera.length > 0 &&
            `${filasFuera.length} ${filasFuera.length === 1 ? "equipo" : "equipos"}${
              sinDiagnosticar > 0 ? ` · ${sinDiagnosticar} sin diagnosticar` : ""
            }`
          }
          alDescargar={
            filasFuera.length > 0
              ? () =>
                  descargarCSV(`equipos-fuera-de-circulacion_actual.csv`, [
                    ["Equipo", "Carro", "Estado", "Falla", "Detalle"],
                    ...filasFuera.map((f) => [
                      f.etiqueta,
                      f.carroNombre ?? "",
                      ETIQUETA_ESTADO_EQUIPO[f.estado] ?? f.estado,
                      f.categoria ?? "Sin diagnosticar",
                      f.ultimaFalla ?? "",
                    ]),
                  ])
              : undefined
          }
        >
          {filasFuera.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              Están todos disponibles: no hay ninguno en mantenimiento ni fuera de
              servicio.
            </p>
          ) : (
            <Table aria-label="Equipos fuera de circulación">
              <TableHeader>
                <TableRow>
                  <TableHead>Equipo</TableHead>
                  <TableHead>Carro</TableHead>
                  <TableHead>Estado</TableHead>
                  <TableHead>Falla</TableHead>
                  <TableHead>Detalle</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filasFuera.map((f) => (
                  <TableRow key={f.equipoId}>
                    <TableCell className="font-medium">{f.etiqueta}</TableCell>
                    <TableCell>{f.carroNombre}</TableCell>
                    <TableCell>{ETIQUETA_ESTADO_EQUIPO[f.estado] ?? f.estado}</TableCell>
                    <TableCell>
                      {f.categoria || (
                        <span className="text-muted-foreground">Sin diagnosticar</span>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {f.ultimaFalla}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Seccion>

        <Seccion
          titulo="Qué se rompe"
          consulta={porCategoria}
          alcance="Todos los ciclos. Fecha de apertura de la incidencia."
          pausa={errorFechas}
          resumen={
            filasCategoria.length > 0 &&
            `${totalPorCategoria} incidencias en ${filasCategoria.length} ${filasCategoria.length === 1 ? "tipo de falla" : "tipos de falla"}`
          }
          alDescargar={
            filasCategoria.length > 0
              ? () =>
                  descargarCSV(`incidencias-por-categoria${rango}.csv`, [
                    ["Falla", "Total", "Abiertas", "Equipos alcanzados"],
                    ...filasCategoria.map((f) => [
                      f.categoria || "Sin diagnosticar",
                      f.total,
                      f.abiertas,
                      f.equiposAlcanzados,
                    ]),
                  ])
              : undefined
          }
        >
          {filasCategoria.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No hay incidencias reportadas en ese período.
            </p>
          ) : (
            <Table aria-label="Qué se rompe">
              <TableHeader>
                <TableRow>
                  <TableHead>Falla</TableHead>
                  <TableHead className="text-right">Total</TableHead>
                  <TableHead className="text-right">Abiertas</TableHead>
                  {/* Las dos cuentas dicen cosas distintas: veinte baterías en
                      veinte máquinas es un problema de lote; veinte en la
                      misma es una máquina para dar de baja. */}
                  <TableHead className="text-right">Equipos</TableHead>
                  <TableHead className="w-40">Del total</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filasCategoria.map((f) => (
                  <TableRow key={f.categoria || "sin-clasificar"}>
                    <TableCell className="font-medium">
                      {f.categoria || (
                        <span className="text-muted-foreground">Sin diagnosticar</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right">{f.total}</TableCell>
                    <TableCell className="text-right">{f.abiertas}</TableCell>
                    <TableCell className="text-right">{f.equiposAlcanzados}</TableCell>
                    <TableCell>
                      <Proporcion parte={f.total} total={totalPorCategoria} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Seccion>
      </div>
      {ciclosListos && <HistoricoPorAnio ciclos={listaCiclos} />}
    </div>
  )
}
