import { useQuery } from "@tanstack/react-query"
import { Link } from "react-router"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useAuth } from "@/features/auth/AuthContext"
import * as adminApi from "@/features/admin/api"
import { useNoLeidas } from "@/features/notificaciones/useNoLeidas"
import * as reservasApi from "@/features/reservas/api"
import { agruparReservas, hoyISO } from "@/features/reservas/types"
import { formatearFechaLargaCapitalizada } from "@/lib/fechas"
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
  valor: number
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
      <p className="text-3xl font-semibold tabular-nums">{valor}</p>
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
          {grupo.reservas.length} {grupo.reservas.length === 1 ? "PC" : "PCs"}
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
  const hoy = hoyISO()
  const noLeidas = useNoLeidas()

  // Desde hoy en adelante: lo que ya pasó no ayuda a nadie a organizarse.
  // El backend ya limita al docente a sus propias reservas.
  const { data: reservas } = useQuery({
    queryKey: ["reservas", "proximas", hoy],
    queryFn: () => reservasApi.listarReservas({ desde: hoy }),
  })

  // Solo un Admin puede listar usuarios; para un docente ni se pregunta.
  const { data: pendientes } = useQuery({
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

  const deHoy = grupos.filter((g) => g.fecha === hoy).length
  const cuentasPendientes = pendientes?.meta.total ?? 0

  return (
    <div className="grid gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight text-balance sm:text-3xl">
          {saludo(new Date())}
          {user ? `, ${user.nombre}` : ""}
        </h1>
        <p className="text-muted-foreground mt-1 text-sm">
          {esAdmin
            ? "Panel de administración del laboratorio."
            : "Reservá las PCs para tus clases y mirá cómo venís esta semana."}
        </p>
      </div>

      {/* Dos columnas en un teléfono y no una sola: son números cortos, y
          apilados obligaban a bajar para ver el último. */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Indicador
          valor={deHoy}
          rotulo={deHoy === 1 ? "clase hoy" : "clases hoy"}
          detalle="Con PCs reservadas"
          a="/reservas"
        />
        <Indicador
          valor={grupos.length}
          rotulo="próximas"
          detalle="Reservas por venir"
          a="/reservas"
        />
        <Indicador
          valor={noLeidas}
          rotulo="sin leer"
          detalle="Avisos del sistema"
          a="/notificaciones"
          destacado={noLeidas > 0}
        />
        {esAdmin && (
          <Indicador
            valor={cuentasPendientes}
            rotulo="por aprobar"
            detalle="Cuentas de docentes"
            a="/admin/aprobacion"
            destacado={cuentasPendientes > 0}
          />
        )}
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
                día, el horario y las PCs.
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
              <Link to="/inventario">Ver carros y PCs</Link>
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
                  <Link to="/admin/inventario">Gestión del inventario</Link>
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
