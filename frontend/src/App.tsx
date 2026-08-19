import { createBrowserRouter, RouterProvider } from "react-router"

import { AppLayout } from "@/components/layout/AppLayout"
import { AcademicoPage } from "@/features/academico/AcademicoPage"
import { AprobacionPage } from "@/features/auth/AprobacionPage"
import { CambiarPasswordPage } from "@/features/auth/CambiarPasswordPage"
import { LoginPage } from "@/features/auth/LoginPage"
import { RecuperarPasswordPage } from "@/features/auth/RecuperarPasswordPage"
import { RegistroPage } from "@/features/auth/RegistroPage"
import { EntregasPage } from "@/features/admin/EntregasPage"
import { InventarioAdminPage } from "@/features/admin/InventarioAdminPage"
import { JornadaPage } from "@/features/admin/JornadaPage"
import { LicenciasPage } from "@/features/admin/LicenciasPage"
import { ReportesPage } from "@/features/admin/ReportesPage"
import { UsuariosPage } from "@/features/admin/UsuariosPage"
import { CalendarioEquipoPage } from "@/features/calendario/CalendarioEquipoPage"
import { DisponibilidadPage } from "@/features/disponibilidad/DisponibilidadPage"
import { InicioPage } from "@/features/inicio/InicioPage"
import { InventarioPage } from "@/features/inventory/InventarioPage"
import { NotificacionesPage } from "@/features/notificaciones/NotificacionesPage"
import { PerfilPage } from "@/features/perfil/PerfilPage"
import { MisMensajesPage } from "@/features/sugerencias/MisMensajesPage"
import { PedidosDeMateriaPage } from "@/features/admin/PedidosDeMateriaPage"
import { SugerenciasPage } from "@/features/admin/SugerenciasPage"
import { BloquearEquiposPage } from "@/features/reservas/BloquearEquiposPage"
import { MisReservasPage } from "@/features/reservas/MisReservasPage"
import { NuevaReservaPage } from "@/features/reservas/NuevaReservaPage"
import { AdminRoute } from "@/routes/AdminRoute"
import { NoEncontrada } from "@/routes/NoEncontrada"
import { PublicOnlyRoute, ProtectedRoute } from "@/routes/ProtectedRoute"

const router = createBrowserRouter([
  {
    element: <PublicOnlyRoute />,
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
    children: [
      {
        element: <AppLayout />,
        children: [
          { index: true, element: <InicioPage /> },
          { path: "/cambiar-password", element: <CambiarPasswordPage /> },
          { path: "/notificaciones", element: <NotificacionesPage /> },
          // El perfil y el buzón son de cualquiera que use el sistema, Admin
          // incluido: un Admin nuevo también se topa con cosas que no
          // entiende.
          { path: "/perfil", element: <PerfilPage /> },
          { path: "/mis-mensajes", element: <MisMensajesPage /> },
          { path: "/reservas", element: <MisReservasPage /> },
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
              { path: "/admin/jornada", element: <JornadaPage /> },
              { path: "/admin/sugerencias", element: <SugerenciasPage /> },
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
])

function App() {
  return <RouterProvider router={router} />
}

export default App
