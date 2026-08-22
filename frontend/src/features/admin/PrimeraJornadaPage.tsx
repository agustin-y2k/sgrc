import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "react-router"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { useAuth } from "@/features/auth/AuthContext"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { JORNADA_KEY } from "@/features/disponibilidad/api"
import { impactoDelError } from "@/features/disponibilidad/types"
import type {
  ImpactoDeJornada,
  PrestamoAfectado,
  TramoDeJornada,
} from "@/features/disponibilidad/types"
import { ImpactoDeLaJornada } from "@/features/admin/ImpactoDeLaJornada"
import {
  CamposDeTramo,
  motivoParaNoGuardar,
  TRAMO_VACIO,
} from "@/features/admin/CamposDeTramo"
import type { FormTramo } from "@/features/admin/CamposDeTramo"
import { etiquetaDeDias, expandirDias } from "@/features/admin/jornada"
import { descartarPedido } from "@/features/admin/pedidoDeJornada"
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
  const { logout } = useAuth()
  const navigate = useNavigate()
  const [tramos, setTramos] = useState<TramoDeJornada[]>([])
  const [nuevo, setNuevo] = useState<FormTramo>(TRAMO_VACIO)
  const [fallo, setFallo] = useState<string | null>(null)
  // La jornada que espera confirmación, con lo que dejaría afuera.
  const [porConfirmar, setPorConfirmar] = useState<{
    tramos: TramoDeJornada[]
    impacto: ImpactoDeJornada
  } | null>(null)
  // Lo que se canceló al guardar, cuando hubo algo. Con esto puesto la
  // pantalla deja de ser un formulario y pasa a ser el resumen de lo que
  // acaba de pasar.
  const [loCancelado, setLoCancelado] = useState<{
    clases: number
    equipos: number
    prestamos: PrestamoAfectado[]
  } | null>(null)

  const guardar = useMutation({
    mutationFn: ({
      jornada,
      confirmado,
    }: {
      jornada: TramoDeJornada[]
      confirmado: boolean
    }) => disponibilidadApi.reemplazarJornada(jornada, confirmado),
    onSuccess: async (respuesta) => {
      setFallo(null)
      const clases = respuesta.clasesCanceladas ?? 0
      // Sin nada que contar se sale de largo, como siempre: un paso extra
      // para decir "no pasó nada" es peor que no decirlo. Con clases caídas,
      // en cambio, este es el único momento del flujo en que el Admin puede
      // hacer algo al respecto.
      if (clases > 0) {
        setLoCancelado({
          clases,
          equipos: respuesta.reservasCanceladas ?? 0,
          prestamos: porConfirmar?.impacto.prestamos ?? [],
        })
      }
      setPorConfirmar(null)
      await queryClient.invalidateQueries({ queryKey: JORNADA_KEY })
      // Hay que sacarlo de acá a mano. Esta ruta vive DENTRO de
      // ProtectedRoute, así que cuando el portón deja de redirigir no pasa
      // nada: la pantalla se vuelve a dibujar a sí misma y el Admin queda
      // mirando el formulario que acaba de guardar, sin señal de que
      // funcionó. Con clases canceladas no se navega: ahí manda el resumen,
      // que tiene su propio botón para entrar.
      if (clases === 0) {
        navigate("/", { replace: true })
      }
    },
    // Acá el 409 no es un detalle de comodidad como en la pantalla de
    // jornada: es la diferencia entre poder seguir y quedar encerrado.
    //
    // Una instalación que venía funcionando SIN jornada declarada llega a esta
    // pantalla con meses de reservas cargadas, y cualquier horario que declare
    // va a dejar alguna afuera. Sin poder confirmar, el Admin no sale de acá
    // —el portón lo devuelve— y su única salida sería dejar la jornada libre,
    // o sea rendirse.
    onError: (e, variables) => {
      const impacto = impactoDelError(e)
      if (impacto !== null) {
        setPorConfirmar({ tramos: variables.jornada, impacto })
        setFallo(null)
        return
      }
      setFallo(getErrorMessage(e))
    },
  })

  function proponer(jornada: TramoDeJornada[]) {
    guardar.mutate({ jornada, confirmado: false })
  }

  // Postergar no guarda nada: no hay jornada que declarar y una jornada vacía
  // ya es el estado actual. Solo se calla el pedido por esta sesión, y en el
  // próximo inicio vuelve.
  function postergar() {
    descartarPedido()
    navigate("/", { replace: true })
  }

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

  // Guardado con clases caídas: la pantalla pasa a contar qué pasó en vez de
  // soltar al Admin adentro del sistema sin decirle nada. Llegar acá solo es
  // posible en una instalación que venía funcionando sin jornada declarada, o
  // sea justo donde el número es grande.
  if (loCancelado !== null) {
    return (
      <div className="mx-auto flex min-h-svh max-w-2xl flex-col justify-center gap-6 p-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            La jornada quedó declarada
          </h1>
          <p className="text-destructive mt-2 text-sm">
            Se cancelaron <strong>{loCancelado.clases}</strong>{" "}
            {loCancelado.clases === 1 ? "clase" : "clases"}
            {loCancelado.equipos !== loCancelado.clases && (
              <> ({loCancelado.equipos} equipos)</>
            )}{" "}
            que quedaban fuera del horario, y ya se les avisó por correo a sus docentes.
          </p>
        </div>

        {/* Se repite lo que ya vio al confirmar, y a propósito: ahí estaba
            decidiendo, acá ya decidió. Este es el momento en que puede ir a
            hablar con esa persona antes de que le llegue el correo. */}
        {loCancelado.prestamos.length > 0 && (
          <div className="rounded-md border p-3">
            <p className="text-sm font-medium">
              {loCancelado.prestamos.length}{" "}
              {loCancelado.prestamos.length === 1
                ? "de esas clases ya tenía su computadora entregada"
                : "de esas clases ya tenían sus computadoras entregadas"}
            </p>
            <ul className="text-muted-foreground mt-1 text-sm">
              {loCancelado.prestamos.map((p) => (
                <li key={p.id}>
                  {p.equipo} · la tiene {p.quien}
                </li>
              ))}
            </ul>
            <p className="text-muted-foreground mt-1 text-sm">
              Las computadoras siguen prestadas y se reciben como siempre. Conviene
              avisarles a mano.
            </p>
          </div>
        )}

        <div>
          <Button onClick={() => navigate("/", { replace: true })}>
            Entrar al sistema
          </Button>
        </div>
      </div>
    )
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

      {porConfirmar !== null && (
        <ImpactoDeLaJornada
          impacto={porConfirmar.impacto}
          guardando={guardar.isPending}
          onConfirmar={() =>
            guardar.mutate({ jornada: porConfirmar.tramos, confirmado: true })
          }
          onCancelar={() => setPorConfirmar(null)}
        />
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
          onClick={() => proponer(tramos)}
        >
          Guardar la jornada
        </Button>
        {/* La salida sin declarar nada tiene que existir y estar dicha con
            todas las letras: quien está probando el sistema no sabe todavía
            qué horario tiene la escuela, y obligarlo a inventar uno es
            producir el error que esta pantalla vino a evitar. */}
        <Button variant="ghost" disabled={guardar.isPending} onClick={postergar}>
          Dejarla libre por ahora
        </Button>
      </div>

      <p className="text-muted-foreground text-sm">
        Si la dejás libre se va a poder reservar cualquier día y a cualquier hora. El
        sistema te lo va a volver a preguntar cada vez que entres, hasta que la declares:
        es la única decisión de la que dependen las reservas de toda la escuela, y
        descubrirla tarde obliga a cancelar clases ya cargadas. También se declara desde{" "}
        <span className="font-medium">Jornada de la escuela</span>.
      </p>

      <p className="text-muted-foreground text-sm">
        Se pueden cargar varios tramos para el mismo día: una escuela con turno mañana y
        turno noche declara, por ejemplo, 07:00–12:00 y 18:00–23:00, y el mediodía queda
        cerrado. Una nocturna declara 20:00–01:00: si la hora de cierre es menor que la de
        apertura, el tramo termina al día siguiente. Los días que no cargues son días en
        que la escuela no abre.
      </p>

      {/* Esta pantalla vive fuera del layout, así que sin este botón no habría
          ninguna forma de salir: alguien que entró con la cuenta equivocada
          quedaría encerrado. Dice "Salir" y no "Cerrar sesión" porque es la
          palabra que usa la barra en todo el resto del sistema, y dos nombres
          para la misma acción se leen como dos acciones distintas. */}
      <div className="border-border border-t pt-4">
        <Button variant="ghost" size="sm" onClick={logout}>
          Salir
        </Button>
      </div>
    </div>
  )
}
