import { useEffect, useId, useRef } from "react"
import { Link } from "react-router"
import { Button } from "@/components/ui/button"
import { formatearFechaLargaCapitalizada } from "@/lib/fechas"
import type { ClaseCalendario, SegmentoClase } from "./reservas"
import { estadoDeClase } from "./reservas"

function titulo(clase: ClaseCalendario) {
  return clase.esBloqueo
    ? "Bloqueo administrativo"
    : clase.materiaNombre || "Reserva de equipos"
}
function cantidad(clase: ClaseCalendario) {
  return clase.reservas.filter((r) => r.estado !== "CANCELADA").length
}
function color(clase: ClaseCalendario) {
  if (cantidad(clase) === 0)
    return "border-slate-300 bg-slate-100 text-slate-700 dark:bg-slate-900 dark:text-slate-200"
  if (clase.esBloqueo)
    return "border-amber-400 bg-amber-50 text-amber-950 dark:bg-amber-950 dark:text-amber-100"
  return clase.propia
    ? "border-sky-400 bg-sky-50 text-sky-950 dark:bg-sky-950 dark:text-sky-100"
    : "border-violet-300 bg-violet-50 text-violet-950 dark:bg-violet-950 dark:text-violet-100"
}

export function Tarjeta({
  segmento,
  abrir,
  compacto = false,
  enGrilla = false,
  resumida = false,
}: {
  segmento: SegmentoClase
  abrir: (clase: ClaseCalendario) => void
  compacto?: boolean
  enGrilla?: boolean
  resumida?: boolean
}) {
  const c = segmento.clase
  const hora = `${c.horaInicio}–${c.horaFin}${c.horaFin <= c.horaInicio ? " (+1 día)" : ""}`
  const equipos = `${cantidad(c)} ${cantidad(c) === 1 ? "equipo" : "equipos"}`
  const estado = estadoDeClase(c)
  const pertenencia = c.propia ? "Tu reserva" : c.esBloqueo ? "Bloqueo" : "Otra materia"
  const descripcion = `${titulo(c)}, ${c.cursoNombre ?? ""}, ${c.fecha}, ${hora}, ${equipos}, ${estado}${c.propia ? ", tu reserva" : ""}`
  return (
    <button
      type="button"
      onClick={() => abrir(c)}
      className={`h-full min-h-11 min-w-11 w-full overflow-hidden rounded-lg sm:min-h-0 sm:min-w-0 border-l-4 text-left shadow-sm transition-shadow hover:shadow-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary ${enGrilla ? "flex flex-col items-start justify-start px-2 py-1.5" : "p-2"} ${color(c)}`}
      aria-label={descripcion}
      title={descripcion}
    >
      <span
        className={`block text-xs font-semibold ${enGrilla ? "w-full shrink-0 whitespace-nowrap leading-4" : ""}`}
      >
        {enGrilla && segmento.continua
          ? `Continúa hasta ${c.horaFin}`
          : `${segmento.continua ? "Continúa · " : ""}${hora}`}
      </span>
      <span
        className={`text-sm font-bold break-words ${enGrilla ? "line-clamp-2 w-full shrink-0 leading-5" : "block"}`}
      >
        {titulo(c)}
      </span>
      {enGrilla ? (
        <>
          {!resumida && (
            <>
              <span className="block w-full shrink-0 truncate text-xs leading-4">
                {[c.cursoNombre, c.cursoModalidad].filter(Boolean).join(" · ") ||
                  c.motivoBloqueo ||
                  ""}
              </span>
              <span className="block w-full shrink-0 text-xs leading-4">{equipos}</span>
            </>
          )}
          <span className="block w-full shrink-0 truncate text-xs leading-4">
            {estado !== "Confirmada" ? `${estado} · ` : ""}
            {pertenencia}
          </span>
        </>
      ) : (
        !compacto && (
          <>
            <span className="block truncate text-xs">
              {[c.cursoNombre, c.cursoModalidad].filter(Boolean).join(" · ") ||
                c.motivoBloqueo ||
                ""}
            </span>
            <span className="block text-xs">{equipos}</span>
            <span className="block text-xs">
              {c.propia ? "Tu reserva" : c.nombreDocenteSnapshot || "Administración"}
            </span>
            <span className="block text-xs break-words">{estado}</span>
          </>
        )
      )}
    </button>
  )
}

