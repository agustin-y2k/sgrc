import { lazy } from "react"
import { createBrowserRouter, Navigate, RouterProvider } from "react-router"

import { AppLayout } from "@/components/layout/AppLayout"
import { LoginPage } from "@/features/auth/LoginPage"
import { RecuperarPasswordPage } from "@/features/auth/RecuperarPasswordPage"
import { RegistroPage } from "@/features/auth/RegistroPage"
import { PrimeraJornadaPage } from "@/features/admin/PrimeraJornadaPage"
import { AdminRoute } from "@/routes/AdminRoute"
import { NoEncontrada } from "@/routes/NoEncontrada"
import { ErrorDePantalla } from "@/routes/ErrorDePantalla"
import {
  PublicOnlyRoute,
  ProtectedRoute,
  RUTA_PRIMERA_JORNADA,
} from "@/routes/ProtectedRoute"

// Cada pantalla se descarga al abrirla; el layout conserva navegación y sesión.
const AcademicoPage = lazy(() =>
  import("@/features/academico/AcademicoPage").then((modulo) => ({
    default: modulo.AcademicoPage,
  }))
)
const AuditoriaPage = lazy(() =>
  import("@/features/auditoria/AuditoriaPage").then((modulo) => ({
    default: modulo.AuditoriaPage,
  }))
)
const AprobacionPage = lazy(() =>
  import("@/features/auth/AprobacionPage").then((modulo) => ({
    default: modulo.AprobacionPage,
  }))
)
const CambiarPasswordPage = lazy(() =>
  import("@/features/auth/CambiarPasswordPage").then((modulo) => ({
    default: modulo.CambiarPasswordPage,
  }))
)
const EntregasPage = lazy(() =>
  import("@/features/admin/EntregasPage").then((modulo) => ({
    default: modulo.EntregasPage,
  }))
)
const InventarioAdminPage = lazy(() =>
  import("@/features/admin/InventarioAdminPage").then((modulo) => ({
    default: modulo.InventarioAdminPage,
  }))
)
const JornadaPage = lazy(() =>
  import("@/features/admin/JornadaPage").then((modulo) => ({
    default: modulo.JornadaPage,
  }))
)
const LicenciasPage = lazy(() =>
  import("@/features/admin/LicenciasPage").then((modulo) => ({
    default: modulo.LicenciasPage,
  }))
)
const ConfiguracionPage = lazy(() =>
  import("@/features/admin/ConfiguracionPage").then((m) => ({
    default: m.ConfiguracionPage,
  }))
)
const ReportesPage = lazy(() =>
  import("@/features/admin/ReportesPage").then((modulo) => ({
    default: modulo.ReportesPage,
  }))
)
const UsuariosPage = lazy(() =>
  import("@/features/admin/UsuariosPage").then((modulo) => ({
    default: modulo.UsuariosPage,
  }))
)
const CalendarioEquipoPage = lazy(() =>
  import("@/features/calendario/CalendarioEquipoPage").then((modulo) => ({
    default: modulo.CalendarioEquipoPage,
  }))
)
const DisponibilidadPage = lazy(() =>
  import("@/features/disponibilidad/DisponibilidadPage").then((modulo) => ({
    default: modulo.DisponibilidadPage,
  }))
)
const InicioPage = lazy(() =>
  import("@/features/inicio/InicioPage").then((modulo) => ({
    default: modulo.InicioPage,
  }))
)
const InventarioPage = lazy(() =>
  import("@/features/inventory/InventarioPage").then((modulo) => ({
    default: modulo.InventarioPage,
  }))
)
const NotificacionesPage = lazy(() =>
  import("@/features/notificaciones/NotificacionesPage").then((modulo) => ({
    default: modulo.NotificacionesPage,
  }))
)
const PerfilPage = lazy(() =>
  import("@/features/perfil/PerfilPage").then((modulo) => ({
    default: modulo.PerfilPage,
  }))
)
const PedidosDeMateriaPage = lazy(() =>
  import("@/features/admin/PedidosDeMateriaPage").then((modulo) => ({
    default: modulo.PedidosDeMateriaPage,
  }))
)
const BloquearEquiposPage = lazy(() =>
  import("@/features/reservas/BloquearEquiposPage").then((modulo) => ({
    default: modulo.BloquearEquiposPage,
  }))
)
const MisReservasPage = lazy(() =>
  import("@/features/reservas/MisReservasPage").then((modulo) => ({
    default: modulo.MisReservasPage,
  }))
)
const CalendarioReservasPage = lazy(() =>
  import("@/features/calendario/CalendarioReservas").then((modulo) => ({
    default: modulo.CalendarioReservasPage,
  }))
)
const NuevaReservaPage = lazy(() =>
  import("@/features/reservas/NuevaReservaPage").then((modulo) => ({
    default: modulo.NuevaReservaPage,
  }))
)

