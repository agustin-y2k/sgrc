import { useEffect, useRef, useState } from "react"
import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router"

import { ChevronDown } from "lucide-react"

import { BotonDeTema } from "@/components/BotonDeTema"
import { PieDeAutoria } from "@/components/PieDeAutoria"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/features/auth/AuthContext"
import { useNoLeidas } from "@/features/notificaciones/useNoLeidas"

/**
 * Los enlaces del menú, en el orden en que se usan.
 *
 * Están en una lista y no escritos a mano en el JSX porque la barra se
 * dibuja dos veces —horizontal en pantallas grandes, desplegable en un
 * teléfono— y dos copias del mismo menú es cómo terminan diferenciándose.
 *
 * Van en dos listas y no en una con `soloAdmin` porque no son la misma
 * clase de cosa. Las primeras son los tres lugares a los que se entra todos
 * los días; las de administración son tareas de mantenimiento que se hacen
 * de a ratos —dar de alta un ciclo, revisar un reporte— y que se usan una
 * vez cada tanto. Puestas todas al mismo nivel eran diez ítems hermanos sin
 * jerarquía: para un Admin, la barra no tenía forma, y a 1024px ni siquiera
 * entraba (de ahí el `overflow-x-auto` que había que dejar de red).
 */
type Enlace = { a: string; texto: string }

const ENLACES: Enlace[] = [
  { a: "/reservas", texto: "Reservas" },
  { a: "/inventario", texto: "Inventario" },
  // RF-07.2: lo ve cualquier usuario autenticado. Para un docente es "a
  // quién busco si necesito algo del laboratorio"; para un Admin es además
  // donde carga su propio horario.
  { a: "/disponibilidad", texto: "Admins" },
]

/**
 * Aprobación queda afuera del grupo, en la barra: es la única tarea de
 * administración que es diaria y reactiva —llegó un registro nuevo y
 * alguien está esperando para poder trabajar—. Enterrarla a un clic la
 * volvería más lenta justo en el caso en que la demora se le nota a otro.
 */
const ENLACE_APROBACION: Enlace = { a: "/admin/aprobacion", texto: "Aprobación" }

const ENLACES_ADMIN: Enlace[] = [
  // Usuarios es el panel completo, del que Aprobación es la vista enfocada.
  { a: "/admin/usuarios", texto: "Usuarios" },
  // Ciclos, cursos y materias: de esto depende todo lo demás, así que va
  // antes que inventario y reportes.
  { a: "/admin/academico", texto: "Académico" },
  { a: "/admin/inventario", texto: "Gestión del inventario" },
  { a: "/admin/reportes", texto: "Reportes" },
  // RF-04.7. Último: es lo que menos se usa y lo que más rompe si se entra
  // sin querer.
  { a: "/admin/bloqueo-evaluacion", texto: "Bloqueo por evaluación" },
]

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
 * El grupo "Administración" de la barra de escritorio.
 *
 * Es un desplegable escrito a mano y no una primitiva de Radix por dos
 * razones: el archivo ya resuelve así el menú del teléfono —un botón con
 * `aria-expanded`/`aria-controls` y un `<nav>` condicional— y traer una
 * segunda mecánica para el mismo gesto es cómo terminan comportándose
 * distinto; y porque lo que hace falta acá es una lista de cinco enlaces,
 * no un menú con navegación por flechas.
 *
 * Lo que sí hay que darle es lo que un usuario espera de cualquier
 * desplegable: que Escape lo cierre, que un clic afuera lo cierre y que
 * navegar lo cierre. Sin eso queda una trampa en la que se hace clic en
 * cualquier lado y no pasa nada.
 */
