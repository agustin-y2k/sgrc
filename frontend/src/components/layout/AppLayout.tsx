import { useEffect, useRef, useState } from "react"
import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router"

import { ChevronDown, LifeBuoy } from "lucide-react"

import { BotonDeTema } from "@/components/BotonDeTema"
import { PieDeAutoria } from "@/components/PieDeAutoria"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/features/auth/AuthContext"
import { useAfuera } from "@/features/admin/entregas/useAfuera"
import { useNoLeidas } from "@/features/notificaciones/useNoLeidas"
import { contar } from "@/lib/plural"
import { Avatar } from "@/components/Avatar"

/** Los enlaces del menú, en el orden en que se usan. */
type Enlace = { a: string; texto: string }

/**
 * El enlace que lleva el contador de máquinas afuera.
 *
 * Va acá y no en Avisos porque no es un aviso: es un estado. El correo del
 * cierre de jornada sale una sola vez, así que si la única huella de una
 * computadora que no volvió fuera ese correo, alcanzaría con no leerlo para
 * que nadie se enterara nunca más. Este número no se va hasta que alguien la
 * recibe.
 */
const ENLACE_CON_AFUERA = "/admin/entregas"

const ENLACES: Enlace[] = [
  { a: "/reservas", texto: "Reservas" },
  // "Computadoras" y no "Inventario": desde la pantalla de inicio se llega
  // acá por un atajo que dice "Ver las computadoras", y el cartel del destino
  // tiene que coincidir con el botón que se apretó.
  { a: "/inventario", texto: "Computadoras" },
  // RF-07.2: lo ve cualquier usuario autenticado.
  { a: "/disponibilidad", texto: "Horario Admins" },
]

/**
 * Aprobación queda afuera del grupo, en la barra: es la única tarea de
 * administración que es diaria y reactiva —llegó un registro nuevo y alguien
 * está esperando para poder trabajar—.
 */
const ENLACE_APROBACION: Enlace = { a: "/admin/aprobacion", texto: "Aprobación" }

const ENLACES_ADMIN: Enlace[] = [
  // Usuarios es el panel completo, del que Aprobación es la vista enfocada.
  { a: "/admin/usuarios", texto: "Usuarios" },
  // Ciclos, cursos y materias: de esto depende todo lo demás, así que va
  // antes que inventario y reportes.
  { a: "/admin/academico", texto: "Académico" },
  // Entregas va primero del grupo de inventario: es lo único de acá que
  // se usa varias veces por día, en el mostrador y con gente esperando.
  { a: "/admin/entregas", texto: "Entregas" },
  { a: "/admin/inventario", texto: "Gestión del inventario" },
  // Al lado del inventario: es la misma máquina mirada por otro lado — qué
  // software con vencimiento tiene y cuándo hay que renovarlo.
  { a: "/admin/licencias", texto: "Licencias" },
  { a: "/admin/reportes", texto: "Reportes" },
  // Los pedidos para dictar una materia. Lo que la gente escribe —ayuda,
  // fallas, ideas— NO está acá: vive dentro de Notificaciones, junto con el
  // resto de lo que espera una respuesta.
  { a: "/admin/pedidos-de-materia", texto: "Pedidos de materia" },
  // La jornada de la escuela: qué días y horas abre.
  { a: "/admin/jornada", texto: "Jornada de la escuela" },
  // RF-04.7. Último: es lo que menos se usa y lo que más rompe si se entra
  // sin querer.
  { a: "/admin/bloquear-equipos", texto: "Bloquear equipos" },
]

/**
 * El texto de un enlace de administración, con el contador cuando
 * corresponde. Compartido entre la barra y el menú del teléfono: dos copias
 * es cómo terminan mostrando números distintos.
 */
function TextoDeEnlaceAdmin({ enlace, afuera }: { enlace: Enlace; afuera: number }) {
  if (enlace.a !== ENLACE_CON_AFUERA || afuera === 0) {
    return <>{enlace.texto}</>
  }
  return (
    <span className="flex w-full items-center justify-between gap-1.5">
      {enlace.texto}
      <Badge variant="secondary" aria-label={`${afuera} fuera del laboratorio`}>
        {afuera}
      </Badge>
    </span>
  )
}

function claseDeEnlace({ isActive }: { isActive: boolean }): string {
  // El activo se marca con fondo y no solo con color: en una barra de diez
  // ítems, un cambio de tono se pierde y nadie sabe dónde está parado.
  return [
    "rounded-lg px-2.5 py-1.5 text-sm font-medium transition-colors",
    isActive
      ? "bg-accent text-accent-foreground"
      : "text-muted-foreground hover:bg-muted hover:text-foreground",
  ].join(" ")
}

