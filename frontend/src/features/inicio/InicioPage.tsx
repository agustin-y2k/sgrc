import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "react-router"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useAuth } from "@/features/auth/AuthContext"
import * as adminApi from "@/features/admin/api"
import { EntregaSuelta } from "@/features/admin/entregas/EntregaSuelta"
import { LoQueEstaAfuera } from "@/features/admin/entregas/LoQueEstaAfuera"
import { EnElLaboratorio } from "@/features/admin/entregas/EnElLaboratorio"
import { PanelDelLaboratorio } from "@/features/admin/entregas/PanelDelLaboratorio"
import {
  PRESTAMOS_KEY,
  REFRESCO_DEL_MOSTRADOR,
} from "@/features/admin/entregas/compartido"
import { Indicador } from "@/features/inicio/Indicador"
import { InicioDocente } from "@/features/inicio/InicioDocente"
import { useNoLeidas } from "@/features/notificaciones/useNoLeidas"
import * as reservasApi from "@/features/reservas/api"
import { agruparReservas, hoyISO } from "@/features/reservas/types"
import { etiquetaDeDia } from "@/lib/fechas"
import { getErrorMessage } from "@/lib/api-client"
import type { GrupoDeReservas } from "@/features/reservas/types"
import { contar } from "@/lib/plural"

/** La primera pantalla después de iniciar sesión. */

/** Cuántas clases resume el panel del Admin antes de mandar al listado. */
const MAX_PROXIMAS = 5

/** HH:MM a minutos, para saber si una clase de hoy ya terminó. */
function enMinutos(hhmm: string): number {
  const [h, m] = hhmm.split(":").map(Number)
  return Number.isFinite(h) && Number.isFinite(m) ? h * 60 + m : 0
}

function saludo(ahora: Date): string {
  const hora = ahora.getHours()
  if (hora < 13) return "Buen día"
  if (hora < 20) return "Buenas tardes"
  return "Buenas noches"
}

function ProximaReserva({ grupo, hoy }: { grupo: GrupoDeReservas; hoy: string }) {
  return (
    <li className="border-border flex flex-col gap-0.5 border-b py-2.5 last:border-0 sm:flex-row sm:items-baseline sm:justify-between sm:gap-4">
      <div className="min-w-0">
        <p className="font-medium">
          {grupo.esBloqueo
            ? (grupo.motivoBloqueo ?? "Bloqueado")
            : (grupo.materiaNombre ?? "Reserva")}
          {grupo.cursoNombre && (
            <span className="text-muted-foreground font-normal">
              {" "}
              · {grupo.cursoNombre}
            </span>
          )}
        </p>
        <p className="text-muted-foreground text-sm">
          {contar(grupo.reservas.length, "computadora")}
          {grupo.esRecurrente && " · se repite"}
        </p>
      </div>
      <p className="text-sm">
        <span className="font-medium">{etiquetaDeDia(grupo.fecha, hoy)}</span>{" "}
        <span className="tabular-nums">
          {grupo.horaInicio}–{grupo.horaFin}
        </span>
      </p>
    </li>
  )
}

