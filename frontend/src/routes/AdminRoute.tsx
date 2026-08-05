import { Navigate, Outlet } from "react-router"

import { useAuth } from "@/features/auth/AuthContext"

// Se usa anidado dentro de <ProtectedRoute> — asume que ya hay un user.
export function AdminRoute() {
  const { user } = useAuth()

  if (user?.rol !== "ADMIN") {
    return <Navigate to="/" replace />
  }

  return <Outlet />
}
