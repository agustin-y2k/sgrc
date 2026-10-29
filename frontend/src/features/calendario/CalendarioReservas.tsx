import { DetalleClase } from "./TarjetaCalendario"
import { VistaSemanal, VistaMensual, VistaAgenda } from "./VistasReservas"
import { useId, useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "react-router"
import { CalendarDays, ChevronLeft, ChevronRight, RefreshCw } from "lucide-react"
import { useAuth } from "@/features/auth/AuthContext"
import { useEsAngosto } from "@/lib/useEsAngosto"
import { getErrorMessage } from "@/lib/api-client"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Select } from "@/components/ui/select"
import { aFechaISO, desdeFechaISO, formatearRangoVisible } from "./semana"
import { cargarCalendario } from "./reservas-api"
import {
  clasesDelCalendario,
  fechasDelCalendario,
  moverCalendario,
  segmentosDeClase,
} from "./reservas"
import type { ClaseCalendario, SegmentoClase, VistaCalendario } from "./reservas"

export function CalendarioReservas() {
  const { user } = useAuth()
  const angosto = useEsAngosto()
  const [referencia, setReferencia] = useState(() => new Date())
  const [vistaElegida, setVista] = useState<VistaCalendario | null>(null)
  const vista = vistaElegida ?? (angosto ? "agenda" : "semana")
  const [ambito, setAmbito] = useState<"mias" | "todas">(() =>
    user?.rol === "ADMIN" ? "todas" : "mias"
  )
  const [busqueda, setBusqueda] = useState("")
  const [filtro, setFiltro] = useState("")
  const [incluirCanceladas, setCanceladas] = useState(false)
  const [seleccion, setSeleccion] = useState<string | null>(null)
  const id = useId()
  const fechas = fechasDelCalendario(referencia, vista)
  const desde = fechas[0],
    hasta = fechas[fechas.length - 1]
  const consulta = useQuery({
    queryKey: [
      "reservas",
      "calendario",
      user?.id,
      desde,
      hasta,
      ambito,
      filtro,
      incluirCanceladas,
    ],
    queryFn: () =>
      cargarCalendario({ desde, hasta, ambito, busqueda: filtro, incluirCanceladas }),
    enabled: !!user,
    refetchInterval: 30000,
  })
  const clases = useMemo(() => clasesDelCalendario(consulta.data ?? []), [consulta.data])
  const porDia = useMemo(() => {
    const mapa = new Map<string, SegmentoClase[]>()
    for (const c of clases)
      for (const s of segmentosDeClase(c)) {
        if (s.fecha < desde || s.fecha > hasta) continue
        mapa.set(s.fecha, [...(mapa.get(s.fecha) ?? []), s])
      }
    for (const lista of mapa.values())
      lista.sort(
        (a, b) => a.inicio - b.inicio || a.clase.clave.localeCompare(b.clase.clave)
      )
    return mapa
  }, [clases, desde, hasta])
  const detalle = clases.find((c) => c.clave === seleccion)
  const visibles = new Set([...porDia.values()].flat().map((s) => s.clase.clave)).size
  const hoy = aFechaISO(new Date())
  const abrir = (c: ClaseCalendario) => setSeleccion(c.clave)
  return (
    <section
      className="min-w-0 rounded-2xl border bg-superficie p-3 shadow-sm sm:p-5"
      aria-label="Calendario de reservas"
      aria-busy={consulta.isFetching}
    >
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-2 text-xl font-semibold">
            <CalendarDays aria-hidden="true" className="size-5" />
            Calendario de reservas
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Tus clases y la ocupación de otras materias, por día y horario.
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
            type="button"
            variant="outline"
            aria-label="Actualizar calendario"
            disabled={consulta.isFetching}
            onClick={() => void consulta.refetch()}
          >
            <RefreshCw aria-hidden="true" className="size-4" />
          </Button>
          <Button className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0" asChild>
            <Link to="/reservas/nueva">Nueva reserva</Link>
          </Button>
        </div>
      </div>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Button
          className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
          type="button"
          variant="outline"
          aria-label="Período anterior"
          onClick={() => setReferencia(moverCalendario(referencia, vista, -1))}
        >
          <ChevronLeft aria-hidden="true" className="size-4" />
        </Button>
        <Button
          className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
          type="button"
          variant="outline"
          onClick={() => setReferencia(new Date())}
        >
          Hoy
        </Button>
        <Button
          className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
          type="button"
          variant="outline"
          aria-label="Período siguiente"
          onClick={() => setReferencia(moverCalendario(referencia, vista, 1))}
        >
          <ChevronRight aria-hidden="true" className="size-4" />
        </Button>
        <p className="font-semibold" aria-live="polite">
          {vista === "mes"
            ? referencia.toLocaleDateString("es-AR", { month: "long", year: "numeric" })
            : formatearRangoVisible(fechas)}
        </p>
      </div>
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <div className="grid gap-1">
          <Label htmlFor={`${id}-fecha`}>Ir a fecha</Label>
          <Input
            className="min-h-11 sm:min-h-8"
            id={`${id}-fecha`}
            type="date"
            value={aFechaISO(referencia)}
            onChange={(e) => {
              if (/^\d{4}-\d{2}-\d{2}$/.test(e.target.value))
                setReferencia(desdeFechaISO(e.target.value))
            }}
          />
        </div>
        <div className="grid gap-1">
          <Label htmlFor={`${id}-ambito`}>Mostrar reservas</Label>
          <Select
            className="min-h-11 sm:min-h-8"
            id={`${id}-ambito`}
            value={ambito}
            onChange={(e) => setAmbito(e.target.value as "mias" | "todas")}
          >
            <option value="mias">Mis reservas</option>
            <option value="todas">Todas las materias</option>
          </Select>
        </div>
        <div className="flex gap-1" role="group" aria-label="Vista del calendario">
          {(
            [
              ["semana", "Semana"],
              ["mes", "Mes"],
              ["agenda", "Agenda"],
            ] as const
          ).map(([v, texto]) => (
            <Button
              className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
              key={v}
              type="button"
              variant={vista === v ? "default" : "outline"}
              aria-pressed={vista === v}
              onClick={() => setVista(v)}
            >
              {texto}
            </Button>
          ))}
        </div>
        <Button
          className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
          asChild
          variant="outline"
        >
          <Link to="/reservas">Listado y gestión</Link>
        </Button>
      </div>
      <form
        className="mb-4 flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          setFiltro(busqueda.trim())
        }}
      >
        <div className="grid min-w-0 flex-1 gap-1">
          <Label htmlFor={`${id}-buscar`}>Buscar materia, curso o modalidad</Label>
          <Input
            className="min-h-11 sm:min-h-8"
            id={`${id}-buscar`}
            maxLength={100}
            value={busqueda}
            onChange={(e) => setBusqueda(e.target.value)}
          />
        </div>
        <Button
          className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
          type="submit"
          variant="outline"
        >
          Buscar en calendario
        </Button>
        <label className="flex min-h-11 min-w-11 cursor-pointer items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={incluirCanceladas}
            onChange={(e) => setCanceladas(e.target.checked)}
          />
          Mostrar canceladas
        </label>
      </form>
      <div
        className="mb-3 flex flex-wrap gap-3 text-xs"
        aria-label="Referencias del calendario"
      >
        <span className="rounded border-l-4 border-sky-400 px-2 py-1">Tus reservas</span>
        <span className="rounded border-l-4 border-violet-300 px-2 py-1">
          Otras materias
        </span>
        <span className="rounded border-l-4 border-amber-400 px-2 py-1">
          Bloqueos administrativos
        </span>
        <span className="text-muted-foreground">
          Seleccioná una clase para ver curso, equipos y responsable.
        </span>
      </div>
      {consulta.isPending ? (
        <p role="status" className="p-6">
          Cargando el calendario…
        </p>
      ) : consulta.error ? (
        <Alert variant="destructive">
          <AlertDescription>
            No pudimos consultar el calendario. {getErrorMessage(consulta.error)}{" "}
            <Button
              className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
              type="button"
              variant="outline"
              onClick={() => void consulta.refetch()}
            >
              Reintentar calendario
            </Button>
          </AlertDescription>
        </Alert>
      ) : (
        <>
          <p className="mb-3 text-sm text-muted-foreground" role="status">
            {visibles}{" "}
            {visibles === 1
              ? "clase o bloqueo en este período"
              : "clases y bloqueos en este período"}
            .
          </p>
          {visibles === 0 && (
            <div className="mb-4 rounded-xl border border-dashed p-4">
              <p>No hay reservas que coincidan con esta vista y estos filtros.</p>
              {ambito === "mias" && (
                <Button
                  type="button"
                  variant="outline"
                  className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0 mt-2"
                  onClick={() => setAmbito("todas")}
                >
                  Ver reservas de otras materias
                </Button>
              )}
            </div>
          )}
          {vista === "semana" && (
            <VistaSemanal fechas={fechas} porDia={porDia} hoy={hoy} abrir={abrir} />
          )}
          {vista === "mes" && (
            <VistaMensual
              fechas={fechas}
              porDia={porDia}
              hoy={hoy}
              abrir={abrir}
              referencia={referencia}
              verDia={(fecha) => {
                setReferencia(fecha)
                setVista("agenda")
              }}
            />
          )}
          {vista === "agenda" && (
            <VistaAgenda fechas={fechas} porDia={porDia} hoy={hoy} abrir={abrir} />
          )}
        </>
      )}
      {detalle && (
        <DetalleClase
          clase={detalle}
          cerrar={() => setSeleccion(null)}
          esAdmin={user?.rol === "ADMIN"}
        />
      )}
    </section>
  )
}

export function CalendarioReservasPage() {
  return (
    <div className="min-w-0">
      <h1 className="mb-4 text-2xl font-semibold">Reservas de equipos</h1>
      <CalendarioReservas />
    </div>
  )
}
