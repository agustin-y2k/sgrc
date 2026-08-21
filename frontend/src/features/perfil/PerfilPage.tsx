import { useQuery } from "@tanstack/react-query"
import { KeyRound } from "lucide-react"
import { Link } from "react-router"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { EstadoBadge } from "@/components/EstadoBadge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useAuth } from "@/features/auth/AuthContext"
import { FotoDePerfil } from "@/features/perfil/FotoDePerfil"
import { MisDatos } from "@/features/perfil/MisDatos"
import { PedirMateria } from "@/features/perfil/PedirMateria"
import * as perfilApi from "@/features/perfil/api"
import { ETIQUETA_ESTADO_PEDIDO, materiaDelPedido } from "@/features/perfil/types"
import * as reservasApi from "@/features/reservas/api"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaLarga } from "@/lib/fechas"

/** El perfil: la pantalla que se abre desde el redondel con las iniciales. */
export function PerfilPage() {
  const { user } = useAuth()

  const { data: materias, error: errorMaterias } = useQuery({
    queryKey: ["mis-materias"],
    queryFn: reservasApi.misMaterias,
  })

  const { data: pedidos } = useQuery({
    queryKey: ["perfil", "mis-pedidos"],
    queryFn: perfilApi.misPedidos,
  })

  if (!user) return null

  const misMaterias = materias?.data ?? []
  const misPedidos = pedidos?.data ?? []

  return (
    <div className="mx-auto grid max-w-3xl gap-4">
      <EncabezadoDePagina
        titulo="Mi perfil"
        descripcion="Tu nombre, tu foto, las materias que das y la contraseña con la que entrás."
      />

      <Card>
        <CardContent className="grid gap-4 pt-6">
          <FotoDePerfil
            usuarioId={user.id}
            nombre={user.nombre}
            apellido={user.apellido}
          />
          <MisDatos usuario={user} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Las materias que das</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3">
          {errorMaterias && (
            <Alert variant="destructive">
              <AlertDescription>{getErrorMessage(errorMaterias)}</AlertDescription>
            </Alert>
          )}

          {misMaterias.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              Todavía no tenés ninguna materia asignada, así que no vas a poder reservar
              computadoras. Pedila acá abajo y el equipo de administración lo resuelve.
            </p>
          ) : (
            <ul className="grid gap-2">
              {misMaterias.map((m) => (
                <li
                  key={`${m.materiaNombre}-${m.cursoNombre}`}
                  className="flex flex-wrap items-center justify-between gap-2 rounded-lg border px-3 py-2"
                >
                  <span className="font-medium">{m.materiaNombre}</span>
                  <span className="text-muted-foreground text-sm">{m.cursoNombre}</span>
                </li>
              ))}
            </ul>
          )}

          <PedirMateria />

          {misPedidos.length > 0 && (
            <div className="grid gap-2">
              <p className="text-sm font-medium">Lo que pediste</p>
              {misPedidos.map((p) => (
                <div key={p.id} className="grid gap-1 rounded-lg border px-3 py-2">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="font-medium">
                      {materiaDelPedido(p, "Una materia de la lista")}
                    </span>
                    <EstadoBadge
                      tono={
                        p.estado === "APROBADO"
                          ? "exito"
                          : p.estado === "RECHAZADO"
                            ? "peligro"
                            : "alerta"
                      }
                    >
                      {ETIQUETA_ESTADO_PEDIDO[p.estado]}
                    </EstadoBadge>
                  </div>
                  <p className="text-muted-foreground text-sm">
                    Lo pediste el {formatearFechaLarga(p.creadoEn.slice(0, 10))}
                  </p>
                  {/* La respuesta se muestra siempre que exista, no solo en
                      los rechazos: en una aprobación suele explicar hasta
                      cuándo, o con quién se habló. */}
                  {p.respuesta && <p className="text-sm">Te contestaron: {p.respuesta}</p>}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Tu contraseña</CardTitle>
        </CardHeader>
        <CardContent>
          <Link
            to="/cambiar-password"
            className="bg-superficie hover:border-primary/40 hover:bg-muted focus-visible:ring-ring flex w-full items-start gap-3 rounded-xl border p-4 text-left transition-colors focus-visible:ring-2 focus-visible:outline-none"
          >
            <span
              aria-hidden="true"
              className="bg-accent text-accent-foreground grid size-10 shrink-0 place-items-center rounded-lg"
            >
              <KeyRound className="size-5" />
            </span>
            <span>
              <span className="block font-medium">Cambiar mi contraseña</span>
              <span className="text-muted-foreground block text-sm">
                Para cambiar la que usás para entrar.
              </span>
            </span>
          </Link>
        </CardContent>
      </Card>
    </div>
  )
}
