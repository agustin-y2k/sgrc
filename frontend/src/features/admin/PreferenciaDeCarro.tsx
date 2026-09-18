import { useState } from "react"
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
  // Arranca sin tocar: el default es "todos", y eso se resuelve al armar la
  // lista. Acá sólo viven los que el Admin destildó a mano.
  const [destildados, setDestildados] = useState<Set<string>>(new Set())
  const [resumen, setResumen] = useState<string | null>(null)

  const {
    data: equiposData,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["equipos", carroId],
    queryFn: () => inventoryApi.listarEquiposDeCarro(carroId),
  })

  const { data: materiasData } = useQuery({
    queryKey: ["materias-en-uso"],
    queryFn: () => adminApi.materiasEnUso(),
  })

  // Si esta falla se sigue sin avisar: lo único que se pierde es el aviso de
  // empate, y marcar —que es para lo que se abre el panel— no depende de ella.
  const { data: marcasData } = useQuery({
    queryKey: ["preferencias", "carro", carroId],
    queryFn: () => adminApi.listarPreferenciasDeCarro(carroId),
  })

  // Sólo lo reservable: una marca de preferencia sólo ordena la lista al
  // reservar, así que sobre un cargador no significaría nada (RF-03.16).
  const equipos = (equiposData?.data ?? []).filter((e) => e.reservable && !e.dadoDeBaja)
  const materias = materiasData?.data ?? []
  const elegidos = equipos.filter((e) => !destildados.has(e.id))

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
          Number(valores.prioridad),
        )

  const alternar = (id: string) => {
    const nuevo = new Set(destildados)
    if (nuevo.has(id)) nuevo.delete(id)
    else nuevo.add(id)
    setDestildados(nuevo)
  }

  const marcar = useMutation({
    mutationFn: () =>
      adminApi.marcarPreferencia({
        equipoIds: elegidos.map((e) => e.id),
        materiaNombre: valores.materiaNombre,
        modalidad: valores.modalidad.trim() || undefined,
        anio: valores.anio ? Number(valores.anio) : undefined,
        division: valores.division.trim() || undefined,
        prioridad: Number(valores.prioridad),
      }),
    onSuccess: async (r) => {
      const creadas = r.creadas.length
      const yaLaTenian = r.equiposQueYaLaTenian?.length ?? 0
      setResumen(
        `Se ${plural(creadas, "marcó", "marcaron")} ${contar(creadas, "equipo")}.` +
          (yaLaTenian === 0
            ? ""
            : ` ${contar(yaLaTenian, "equipo")} ya ${plural(yaLaTenian, "tenía", "tenían")} esa misma marca.`),
      )
      // Los campos y la selección se limpian juntos: dejar destildado lo de la
      // vuelta anterior con el formulario en blanco es un formulario que miente.
      setValores(MARCA_VACIA)
      setDestildados(new Set())
      await queryClient.invalidateQueries({ queryKey: ["preferencias"] })
    },
  })

  if (isLoading) {
    return <p className="text-muted-foreground text-sm">Cargando equipos…</p>
  }
  // Sin los equipos no se sabe nada del carro: decir que no tiene ninguno
  // reservable sería afirmar un dato de inventario que no llegó.
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{getErrorMessage(error)}</AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <div>
        <h3 className="text-sm font-medium">Marcar preferencia para todo el carro</h3>
        <p className="text-muted-foreground text-xs">
          Al reservar, esa materia ve estos equipos primero y las demás los ven al final.
          Nadie queda excluido: cualquiera los puede reservar igual.
        </p>
      </div>

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
                    checked={!destildados.has(e.id)}
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
              className="grid gap-2 sm:grid-cols-[1fr_auto_auto_auto_auto] sm:items-end"
              onSubmit={(e) => {
                e.preventDefault()
                setResumen(null)
                marcar.mutate()
              }}
            >
              {empatadas.length > 0 && (
                <Alert className="sm:col-span-full">
                  <AlertDescription>
                    Equipos de este carro ya son preferentes de {empatadas.join(", ")} con
                    prioridad {valores.prioridad}. Con la misma prioridad ninguna le gana a
                    la otra: si querés que una mande, ponele un número distinto.
                  </AlertDescription>
                </Alert>
              )}
              <CamposDeMarca
                valores={valores}
                onChange={setValores}
                materias={materias}
                idBase={`carro-${carroId}`}
              />
              <span className="flex gap-2">
                <Button
                  type="submit"
                  size="sm"
                  disabled={
                    valores.materiaNombre === "" || elegidos.length === 0 || marcar.isPending
                  }
                >
                  Marcar
                </Button>
                <Button type="button" variant="outline" size="sm" onClick={onListo}>
                  Cerrar
                </Button>
              </span>
            </form>
          )}
        </>
      )}
    </div>
  )
}
