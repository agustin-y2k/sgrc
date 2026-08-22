import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { JORNADA_KEY } from "@/features/disponibilidad/api"
import type { TramoDeJornada } from "@/features/disponibilidad/types"
import {
  CamposDeTramo,
  motivoParaNoGuardar,
  TRAMO_VACIO,
} from "@/features/admin/CamposDeTramo"
import type { FormTramo } from "@/features/admin/CamposDeTramo"
import { etiquetaDeDias, expandirDias } from "@/features/admin/jornada"
import { getErrorMessage } from "@/lib/api-client"

/**
 * El primer arranque: la institución declara su jornada, o dice que no quiere
 * restringir nada.
 *
 * Existe porque las dos cosas se veían iguales desde adentro —una lista de
 * tramos vacía— y el sistema no tenía forma de saber si preguntar. Mientras
 * no se responda, un Admin no puede navegar a otra parte: es la única
 * decisión de la que dependen las reservas de toda la escuela, y descubrirla
 * tarde es lo que obliga a cancelar clases ya cargadas.
 *
 * No se pregunta nada más acá. Un asistente de primer arranque que pida
 * también los equipos y las materias es un cuestionario para adivinar, y esta
 * es la única respuesta que no se puede diferir sin costo.
 */
export function PrimeraJornadaPage() {
  const queryClient = useQueryClient()
  const [tramos, setTramos] = useState<TramoDeJornada[]>([])
  const [nuevo, setNuevo] = useState<FormTramo>(TRAMO_VACIO)
  const [fallo, setFallo] = useState<string | null>(null)

  const guardar = useMutation({
    mutationFn: (jornada: TramoDeJornada[]) =>
      disponibilidadApi.reemplazarJornada(jornada),
    // Sin navigate: en cuanto la jornada queda definida, el portón de
    // ProtectedRoute deja de mandar acá y la pantalla que el Admin quería
    // aparece sola. Redirigir a mano competiría con eso.
    onSuccess: async () => {
      setFallo(null)
      await queryClient.invalidateQueries({ queryKey: JORNADA_KEY })
    },
    onError: (e) => setFallo(getErrorMessage(e)),
  })

  const motivoNuevo = motivoParaNoGuardar(nuevo)

  function agregarTramo() {
    setTramos((actuales) => [
      ...actuales,
      ...expandirDias(nuevo.dias, nuevo.horaInicio, nuevo.horaFin),
    ])
    setNuevo(TRAMO_VACIO)
  }

  // Los tramos se juntan por horario solo para leerlos: "Lunes a viernes de
  // 08:00 a 12:00" en vez de cinco líneas iguales.
  const porHorario = new Map<string, TramoDeJornada[]>()
  for (const t of tramos) {
    const clave = `${t.horaInicio}-${t.horaFin}`
    porHorario.set(clave, [...(porHorario.get(clave) ?? []), t])
  }

  return (
    <div className="mx-auto flex min-h-svh max-w-2xl flex-col justify-center gap-6 p-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          ¿Cuándo abre la escuela?
        </h1>
        <p className="text-muted-foreground mt-2 text-sm">
          Los días y horas que declares acá son los que el sistema va a aceptar para
          reservar. Se puede cambiar después, pero conviene acertarle ahora: cuando ya
          haya reservas cargadas, achicar la jornada obliga a cancelar las que queden
          afuera.
        </p>
      </div>

      {fallo !== null && (
        <Alert variant="destructive">
          <AlertDescription>{fallo}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardContent className="grid gap-4 pt-6">
          <CamposDeTramo valor={nuevo} onCambio={setNuevo} idPrefijo="primera" />
          <div className="flex flex-wrap items-center gap-3">
            <Button
              variant="outline"
              disabled={motivoNuevo !== "" || guardar.isPending}
              onClick={agregarTramo}
            >
              Agregar tramo
            </Button>
            {motivoNuevo !== "" && (
              <p className="text-muted-foreground text-sm">{motivoNuevo}</p>
            )}
          </div>
        </CardContent>
      </Card>

      {tramos.length > 0 && (
        <ul className="grid gap-1.5">
          {[...porHorario.values()].map((delMismoHorario) => {
            const dias = delMismoHorario.map((t) => t.diaSemana)
            const { horaInicio, horaFin } = delMismoHorario[0]
            return (
              <li
                key={`${horaInicio}-${horaFin}`}
                className="flex flex-wrap items-center justify-between gap-2 rounded-md border px-4 py-2 text-sm"
              >
                <span>
                  <span className="font-medium">{etiquetaDeDias(dias)}</span>{" "}
                  <span className="text-muted-foreground">
                    de {horaInicio} a {horaFin}
                  </span>
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={guardar.isPending}
                  aria-label={`Quitar ${etiquetaDeDias(dias)} de ${horaInicio} a ${horaFin}`}
                  onClick={() =>
                    setTramos((actuales) =>
                      actuales.filter((t) => !delMismoHorario.includes(t))
                    )
                  }
                >
                  Quitar
                </Button>
              </li>
            )
          })}
        </ul>
      )}

      <div className="flex flex-wrap items-center gap-3">
        <Button
          disabled={tramos.length === 0 || guardar.isPending}
          onClick={() => guardar.mutate(tramos)}
        >
          Guardar la jornada
        </Button>
        {/* La salida sin declarar nada tiene que existir y estar dicha con
            todas las letras: quien está probando el sistema no sabe todavía
            qué horario tiene la escuela, y obligarlo a inventar uno es
            producir el error que esta pantalla vino a evitar. */}
        <Button
          variant="ghost"
          disabled={guardar.isPending}
          onClick={() => guardar.mutate([])}
        >
          Dejarla libre por ahora
        </Button>
      </div>

      <p className="text-muted-foreground text-sm">
        Si la dejás libre se va a poder reservar cualquier día y a cualquier hora, y no se
        vuelve a preguntar. La jornada se declara después desde{" "}
        <span className="font-medium">Jornada de la escuela</span>.
      </p>

      <p className="text-muted-foreground text-sm">
        Se pueden cargar varios tramos para el mismo día: una escuela con turno mañana y
        turno noche declara, por ejemplo, 07:00–12:00 y 18:00–23:00, y el mediodía queda
        cerrado. Una nocturna declara 20:00–01:00: si la hora de cierre es menor que la de
        apertura, el tramo termina al día siguiente. Los días que no cargues son días en
        que la escuela no abre.
      </p>
    </div>
  )
}
