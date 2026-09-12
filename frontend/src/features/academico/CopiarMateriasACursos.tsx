import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import * as academicoApi from "@/features/academico/api"
import { etiquetaDeCurso } from "@/features/academico/types"
import type { Curso, Materia } from "@/features/academico/types"
import { getErrorMessage } from "@/lib/api-client"
import { contar } from "@/lib/plural"

/**
 * RF-02.12 — las materias de un curso a varios otros, de una vez.
 *
 * Existe porque la estructura académica se repite entre divisiones del mismo
 * año: las seis primeras del ciclo básico tienen las mismas diez materias, y
 * cargarlas una por una son sesenta formularios para escribir diez nombres.
 *
 * Copia NOMBRES y no materias: una materia es propia de su curso —la Matemática
 * de 1°1 no es la de 1°2, tienen docentes y reservas distintas— así que lo que
 * se comparte es cómo se llama. Los docentes asignados no viajan, por el mismo
 * motivo que no viajan al clonar un ciclo (RF-02.5): quién dicta qué es una
 * decisión por curso.
 */
export function CopiarMateriasACursos({
  origen,
  materias,
  cursosDelCiclo,
  onCerrar,
}: {
  origen: Curso
  materias: Materia[]
  cursosDelCiclo: Curso[]
  onCerrar: () => void
}) {
  const queryClient = useQueryClient()
  const [destinos, setDestinos] = useState<Set<string>>(new Set())

  const candidatos = cursosDelCiclo.filter((c) => c.id !== origen.id && !c.archivado)

  const copiar = useMutation({
    mutationFn: () => academicoApi.copiarMaterias(origen.id, [...destinos]),
    onSuccess: async (res) => {
      // Cambian las materias de varios cursos a la vez, y cuál de ellos está
      // desplegado en pantalla no se sabe desde acá: se invalidan todas.
      await queryClient.invalidateQueries({ queryKey: ["materias"] })
      setResultado(res)
      setDestinos(new Set())
    },
  })
  const [resultado, setResultado] = useState<Awaited<
    ReturnType<typeof academicoApi.copiarMaterias>
  > | null>(null)

  const alternar = (id: string) => {
    setResultado(null)
    setDestinos((previos) => {
      const siguiente = new Set(previos)
      if (siguiente.has(id)) siguiente.delete(id)
      else siguiente.add(id)
      return siguiente
    })
  }

  // Los del mismo año primero: es a los que se copia en el 90% de los casos, y
  // en un ciclo de treinta cursos tenerlos arriba evita buscarlos.
  const mismoAnio = candidatos.filter((c) => c.anio === origen.anio)
  const resto = candidatos.filter((c) => c.anio !== origen.anio)

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <div>
        <p className="text-sm font-medium">
          Copiar las {contar(materias.length, "materia")} de «{etiquetaDeCurso(origen)}»
        </p>
        <p className="text-muted-foreground text-sm">
          {materias.map((m) => m.nombre).join(", ")}
        </p>
      </div>

      {copiar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(copiar.error)}</AlertDescription>
        </Alert>
      )}

      {resultado && (
        <Alert>
          <AlertDescription>
            {resultado.materiasCreadas === 0
              ? `Los ${contar(resultado.cursosDestino, "curso")} ya tenían todas estas materias: no se agregó ninguna.`
              : `Se agregaron ${contar(resultado.materiasCreadas, "materia")} en ${contar(resultado.cursosDestino, "curso")}.`}
            {resultado.materiasCreadas > 0 &&
              resultado.materiasExistentes > 0 &&
              ` Otras ${contar(resultado.materiasExistentes, "ya estaba", "ya estaban")} cargadas.`}
          </AlertDescription>
        </Alert>
      )}

      {candidatos.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          Este ciclo no tiene otros cursos a los que copiar. Creá los demás cursos
          primero.
        </p>
      ) : (
        <>
          <div className="grid gap-1.5">
            <Label>Copiar a</Label>
            <div className="grid gap-2 sm:grid-cols-2">
              {[...mismoAnio, ...resto].map((curso) => (
                <label
                  key={curso.id}
                  className="flex items-center gap-2 text-sm"
                  htmlFor={`destino-${origen.id}-${curso.id}`}
                >
                  <Checkbox
                    id={`destino-${origen.id}-${curso.id}`}
                    checked={destinos.has(curso.id)}
                    onCheckedChange={() => alternar(curso.id)}
                  />
                  {etiquetaDeCurso(curso)}
                </label>
              ))}
            </div>
          </div>

          {/* «Los de 1°» y no «todos»: copiar a los treinta cursos del ciclo es
              casi siempre un error de clic, y ofrecerlo de un botón lo hace
              barato. El año es el recorte que sí se usa. */}
          {mismoAnio.length > 1 && (
            <div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  setResultado(null)
                  setDestinos(new Set(mismoAnio.map((c) => c.id)))
                }}
              >
                Elegir los {mismoAnio.length} cursos de {origen.anio}°
              </Button>
            </div>
          )}

          <p className="text-muted-foreground text-xs">
            Se agregan las materias que falten en cada curso y no se toca nada más: las
            que ya estén se saltean, y los docentes asignados no se copian.
          </p>
        </>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          disabled={destinos.size === 0 || copiar.isPending}
          onClick={() => copiar.mutate()}
        >
          {copiar.isPending ? "Copiando…" : `Copiar a ${contar(destinos.size, "curso")}`}
        </Button>
        <Button variant="outline" size="sm" onClick={onCerrar}>
          {resultado ? "Listo" : "Cancelar"}
        </Button>
      </div>
    </div>
  )
}
