import { datosDeMarca, marcaValida } from "./alcanceDeMarca"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { invalidarInventario } from "./invalidarInventario"
import { useEffect, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import * as adminApi from "@/features/admin/api"
import { CamposDeMarca, MARCA_VACIA } from "@/features/admin/CamposDeMarca"
import type { ValoresDeMarca } from "@/features/admin/CamposDeMarca"
import { materiasConLaMismaPrioridad } from "@/features/admin/prioridadEmpatada"
import * as inventoryApi from "@/features/inventory/api"
import { getErrorMessage } from "@/lib/api-client"
import { contar, plural } from "@/lib/plural"
import { AyudaDePrioridad } from "@/features/admin/AyudaDePrioridad"

/**
 * RF-03.21 — la misma marca sobre todo un carro.
 *
 * Existe porque el caso real es por carro entero: "estas notebooks tienen
 * AutoCAD" se dice de las ocho juntas, y marcarlas de a una era abrir ocho
 * fichas. El endpoint ya aceptaba una lista de equipos; lo que faltaba era
 * desde dónde armarla.
 */
export function PreferenciaDeCarro({
  carroId,
  onListo,
}: {
  carroId: string
  onListo: () => void
}) {
  const queryClient = useQueryClient()
  const [valores, setValores] = useState<ValoresDeMarca>(MARCA_VACIA)
  // Se toma la selección inicial una vez: una consulta posterior no agrega
  // equipos nuevos a una operación que ya estaba preparada.
  const [incluidos, setIncluidos] = useState<Set<string> | null>(null)
  const [resumen, setResumen] = useState<string | null>(null)

  const {
    data: equiposData,
    isLoading,
    error,
    isSuccess,
    isFetching,
    refetch,
  } = useQuery({
    queryKey: ["equipos", carroId],
    queryFn: () => inventoryApi.listarEquiposDeCarro(carroId),
  })

  const consultaMaterias = useQuery({
    queryKey: ["catalogo-prioridades"],
    queryFn: () => adminApi.catalogoDePrioridades(),
  })

  const consultaMarcas = useQuery({
    queryKey: ["preferencias", "carro", carroId],
    queryFn: () => adminApi.listarPreferenciasDeCarro(carroId),
  })

  // Sólo lo reservable: una marca de preferencia sólo ordena la lista al
  // reservar, así que sobre un cargador no significaría nada (RF-03.16).
  const equipos = (equiposData?.data ?? []).filter((e) => e.reservable && !e.dadoDeBaja)
  const catalogo = consultaMaterias.data?.data ?? []
  const materias = [...new Set(catalogo.map((f) => f.materiaNombre))]
  const marcasData = consultaMarcas.data
  const errorConsulta = error ?? consultaMaterias.error ?? consultaMarcas.error
  const listo =
    isSuccess &&
    !isFetching &&
    !errorConsulta &&
    [consultaMaterias, consultaMarcas].every((q) => q.isSuccess && !q.isFetching)
  useEffect(() => {
    if (isSuccess && incluidos === null)
      setIncluidos(
        new Set(
          (equiposData?.data ?? [])
            .filter((e) => e.reservable && !e.dadoDeBaja)
            .map((e) => e.id)
        )
      )
  }, [isSuccess, equiposData, incluidos])
  const elegidos = equipos.filter((e) => incluidos?.has(e.id))
  const seleccionValida =
    !!incluidos?.size && [...incluidos].every((id) => equipos.some((e) => e.id === id))

  // Sólo las de los equipos que quedan tildados: avisar por una máquina que el
  // Admin acaba de destildar sería avisar de algo que no va a pasar.
  //
  // Y sólo con una materia ya elegida: con el formulario en blanco no hay nada
  // que comparar, y el string vacío no coincide con ninguna marca existente, así
  // que TODA marca en prioridad 1 se leería como empate apenas se abre el panel.
  const idsElegidos = new Set(elegidos.map((e) => e.id))
  const empatadas =
    valores.materiaNombre === ""
      ? []
      : materiasConLaMismaPrioridad(
          (marcasData?.data ?? []).filter((m) => idsElegidos.has(m.equipoId)),
          valores.materiaNombre,
          Number(valores.prioridad)
        )

  // El formulario sólo existe si hay a quién marcar y con qué: cada uno de los
  // dos estados vacíos lo reemplaza por su explicación.
  const hayFormulario = equipos.length > 0 && materias.length > 0
  const idDelFormulario = `preferencia-carro-${carroId}`

  const alternar = (id: string) => {
    const nuevo = new Set(incluidos)
    if (nuevo.has(id)) nuevo.delete(id)
    else nuevo.add(id)
    setIncluidos(nuevo)
  }

  const marcar = useMutation({
    mutationFn: () => {
      if (!listo || !marcaValida(valores, consultaMaterias.data?.data ?? []))
        throw new Error("Revisá la materia y el alcance antes de marcar.")
      return adminApi.marcarPreferencia({
        equipoIds: elegidos.map((e) => e.id),
        ...datosDeMarca(valores),
      })
    },
    onSuccess: async (r) => {
      const creadas = r.creadas.length
      const yaLaTenian = r.equiposQueYaLaTenian?.length ?? 0
      setResumen(
        `Se ${plural(creadas, "marcó", "marcaron")} ${contar(creadas, "equipo")}.` +
          (yaLaTenian === 0
            ? ""
            : ` ${contar(yaLaTenian, "equipo")} ya ${plural(yaLaTenian, "tenía", "tenían")} esa misma marca.`)
      )
      // Los campos y la selección se limpian juntos: dejar destildado lo de la
      // vuelta anterior con el formulario en blanco es un formulario que miente.
      setValores(MARCA_VACIA)
      setIncluidos(null)
      await invalidarInventario(queryClient)
    },
  })

  if (isLoading) {
    return <p className="text-muted-foreground text-sm">Cargando equipos…</p>
  }
  // Sin los equipos no se sabe nada del carro: decir que no tiene ninguno
  // reservable sería afirmar un dato de inventario que no llegó.
  if (errorConsulta)
    return (
      <div className="grid gap-2">
        <ErrorDeConsulta
          mensaje="No pudimos verificar los equipos, las materias y sus prioridades."
          reintentando={
            isFetching || consultaMaterias.isFetching || consultaMarcas.isFetching
          }
          onReintentar={() => {
            void refetch()
            void consultaMaterias.refetch()
            void consultaMarcas.refetch()
          }}
        />
        <Button variant="outline" onClick={onListo}>
          Cerrar
        </Button>
      </div>
    )
  if (!listo) return <p role="status">Consultando equipos, materias y prioridades…</p>

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <fieldset disabled={marcar.isPending} className="contents">
        <div>
          <h3 className="text-sm font-medium">Marcar preferencia para todo el carro</h3>
          <p className="text-muted-foreground text-xs">
            La misma marca se guarda en cada dispositivo seleccionado. No convierte al
            carro en una prioridad global ni afecta a equipos que agregues después.
          </p>
        </div>
        <AyudaDePrioridad />

        {marcar.error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(marcar.error)}</AlertDescription>
          </Alert>
        )}

        {/* El número se muestra después y no antes: cuántas se marcaron de verdad
          sólo se sabe una vez hecho, porque las que ya tenían la marca se
          saltean sin ser un error. */}
        {resumen && (
          <Alert>
            <AlertDescription>{resumen}</AlertDescription>
          </Alert>
        )}

        {/* Sin equipos reservables no hay nada que elegir ni marcar: mostrar el
          formulario igual sería un formulario sin sentido, con el botón
          Marcar deshabilitado desde que se abre. */}
        {equipos.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            Este carro no tiene equipos reservables.
          </p>
        ) : (
          <>
            <div className="grid gap-2">
              <Label>Equipos que quedan marcados</Label>
              <div className="grid max-h-56 gap-1 overflow-y-auto rounded-md border p-2 sm:grid-cols-2">
                {equipos.map((e) => (
                  <label key={e.id} className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={incluidos?.has(e.id) ?? false}
                      onChange={() => alternar(e.id)}
                    />
                    {e.etiqueta}
                  </label>
                ))}
              </div>
            </div>

            {/* Sin materias cargadas no hay nada que marcar, y acá tampoco hay
              marcas viejas que corregir: el formulario entero sobra. */}
            {materias.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                Todavía no hay materias cargadas. Se crean desde Académico.
              </p>
            ) : (
              <form
                id={idDelFormulario}
                className="grid gap-2 sm:grid-cols-2 sm:items-end lg:grid-cols-3"
                onSubmit={(e) => {
                  e.preventDefault()
                  setResumen(null)
                  if (
                    listo &&
                    seleccionValida &&
                    marcaValida(valores, catalogo) &&
                    !marcar.isPending
                  )
                    marcar.mutate()
                }}
              >
                {empatadas.length > 0 && (
                  <Alert className="sm:col-span-full">
                    <AlertDescription>
                      Equipos de este carro ya son preferentes de {empatadas.join(", ")}{" "}
                      con prioridad {valores.prioridad}. Cuando ambas marcas aplican, con
                      la misma prioridad ninguna desplaza a la otra. Revisá sus alcances y
                      elegí un número menor para la que deba tener mayor prioridad.
                    </AlertDescription>
                  </Alert>
                )}
                <CamposDeMarca
                  valores={valores}
                  onChange={setValores}
                  catalogo={catalogo}
                  idBase={`carro-${carroId}`}
                />
              </form>
            )}
          </>
        )}

        {/* El pie vive fuera del formulario para que Cerrar esté siempre, también
          en los dos estados donde no hay nada que hacer acá —sin equipos
          reservables y sin materias cargadas—: un panel que en algún estado no
          se puede cerrar desde adentro es un callejón sin salida.
          Marcar se queda al lado igual: `form` lo ata al formulario de arriba,
          que es lo que evita tener que separarlos en dos filas. */}
        {!seleccionValida && !!incluidos?.size && (
          <p role="status">Cambió el inventario. Revisá la selección antes de marcar.</p>
        )}
        <Button
          type="button"
          variant="outline"
          onClick={() => setIncluidos(new Set(equipos.map((e) => e.id)))}
        >
          Seleccionar los equipos actuales
        </Button>
        <span className="flex flex-wrap gap-2">
          {hayFormulario && (
            <Button
              type="submit"
              form={idDelFormulario}
              size="sm"
              disabled={
                !listo ||
                !seleccionValida ||
                !marcaValida(valores, catalogo) ||
                marcar.isPending
              }
            >
              Marcar
            </Button>
          )}
          <Button type="button" variant="outline" size="sm" onClick={onListo}>
            Cerrar
          </Button>
        </span>
      </fieldset>
    </div>
  )
}
