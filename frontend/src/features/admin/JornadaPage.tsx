import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { SelectorDeHora } from "@/components/SelectorDeHora"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { JORNADA_KEY } from "@/features/disponibilidad/api"
import { DIAS_SEMANA, etiquetaDia, ordenarPorDia } from "@/features/disponibilidad/types"
import type { BloqueHorario, DiaSemana } from "@/features/disponibilidad/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * La jornada de la institución: qué días y en qué horas abre la escuela.
 *
 * Es la pantalla que reemplaza a una suposición del código. Hasta acá el
 * sistema daba por sentado "lunes a viernes", lo cual dejaba afuera a las
 * escuelas de jornada extendida o albergue —que dictan el fin de semana— y a
 * las nocturnas, cuyo horario no se parece al de la mañana.
 *
 * Sin bloques cargados no hay restricción, y eso es deliberado: el sistema
 * no inventa un calendario que nadie le dijo. Recién cuando alguien declara
 * el primer tramo empieza a haber días cerrados.
 */
export function JornadaPage() {
  const queryClient = useQueryClient()
  const [dia, setDia] = useState<DiaSemana>("LUNES")
  const [horaInicio, setHoraInicio] = useState("08:00")
  const [horaFin, setHoraFin] = useState("12:00")
  const [error, setError] = useState("")

  const { data, isPending } = useQuery({
    queryKey: JORNADA_KEY,
    queryFn: disponibilidadApi.jornadaDeLaInstitucion,
  })
  const bloques = data?.data ?? []

  const invalidar = () => queryClient.invalidateQueries({ queryKey: JORNADA_KEY })

  const agregar = useMutation({
    mutationFn: () => disponibilidadApi.agregarBloqueDeJornada(dia, horaInicio, horaFin),
    onSuccess: () => {
      setError("")
      void invalidar()
    },
    onError: (e) => setError(getErrorMessage(e)),
  })

  const eliminar = useMutation({
    mutationFn: (id: string) => disponibilidadApi.eliminarBloqueDeJornada(id),
    onSuccess: () => {
      setError("")
      void invalidar()
    },
    onError: (e) => setError(getErrorMessage(e)),
  })

  const rangoInvalido = horaFin <= horaInicio

  return (
    <div className="mx-auto max-w-3xl">
      <EncabezadoDePagina
        titulo="Jornada de la escuela"
        descripcion="Los días y horas en que la escuela está abierta. Las reservas fuera de este horario se rechazan."
      />

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card className="mb-6">
        <CardContent className="grid gap-4 pt-6 sm:grid-cols-4 sm:items-end">
          <div className="grid gap-2">
            <Label htmlFor="dia">Día</Label>
            <Select
              id="dia"
              value={dia}
              onChange={(e) => setDia(e.target.value as DiaSemana)}
            >
              {DIAS_SEMANA.map((d) => (
                <option key={d.valor} value={d.valor}>
                  {d.etiqueta}
                </option>
              ))}
            </Select>
          </div>

          <SelectorDeHora
            id="jornada-inicio"
            etiqueta="Abre"
            valor={horaInicio}
            onCambio={setHoraInicio}
          />

          <SelectorDeHora
            id="jornada-fin"
            etiqueta="Cierra"
            valor={horaFin}
            onCambio={setHoraFin}
          />

          <Button
            onClick={() => agregar.mutate()}
            disabled={rangoInvalido || agregar.isPending}
          >
            Agregar tramo
          </Button>

          {rangoInvalido && (
            <p className="text-destructive text-sm sm:col-span-4">
              La hora de cierre tiene que ser posterior a la de apertura.
            </p>
          )}
        </CardContent>
      </Card>

      {isPending ? (
        <p className="text-muted-foreground text-sm">Cargando…</p>
      ) : bloques.length === 0 ? (
        // El texto importa: sin jornada cargada el sistema NO bloquea nada, y
        // quien mira esta pantalla vacía tiene que entender que eso es una
        // decisión y no una configuración a medias.
        <Alert>
          <AlertDescription>
            Todavía no se declaró la jornada, así que se puede reservar cualquier día y a
            cualquier hora. Cargá los tramos en que la escuela abre para que el sistema
            empiece a rechazar lo que queda afuera.
          </AlertDescription>
        </Alert>
      ) : (
        <ul className="grid gap-2">
          {ordenarPorDia(bloques).map((b: BloqueHorario) => (
            <li
              key={b.id}
              className="flex items-center justify-between rounded-md border px-4 py-3"
            >
              <span>
                <span className="font-medium">{etiquetaDia(b.diaSemana)}</span>{" "}
                <span className="text-muted-foreground">
                  de {b.horaInicio} a {b.horaFin}
                </span>
              </span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => eliminar.mutate(b.id)}
                disabled={eliminar.isPending}
              >
                Quitar
              </Button>
            </li>
          ))}
        </ul>
      )}

      <p className="text-muted-foreground mt-6 text-sm">
        Se pueden cargar varios tramos en el mismo día: una escuela con turno mañana y
        turno noche declara, por ejemplo, 07:00–12:00 y 18:00–23:00, y el mediodía queda
        cerrado. Los días sin ningún tramo son días en que la escuela no abre.
      </p>
    </div>
  )
}