export function DetalleClase({
  clase,
  cerrar,
  esAdmin,
}: {
  clase: ClaseCalendario
  cerrar: () => void
  esAdmin: boolean
}) {
  const ref = useRef<HTMLDialogElement>(null)
  const id = useId()
  useEffect(() => {
    const anterior = document.activeElement as HTMLElement | null
    const dialogo = ref.current
    dialogo?.showModal()
    return () => {
      dialogo?.close()
      anterior?.focus()
    }
  }, [])
  const params = new URLSearchParams({
    desde: clase.fecha,
    hasta: clase.fecha,
    canceladas: "true",
  })
  if (esAdmin && clase.creadoPor) params.set("creadoPor", clase.creadoPor)
  return (
    <dialog
      ref={ref}
      aria-labelledby={id}
      onCancel={(e) => {
        e.preventDefault()
        cerrar()
      }}
      className="bg-background text-foreground fixed inset-0 m-auto max-h-[85dvh] w-[calc(100%-2rem)] max-w-xl overflow-y-auto rounded-xl border p-5 shadow-xl backdrop:bg-black/50"
    >
      <h2 id={id} className="text-xl font-semibold">
        {titulo(clase)}
      </h2>
      <p className="mt-1 text-sm text-muted-foreground">
        {clase.propia ? "Tu reserva" : "Ocupación compartida"} · {estadoDeClase(clase)}
      </p>
      <dl className="my-4 grid gap-3 text-sm sm:grid-cols-2">
        <div>
          <dt className="font-medium">Fecha y horario</dt>
          <dd>
            {formatearFechaLargaCapitalizada(clase.fecha)} · {clase.horaInicio}–
            {clase.horaFin}
            {clase.horaFin <= clase.horaInicio ? " (termina al día siguiente)" : ""}
          </dd>
        </div>
        <div>
          <dt className="font-medium">Curso</dt>
          <dd>
            {[clase.cursoNombre, clase.cursoModalidad].filter(Boolean).join(" · ") ||
              "No corresponde"}
          </dd>
        </div>
        <div>
          <dt className="font-medium">Responsable</dt>
          <dd>{clase.nombreDocenteSnapshot || "Administración"}</dd>
        </div>
        <div>
          <dt className="font-medium">Reserva</dt>
          <dd>
            {clase.esRecurrente ? "Parte de una serie recurrente" : "Para esta fecha"}
          </dd>
        </div>
      </dl>
      {clase.esBloqueo && <p className="mb-4 text-sm">{clase.motivoBloqueo}</p>}
      <h3 className="mb-2 font-medium">Equipos ({cantidad(clase)} vigentes)</h3>
      <ul className="mb-4 max-h-52 space-y-2 overflow-y-auto rounded-lg border p-3 text-sm">
        {clase.reservas.map((r) => (
          <li key={r.id} className="flex flex-wrap items-center justify-between gap-2">
            <Link
              className="inline-flex min-h-11 min-w-11 items-center text-primary underline sm:min-h-0 sm:min-w-0"
              to={`/inventario/equipos/${r.equipoId}/calendario`}
            >
              {r.etiqueta}
              {r.carroNombre ? ` · ${r.carroNombre}` : ""}
            </Link>
            <span>
              {r.estado === "CANCELADA"
                ? "Cancelada / liberada"
                : r.estado.replaceAll("_", " ").toLowerCase()}
            </span>
          </li>
        ))}
      </ul>
      {!clase.propia && !esAdmin && (
        <p className="mb-4 text-sm text-muted-foreground">
          Podés consultar esta reserva. La disponibilidad y la prioridad de cada equipo se
          verifican al realizar una nueva reserva.
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        {(clase.propia || esAdmin) && (
          <Button className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0" asChild>
            <Link to={`/reservas?${params}`}>Gestionar reserva</Link>
          </Button>
        )}
        <Button
          className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-0"
          type="button"
          variant="outline"
          onClick={cerrar}
        >
          Cerrar detalles
        </Button>
      </div>
    </dialog>
  )
}