/**
 * Lo mismo, para el menú desplegable del teléfono: ahí cada enlace se toca
 * con el pulgar y necesita los 44px de WCAG 2.5.5, que con el `py-1.5` de la
 * barra no llegaba (32px).
 */
function claseDeEnlaceMovil(estado: { isActive: boolean }): string {
  return `${claseDeEnlace(estado)} flex min-h-11 items-center`
}

/** El grupo "Administración" de la barra de escritorio. */
function MenuAdministracion({ afuera }: { afuera: number }) {
  const [abierto, setAbierto] = useState(false)
  const contenedor = useRef<HTMLDivElement>(null)
  const { pathname } = useLocation()

  // Con el menú cerrado, estar parado en una de sus pantallas tiene que verse
  // igual: si no, en /admin/reportes la barra no marca nada y no hay forma de
  // saber dónde se está.
  const enElGrupo = ENLACES_ADMIN.some((e) => pathname.startsWith(e.a))

  useEffect(() => {
    setAbierto(false)
  }, [pathname])

  useEffect(() => {
    if (!abierto) return

    function alApuntarAfuera(evento: MouseEvent) {
      if (!contenedor.current?.contains(evento.target as Node)) setAbierto(false)
    }
    function alTeclear(evento: KeyboardEvent) {
      if (evento.key === "Escape") setAbierto(false)
    }

    document.addEventListener("mousedown", alApuntarAfuera)
    document.addEventListener("keydown", alTeclear)
    return () => {
      document.removeEventListener("mousedown", alApuntarAfuera)
      document.removeEventListener("keydown", alTeclear)
    }
  }, [abierto])

  return (
    <div className="relative shrink-0" ref={contenedor}>
      <button
        type="button"
        aria-expanded={abierto}
        aria-controls="menu-administracion"
        onClick={() => setAbierto((estaba) => !estaba)}
        className={[
          "flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-sm font-medium transition-colors",
          "focus-visible:ring-ring focus-visible:ring-2 focus-visible:outline-none",
          enElGrupo || abierto
            ? "bg-accent text-accent-foreground"
            : "text-muted-foreground hover:bg-muted hover:text-foreground",
        ].join(" ")}
      >
        Administración
        <ChevronDown
          aria-hidden="true"
          className={`size-3.5 transition-transform ${abierto ? "rotate-180" : ""}`}
        />
      </button>

      {abierto && (
        <nav
          id="menu-administracion"
          aria-label="Administración"
          className="bg-superficie border-border absolute right-0 z-30 mt-1 grid w-56 gap-0.5 rounded-xl border p-1 shadow-lg"
        >
          {ENLACES_ADMIN.map((e) => (
            <NavLink key={e.a} to={e.a} className={claseDeEnlace}>
              <TextoDeEnlaceAdmin enlace={e} afuera={afuera} />
            </NavLink>
          ))}
        </nav>
      )}
    </div>
  )
}

