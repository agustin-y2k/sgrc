import { useEffect, useRef, useState } from "react"

import * as authApi from "@/features/auth/api"
import { cargarGoogleIdentity } from "@/lib/google-identity"

/**
 * El botón "Iniciar sesión con Google". Lo dibuja Google, no nosotros: la
 * biblioteca lo renderiza dentro de un iframe propio, y ese es el único
 * botón que su política de marca permite.
 *
 * No decide nada sobre la sesión — cuando Google devuelve el token, lo
 * entrega por `onCredential` y quien lo usa decide qué hacer (entrar,
 * mandar a completar el registro, mostrar un error).
 *
 * Si el despliegue no tiene GOOGLE_CLIENT_ID, o si el script de Google no
 * carga, no se dibuja nada: el formulario de email y contraseña sigue
 * estando y es un camino completo por sí solo.
 */
export function BotonGoogle({
  onCredential,
  texto = "signin_with",
}: {
  onCredential: (credential: string) => void
  /** "signup_with" dice "Registrarse con Google" en vez de "Iniciar sesión". */
  texto?: "signin_with" | "signup_with"
}) {
  const contenedor = useRef<HTMLDivElement>(null)
  const [listo, setListo] = useState(false)

  // El callback se guarda en un ref porque a Google se le pasa UNA sola vez,
  // al inicializar: si se leyera la prop directamente, el botón se quedaría
  // llamando a la versión de `onCredential` que existía en el primer render.
  const callback = useRef(onCredential)
  useEffect(() => {
    callback.current = onCredential
  }, [onCredential])

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
        if (cancelado || !contenedor.current) return

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

        google.accounts.id.renderButton(contenedor.current, {
          type: "standard",
          theme: "outline",
          size: "large",
          text: texto,
          shape: "rectangular",
          locale: "es",
        })
        setListo(true)
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
  }, [texto])

  return (
    <div
      // hidden mientras no esté listo, en vez de no renderizar el div:
      // Google necesita el elemento montado para dibujar el botón adentro.
      className={listo ? "flex flex-col gap-4" : "hidden"}
      data-testid="boton-google"
    >
      <div className="flex items-center gap-3">
        <span className="bg-border h-px flex-1" />
        <span className="text-muted-foreground text-xs">o</span>
        <span className="bg-border h-px flex-1" />
      </div>
      <div ref={contenedor} className="flex justify-center" />
    </div>
  )
}
