import { Navigate, Outlet, useLocation } from "react-router"
import { useQuery } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"
import { useAuth } from "@/features/auth/AuthContext"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { JORNADA_KEY } from "@/features/disponibilidad/api"

export const RUTA_PRIMERA_JORNADA = "/primera-jornada"

export function ProtectedRoute() {
  const { user, isLoading, errorDeSesion, refetchUser } = useAuth()
  const location = useLocation()

  // Solo se pregunta por la jornada cuando hay un Admin que pueda
  // contestarla, y recién después de la contraseña temporal: son dos portones
  // en fila y el orden importa, porque la contraseña es de la persona y la
  // jornada es de la escuela.
  //
  // Un DOCENTE nunca queda atrapado acá: no tiene permiso para declararla, así
  // que bloquearlo dejaría a la escuela entera afuera esperando que entre un
  // Admin.
  const esAdminEnCondiciones = user?.rol === "ADMIN" && !user.debeCambiarPassword
  const { data: jornada } = useQuery({
    queryKey: JORNADA_KEY,
    queryFn: disponibilidadApi.jornadaDeLaInstitucion,
    enabled: esAdminEnCondiciones,
  })

  if (isLoading) {
    return (
      <div className="flex min-h-svh items-center justify-center">
        <p className="text-muted-foreground">Cargando…</p>
      </div>
    )
  }

  // La sesión no se pudo verificar por una falla de red, no porque el token
  // sea inválido: se ofrece reintentar en vez de mandar al login y hacerle
  // escribir la contraseña de nuevo por un corte momentáneo.
  if (!user && errorDeSesion) {
    return (
      <div className="flex min-h-svh flex-col items-center justify-center gap-4 p-4">
        <p className="text-muted-foreground text-center">{errorDeSesion}</p>
        <Button onClick={() => void refetchUser().catch(() => {})}>Reintentar</Button>
      </div>
    )
  }

  if (!user) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  // RF-01.6: bloquea toda navegación hasta que cambie la contraseña
  // temporal, salvo la propia pantalla de cambio.
  if (user.debeCambiarPassword && location.pathname !== "/cambiar-password") {
    return <Navigate to="/cambiar-password" replace />
  }

  // El Admin declara la jornada de la institución antes de hacer nada más.
  //
  // Se exige `definida === false` y no "la lista está vacía": son dos cosas
  // distintas y confundirlas volvería a preguntarle para siempre a la escuela
  // que ya eligió no restringir nada.
  //
  // Mientras la consulta no respondió, `jornada` es undefined y no se
  // bloquea. Es a propósito: si falla, lo peor que pasa es que no se pregunte
  // esta vez, mientras que bloquear ante la duda dejaría al Admin sin sistema
  // por un error de red.
  if (
    esAdminEnCondiciones &&
    jornada?.definida === false &&
    location.pathname !== RUTA_PRIMERA_JORNADA
  ) {
    return <Navigate to={RUTA_PRIMERA_JORNADA} replace />
  }

  return <Outlet />
}

export function PublicOnlyRoute() {
  const { user, isLoading } = useAuth()

  if (!isLoading && user) {
    return <Navigate to="/" replace />
  }

  return <Outlet />
}
