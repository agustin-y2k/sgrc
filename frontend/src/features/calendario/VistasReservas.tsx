import { Tarjeta } from "./TarjetaCalendario"
import { distribuirClases, crearEjeSemanal } from "./reservas"
import type { ClaseCalendario, SegmentoClase } from "./reservas"
import { desdeFechaISO, etiquetaDeDia } from "./semana"
import { formatearFechaLargaCapitalizada } from "@/lib/fechas"

type Props = {
  fechas: string[]
  porDia: Map<string, SegmentoClase[]>
  hoy: string
  abrir: (clase: ClaseCalendario) => void
}

export function VistaSemanal({ fechas, porDia, hoy, abrir }: Props) {
  const segmentos = [...porDia.values()].flat()
  const inicio = Math.min(7 * 60, ...segmentos.map((s) => Math.floor(s.inicio / 60) * 60))
  const fin = Math.max(19 * 60, ...segmentos.map((s) => Math.ceil(s.fin / 60) * 60))
  const eje = crearEjeSemanal(inicio, fin, segmentos, 90)
  const distribuidos = fechas.map((f) => distribuirClases(porDia.get(f) ?? []))
  const columnas = distribuidos.map((eventos) =>
    Math.max(112, ...eventos.map((e) => e.columnas * 176))
  )
  return (
    <>
      {eje.ampliado && (
        <p className="mb-2 text-xs text-muted-foreground">
          Los horarios indican la duración real; las clases cortas se muestran ampliadas.
        </p>
      )}
      <div
        className="max-h-[650px] overflow-auto rounded-xl border"
        tabIndex={0}
        aria-label="Grilla semanal de reservas, desplazable"
      >
        <div
          className="grid w-full"
          style={{
            minWidth: 52 + columnas.reduce((total, ancho) => total + ancho, 0),
            gridTemplateColumns: `52px ${columnas.map((ancho) => `minmax(${ancho}px, 1fr)`).join(" ")}`,
          }}
        >
          <div className="sticky top-0 z-20 border-b bg-superficie p-2 text-xs">Hora</div>
          {fechas.map((f) => (
            <div
              key={f}
              className={`sticky top-0 z-20 border-b border-l bg-superficie p-2 text-center text-sm ${f === hoy ? "font-bold text-primary" : ""}`}
            >
              {etiquetaDeDia(f)}
              <br />
              {desdeFechaISO(f).getDate()}/{desdeFechaISO(f).getMonth() + 1}
              {f === hoy ? " · Hoy" : ""}
            </div>
          ))}
          <div className="relative bg-superficie" style={{ height: eje.alto }}>
            {Array.from({ length: (fin - inicio) / 60 }, (_, i) => (
              <span
                key={i}
                className="absolute left-1 text-xs text-muted-foreground"
                style={{ top: eje.posicion(inicio + i * 60) }}
              >
                {String(inicio / 60 + i).padStart(2, "0")}:00
              </span>
            ))}
          </div>
          {fechas.map((f, indice) => (
            <div
              key={f}
              className={`relative border-l ${f === hoy ? "bg-primary/5" : ""}`}
              style={{ height: eje.alto }}
            >
              {Array.from({ length: (fin - inicio) / 60 }, (_, i) => (
                <div
                  key={i}
                  className="absolute w-full border-t border-dashed"
                  style={{ top: eje.posicion(inicio + i * 60) }}
                />
              ))}
              {distribuidos[indice].map((s) => {
                const alto = eje.posicion(s.fin) - eje.posicion(s.inicio) - 2
                return (
                  <div
                    key={s.clase.clave}
                    className="absolute px-0.5"
                    style={{
                      top: eje.posicion(s.inicio),
                      height: alto,
                      left: `${(100 * s.columna) / s.columnas}%`,
                      width: `${100 / s.columnas}%`,
                    }}
                  >
                    <Tarjeta segmento={s} abrir={abrir} enGrilla resumida={alto < 128} />
                  </div>
                )
              })}
            </div>
          ))}
        </div>
      </div>
    </>
  )
}

export function VistaMensual({
  fechas,
  porDia,
  hoy,
  abrir,
  referencia,
  verDia,
}: Props & { referencia: Date; verDia: (fecha: Date) => void }) {
  return (
    <div
      className="overflow-x-auto rounded-xl border"
      tabIndex={0}
      aria-label="Calendario mensual, desplazable"
    >
      <div className="grid min-w-[700px] grid-cols-7">
        {["Lun", "Mar", "Mié", "Jue", "Vie", "Sáb", "Dom"].map((d) => (
          <div key={d} className="border-b p-2 text-center text-sm font-semibold">
            {d}
          </div>
        ))}
        {fechas.map((f) => {
          const eventos = porDia.get(f) ?? []
          const fecha = desdeFechaISO(f)
          return (
            <div
              key={f}
              className={`min-h-36 border-b border-r p-1.5 ${fecha.getMonth() !== referencia.getMonth() ? "bg-muted/40" : ""} ${f === hoy ? "ring-2 ring-inset ring-primary" : ""}`}
            >
              <button
                type="button"
                className="mb-1 min-h-11 min-w-11 rounded sm:min-h-8 sm:min-w-8 px-2 text-sm font-semibold hover:bg-muted"
                aria-label={`Ver agenda del ${f}`}
                onClick={() => {
                  verDia(fecha)
                }}
              >
                {fecha.getDate()}
                {f === hoy ? " · Hoy" : ""}
              </button>
              <div className="grid gap-1">
                {eventos.slice(0, 3).map((s) => (
                  <Tarjeta key={s.clase.clave} segmento={s} abrir={abrir} compacto />
                ))}
                {eventos.length > 3 && (
                  <button
                    type="button"
                    className="min-h-11 min-w-11 text-sm sm:min-h-8 sm:min-w-0 font-medium text-primary underline"
                    onClick={() => {
                      verDia(fecha)
                    }}
                  >
                    Ver {eventos.length - 3} más
                  </button>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function VistaAgenda({ fechas, porDia, hoy, abrir }: Props) {
  return (
    <div className="grid gap-4">
      {fechas.map((f) => (
        <section
          key={f}
          className={`rounded-xl border p-3 ${f === hoy ? "border-primary bg-primary/5" : ""}`}
        >
          <h3 className="mb-2 font-semibold">
            {formatearFechaLargaCapitalizada(f)}
            {f === hoy ? " · Hoy" : ""}
          </h3>
          {porDia.has(f) ? (
            <div className="grid gap-2 sm:grid-cols-2">
              {porDia.get(f)!.map((s) => (
                <Tarjeta key={s.clase.clave} segmento={s} abrir={abrir} />
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">Sin reservas en esta vista.</p>
          )}
        </section>
      ))}
    </div>
  )
}
