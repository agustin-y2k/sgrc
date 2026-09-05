import { useEffect, useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

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
import * as academicoApi from "@/features/academico/api"
import { cursoDe, etiquetaDeCurso } from "@/features/academico/types"
import { useCursosDelCicloActivo } from "@/features/academico/useCursosDelCicloActivo"
import * as adminApi from "@/features/admin/api"
import {
  LUGARES_DE_LA_ESCUELA,
  PRESTAMOS_KEY,
} from "@/features/admin/entregas/compartido"
import {
  SelectorDeEquipos,
  type EquipoParaEntregar,
} from "@/features/admin/entregas/SelectorDeEquipos"
import * as inventoryApi from "@/features/inventory/api"
import * as reservasApi from "@/features/reservas/api"
import { getErrorMessage } from "@/lib/api-client"
import { contar } from "@/lib/plural"
import { sinTildes } from "@/lib/texto"

/**
 * Entregar algo sin reserva detrás: "necesito una compu para hacer un
 * trámite", "me llevo el proyector".
 *
 * Es la entrega que más se usa, así que ocupa todo el ancho que le den y no
 * media columna: quien la completa tiene a alguien esperando enfrente, y los
 * equipos se eligen de una grilla que se ve entera, no de una lista de tres
 * renglones con barra de desplazamiento.
 */
export function EntregaSuelta({
  yaAfuera,
  onCerrar,
}: {
  yaAfuera: Set<string>
  /** Con un resumen cuando la entrega salió; sin nada si cerraron el cuadro. */
  onCerrar: (resumen?: string) => void
}) {
  const queryClient = useQueryClient()
  const [nombre, setNombre] = useState("")
  const [destino, setDestino] = useState("")
  // Si el Admin ya escribió algo en el destino. Mientras no lo haya tocado, el
  // sistema lo completa con el curso de quien retira y lo corrige si cambia la
  // persona; en cuanto lo toca —aunque sea para vaciarlo— no lo pisa más.
  const [destinoTocado, setDestinoTocado] = useState(false)
  const [devolucion, setDevolucion] = useState("")
  const [seleccionadas, setSeleccionadas] = useState<Set<string>>(new Set())

  // Los cursos del ciclo, para sugerirlos como destino. Es la mitad de la
  // lista: la otra son los lugares que no son un curso.
  const cursos = useCursosDelCicloActivo()

  // Quién puede estar del otro lado del mostrador. Sólo las cuentas
  // aprobadas: una pendiente todavía no es nadie para el sistema, y asociarle
  // una entrega sería darle una existencia que no tiene.
  const { data: usuarios } = useQuery({
    queryKey: ["usuarios", "aprobados"],
    queryFn: () => adminApi.listarUsuarios({ estado: "APROBADA", pageSize: 200 }),
  })

  const personas = useMemo(
    () =>
      (usuarios?.data ?? []).map((u) => ({
        id: u.id,
        nombre: `${u.nombre} ${u.apellido}`,
      })),
    [usuarios]
  )

  // El nombre sigue siendo LIBRE: quien viene puede no tener cuenta, y ése es
  // el caso normal en el mostrador. Lo que se busca es la coincidencia exacta
  // —sin tildes ni mayúsculas— para poder asociar la entrega a esa cuenta; si
  // no la hay, se guarda el texto tal como se escribió, como siempre.
  const homonimos = useMemo(() => {
    const buscado = sinTildes(nombre.trim())
    if (!buscado) return []
    return personas.filter((p) => sinTildes(p.nombre) === buscado)
  }, [personas, nombre])

  // Con dos cuentas del mismo nombre no hay a cuál asociarla, y elegir una
  // sería inventar: queda como texto y se avisa.
  const persona = homonimos.length === 1 ? homonimos[0] : undefined

  const { data: materiasDeLaPersona } = useQuery({
    queryKey: ["materias-de-docente", persona?.id],
    queryFn: () => academicoApi.materiasDeDocente(persona!.id),
    enabled: !!persona,
  })

  // Dónde da clase, sin repetir: es el destino más probable de la entrega. Con
  // la modalidad al lado cuando la hay, porque el nombre solo puede repetirse
  // entre dos carreras y el destino tiene que decir a cuál se fue.
  const cursosDeLaPersona = useMemo(
    () => [
      ...new Set(
        (materiasDeLaPersona?.data ?? []).map((m) => etiquetaDeCurso(cursoDe(m)))
      ),
    ],
    [materiasDeLaPersona]
  )

  // Con un solo curso el destino se completa solo; con varios no se adivina,
  // pero quedan primeros en la lista. Y siempre se puede borrar: puede que la
  // pida para ella, que la lleve a sala de profesores o que no lo declare.
  const unicoCurso = cursosDeLaPersona.length === 1 ? cursosDeLaPersona[0] : undefined
  useEffect(() => {
    if (!destinoTocado) setDestino(unicoCurso ?? "")
  }, [unicoCurso, destinoTocado])

  const destinoSugerido = !destinoTocado && destino !== "" && destino === unicoCurso

  const { data: carros } = useQuery({
    queryKey: ["carros"],
    queryFn: inventoryApi.listarCarros,
  })

  // Todo el inventario en UNA consulta: el endpoint sin filtro ya devuelve
  // los de carro y los sueltos juntos, así que no hace falta pedir carro por
  // carro y unir las respuestas.
  const { data: todos } = useQuery({
    queryKey: ["equipos"],
    queryFn: () => inventoryApi.listarEquipos(),
  })

  const nombreDeCarro = useMemo(
    () => new Map((carros?.data ?? []).map((c) => [c.id, c.nombre])),
    [carros]
  )

  // Se ofrece lo que está en el inventario, en condiciones de prestarse y no
  // está ya afuera. Un equipo en mantenimiento o fuera de servicio está acá y
  // no se le da a nadie (RF-08.17): para sacarlo del laboratorio está la
  // salida a reparación, que es otra pantalla a propósito.
  const equipos: EquipoParaEntregar[] = useMemo(
    () =>
      (todos?.data ?? [])
        .filter(
          (eq) => !eq.dadoDeBaja && eq.estado === "DISPONIBLE" && !yaAfuera.has(eq.id)
        )
        .map((eq) => ({
          id: eq.id,
          etiqueta: eq.etiqueta,
          donde: eq.carroId ? (nombreDeCarro.get(eq.carroId) ?? "") : eq.tipo,
        })),
    [todos, nombreDeCarro, yaAfuera]
  )

  // Qué se está por entregar, escrito con los nombres que se leen en la
  // etiqueta de la máquina: la grilla puede quedar desplazada y la selección
  // fuera de la vista justo cuando se aprieta el botón.
  const nombresElegidos = useMemo(
    () => equipos.filter((eq) => seleccionadas.has(eq.id)).map((eq) => eq.etiqueta),
    [equipos, seleccionadas]
  )

  const entregar = useMutation({
    mutationFn: () =>
      reservasApi.entregarSuelta({
        equipoIds: [...seleccionadas],
        nombre: nombre.trim(),
        // El nombre va SIEMPRE, tenga cuenta o no: es un snapshot, para que el
        // registro siga diciendo quién se la llevó si esa cuenta se elimina.
        usuarioId: persona?.id,
        destino: destino.trim() || undefined,
        devolucionEstimada: devolucion ? new Date(devolucion).toISOString() : undefined,
      }),
    onSuccess: async (respuesta) => {
      const avisos = respuesta.avisos ?? []
      const noSalieron = respuesta.noEntregadas ?? []
      const partes = [`Salieron ${contar(respuesta.entregadas.length, "equipo")}.`]
      if (noSalieron.length > 0) {
        partes.push(
          `No salieron ${noSalieron.length}: ${noSalieron.map((n) => n.detalle).join("; ")}`
        )
      }
      // El aviso no impidió nada: el sistema no sabe cuánto dura un trámite,
      // así que la decisión es del Admin.
      for (const a of avisos) {
        partes.push(
          `Ojo: esa máquina tiene reserva ${a.fecha} de ${a.horaInicio} a ${a.horaFin}${a.docente ? ` (${a.docente})` : ""}.`
        )
      }
      await queryClient.invalidateQueries({ queryKey: PRESTAMOS_KEY })
      // El cuadro se cierra solo: entregar es lo último que se hace acá, y
      // dejarlo abierto obliga a apretar «Cerrar» con alguien esperando
      // enfrente. El resumen sube a la página, arriba de la lista de lo que
      // está afuera, donde las máquinas que acaban de salir ya figuran.
      onCerrar(partes.join(" "))
    },
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Entregar sin reserva</CardTitle>
        <CardDescription>
          Para cuando piden algo en el momento — una computadora para un trámite, el
          proyector para una charla. No hace falta que la persona tenga cuenta en el
          sistema.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-6"
          onSubmit={(e) => {
            e.preventDefault()
            entregar.mutate()
          }}
        >
          {/* Los datos de la persona a la izquierda y los equipos a la
              derecha, que es el orden en que se pregunta en el mostrador. En
              un teléfono se apilan en ese mismo orden. */}
          <div className="grid gap-6 lg:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]">
            <div className="grid content-start gap-4">
              {/* Se sugieren las cuentas del sistema, pero el campo es libre:
                  quien viene a buscar algo muchas veces no tiene cuenta —de
                  secretaría, de preceptoría, un alumno— y ése es el caso
                  normal. Reconocerla sólo agrega: la entrega queda en el
                  historial de esa persona y el reclamo de devolución le llega
                  por correo. */}
              <div className="grid gap-1.5">
                <Label htmlFor="entrega-nombre">¿A quién?</Label>
                <Input
                  id="entrega-nombre"
                  value={nombre}
                  onChange={(e) => setNombre(e.target.value)}
                  list="personas-del-sistema"
                  placeholder="Ej.: Marta (secretaría)"
                  required
                />
                <datalist id="personas-del-sistema">
                  {personas.map((p) => (
                    <option key={p.id} value={p.nombre} />
                  ))}
                </datalist>
                {persona && (
                  <p className="text-muted-foreground text-xs">
                    Tiene cuenta: la entrega le queda anotada y, si se pasa de hora, el
                    reclamo le llega por correo.
                    {cursosDeLaPersona.length > 0 &&
                      ` Da clase en ${cursosDeLaPersona.join(", ")}.`}
                  </p>
                )}
                {homonimos.length > 1 && (
                  <p className="text-muted-foreground text-xs">
                    Hay {homonimos.length} cuentas con ese nombre, así que no se asocia a
                    ninguna: queda anotado como texto.
                  </p>
                )}
              </div>

              {/* A dónde va, y no para qué: el «para qué» se contesta
                  siempre igual —"para dar clase", "para un trámite"— y no
                  sirve para ir a buscar la máquina a las cinco de la tarde.
                  Sugiere los cursos del ciclo y los lugares de la escuela,
                  pero es libre: el primer destino no previsto tiene que poder
                  anotarse igual. */}
              <div className="grid gap-1.5">
                <Label htmlFor="entrega-destino">¿A dónde va? (opcional)</Label>
                <Input
                  id="entrega-destino"
                  value={destino}
                  onChange={(e) => {
                    setDestino(e.target.value)
                    setDestinoTocado(true)
                  }}
                  list="destinos-de-entrega"
                  placeholder="Ej.: 1°4°, Biblioteca"
                />
                {/* Los cursos de quien retira van primero: es lo más probable
                    cuando la persona tiene cuenta. Después el resto del ciclo
                    y los lugares que no son un curso. */}
                <datalist id="destinos-de-entrega">
                  {cursosDeLaPersona.map((c) => (
                    <option key={`suyo-${c}`} value={c} />
                  ))}
                  {cursos
                    .filter((c) => !cursosDeLaPersona.includes(etiquetaDeCurso(c)))
                    .map((c) => (
                      <option key={c.id} value={etiquetaDeCurso(c)} />
                    ))}
                  {LUGARES_DE_LA_ESCUELA.map((l) => (
                    <option key={l} value={l} />
                  ))}
                </datalist>
                {destinoSugerido && (
                  <p className="text-muted-foreground text-xs">
                    Lo puso el sistema porque es el único curso que da. Si se la lleva a
                    otro lado —o no lo dice—, cambialo o borralo.
                  </p>
                )}
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="entrega-devolucion">
                  ¿Cuándo la devuelve? (opcional)
                </Label>
                <Input
                  id="entrega-devolucion"
                  type="datetime-local"
                  value={devolucion}
                  onChange={(e) => setDevolucion(e.target.value)}
                />
                {/* Sin hora pactada no se le reclama nada: "vengo en un rato" es
                    una respuesta válida, y una hora inventada solo generaría
                    reclamos falsos. */}
                <p className="text-muted-foreground text-xs">
                  Si no la sabés, dejalo vacío: no se le va a reclamar la devolución.
                </p>
              </div>
            </div>

            <SelectorDeEquipos
              titulo="¿Qué equipos?"
              equipos={equipos}
              seleccionados={seleccionadas}
              onSeleccionar={setSeleccionadas}
              vacio="No hay equipos disponibles para entregar. Los que están en mantenimiento o fuera de servicio no se prestan, y los que ya salieron figuran en «Afuera del laboratorio»."
            />
          </div>

          {entregar.error && (
            <Alert variant="destructive">
              <AlertDescription>{getErrorMessage(entregar.error)}</AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t pt-4">
            <Button
              type="submit"
              size="lg"
              disabled={entregar.isPending || seleccionadas.size === 0}
            >
              {seleccionadas.size === 0
                ? "Entregar"
                : `Entregar ${contar(seleccionadas.size, "equipo")}`}
            </Button>
            <Button type="button" variant="outline" onClick={() => onCerrar()}>
              Cerrar
            </Button>
            {nombresElegidos.length > 0 && (
              <p className="text-muted-foreground min-w-0 flex-1 text-sm">
                Salen: {nombresElegidos.join(", ")}
              </p>
            )}
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
