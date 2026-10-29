import { useState } from "react"
import { useMutation } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { SelectorDeCurso } from "./SelectorDeCurso"
import type { DatosDeCurso } from "./SelectorDeCurso"
import * as api from "./api"
import type { Curso } from "./types"
import { ApiError, getErrorMessage } from "@/lib/api-client"

export function EditarCurso({
  curso,
  divisiones,
  modalidades,
  onCancelar,
  onGuardado,
}: {
  curso: Curso
  divisiones: string[]
  modalidades: string[]
  onCancelar: () => void
  onGuardado: () => Promise<unknown>
}) {
  const [datos, setDatos] = useState<DatosDeCurso>({
    anio: curso.anio,
    division: curso.division ?? "",
    modalidad: curso.modalidad ?? "",
  })
  const [impacto, setImpacto] = useState<api.ImpactoCurso | null>(null)
  const [decision, setDecision] = useState<api.DecisionPrioridadesCurso | null>(
    "conservar_exactas"
  )
  const editar = useMutation({
    mutationFn: (confirmacion: {
      versionImpacto: string
      decisionPrioridades?: api.DecisionPrioridadesCurso
    }) => api.editarCurso(curso.id, { ...datos, ...confirmacion }),
    onSuccess: onGuardado,
    onError: (error) => {
      if (error instanceof ApiError && error.status === 409) setImpacto(null)
    },
  })
  const revisar = useMutation({
    mutationFn: () => api.previsualizarCurso(curso.id, datos),
    onSuccess: async (resultado) => {
      if (resultado.cambios.length === 0) {
        await editar.mutateAsync({ versionImpacto: resultado.version })
      } else {
        setImpacto(resultado)
        setDecision(resultado.puedeConservarExactas ? "conservar_exactas" : null)
      }
    },
  })
  const pendiente = revisar.isPending || editar.isPending
  const error = editar.error ?? revisar.error
  return (
    <form
      className="grid min-w-0 gap-3"
      onSubmit={(e) => {
        e.preventDefault()
        if (pendiente) return
        editar.reset()
        revisar.reset()
        if (impacto && decision)
          editar.mutate({
            versionImpacto: impacto.version,
            decisionPrioridades: decision,
          })
        else if (!impacto) revisar.mutate()
      }}
    >
      <SelectorDeCurso
        idPrefijo={`editar-curso-${curso.id}`}
        valor={datos}
        deshabilitado={pendiente}
        divisionesSugeridas={divisiones}
        modalidadesSugeridas={modalidades}
        onCambio={(nuevo) => {
          setDatos(nuevo)
          setImpacto(null)
          editar.reset()
          revisar.reset()
        }}
      />
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}
      {impacto && (
        <div
          className="grid gap-3 rounded-md border p-3"
          aria-label="Impacto sobre las prioridades"
        >
          <p className="font-medium">Revisá las prioridades antes de guardar</p>
          <p className="text-sm">
            Al cambiar el curso, estas marcas cambian de alcance. Las reservas existentes
            conservan sus equipos y fechas. Las prioridades se aplicarán a las próximas
            reservas y desplazamientos.
          </p>
          <ul
            className="max-h-64 space-y-2 overflow-auto text-sm"
            tabIndex={0}
            aria-label="Prioridades afectadas"
          >
            {impacto.cambios.map((m) => (
              <li key={m.id}>
                <strong>
                  {m.equipoEtiqueta}
                  {m.carroNombre ? ` · ${m.carroNombre}` : ""}
                </strong>
                : {m.materiaNombre}, prioridad {m.prioridad}.
                {m.antes ? " Dejaría de aplicar." : " Empezaría a aplicar."}
                {m.exacta
                  ? " Marca de curso concreto."
                  : " Regla general: mantiene su alcance original."}
                {m.exacta &&
                  m.antes &&
                  m.compartida &&
                  " También se usa en otro curso o ciclo: se conservará allí y se copiará para este curso."}
                {m.exacta &&
                  m.antes &&
                  m.destinoId &&
                  m.destinoPrioridad !== m.prioridad &&
                  ` Ya existe una prioridad ${m.destinoPrioridad} para el destino.`}
              </li>
            ))}
          </ul>
          <fieldset className="grid gap-2" disabled={pendiente}>
            <legend className="mb-2 text-sm font-medium">Cómo guardar el cambio</legend>
            <label className="flex min-h-11 items-center gap-2 text-sm">
              <input
                type="radio"
                name={`decision-${curso.id}`}
                value="conservar_exactas"
                checked={decision === "conservar_exactas"}
                disabled={!impacto.puedeConservarExactas}
                onChange={() => setDecision("conservar_exactas")}
              />
              Conservar las prioridades exactas con el curso
            </label>
            {!impacto.puedeConservarExactas && (
              <p className="text-sm">
                Para conservarlas, primero resolvé las prioridades distintas del destino
                en inventario.
              </p>
            )}
            <label className="flex min-h-11 items-center gap-2 text-sm">
              <input
                type="radio"
                name={`decision-${curso.id}`}
                value="aceptar_alcance"
                checked={decision === "aceptar_alcance"}
                onChange={() => setDecision("aceptar_alcance")}
              />
              Mantener todas las marcas en su alcance original y aceptar los cambios
              indicados
            </label>
          </fieldset>
          <p className="text-sm">
            Las reglas generales y las marcas del destino no se trasladan. Los cambios de
            alcance indicados para ellas se aplican en ambas opciones.
          </p>
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={pendiente || (!!impacto && !decision)}>
          {pendiente ? "Comprobando…" : impacto ? "Confirmar cambio de curso" : "Guardar"}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={pendiente}
          onClick={onCancelar}
        >
          Cancelar
        </Button>
      </div>
    </form>
  )
}
