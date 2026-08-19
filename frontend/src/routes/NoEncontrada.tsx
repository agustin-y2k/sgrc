import {
  CalendarPlus,
  CalendarDays,
  Laptop,
  PackageOpen,
  UserRound,
  Users,
} from "lucide-react"
import { useLocation, useNavigate } from "react-router"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/features/auth/AuthContext"
import { AccesoDirecto } from "@/features/inicio/AccesoDirecto"

/** La pantalla de una dirección que no existe. */
export function NoEncontrada() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { user } = useAuth()

  const esAdmin = user?.rol === "ADMIN"

  return (
    <div className="mx-auto max-w-3xl">
      <EncabezadoDePagina
        titulo="Esa página no existe"
        descripcion="Puede que el enlace haya cambiado, que la dirección tenga algún error de tipeo, o que lo que buscabas se haya dado de baja."
      />

      {/* La dirección que se intentó, textual. Sirve para dos cosas: quien
          se equivocó tipeando ve dónde, y quien llegó por un enlace roto
          tiene algo concreto que pasarle a un Admin. Va en monoespaciada y
          se corta sola porque una dirección larga no puede desbordar la
          pantalla a lo ancho. */}
      <p className="text-muted-foreground mb-6 text-sm">
        Intentaste entrar a{" "}
        <code className="bg-muted text-foreground rounded px-1.5 py-0.5 break-all">
          {pathname}
        </code>
      </p>

      {/* `h-11 sm:h-9`: 44px de blanco táctil en el teléfono, que es lo que
          pide WCAG 2.5.5 y lo que verifica `e2e/tactil.spec.ts`. Con el alto
          por defecto quedaban en 36px, y en esta pantalla en particular son
          la única salida que no exige abrir el menú. */}
      <div className="mb-6 flex flex-wrap gap-2">
        <Button className="h-11 px-4 sm:h-9" onClick={() => void navigate("/")}>
          Ir al inicio
        </Button>
        {/* Volver atrás es lo que la mayoría quiere y el navegador ya sabe
            hacer, pero desde el teclado o en un teléfono no siempre está a
            mano. Si se llegó acá pegando la dirección en la barra no hay a
            dónde volver, y por eso no es la acción principal. */}
        <Button
          variant="outline"
          className="h-11 px-4 sm:h-9"
          onClick={() => void navigate(-1)}
        >
          Volver a la pantalla anterior
        </Button>
      </div>

      <h2 className="mb-3 text-sm font-semibold tracking-wide uppercase">
        ¿A dónde querías ir?
      </h2>

      {/* Los atajos son los mismos que usa la pantalla de inicio, y cambian
          según el rol: ofrecerle "Gestión del inventario" a un docente sería
          mandarlo a una pantalla que ni siquiera puede abrir. */}
      <div className="grid gap-3 sm:grid-cols-2">
        {esAdmin ? (
          <>
            <AccesoDirecto
              icono={PackageOpen}
              titulo="Entregar y recibir equipos"
              ayuda="Qué hay que entregar ahora y qué falta que vuelva."
              a="/admin/entregas"
            />
            <AccesoDirecto
              icono={Users}
              titulo="Aprobar cuentas"
              ayuda="Docentes que se registraron y esperan aprobación."
              a="/admin/aprobacion"
            />
            <AccesoDirecto
              icono={Laptop}
              titulo="Gestionar el inventario"
              ayuda="Carros, equipos, incidencias y licencias."
              a="/admin/inventario"
            />
            <AccesoDirecto
              icono={CalendarDays}
              titulo="Ver todas las reservas"
              ayuda="Las clases con equipos reservados y los bloqueos."
              a="/reservas"
            />
          </>
        ) : (
          <>
            <AccesoDirecto
              icono={CalendarPlus}
              titulo="Reservar computadoras"
              ayuda="Para una clase o para todas las semanas."
              a="/reservas/nueva"
            />
            <AccesoDirecto
              icono={CalendarDays}
              titulo="Ver todas mis reservas"
              ayuda="Las que vienen y las que ya pasaron, con su estado."
              a="/reservas"
            />
            <AccesoDirecto
              icono={Laptop}
              titulo="Ver las computadoras"
              ayuda="Cuáles hay en cada carro y qué programas tienen instalados."
              a="/inventario"
            />
            <AccesoDirecto
              icono={UserRound}
              titulo="Quién te puede ayudar"
              ayuda="En qué días y horarios está cada Admin del laboratorio."
              a="/disponibilidad"
            />
          </>
        )}
      </div>
    </div>
  )
}
