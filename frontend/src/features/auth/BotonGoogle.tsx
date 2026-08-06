import { useEffect, useRef, useState } from "react"

import * as authApi from "@/features/auth/api"
import { cargarGoogleIdentity, type GoogleIdentity } from "@/lib/google-identity"

/**
 * El botón "Iniciar sesión con Google". Lo dibuja Google, no nosotros: la
 * biblioteca lo renderiza dentro de un iframe propio, y ese es el único
 * botón que su política de marca permite.
 *
 * Que el iframe sea de Google significa que NO se puede tocar con CSS: no
 * hay clase, variable de tema ni `!important` que entre ahí. Todo lo que se
 * puede ajustar son las opciones de `renderButton`, y son justo las que
 * hacen que el botón parezca parte de la pantalla en vez de un recorte
 * pegado encima:
 *
 *   - `width` a la medida del formulario. Es lo que más se notaba: el botón
 *     salía con el ancho de su propio texto, centrado, mientras el de
 *     "Iniciar sesión" ocupaba toda la fila. Dos botones uno sobre otro con
 *     anchos distintos se leen como si el de abajo fuera de otra página.
 *   - `theme` según el tema de la app. En oscuro, el botón `outline` es una
 *     tarjeta blanca en medio de un formulario oscuro.
 *   - `logo_alignment: "left"`, que centra el texto en el resto del botón
 *     igual que los nuestros.
 *
 * No decide nada sobre la sesión — cuando Google devuelve el token, lo
 * entrega por `onCredential` y quien lo usa decide qué hacer (entrar,
 * mandar a completar el registro, mostrar un error).
 *
 * Si el despliegue no tiene GOOGLE_CLIENT_ID, o si el script de Google no
 * carga, no se dibuja nada: el formulario de email y contraseña sigue
 * estando y es un camino completo por sí solo.
 */

/**
 * El tope que acepta `renderButton`. Pedirle más no agranda el botón: lo
 * ignora y vuelve al ancho del texto, que es exactamente el defecto que
 * estamos tratando de evitar.
 */
const ANCHO_MAXIMO = 400

export function BotonGoogle({
  onCredential,
  texto = "signin_with",
}: {
  onCredential: (credential: string) => void
  /** "signup_with" dice "Registrarse con Google" en vez de "Iniciar sesión". */
  texto?: "signin_with" | "signup_with"
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
    // cancelado evita tocar el DOM (o el estado) si el componente se
    // desmontó mientras se cargaba el script — pasa al navegar rápido entre
    // login y registro.
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
          // solo y tapar el formulario de login. El botón explícito hace lo
          // mismo sin sorprender a nadie.
          auto_select: false,
          cancel_on_tap_outside: true,
        })
        setGoogle(google)
      } catch {
        // Sin botón de Google, el formulario de siempre sigue funcionando.
        // No se muestra ningún error: para quien iba a entrar con email y
        // contraseña, un cartel rojo sobre algo que no pensaba usar es
        // ruido.
      }
    }

    void preparar()
    return () => {
      cancelado = true
    }
  }, [])

  // ── Dibujar, y volver a dibujar si cambia el ancho o el tema ──────
  //
  // Son dos efectos y no uno porque tienen frecuencias distintas: cargar el
  // script y registrar el callback pasa una vez, y redibujar pasa cada vez
  // que alguien rota el teléfono o toca el interruptor de tema. Meterlos
  // juntos volvería a pedirle la config al backend en cada resize.
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
      {/* El iframe de Google se centra solo dentro de esta fila cuando no
          llega a ocupar todo el ancho (pantallas más anchas que el tope de
          400px que acepta renderButton). */}
      <div ref={contenedor} className="flex justify-center" />
    </div>
  )
}

/**
 * El ancho que tiene disponible el botón, en píxeles.
 *
 * Se mide el PADRE y no el propio nodo porque el nuestro está `hidden`
 * hasta que Google contesta, y un elemento con `display:none` mide cero.
 * El padre es el formulario, que siempre está en pantalla y es justo la
 * caja contra la que queremos alinearnos.
 */
function useAnchoDisponible(ref: React.RefObject<HTMLElement | null>) {
  const [ancho, setAncho] = useState(0)

  useEffect(() => {
    const medir = () => setAncho(ref.current?.parentElement?.clientWidth ?? 0)
    medir()
    // Un listener de resize y no un ResizeObserver: lo único que cambia
    // este ancho es el viewport (rotar el teléfono, achicar la ventana), y
    // el formulario no se redimensiona por su cuenta.
    window.addEventListener("resize", medir)
    return () => window.removeEventListener("resize", medir)
  }, [ref])

  return ancho
}

/**
 * Si la app está en tema oscuro, para pedirle a Google el botón que
 * corresponde. Observa la clase del `<html>` en vez de leer la preferencia
 * guardada porque esa clase es la única fuente de verdad — la ponen tanto
 * el interruptor de la barra como el script inline de index.html (ver
 * lib/tema.ts).
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