export function InicioPage() {
  const { user } = useAuth()
  const esAdmin = user?.rol === "ADMIN"
  const [entregandoSuelta, setEntregandoSuelta] = useState(false)
  // El resumen de la última entrega vive acá y no en el formulario, que se
  // cierra solo al terminar (RF-08.24): adentro se iría con él.
  const [resumenDeEntrega, setResumenDeEntrega] = useState<string | null>(null)
  const hoy = hoyISO()
  const noLeidas = useNoLeidas()

  // Desde hoy en adelante: lo que ya pasó no ayuda a nadie a organizarse.
  // El backend ya limita al docente a sus propias reservas.
  //
  // `pageSize` al máximo por la misma razón que el contador: paginando de a
  // 50, un día a full son 95 reservas y la primera página no llega ni al
  // mediodía. «Lo que viene» mostraba dos clases de las siete que quedaban,
  // porque las cinco que faltaban estaban en la página dos.
  const { data: reservas, error: errorReservas } = useQuery({
    queryKey: ["reservas", "proximas", hoy],
    queryFn: () => reservasApi.listarReservas({ desde: hoy, pageSize: 200 }),
  })

  // Lo que está afuera del laboratorio: alimenta el indicador y el formulario
  // de entrega suelta, que necesita saber qué máquinas NO ofrecer.
  const { data: prestamos, error: errorPrestamos } = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    enabled: esAdmin,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  // El día de hoy, completo. Misma clave que PanelDelLaboratorio: el Admin ya
  // la tiene en caché, así que esto no pide nada de más.
  //
  // Hace falta aparte de "proximas" por el `pageSize`: aquella pagina de a 50
  // y un día a full son 95 reservas, así que el contador veía la mitad del día
  // y decía "6 clases hoy" cuando había once.
  const { data: delDia, error: errorDelDia } = useQuery({
    queryKey: ["reservas", "del-dia", hoy],
    queryFn: () => reservasApi.listarReservas({ desde: hoy, hasta: hoy, pageSize: 200 }),
    enabled: esAdmin,
  })

  // Solo un Admin puede listar usuarios; para un docente ni se pregunta.
  const { data: pendientes, error: errorPendientes } = useQuery({
    queryKey: ["admin", "usuarios", "PENDIENTE"],
    queryFn: () => adminApi.listarUsuarios({ estado: "PENDIENTE" }),
    enabled: esAdmin,
  })

  // Sin cortar: cada vista decide cuántas muestra, pero los contadores se
  // calculan sobre todas.
  //
  // El filtro es por fecha Y HORA, no solo por fecha. El backend responde
  // "desde hoy", que es lo correcto para él —no sabe qué hora es en esta
  // pantalla—, pero una clase que terminó a las nueve no es "lo que viene" a
  // las once. Se veía en la portada del Admin: los tres primeros renglones de
  // «Lo que viene» eran del turno mañana, ya pasado.
  const ahoraEnMinutos = new Date().getHours() * 60 + new Date().getMinutes()
  const grupos = agruparReservas(
    (reservas?.data ?? []).filter((r) => r.estado === "CONFIRMADA")
  )
    .filter((g) => g.fecha > hoy || enMinutos(g.horaFin) > ahoraEnMinutos)
    .sort((a, b) =>
      a.fecha === b.fecha
        ? a.horaInicio.localeCompare(b.horaInicio)
        : a.fecha.localeCompare(b.fecha)
    )

  // Cada número sale en null si su consulta falló, para que el indicador
  // muestre "—" en vez de un cero inventado.
  const deHoy = errorDelDia
    ? null
    : agruparReservas(
        (delDia?.data ?? []).filter((r) => r.estado === "CONFIRMADA" && r.tipo !== "BLOQUEO")
      ).length
  const cuentasPendientes = errorPendientes ? null : (pendientes?.meta.total ?? 0)

  const afuera = prestamos?.data ?? []
  const sinDevolverAHorario = errorPrestamos
    ? null
    : afuera.filter((p) => p.demorado).length
  const yaAfuera = useMemo(() => new Set(afuera.map((p) => p.equipoId)), [afuera])

  // Un solo aviso para las tres: al que está en el mostrador le importa que
  // lo que ve puede estar incompleto, no cuál de las consultas falló.
  const algoFallo = errorReservas ?? errorPrestamos ?? errorPendientes ?? errorDelDia

  // Para el Admin, «Lo que viene» empieza MAÑANA: lo de hoy ya está arriba, en
  // la cola del mostrador, con el docente, los equipos y el botón para
  // entregarlos. Repetido abajo en un renglón más pobre no agregaba nada y en
  // un día a full llenaba la tarjeta entera con las mismas cinco clases.
  //
  // El docente no tiene cola: para él «Lo que viene» sigue incluyendo hoy, que
  // es donde ve su próxima clase.
  const proximosDias = grupos.filter((g) => g.fecha > hoy)

  return (
    <div className="grid gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight text-balance sm:text-3xl">
          {saludo(new Date())}
          {user ? `, ${user.nombre}` : ""}
        </h1>
        <p className="text-muted-foreground mt-1 text-sm">
          {esAdmin
            ? "Qué hay que entregar ahora, qué falta que vuelva y con qué se cuenta."
            : "Acá tenés todo a mano: lo que ya reservaste y todo lo que podés hacer."}
        </p>
      </div>

      {/* Lo que esta pantalla NO puede hacer es callarse un fallo: sin este
          aviso, una consulta caída se leía como "no hay nada" —ninguna clase
          hoy, ninguna computadora afuera— y eso manda al Admin a cerrar el
          laboratorio con ocho máquinas todavía prestadas. */}
      {algoFallo && (
        <Alert variant="destructive">
          <AlertDescription>
            No se pudo consultar todo: lo que ves acá puede estar incompleto. Probá
            recargar. ({getErrorMessage(algoFallo)})
          </AlertDescription>
        </Alert>
      )}

      {esAdmin ? (
        <>
          {/* El formulario de entrega suelta, cuando está abierto, ocupa el
              ancho entero y va arriba de todo: es lo que se está haciendo en
              ese momento. */}
          {entregandoSuelta && (
            <EntregaSuelta
              yaAfuera={yaAfuera}
              onCerrar={(resumen) => {
                setEntregandoSuelta(false)
                setResumenDeEntrega(resumen ?? null)
              }}
            />
          )}
          {/* Arriba de «Afuera del laboratorio», que es donde las máquinas que
              acaban de salir ya figuran. */}
          {resumenDeEntrega && (
            <Alert>
              <AlertDescription>{resumenDeEntrega}</AlertDescription>
            </Alert>
          )}

          {/* La tira de estado: cinco números de alto fijo, arriba de todo.
              Es lo único de esta pantalla que no crece con el día.

              Antes «En el laboratorio ahora» era una tarjeta con su propio
              título y los otros cuatro números vivían mil ochocientos píxeles
              más abajo, debajo de todo lo que sí crece. Son la misma clase de
              dato —lo que se mira de reojo al llegar— y ahora están juntos.

              Van ARRIBA del mostrador, al revés que antes. La razón de ponerlo
              abajo era que atender gente es más urgente que mirar contadores,
              y sigue siendo cierta; lo que cambió es que la tira ahora lleva
              «acá ahora», que es la pregunta del mostrador —con qué cuento si
              golpean la puerta— y no un contador administrativo. Son cien
              píxeles y la cola sigue entrando en la primera pantalla.

              Dos columnas en el teléfono y no una: son números cortos, y
              apilados obligaban a bajar para ver el último. */}
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
            <EnElLaboratorio />
            {/* "Afuera" y no "próximas": lo que viene ya se lo dice la cola
                del mostrador con detalle, y lo que no puede faltarle de un
                vistazo es cuántas máquinas están fuera del laboratorio. */}
            <Indicador
              valor={errorPrestamos ? null : afuera.length}
              rotulo="afuera"
              detalle={
                errorPrestamos
                  ? "No se pudo consultar"
                  : sinDevolverAHorario
                    ? `${sinDevolverAHorario} sin devolver a horario`
                    : "Equipos entregados"
              }
              a="/admin/entregas"
              destacado={!!sinDevolverAHorario}
            />
            <Indicador
              valor={deHoy}
              rotulo={deHoy === 1 ? "clase hoy" : "clases hoy"}
              detalle={errorDelDia ? "No se pudo consultar" : "Con equipos reservados"}
              a="/reservas"
            />
            {/* Una cuenta pendiente es un docente que no puede trabajar, y
                nadie va a entrar a buscarla si nada se la nombra. */}
            <Indicador
              valor={cuentasPendientes}
              rotulo="por aprobar"
              detalle={errorPendientes ? "No se pudo consultar" : "Cuentas de docentes"}
              a="/admin/aprobacion"
              destacado={!!cuentasPendientes && cuentasPendientes > 0}
            />
            <Indicador
              valor={noLeidas}
              rotulo="sin leer"
              detalle="Avisos del sistema"
              a="/notificaciones"
              destacado={noLeidas > 0}
            />
          </div>

          {/* La cola del mostrador, con «Entregar sin reserva» en su cabecera.
              Era una tarjeta de 150px con tres renglones de explicación para
              un botón; la explicación vive ahora en el formulario que abre,
              que es donde se lee sin ocupar la portada todos los días. */}
          <PanelDelLaboratorio
            accion={
              !entregandoSuelta && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setResumenDeEntrega(null)
                    setEntregandoSuelta(true)
                  }}
                >
                  Entregar sin reserva
                </Button>
              )
            }
          />

          {/* A ancho completo: es lo que más crece de la pantalla —diecinueve
              equipos un lunes cualquiera— y compartía la fila con una columna
              de 300px, así que dejaba mil doscientos píxeles en blanco al
              lado. */}
          <LoQueEstaAfuera compacto />

          <Card>
            <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
              <CardTitle>Lo que viene</CardTitle>
              <Button asChild size="sm">
                <Link to="/reservas/nueva">Nueva reserva</Link>
              </Button>
            </CardHeader>
            <CardContent>
              {proximosDias.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  No hay nada reservado para los próximos días. Lo de hoy está arriba, en
                  «Para entregar hoy».
                </p>
              ) : (
                <ul>
                  {/* Un resumen, no la agenda entera: para operar el
                      mostrador alcanza con lo que sigue, y el listado
                      completo está a un clic. */}
                  {proximosDias.slice(0, MAX_PROXIMAS).map((g) => (
                    <ProximaReserva
                      key={`${g.grupoId ?? g.fecha}-${g.horaInicio}-${g.reservas[0]?.id}`}
                      grupo={g}
                      hoy={hoy}
                    />
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </>
      ) : (
        <InicioDocente
          grupos={grupos}
          hoy={hoy}
          noLeidas={noLeidas}
          hayError={errorReservas !== null}
        />
      )}
    </div>
  )
}
