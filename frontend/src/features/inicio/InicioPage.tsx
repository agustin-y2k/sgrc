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
import { useNoLeidas } from "@/features/notificaciones/useNoLeidas"
import * as reservasApi from "@/features/reservas/api"
import { agruparReservas, hoyISO } from "@/features/reservas/types"
import { formatearFechaLargaCapitalizada } from "@/lib/fechas"
import { getErrorMessage } from "@/lib/api-client"
import type { GrupoDeReservas } from "@/features/reservas/types"

/**
 * La primera pantalla después de iniciar sesión.
 *
 * Es donde cae todo el mundo todos los días, así que responde las dos
 * preguntas con las que alguien entra —"¿qué tengo hoy?" y "¿hay algo
 * esperándome?"— y ofrece el atajo a lo que venía a hacer, en vez de un
 * párrafo que explique dónde queda cada sección.
 *
 * Cada tarjeta lleva a una pantalla que ya existe. Nada de esto es una
 * función nueva del sistema: es un índice de lo que ya hay.
 */

function saludo(ahora: Date): string {
  const hora = ahora.getHours()
  if (hora < 13) return "Buen día"
  if (hora < 20) return "Buenas tardes"
  return "Buenas noches"
}

function etiquetaDeDia(fechaISO: string, hoy: string): string {
  return fechaISO === hoy ? "Hoy" : formatearFechaLargaCapitalizada(fechaISO)
}

/** Un número grande con su rótulo, que además es un enlace. */
function Indicador({
  valor,
  rotulo,
  detalle,
  a,
  destacado,
}: {
  /**
   * null cuando la consulta que lo alimenta falló. No es lo mismo que cero:
   * "no hay nada afuera" y "no pude preguntar qué hay afuera" llevan a
   * decisiones distintas en el mostrador, y mostrar 0 en el segundo caso es
   * afirmar algo que el sistema no sabe.
   */
  valor: number | null
  rotulo: string
  detalle: string
  a: string
  /** Pide atención: hay algo pendiente de hacer. */
  destacado?: boolean
}) {
  return (
    <Link
      to={a}
      className={[
        "focus-visible:ring-ring rounded-xl border p-4 transition-colors focus-visible:ring-2 focus-visible:outline-none",
        destacado
          ? "border-alerta/40 bg-alerta/10 hover:bg-alerta/20"
          : "bg-superficie hover:bg-muted",
      ].join(" ")}
    >
      <p className="text-3xl font-semibold tabular-nums">
        {valor ?? <span className="text-muted-foreground">—</span>}
      </p>
      <p className="mt-0.5 text-sm font-medium">{rotulo}</p>
      <p className="text-muted-foreground text-xs">{detalle}</p>
    </Link>
  )
}