export function AppLayout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const noLeidas = useNoLeidas()
  const [menuAbierto, setMenuAbierto] = useState(false)

  const esAdmin = user?.rol === "ADMIN"
  const afuera = useAfuera(esAdmin)
  const enlaces = esAdmin ? [...ENLACES, ENLACE_APROBACION] : ENLACES

  function handleLogout() {
    logout()
    navigate("/login", { replace: true })
  }

  // Navegar cierra el menú del teléfono: si quedara abierto, taparía la
  // pantalla a la que se acaba de entrar.
  const cerrarMenu = () => setMenuAbierto(false)

  // Recibe la clase porque el mismo enlace se dibuja en la barra y en el
  // menú del teléfono, y ahí el alto mínimo no es el mismo.
  const avisos = (clase: (estado: { isActive: boolean }) => string) => (
    <NavLink to="/notificaciones" className={clase} onClick={cerrarMenu}>
      <span className="flex items-center gap-1.5">
        Avisos
        {/* RF-05.7: las notificaciones tienen que verse al ingresar. El
            contador es lo que hace que alguien entre acá — nadie revisa una
            bandeja por las dudas. */}
        {noLeidas > 0 && (
          <Badge
            className="bg-destructive text-white"
            aria-label={`${noLeidas} sin leer`}
          >
            {noLeidas}
          </Badge>
        )}
      </span>
    </NavLink>
  )

  return (
    <div className="flex min-h-svh flex-col">
      {/* Sticky: en las pantallas con listados largos, volver al menú
          obligaba a subir hasta el principio de la página. */}
      <header className="bg-superficie/90 border-border sticky top-0 z-20 border-b backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center gap-3 px-4 py-2.5">
          {/* `min-h-11` solo en el teléfono, por el blanco táctil: en la
              barra de escritorio el alto lo fija el contenido. */}
          <Link
            to="/"
            className="flex min-h-11 shrink-0 items-center gap-2 font-semibold sm:min-h-0"
            onClick={cerrarMenu}
          >
            <span
              aria-hidden="true"
              className="bg-primary text-primary-foreground grid size-7 place-items-center rounded-lg text-xs font-bold"
            >
              SG
            </span>
            <span>SGRC</span>
          </Link>

          {/* Barra horizontal solo cuando hay lugar de verdad. Con el grupo
              de administración plegado son cinco ítems y entran holgados;
              `min-w-0` + `overflow-x-auto` quedan igual de red, porque el
              nombre del usuario todavía puede crecer.

              `min-[1100px]` y no `lg` (1024): entre 1024 y 1087px la barra de
              un Admin entra, pero apretada —"Horario Admins" se partía en dos
              renglones y el header pasaba de 57 a 73px—. A 1088px deja de
              partirse; 1100 es ese número con un poco de aire. En esa franja
              ahora se ve el menú del teléfono, que ahí anda bien. El valor
              está en CINCO clases de este archivo (esta barra, "Pedir ayuda",
              "Salir", el botón "Menú" y el menú desplegable) y las cinco
              tienen que decir lo mismo: si una queda en `lg`, hay un ancho en
              el que se ven los dos menús a la vez, o ninguno.

              OJO con dónde termina ese `overflow-x-auto`: tiene que envolver
              los enlaces y nada más. El desplegable de administración va
              afuera, porque `overflow-x: auto` con `overflow-y: visible`
              computa a `overflow-y: auto` —lo dice la especificación, no es
              una rareza de un navegador— y un panel absoluto que cuelga por
              debajo de la barra queda recortado contra el borde. El botón
              responde, el panel se monta, y no se ve nada. */}
          <nav
            aria-label="Principal"
            className="hidden min-w-0 flex-1 items-center gap-0.5 min-[1100px]:flex"
          >
            <div className="flex min-w-0 items-center gap-0.5 overflow-x-auto">
              {enlaces.map((e) => (
                <NavLink key={e.a} to={e.a} className={claseDeEnlace}>
                  {e.texto}
                </NavLink>
              ))}
              {avisos(claseDeEnlace)}
            </div>
            {esAdmin && <MenuAdministracion afuera={afuera} />}
          </nav>

          <div className="ml-auto flex shrink-0 items-center gap-2">
            {/* El redondel lleva al perfil y ya no directo a cambiar la
                contraseña: esa es UNA de las cosas que alguien hace sobre su
                cuenta, y la cara es donde se busca el perfil en cualquier
                aplicación. La contraseña sigue estando, adentro. */}
            {user && (
              <Link
                to="/perfil"
                onClick={cerrarMenu}
                className="hover:bg-muted hidden min-w-0 items-center gap-2 rounded-lg px-2 py-1 sm:flex"
                title={`${user.nombre} ${user.apellido} — ${esAdmin ? "Administración" : "Docente"}`}
              >
                <Avatar
                  usuarioId={user.id}
                  nombre={user.nombre}
                  apellido={user.apellido}
                />
                {/* El nombre escrito es SOLO para el docente, y solo cuando
                    sobra ancho de verdad. El Admin ve nada más el redondel:
                    su barra suma Aprobación y el grupo Administración, y
                    medida a 1536px no le queda lugar para este bloque —con
                    los enlaces en una sola línea el nombre entra en 80px, y
                    ni "Ada Lovelace" mide tan poco—. Cuando se lo mostrábamos
                    igual, el que cedía era el `nav`: "Horario Admins" se
                    partía en dos renglones y el header pasaba de 57 a 73px.
                    No es una pérdida para el Admin: entre 1024 y 1535px ya
                    venía viendo solo el redondel, y el nombre completo está
                    en el `title` de acá y en /perfil.

                    Ojo con el ancho: NO depende de la pantalla. El contenedor
                    es `max-w-6xl`, así que de 1152px en adelante da igual
                    tener 1536 o 1920 —subir el punto de corte no consigue ni
                    un píxel más—. Lo único que lo mueve es cuántos enlaces
                    hay al lado.

                    Para el docente, `max-w-64` + `truncate`: sin tope el
                    bloque crece sin límite, el `nav` de al lado (que es quien
                    tiene `flex-1`) se queda sin ancho y los enlaces de TODOS
                    terminan detrás de una barra de desplazamiento para
                    mostrar entero el apellido de UNO. Su presupuesto medido
                    es 322px; 256 deja margen. */}
                {!esAdmin && (
                  <span className="hidden max-w-64 min-w-0 text-left leading-tight 2xl:block">
                    <span className="block truncate text-sm font-medium">
                      {user.nombre} {user.apellido}
                    </span>
                    <span className="text-muted-foreground block truncate text-xs">
                      Docente
                    </span>
                  </span>
                )}
                <span className="sr-only">Mi cuenta</span>
              </Link>
            )}
            {/* Pedir ayuda vive en la barra y no adentro de un menú: quien
                lo necesita está trabado en alguna pantalla, y hacerlo buscar
                dónde escribir es exactamente el momento en que deja de
                intentarlo y va a golpear la puerta del laboratorio. */}
            <Button
              variant="outline"
              size="sm"
              className="hidden min-[1100px]:inline-flex"
              onClick={() => {
                cerrarMenu()
                navigate("/notificaciones?soporte=nuevo")
              }}
            >
              <LifeBuoy aria-hidden="true" className="size-4" />
              Pedir ayuda
            </Button>

            <BotonDeTema />

            <Button
              variant="outline"
              size="sm"
              className="hidden min-[1100px]:inline-flex"
              onClick={handleLogout}
            >
              Salir
            </Button>

            {/* `h-11` en el teléfono: es el control que abre todo lo demás,
                así que es el último que puede quedar chico para un dedo. */}
            <Button
              variant="outline"
              size="sm"
              className="h-11 px-4 min-[1100px]:hidden"
              aria-expanded={menuAbierto}
              aria-controls="menu-principal"
              onClick={() => setMenuAbierto((abierto) => !abierto)}
            >
              {menuAbierto ? "Cerrar" : "Menú"}
              {!menuAbierto && noLeidas > 0 && (
                <>
                  <span
                    aria-hidden="true"
                    className="bg-destructive ml-1.5 size-2 rounded-full"
                  />
                  {/* El punto rojo se ve, pero no dice qué es: en la barra
                      ancha el mismo aviso está escrito ("Avisos 3") y en el
                      teléfono quedaba un punto mudo. Va como texto solo para
                      lectores de pantalla, que además es lo que se escucha al
                      llegar al botón. */}
                  <span className="sr-only">, {contar(noLeidas, "aviso")} sin leer</span>
                </>
              )}
            </Button>
          </div>
        </div>

        {/* Menú del teléfono: una columna, con área de toque real. Antes los
            diez enlaces se envolvían en varias líneas de texto chico, sin
            separación entre ellos. */}
        {menuAbierto && (
          <nav
            id="menu-principal"
            className="border-border grid gap-0.5 border-t px-4 py-2 min-[1100px]:hidden"
          >
            {enlaces.map((e) => (
              <NavLink
                key={e.a}
                to={e.a}
                className={claseDeEnlaceMovil}
                onClick={cerrarMenu}
              >
                {e.texto}
              </NavLink>
            ))}
            {avisos(claseDeEnlaceMovil)}

            {/* En el teléfono el grupo va desplegado bajo un título en vez
                de detrás de otro clic: el menú ya es una lista vertical que
                se recorre con el pulgar, y meter un desplegable adentro de
                un desplegable agrega un paso sin ahorrar nada de espacio.

                El título necesita la línea y el aire de arriba: en una
                columna de renglones todos iguales, versalitas grises a la
                misma altura y con la misma sangría que los enlaces se leen
                como un ítem más que no responde al toque. La regla dice
                "acá empieza otra cosa" antes de que nadie intente
                tocarlo. */}
            {esAdmin && (
              <>
                <p className="text-muted-foreground border-border mt-3 border-t px-2.5 pt-3 pb-1 text-xs font-semibold tracking-wide uppercase">
                  Administración
                </p>
                {ENLACES_ADMIN.map((e) => (
                  <NavLink
                    key={e.a}
                    to={e.a}
                    className={claseDeEnlaceMovil}
                    onClick={cerrarMenu}
                  >
                    <TextoDeEnlaceAdmin enlace={e} afuera={afuera} />
                  </NavLink>
                ))}
              </>
            )}

            {/* En el teléfono no hay lugar en la barra, pero tampoco puede
                quedar enterrado: va arriba de "Mi cuenta", que es lo último
                de la lista. */}
            <NavLink
              to="/notificaciones?soporte=nuevo"
              className={claseDeEnlaceMovil}
              onClick={cerrarMenu}
            >
              Pedir ayuda
            </NavLink>
            <NavLink
              to="/cambiar-password"
              className={claseDeEnlaceMovil}
              onClick={cerrarMenu}
            >
              Mi cuenta
            </NavLink>
            <Button
              variant="outline"
              size="sm"
              className="mt-1 h-11"
              onClick={handleLogout}
            >
              Cerrar sesión
            </Button>
          </nav>
        )}
      </header>

      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6">
        <Outlet />
      </main>

      {/* Al fondo de todo, no fijo: la columna es `min-h-svh flex-col` con el
          `main` en `flex-1`, así que en una pantalla corta el pie se apoya en
          el borde inferior y en una larga espera al final del contenido. En
          ningún caso le come lugar a lo que la persona vino a hacer. */}
      <PieDeAutoria className="border-border border-t px-4 py-4" />
    </div>
  )
}