function MenuAdministracion() {
  const [abierto, setAbierto] = useState(false)
  const contenedor = useRef<HTMLDivElement>(null)
  const { pathname } = useLocation()

  // Con el menú cerrado, estar parado en una de sus pantallas tiene que
  // verse igual: si no, en /admin/reportes la barra no marca nada y no hay
  // forma de saber dónde se está.
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
              {e.texto}
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
  const enlaces = esAdmin ? [...ENLACES, ENLACE_APROBACION] : ENLACES

  function handleLogout() {
    logout()
    navigate("/login", { replace: true })
  }

  // Navegar cierra el menú del teléfono: si quedara abierto, taparía la
  // pantalla a la que se acaba de entrar.
  const cerrarMenu = () => setMenuAbierto(false)

  const avisos = (
    <NavLink to="/notificaciones" className={claseDeEnlace} onClick={cerrarMenu}>
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
          <Link
            to="/"
            className="flex shrink-0 items-center gap-2 font-semibold"
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

              OJO con dónde termina ese `overflow-x-auto`: tiene que envolver
              los enlaces y nada más. El desplegable de administración va
              afuera, porque `overflow-x: auto` con `overflow-y: visible`
              computa a `overflow-y: auto` —lo dice la especificación, no es
              una rareza de un navegador— y un panel absoluto que cuelga por
              debajo de la barra queda recortado contra el borde. El botón
              responde, el panel se monta, y no se ve nada. */}
          <nav
            aria-label="Principal"
            className="hidden min-w-0 flex-1 items-center gap-0.5 lg:flex"
          >
            <div className="flex min-w-0 items-center gap-0.5 overflow-x-auto">
              {enlaces.map((e) => (
                <NavLink key={e.a} to={e.a} className={claseDeEnlace}>
                  {e.texto}
                </NavLink>
              ))}
              {avisos}
            </div>
            {esAdmin && <MenuAdministracion />}
          </nav>

          <div className="ml-auto flex shrink-0 items-center gap-2">
            {user && (
              <Link
                to="/cambiar-password"
                onClick={cerrarMenu}
                className="hover:bg-muted hidden items-center gap-2 rounded-lg px-2 py-1 sm:flex"
                title={`${user.nombre} ${user.apellido} — ${esAdmin ? "Administración" : "Docente"}`}
              >
                <span
                  aria-hidden="true"
                  className="bg-accent text-accent-foreground grid size-7 shrink-0 place-items-center rounded-full text-xs font-semibold"
                >
                  {(user.nombre[0] ?? "") + (user.apellido[0] ?? "")}
                </span>
                {/* El nombre completo solo cuando sobra ancho de verdad. En un
                    portátil de 1024 o 1280, con el menú de Admin desplegado,
                    estas dos líneas de texto eran justo lo que no entraba. La
                    inicial sigue estando, y el nombre en el `title`. */}
                <span className="hidden text-left leading-tight 2xl:block">
                  <span className="block text-sm font-medium">
                    {user.nombre} {user.apellido}
                  </span>
                  <span className="text-muted-foreground block text-xs">
                    {esAdmin ? "Administración" : "Docente"}
                  </span>
                </span>
                <span className="sr-only">Mi cuenta</span>
              </Link>
            )}
            <BotonDeTema />

            <Button
              variant="outline"
              size="sm"
              className="hidden lg:inline-flex"
              onClick={handleLogout}
            >
              Salir
            </Button>

            <Button
              variant="outline"
              size="sm"
              className="lg:hidden"
              aria-expanded={menuAbierto}
              aria-controls="menu-principal"
              onClick={() => setMenuAbierto((abierto) => !abierto)}
            >
              {menuAbierto ? "Cerrar" : "Menú"}
              {!menuAbierto && noLeidas > 0 && (
                <span
                  aria-hidden="true"
                  className="bg-destructive ml-1.5 size-2 rounded-full"
                />
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
            className="border-border grid gap-0.5 border-t px-4 py-2 lg:hidden"
          >
            {enlaces.map((e) => (
              <NavLink key={e.a} to={e.a} className={claseDeEnlace} onClick={cerrarMenu}>
                {e.texto}
              </NavLink>
            ))}
            {avisos}

            {/* En el teléfono el grupo va desplegado bajo un título en vez
                de detrás de otro clic: el menú ya es una lista vertical que
                se recorre con el pulgar, y meter un desplegable adentro de
                un desplegable agrega un paso sin ahorrar nada de espacio. */}
            {esAdmin && (
              <>
                <p className="text-muted-foreground mt-2 px-2.5 text-xs font-semibold tracking-wide uppercase">
                  Administración
                </p>
                {ENLACES_ADMIN.map((e) => (
                  <NavLink
                    key={e.a}
                    to={e.a}
                    className={claseDeEnlace}
                    onClick={cerrarMenu}
                  >
                    {e.texto}
                  </NavLink>
                ))}
              </>
            )}

            <NavLink
              to="/cambiar-password"
              className={claseDeEnlace}
              onClick={cerrarMenu}
            >
              Mi cuenta
            </NavLink>
            <Button variant="outline" size="sm" className="mt-1" onClick={handleLogout}>
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