const router = createBrowserRouter([
  {
    element: <PublicOnlyRoute />,
    errorElement: <ErrorDePantalla />,
    children: [
      { path: "/login", element: <LoginPage /> },
      { path: "/registro", element: <RegistroPage /> },
      // Pública y dentro de PublicOnlyRoute: quien olvidó la contraseña no
      // tiene sesión, y quien sí la tiene no necesita esta pantalla (para
      // cambiarla teniendo sesión está /cambiar-password).
      { path: "/recuperar-password", element: <RecuperarPasswordPage /> },
    ],
  },
  {
    element: <ProtectedRoute />,
    errorElement: <ErrorDePantalla />,
    children: [
      // Fuera del AppLayout a propósito: es un portón, no una página. Con la
      // barra de navegación alrededor ofrecería salidas que ProtectedRoute
      // deshace enseguida, y un menú que no lleva a ningún lado se lee como
      // que el sistema está roto.
      { path: RUTA_PRIMERA_JORNADA, element: <PrimeraJornadaPage /> },
      {
        element: <AppLayout />,
        children: [
          {
            errorElement: <ErrorDePantalla />,
            children: [
              { index: true, element: <InicioPage /> },
              { path: "/cambiar-password", element: <CambiarPasswordPage /> },
              { path: "/notificaciones", element: <NotificacionesPage /> },
              // El perfil es de cualquiera que use el sistema, Admin incluido:
              // la foto, las materias y la contraseña son de la persona, no del
              // rol.
              { path: "/perfil", element: <PerfilPage /> },
              // Escribir y seguir las conversaciones pasó a vivir dentro de
              // Notificaciones: un solo lugar donde mirar si alguien espera algo.
              // La ruta vieja queda como redirección porque está en correos ya
              // enviados y en el historial de los navegadores.
              {
                path: "/mis-mensajes",
                element: <Navigate to="/notificaciones" replace />,
              },
              { path: "/reservas", element: <MisReservasPage /> },
              { path: "/reservas/calendario", element: <CalendarioReservasPage /> },
              { path: "/reservas/nueva", element: <NuevaReservaPage /> },
              { path: "/inventario", element: <InventarioPage /> },
              // RF-07.2: cualquier usuario autenticado, no solo Admins. Editar
              // el horario propio está dentro de la página, condicionado al rol.
              { path: "/disponibilidad", element: <DisponibilidadPage /> },
              {
                path: "/inventario/equipos/:equipoId/calendario",
                element: <CalendarioEquipoPage />,
              },
              {
                element: <AdminRoute />,
                children: [
                  { path: "/admin/aprobacion", element: <AprobacionPage /> },
                  { path: "/admin/academico", element: <AcademicoPage /> },
                  { path: "/admin/usuarios", element: <UsuariosPage /> },
                  { path: "/admin/inventario", element: <InventarioAdminPage /> },
                  { path: "/admin/licencias", element: <LicenciasPage /> },
                  { path: "/admin/entregas", element: <EntregasPage /> },
                  { path: "/admin/reportes", element: <ReportesPage /> },
                  { path: "/admin/configuracion", element: <ConfiguracionPage /> },
                  { path: "/admin/jornada", element: <JornadaPage /> },
                  { path: "/admin/auditoria", element: <AuditoriaPage /> },
                  {
                    path: "/admin/pedidos-de-materia",
                    element: <PedidosDeMateriaPage />,
                  },
                  {
                    path: "/admin/bloquear-equipos",
                    element: <BloquearEquiposPage />,
                  },
                ],
              },
              // El comodín va al final y DENTRO del layout: así una dirección que
              // no existe conserva la barra de navegación, que es la salida más
              // rápida.
              { path: "*", element: <NoEncontrada /> },
            ],
          },
        ],
      },
    ],
  },
])

function App() {
  return <RouterProvider router={router} />
}

export default App
