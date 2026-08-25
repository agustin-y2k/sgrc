import { useEffect, useRef, useState } from "react"

import * as authApi from "@/features/auth/api"
import { cargarGoogleIdentity, type GoogleIdentity } from "@/lib/google-identity"

/** El botón "Iniciar sesión con Google". */

/** El tope que acepta `renderButton`. */
const ANCHO_MAXIMO = 400

export function BotonGoogle({
  onCredential,
  texto = "signin_with",
  children,
}: {
  onCredential: (credential: string) => void
  /** "signup_with" dice "Registrarse con Google" en vez de "Iniciar sesión". */
  texto?: "signin_with" | "signup_with"
  /**
   * Lo que va entre el separador y el botón — hoy, la casilla de mantener la
   * sesión iniciada del ingreso. Se recibe acá adentro y no se dibuja al lado
   * en la pantalla para que se esconda JUNTO con el botón: sin Google
   * configurado, una casilla suelta arriba de un hueco no dice nada.
   */
  children?: React.ReactNode
}) {
  const raiz = useRef<HTMLDivElement>(null)
  const contenedor = useRef<HTMLDivElement>(null)
  const [google, setGoogle] = useState<GoogleIdentity | null>(null)
  const ancho = useAnchoDisponible(raiz)
  const oscuro = useTemaOscuro()

  // El callback se guarda en un ref porque a Google se le pasa UNA sola vez,
  // al inicializar: si se leyera la prop directamente, el botón se quedaría
  // llamando a la versión de `onCredential` que existía en el primer render.
  const callback = useRef(onCredential)
  useEffect(() => {
    callback.current = onCredential
  }, [onCredential])

  // ── Cargar e inicializar, una sola vez ────────────────────────────
  useEffect(() => {
    // cancelado evita tocar el DOM (o el estado) si el componente se desmontó
    // mientras se cargaba el script — pasa al navegar rápido entre login y
    // registro.
    let cancelado = false

    async function preparar() {
      try {
        const { googleClientId } = await authApi.configPublica()
        if (!googleClientId || cancelado) return

        const google = await cargarGoogleIdentity()
        if (cancelado) return

        google.accounts.id.initialize({
          client_id: googleClientId,
          callback: ({ credential }) => {
            if (credential) callback.current(credential)
          },
          // One Tap apagado: el diálogo flotante de Google puede aparecer
          // solo y tapar el formulario de login.
          auto_select: false,
          cancel_on_tap_outside: true,
        })
        setGoogle(google)
      } catch {
        // Sin botón de Google, el formulario de siempre sigue funcionando.
      }
    }

    void preparar()
    return () => {
      cancelado = true
    }
  }, [])

  // ── Dibujar, y volver a dibujar si cambia el ancho o el tema ────── Son
  // dos efectos y no uno porque tienen frecuencias distintas: cargar el
  // script y registrar el callback pasa una vez, y redibujar pasa cada vez
  // que alguien rota el teléfono o toca el interruptor de tema.
  useEffect(() => {
    if (!google || !contenedor.current) return

    // Google APPEND-ea: sin vaciar, cada redibujado deja el botón anterior
    // arriba y se van apilando.
    contenedor.current.replaceChildren()
    google.accounts.id.renderButton(contenedor.current, {
      type: "standard",
      theme: oscuro ? "filled_black" : "outline",
      size: "large",
      text: texto,
      shape: "rectangular",
      logo_alignment: "left",
      locale: "es",
      // Sin medida todavía (el primer render, o jsdom en los tests) se
      // omite y Google usa su ancho automático, que es lo que hacía antes.
      ...(ancho > 0 ? { width: Math.min(ancho, ANCHO_MAXIMO) } : {}),
    })
  }, [google, ancho, oscuro, texto])

  return (
    <div
      ref={raiz}
      // hidden mientras no esté listo, en vez de no renderizar el div:
      // Google necesita el elemento montado para dibujar el botón adentro.
      className={google ? "flex flex-col gap-4" : "hidden"}
      data-testid="boton-google"
    >
      <div className="flex items-center gap-3">
        <span className="bg-border h-px flex-1" />
        <span className="text-muted-foreground text-xs">o</span>
        <span className="bg-border h-px flex-1" />
      </div>
      {children}
      {/* El iframe de Google se centra solo dentro de esta fila cuando no
          llega a ocupar todo el ancho (pantallas más anchas que el tope de
          400px que acepta renderButton). */}
      <div ref={contenedor} className="flex justify-center" />
    </div>
  )
}

/** El ancho que tiene disponible el botón, en píxeles. */
function useAnchoDisponible(ref: React.RefObject<HTMLElement | null>) {
  const [ancho, setAncho] = useState(0)

  useEffect(() => {
    const medir = () => setAncho(ref.current?.parentElement?.clientWidth ?? 0)
    medir()
    // Un listener de resize y no un ResizeObserver: lo único que cambia este
    // ancho es el viewport (rotar el teléfono, achicar la ventana), y el
    // formulario no se redimensiona por su cuenta.
    window.addEventListener("resize", medir)
    return () => window.removeEventListener("resize", medir)
  }, [ref])

  return ancho
}

/**
 * Si la app está en tema oscuro, para pedirle a Google el botón que
 * corresponde.
 */
function useTemaOscuro() {
  const leer = () => document.documentElement.classList.contains("dark")
  const [oscuro, setOscuro] = useState(leer)

  useEffect(() => {
    const observador = new MutationObserver(() => setOscuro(leer()))
    observador.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    })
    return () => observador.disconnect()
  }, [])

  return oscuro
}
