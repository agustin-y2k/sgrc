import { useRef, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import * as academicoApi from "@/features/academico/api"
import {
  estructuraAFilas,
  leerEstructuraCSV,
  nombreDeArchivo,
} from "@/features/academico/estructuraCSV"
import type { CursoConMaterias, FilaRechazada } from "@/features/academico/estructuraCSV"
import type { CicloLectivo, ResultadoImportacion } from "@/features/academico/types"
import { getErrorMessage } from "@/lib/api-client"
import { descargarCSV } from "@/lib/csv"
import { contar } from "@/lib/plural"
import { claveDeNombre } from "@/lib/texto"

/**
 * RF-02.12 — la estructura del ciclo entera, en las dos direcciones.
 *
 * Cargar una institución curso por curso y materia por materia son cientos de
 * formularios, y es trabajo que ya está hecho en una planilla: la que la
 * escuela usa para armar los horarios. Esto la lee.
 *
 * La carga AGREGA lo que falta y no toca nada más. Es la única semántica segura
 * para algo que se reintenta: subir dos veces el mismo archivo tiene que ser
 * inofensivo, y un archivo al que le falta una materia que alguien agregó a mano
 * no puede hacerla desaparecer —ni, peor, llevarse por delante sus reservas—.
 */

/** Lo que se va a cargar, ya leído del archivo y comparado contra el ciclo. */
type Previsualizacion = {
  nombreArchivo: string
  cursos: CursoConMaterias[]
  rechazadas: FilaRechazada[]
  cursosNuevos: number
  materiasNuevas: number
}

/** La misma terna normalizada con la que la base identifica un curso. */
function claveDeCurso(c: {
  anio: number
  division?: string
  modalidad?: string
}): string {
  return `${c.anio}|${claveDeNombre(c.division ?? "")}|${claveDeNombre(c.modalidad ?? "")}`
}

export function EstructuraDelCiclo({ ciclo }: { ciclo: CicloLectivo }) {
  const queryClient = useQueryClient()
  const inputArchivo = useRef<HTMLInputElement>(null)
  const [previa, setPrevia] = useState<Previsualizacion | null>(null)
  const [errorLectura, setErrorLectura] = useState<string | null>(null)
  const [resultado, setResultado] = useState<ResultadoImportacion | null>(null)

  // La estructura actual sirve para las dos cosas: es lo que se descarga, y es
  // contra lo que se compara el archivo para decir qué va a crear. Una sola
  // consulta, no una por curso.
  const estructuraKey = ["estructura", ciclo.id]
  const { data, isLoading } = useQuery({
    queryKey: estructuraKey,
    queryFn: () => academicoApi.exportarEstructura(ciclo.id),
  })
  const actual = data?.cursos ?? []

  const importar = useMutation({
    mutationFn: (cursos: CursoConMaterias[]) =>
      academicoApi.importarEstructura(ciclo.id, cursos),
    onSuccess: async (res) => {
      setResultado(res)
      setPrevia(null)
      // Aparecen cursos y materias nuevos en todo el árbol del ciclo.
      await queryClient.invalidateQueries()
    },
  })

  const descargar = () => {
    descargarCSV(nombreDeArchivo(ciclo.anio), estructuraAFilas(actual))
  }

  const elegirArchivo = async (archivo: File) => {
    setErrorLectura(null)
    setResultado(null)
    setPrevia(null)

    const texto = await archivo.text()
    const { cursos, rechazadas } = leerEstructuraCSV(texto)

    if (cursos.length === 0) {
      setErrorLectura(
        rechazadas[0]?.motivo ??
          "El archivo no trae ninguna fila de curso que se pueda leer."
      )
      // Que se pueda volver a elegir EL MISMO archivo después de corregirlo:
      // sin esto el input no dispara `change` la segunda vez y parece colgado.
      if (inputArchivo.current) inputArchivo.current.value = ""
      return
    }

    const clavesActuales = new Set(actual.map(claveDeCurso))
    const materiasPorCurso = new Map(
      actual.map((c) => [claveDeCurso(c), new Set(c.materias.map(claveDeNombre))])
    )

    let cursosNuevos = 0
    let materiasNuevas = 0
    for (const curso of cursos) {
      const clave = claveDeCurso(curso)
      if (!clavesActuales.has(clave)) cursosNuevos++
      const yaTiene = materiasPorCurso.get(clave) ?? new Set<string>()
      // Se cuentan sin repetir dentro de la fila: una planilla trae la misma
      // materia dos veces sin que nadie lo note, y el backend la carga una sola
      // vez. El resumen tiene que anticipar eso, no contradecirlo.
      const nuevasDeEsteCurso = new Set(
        curso.materias.map(claveDeNombre).filter((m) => !yaTiene.has(m))
      )
      materiasNuevas += nuevasDeEsteCurso.size
    }

    setPrevia({
      nombreArchivo: archivo.name,
      cursos,
      rechazadas,
      cursosNuevos,
      materiasNuevas,
    })
    if (inputArchivo.current) inputArchivo.current.value = ""
  }

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <div>
        <p className="text-sm font-medium">Cursos y materias en una planilla</p>
        <p className="text-muted-foreground text-sm">
          Una fila por curso, con sus materias separadas por comas en la última columna.
          Sirve para cargar el año entero de una vez, y para armar el siguiente a partir
          de este.
        </p>
      </div>

      {(errorLectura || importar.error) && (
        <Alert variant="destructive">
          <AlertDescription>
            {errorLectura ?? getErrorMessage(importar.error)}
          </AlertDescription>
        </Alert>
      )}

      {resultado && (
        <Alert>
          <AlertDescription>
            {resultado.cursosCreados === 0 && resultado.materiasCreadas === 0
              ? "El ciclo ya tenía todo lo que trae el archivo: no se creó nada."
              : `Se cargaron ${contar(resultado.cursosCreados, "curso")} y ${contar(resultado.materiasCreadas, "materia")}.`}
            {(resultado.cursosExistentes > 0 || resultado.materiasExistentes > 0) &&
              resultado.cursosCreados + resultado.materiasCreadas > 0 &&
              ` El resto ya estaba: ${contar(resultado.cursosExistentes, "curso")} y ${contar(resultado.materiasExistentes, "materia")}.`}{" "}
            Falta asignarles docentes.
          </AlertDescription>
        </Alert>
      )}

      {previa && (
        <div className="grid gap-2 rounded-md border p-3">
          <p className="text-sm font-medium">{previa.nombreArchivo}</p>
          <p className="text-sm">
            Trae {contar(previa.cursos.length, "curso")}.{" "}
            {previa.cursosNuevos === 0 && previa.materiasNuevas === 0 ? (
              <>Todo eso ya está cargado: no se va a crear nada.</>
            ) : (
              <>
                Se van a crear {contar(previa.cursosNuevos, "curso")} y{" "}
                {contar(previa.materiasNuevas, "materia")}.
              </>
            )}
          </p>
          <p className="text-muted-foreground text-sm">
            No se borra ni se renombra nada de lo que ya está cargado.
          </p>

          {previa.rechazadas.length > 0 && (
            <Alert variant="destructive">
              <AlertDescription>
                <p className="font-medium">
                  {contar(previa.rechazadas.length, "fila")} del archivo no se van a
                  cargar:
                </p>
                <ul className="mt-1 grid gap-0.5 text-sm">
                  {previa.rechazadas.map((r) => (
                    <li key={r.linea}>
                      · Fila {r.linea}: {r.motivo}
                    </li>
                  ))}
                </ul>
                {/* Que se pueda seguir igual es deliberado: frenar treinta
                    cursos buenos por tres renglones malos convierte la carga en
                    prueba y error. Se corrigen las tres y se vuelve a subir, que
                    no duplica nada. */}
                <p className="mt-1 text-sm">
                  El resto se puede cargar igual; corregí estas y volvé a subir el
                  archivo.
                </p>
              </AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              disabled={importar.isPending}
              onClick={() => importar.mutate(previa.cursos)}
            >
              {importar.isPending ? "Cargando…" : "Cargar en el ciclo"}
            </Button>
            <Button variant="outline" size="sm" onClick={() => setPrevia(null)}>
              Cancelar
            </Button>
          </div>
        </div>
      )}

      <div className="grid gap-2 sm:flex sm:flex-wrap sm:items-end">
        <div className="grid gap-1.5">
          <Label htmlFor={`archivo-${ciclo.id}`}>Cargar desde una planilla</Label>
          <input
            ref={inputArchivo}
            id={`archivo-${ciclo.id}`}
            type="file"
            accept=".csv,text/csv"
            className="text-sm file:mr-3 file:rounded-md file:border file:border-input file:bg-background file:px-3 file:py-1.5 file:text-sm"
            onChange={(e) => {
              const archivo = e.target.files?.[0]
              if (archivo) void elegirArchivo(archivo)
            }}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={isLoading || actual.length === 0}
          onClick={descargar}
        >
          Descargar los {contar(actual.length, "curso")}
        </Button>
      </div>

      <p className="text-muted-foreground text-xs">
        La planilla se descarga con el mismo formato que se carga, así que lo que baja se
        puede corregir y volver a subir. Se aceptan archivos separados por punto y coma,
        por coma o por tabulación.
      </p>
    </div>
  )
}
