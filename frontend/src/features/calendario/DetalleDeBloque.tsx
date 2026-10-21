import { useEffect, useRef } from "react"
import { Button } from "@/components/ui/button"
import type { BloqueCalendario } from "@/features/calendario/types"
import { formatearFechaLargaCapitalizada } from "@/lib/fechas"

export function DetalleDeBloque({
  bloque,
  cerrar,
  etiqueta,
}: {
  bloque: BloqueCalendario
  cerrar: () => void
  etiqueta: string
}) {
  const dialogo = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const elemento = dialogo.current
    const anterior = document.activeElement as HTMLElement | null
    elemento?.showModal()
    return () => {
      elemento?.close()
      anterior?.focus()
    }
  }, [])
  return (
    <dialog
      ref={dialogo}
      aria-labelledby="titulo-bloque"
      onCancel={(e) => {
        e.preventDefault()
        cerrar()
      }}
      className="bg-background text-foreground fixed inset-0 m-auto max-h-[85dvh] w-[calc(100%-2rem)] max-w-lg overflow-y-auto rounded-lg border p-5 shadow-lg backdrop:bg-black/50"
    >
      <h2 id="titulo-bloque" className="mb-4 text-xl font-semibold">
        {bloque.tipo === "BLOQUEO" ? "Bloqueo del equipo" : "Detalle de la reserva"}
      </h2>
      <dl className="mb-4 space-y-2 text-sm">
        <div>
          <dt className="font-medium">Equipo</dt>
          <dd>{etiqueta}</dd>
        </div>
        <div>
          <dt className="font-medium">Fecha</dt>
          <dd>{formatearFechaLargaCapitalizada(bloque.fecha)}</dd>
        </div>
        <div>
          <dt className="font-medium">Horario</dt>
          <dd>
            {bloque.horaInicio}–{bloque.horaFin}
            {bloque.horaFin <= bloque.horaInicio ? " (termina al día siguiente)" : ""}
          </dd>
        </div>
        {bloque.tipo === "BLOQUEO" ? (
          <div>
            <dt className="font-medium">Motivo</dt>
            <dd>{bloque.motivoBloqueo || "Bloqueado por administración"}</dd>
          </div>
        ) : (
          <>
            <div>
              <dt className="font-medium">Materia</dt>
              <dd>{bloque.materiaNombre || "Sin información"}</dd>
            </div>
            <div>
              <dt className="font-medium">Curso</dt>
              <dd>{bloque.cursoNombre || "Sin información"}</dd>
            </div>
            <div>
              <dt className="font-medium">Docente</dt>
              <dd>{bloque.docente || "Sin información"}</dd>
            </div>
          </>
        )}
      </dl>
      <Button type="button" variant="outline" className="h-11" onClick={cerrar}>
        Cerrar detalles
      </Button>
    </dialog>
  )
}
