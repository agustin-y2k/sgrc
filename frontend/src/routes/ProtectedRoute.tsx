import { Navigate, Outlet, useLocation } from "react-router"

import { Button } from "@/components/ui/button"
import { useAuth } from "@/features/auth/AuthContext"

export function ProtectedRoute() {
  const { user, isLoading, errorDeSesion, refetchUser } = useAuth()
  const location = useLocation()

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

  return <Outlet />
}

export function PublicOnlyRoute() {
  const { user, isLoading } = useAuth()

  if (!isLoading && user) {
    return <Navigate to="/" replace />
  }

  return <Outlet />
}
