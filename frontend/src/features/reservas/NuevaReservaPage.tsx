import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "react-router"

import { SelectorDeHora } from "@/components/SelectorDeHora"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { JORNADA_KEY } from "@/features/disponibilidad/api"
import { dentroDeLaJornada } from "@/features/disponibilidad/types"
import * as reservasApi from "@/features/reservas/api"
import { SelectorDeEquipos } from "@/features/reservas/SelectorDeEquipos"
import {
  DIAS_SEMANA,
  MAX_HORAS_RESERVA,
  cruzaMedianoche,
  excedeDuracionMaxima,
  hoyISO,
  type DiaSemana,
} from "@/features/reservas/types"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaLargaCapitalizada } from "@/lib/fechas"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { contar } from "@/lib/plural"

type Modo = "simple" | "recurrente"

/** ["a", "b", "c"] → "a, b y c". Un `join(", ")` suelto se lee a los tirones. */
function enumerar(partes: string[]): string {
  if (partes.length <= 1) return partes.join("")
  return `${partes.slice(0, -1).join(", ")} y ${partes[partes.length - 1]}`
}

// RF-04.2 (reserva puntual) y RF-04.5 (recurrente). Los dos modos comparten
// materia, horario y selección de equipos; lo único que cambia es si se pide una
// fecha o un día de la semana más un rango.
export function NuevaReservaPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [modo, setModo] = useState<Modo>("simple")
  const [materiaId, setMateriaId] = useState("")
  const [fecha, setFecha] = useState("")
  const [horaInicio, setHoraInicio] = useState("")
  const [horaFin, setHoraFin] = useState("")
  const [diaSemana, setDiaSemana] = useState<DiaSemana>("LUNES")
  const [fechaInicio, setFechaInicio] = useState("")
  const [fechaFin, setFechaFin] = useState("")
  const [equipoIds, setPcIds] = useState<string[]>([])

  const { data: materias, isLoading: cargandoMaterias } = useQuery({
    queryKey: ["mis-materias"],
    queryFn: reservasApi.misMaterias,
  })

  const crear = useMutation({
    /**
     * Devuelve la frase que va a leer el usuario en el listado, no el
     * cuerpo de la respuesta: los dos endpoints responden cosas distintas
     * y ninguna de las dos se usa acá. Se arma con lo que el formulario ya
     * tiene a mano.
     */
    mutationFn: async (): Promise<string> => {
      const cuantosEquipos = `${contar(equipoIds.length, "computadora")}`

      if (modo === "simple") {
        await reservasApi.crearReserva({
          materiaId,
          fecha,
          horaInicio,
          horaFin,
          equipoIds,
        })
        return `Reserva confirmada para el ${formatearFechaLargaCapitalizada(fecha)}, de ${horaInicio} a ${horaFin}, con ${cuantosEquipos}.`
      }

      await reservasApi.crearReservaRecurrente({
        materiaId,
        diaSemana,
        horaInicio,
        horaFin,
        fechaInicio,
        fechaFin,
        equipoIds,
      })
      const dia = DIAS_SEMANA.find((d) => d.valor === diaSemana)?.etiqueta ?? diaSemana
      return `Reserva recurrente confirmada: todos los ${dia.toLowerCase()} de ${horaInicio} a ${horaFin}, con ${cuantosEquipos}, hasta el ${formatearFechaLargaCapitalizada(fechaFin)}.`
    },
    // El listado es la confirmación: la reserva recién creada aparece ahí.
    // Pero llegar a una pantalla nueva no alcanza para saber que salió bien
    // —sobre todo en el modo recurrente, donde se crearon varias de una— así
    // que el mensaje viaja en el estado de navegación y se muestra al llegar.
    onSuccess: async (confirmacion) => {
      await queryClient.invalidateQueries({ queryKey: ["reservas"] })
      navigate("/reservas", { state: { confirmacion } })
    },
  })

  // La franja de la que depende el selector de equipos: en modo recurrente se
  // usa la primera fecha del rango como muestra, porque el backend valida
  // la disponibilidad de TODAS las ocurrencias al crear (RF-04.5) — acá
  // solo se necesita una lista razonable para tildar.
  const fechaParaDisponibilidad = modo === "simple" ? fecha : fechaInicio

  const materiasDisponibles = materias?.data ?? []
  // El `min` de los inputs cubre la fecha; esto cubre el horario, que el
  // navegador no puede limitar solo.
  const duracionExcesiva = excedeDuracionMaxima(horaInicio, horaFin)

  // La jornada declarada por la institución. Se consulta para avisar acá en
  // vez de dejar que el backend conteste 400 después de completar todo el
  // formulario: el error es el mismo, pero llega cuando ya no se puede
  // corregir sin volver a empezar.
  //
  // Sin jornada declarada, dentroDeLaJornada devuelve true y este aviso no
  // aparece nunca — que es exactamente lo que tiene que pasar mientras nadie
  // haya dicho qué días abre la escuela.
  const { data: jornada } = useQuery({
    queryKey: JORNADA_KEY,
    queryFn: disponibilidadApi.jornadaDeLaInstitucion,
  })
  const bloquesDeJornada = jornada?.data ?? []
  const fueraDeLaJornada =
    fechaParaDisponibilidad !== "" &&
    horaInicio !== "" &&
    horaFin !== horaInicio &&
    !dentroDeLaJornada(bloquesDeJornada, fechaParaDisponibilidad, horaInicio, horaFin)

  /**
   * Lo que todavía falta para poder confirmar, dicho en palabras.
   *
   * Un booleano que apague el botón y nada más no alcanza: en un formulario
   * de siete campos, un botón gris no dice cuál de los siete es el que
   * falta, y la persona no tiene forma de saber qué corregir. Algunos
   * errores se explican abajo del campo (la duración, la jornada de la escuela),
   * pero los más comunes —no elegí materia, no tildé ningún equipo— no tienen
   * dónde aparecer.
   *
   * Los que ya tienen su propio mensaje al lado del campo no se repiten
   * acá: alcanza con que el botón siga apagado mientras ese texto en rojo
   * esté a la vista.
   */
  const faltantes: string[] = []
  if (materiaId === "") faltantes.push("elegir la materia")
  if (modo === "simple") {
    if (fecha === "") faltantes.push("elegir la fecha")
  } else {
    if (fechaInicio === "") faltantes.push("indicar desde qué fecha se repite")
    if (fechaFin === "") faltantes.push("indicar hasta qué fecha se repite")
  }
  if (horaInicio === "") faltantes.push("elegir la hora de inicio")
  if (horaFin === "") faltantes.push("elegir la hora de fin")
  if (equipoIds.length === 0) faltantes.push("tildar al menos una computadora")

  const listoParaEnviar =
    faltantes.length === 0 &&
    horaFin !== horaInicio &&
    !duracionExcesiva &&
    !fueraDeLaJornada

  return (
    <div className="mx-auto max-w-3xl">
      <EncabezadoDePagina
        titulo="Nueva reserva"
        descripcion="Elegí el día, desde qué hora hasta qué hora, y qué computadoras necesitás. Los horarios van en formato de 24 horas."
      />

      {!cargandoMaterias && materiasDisponibles.length === 0 && (
        <Alert className="mb-4">
          <AlertDescription>
            No estás asignado a ninguna materia de un ciclo activo, así que todavía no
            podés reservar. Pedile a un Admin que te asigne.
          </AlertDescription>
        </Alert>
      )}

      {crear.error && (
        <Alert variant="destructive" className="mb-4">
          {/* RF-04.3: el mensaje del backend nombra qué equipo choca, en qué
              fecha y con quién. Se muestra tal cual: uno genérico obligaría a
              destildar de a uno para encontrar el ocupado. */}
          <AlertDescription>{getErrorMessage(crear.error)}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Datos de la reserva</CardTitle>
          <CardDescription>
            Podés combinar computadoras de distintos carros en la misma reserva.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="grid gap-5"
            onSubmit={(e) => {
              e.preventDefault()
              crear.mutate()
            }}
          >
            <div className="flex gap-2">
              <Button
                type="button"
                variant={modo === "simple" ? "default" : "outline"}
                size="sm"
                onClick={() => setModo("simple")}
              >
                Una sola fecha
              </Button>
              <Button
                type="button"
                variant={modo === "recurrente" ? "default" : "outline"}
                size="sm"
                onClick={() => setModo("recurrente")}
              >
                Se repite todas las semanas
              </Button>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="materia">Materia</Label>
              <Select
                id="materia"
                value={materiaId}
                onChange={(e) => setMateriaId(e.target.value)}
              >
                <option value="">Elegí una materia…</option>
                {materiasDisponibles.map((m) => (
                  <option key={m.materiaId} value={m.materiaId}>
                    {m.materiaNombre} — {m.cursoNombre} ({m.cicloAnio})
                  </option>
                ))}
              </Select>
            </div>

            {modo === "simple" ? (
              <div className="grid gap-2">
                <Label htmlFor="fecha">Fecha</Label>
                <Input
                  id="fecha"
                  type="date"
                  min={hoyISO()}
                  value={fecha}
                  onChange={(e) => setFecha(e.target.value)}
                />
              </div>
            ) : (
              <div className="grid gap-4 sm:grid-cols-3">
                <div className="grid gap-2">
                  <Label htmlFor="diaSemana">Día de la semana</Label>
                  <Select
                    id="diaSemana"
                    value={diaSemana}
                    onChange={(e) => setDiaSemana(e.target.value as DiaSemana)}
                  >
                    {DIAS_SEMANA.map((d) => (
                      <option key={d.valor} value={d.valor}>
                        {d.etiqueta}
                      </option>
                    ))}
                  </Select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="fechaInicio">Desde</Label>
                  <Input
                    id="fechaInicio"
                    type="date"
                    min={hoyISO()}
                    value={fechaInicio}
                    onChange={(e) => setFechaInicio(e.target.value)}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="fechaFin">Hasta</Label>
                  <Input
                    id="fechaFin"
                    type="date"
                    min={fechaInicio !== "" ? fechaInicio : hoyISO()}
                    value={fechaFin}
                    onChange={(e) => setFechaFin(e.target.value)}
                  />
                </div>
              </div>
            )}

            <div className="grid gap-4 sm:grid-cols-2">
              <SelectorDeHora
                id="horaInicio"
                etiqueta="Hora de inicio"
                valor={horaInicio}
                onCambio={setHoraInicio}
              />
              <SelectorDeHora
                id="horaFin"
                etiqueta="Hora de fin"
                valor={horaFin}
                onCambio={setHoraFin}
              />
            </div>

            {horaInicio !== "" && horaFin !== "" && horaFin === horaInicio && (
              <p className="text-destructive text-sm">
                La hora de fin no puede ser igual a la de inicio.
              </p>
            )}

            {/* Que la clase termine al día siguiente es válido y es lo normal
                en una escuela nocturna, pero conviene decirlo: alguien que
                puso 01:00 por error tiene que poder darse cuenta antes de
                confirmar. */}
            {cruzaMedianoche(horaInicio, horaFin) && (
              <p className="text-muted-foreground text-sm">
                Esta reserva termina al día siguiente, a las {horaFin}.
              </p>
            )}

            {duracionExcesiva && (
              <p className="text-destructive text-sm">
                Una reserva no puede durar más de {MAX_HORAS_RESERVA} horas.
              </p>
            )}

            {fueraDeLaJornada && (
              <p className="text-destructive text-sm">
                Ese día y horario quedan fuera de la jornada de la escuela. Consultá con
                un Admin si el horario de la escuela cambió.
              </p>
            )}

            <div className="grid gap-2">
              <Label>Qué computadoras necesitás</Label>
              {modo === "recurrente" && (
                <p className="text-muted-foreground text-xs">
                  Se muestran las libres en la fecha de inicio. Al confirmar, el sistema
                  valida todas las fechas de la serie y no crea ninguna si alguna está
                  ocupada.
                </p>
              )}
              <SelectorDeEquipos
                fecha={fechaParaDisponibilidad}
                horaInicio={horaInicio}
                horaFin={horaFin}
                seleccionadas={equipoIds}
                onCambio={setPcIds}
                materiaId={materiaId}
              />
            </div>

            {/* `aria-live`: la lista cambia sola a medida que se completa el
                formulario, sin que nada reciba el foco. Sin esto, alguien
                que usa lector de pantalla no se entera de que ya está. */}
            {faltantes.length > 0 && (
              <p className="text-muted-foreground text-sm" aria-live="polite">
                {faltantes.length === 1
                  ? "Para confirmar falta"
                  : "Para confirmar faltan"}{" "}
                {enumerar(faltantes)}.
              </p>
            )}

            <div className="flex gap-2">
              <Button type="submit" disabled={!listoParaEnviar || crear.isPending}>
                {crear.isPending ? "Reservando…" : "Confirmar reserva"}
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => navigate("/reservas")}
              >
                Cancelar
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