function ProximaReserva({ grupo, hoy }: { grupo: GrupoDeReservas; hoy: string }) {
  return (
    <li className="border-border flex flex-col gap-0.5 border-b py-2.5 last:border-0 sm:flex-row sm:items-baseline sm:justify-between sm:gap-4">
      <div className="min-w-0">
        <p className="font-medium">
          {grupo.esBloqueoEvaluacion
            ? "Bloqueo por evaluación"
            : (grupo.materiaNombre ?? "Reserva")}
          {grupo.cursoNombre && (
            <span className="text-muted-foreground font-normal">
              {" "}
              · {grupo.cursoNombre}
            </span>
          )}
        </p>
        <p className="text-muted-foreground text-sm">
          {grupo.reservas.length} {grupo.reservas.length === 1 ? "equipo" : "equipos"}
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
  const hoy = hoyISO()
  const noLeidas = useNoLeidas()

  // Desde hoy en adelante: lo que ya pasó no ayuda a nadie a organizarse.
  // El backend ya limita al docente a sus propias reservas.
  const { data: reservas, error: errorReservas } = useQuery({
    queryKey: ["reservas", "proximas", hoy],
    queryFn: () => reservasApi.listarReservas({ desde: hoy }),
  })

  // Lo que está afuera del laboratorio: alimenta el indicador y el
  // formulario de entrega suelta, que necesita saber qué máquinas NO
  // ofrecer. Solo para Admin — un docente no tiene por qué verlo.
  const { data: prestamos, error: errorPrestamos } = useQuery({
    queryKey: PRESTAMOS_KEY,
    queryFn: reservasApi.listarPrestamosAbiertos,
    enabled: esAdmin,
    refetchInterval: REFRESCO_DEL_MOSTRADOR,
  })

  // Solo un Admin puede listar usuarios; para un docente ni se pregunta.
  const { data: pendientes, error: errorPendientes } = useQuery({
    queryKey: ["admin", "usuarios", "PENDIENTE"],
    queryFn: () => adminApi.listarUsuarios({ estado: "PENDIENTE" }),
    enabled: esAdmin,
  })

  const grupos = agruparReservas(
    (reservas?.data ?? []).filter((r) => r.estado === "CONFIRMADA")
  )
    .sort((a, b) =>
      a.fecha === b.fecha
        ? a.horaInicio.localeCompare(b.horaInicio)
        : a.fecha.localeCompare(b.fecha)
    )
    .slice(0, 5)

  // Cada número sale en null si su consulta falló, para que el indicador
  // muestre "—" en vez de un cero inventado. Sin esto, un fallo de red se
  // leía como "no hay ninguna computadora afuera del laboratorio".
  const deHoy = errorReservas ? null : grupos.filter((g) => g.fecha === hoy).length
  const cuentasPendientes = errorPendientes ? null : (pendientes?.meta.total ?? 0)

  const afuera = prestamos?.data ?? []
  const sinDevolverAHorario = errorPrestamos
    ? null
    : afuera.filter((p) => p.demorado).length
  const yaAfuera = useMemo(() => new Set(afuera.map((p) => p.equipoId)), [afuera])

  // Un solo aviso para las tres: al que está en el mostrador le importa que
  // lo que ve puede estar incompleto, no cuál de las consultas falló.
  const algoFallo = errorReservas ?? errorPrestamos ?? errorPendientes

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
            : "Reservá los equipos para tus clases y mirá cómo venís esta semana."}
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

      {/* El mostrador va PRIMERO, arriba incluso de los contadores: es lo
          único de esta pantalla que se opera con gente esperando del otro
          lado, y reemplaza al papel donde se anotaba qué salió y qué volvió.
          Cuántas cuentas esperan aprobación y cuántos avisos hay sin leer son
          cosas que se miran una vez al día; entregar y recibir, todo el día.
          Por eso los contadores quedan abajo y no arriba. */}
      {esAdmin && (
        <>
          <PanelDelLaboratorio />
          {/* "Afuera" y "acá" son la misma pregunta dada vuelta, así que van
              enfrentadas: a la izquierda lo que salió, arriba a la derecha con
              qué se cuenta si alguien golpea la puerta ahora. */}
          <div className="grid gap-4 lg:grid-cols-2">
            <LoQueEstaAfuera compacto />
            <div className="grid content-start gap-4">
              <EnElLaboratorio />
              {entregandoSuelta ? (
                <EntregaSuelta
                  yaAfuera={yaAfuera}
                  onCerrar={() => setEntregandoSuelta(false)}
                />
              ) : (
                <Card>
                  <CardHeader>
                    <CardTitle>Entregar sin reserva</CardTitle>
                  </CardHeader>
                  <CardContent className="grid gap-2">
                    <p className="text-muted-foreground text-sm">
                      Para cuando piden una computadora en el momento — un trámite, algo
                      puntual. La persona no necesita tener cuenta en el sistema.
                    </p>
                    <div>
                      <Button onClick={() => setEntregandoSuelta(true)}>
                        Entregar sin reserva
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              )}
            </div>
          </div>
        </>
      )}

      {/* Dos columnas en un teléfono y no una sola: son números cortos, y
          apilados obligaban a bajar para ver el último.

          Para el Admin esto va DEBAJO del mostrador (ver arriba). Sigue acá
          y no se saca: una cuenta pendiente es un docente que no puede
          trabajar, y nadie va a entrar a buscarla si nada se la nombra. */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Indicador
          valor={deHoy}
          rotulo={deHoy === 1 ? "clase hoy" : "clases hoy"}
          detalle={errorReservas ? "No se pudo consultar" : "Con equipos reservados"}
          a="/reservas"
        />
        {/* Para el Admin, "afuera" reemplaza a "próximas": lo que viene ya
            se lo dice el panel del mostrador con detalle, y lo que no puede
            faltarle de un vistazo es cuántas máquinas están fuera del
            laboratorio. */}
        {esAdmin ? (
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
        ) : (
          <Indicador
            valor={errorReservas ? null : grupos.length}
            rotulo="próximas"
            detalle={errorReservas ? "No se pudo consultar" : "Reservas por venir"}
            a="/reservas"
          />
        )}
        {esAdmin && (
          <Indicador
            valor={cuentasPendientes}
            rotulo="por aprobar"
            detalle={errorPendientes ? "No se pudo consultar" : "Cuentas de docentes"}
            a="/admin/aprobacion"
            destacado={!!cuentasPendientes && cuentasPendientes > 0}
          />
        )}
        <Indicador
          valor={noLeidas}
          rotulo="sin leer"
          detalle="Avisos del sistema"
          a="/notificaciones"
          destacado={noLeidas > 0}
        />
      </div>


      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
            <CardTitle>Lo que viene</CardTitle>
            <Button asChild size="sm">
              <Link to="/reservas/nueva">Nueva reserva</Link>
            </Button>
          </CardHeader>
          <CardContent>
            {grupos.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                No tenés reservas próximas. Cuando crees una, va a aparecer acá con el
                día, el horario y los equipos.
              </p>
            ) : (
              <ul>
                {grupos.map((g) => (
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

        <Card>
          <CardHeader>
            <CardTitle>Accesos rápidos</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-2">
            <Button asChild variant="outline" className="justify-start">
              <Link to="/inventario">Ver carros y equipos</Link>
            </Button>
            <Button asChild variant="outline" className="justify-start">
              <Link to="/disponibilidad">Disponibilidad de Admins</Link>
            </Button>
            {esAdmin ? (
              <>
                <Button asChild variant="outline" className="justify-start">
                  <Link to="/admin/academico">Ciclos, cursos y materias</Link>
                </Button>
                <Button asChild variant="outline" className="justify-start">
                  <Link to="/admin/entregas">Entregas y devoluciones</Link>
                </Button>
                <Button asChild variant="outline" className="justify-start">
                  <Link to="/admin/inventario">Gestión del inventario</Link>
                </Button>
                <Button asChild variant="outline" className="justify-start">
                  <Link to="/admin/licencias">Licencias de software</Link>
                </Button>
                <Button asChild variant="outline" className="justify-start">
                  <Link to="/admin/bloqueo-evaluacion">Bloqueo por evaluación</Link>
                </Button>
              </>
            ) : (
              <Button asChild variant="outline" className="justify-start">
                <Link to="/reservas">Mis reservas</Link>
              </Button>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
