/**
 * Carga de Google Identity Services (el botón "Iniciar sesión con Google") y
 * lectura del token que devuelve.
 */

const URL_SCRIPT = "https://accounts.google.com/gsi/client"

/** Lo poquito que usamos de la API de Google, tipado a mano. */
export type GoogleIdentity = {
  accounts: {
    id: {
      initialize: (config: {
        client_id: string
        callback: (respuesta: { credential?: string }) => void
        // Sin esto, Google puede mostrar el diálogo de "One Tap" por su
        // cuenta encima de la pantalla de login.
        auto_select?: boolean
        cancel_on_tap_outside?: boolean
      }) => void
      renderButton: (
        contenedor: HTMLElement,
        opciones: {
          type?: "standard" | "icon"
          theme?: "outline" | "filled_blue" | "filled_black"
          size?: "small" | "medium" | "large"
          text?: "signin_with" | "signup_with" | "continue_with"
          shape?: "rectangular" | "pill"
          width?: number
          locale?: string
          // Con el logo a la izquierda y el texto centrado en el resto, el
          // botón se lee como los demás de la pantalla.
          logo_alignment?: "left" | "center"
        }
      ) => void
    }
  }
}

declare global {
  interface Window {
    google?: GoogleIdentity
  }
}

/**
 * Carga el script una sola vez por pestaña, aunque se lo pidan varios
 * componentes a la vez: la promesa se guarda y se reutiliza.
 */
let cargando: Promise<GoogleIdentity> | null = null

export function cargarGoogleIdentity(): Promise<GoogleIdentity> {
  if (window.google) return Promise.resolve(window.google)
  if (cargando) return cargando

  cargando = new Promise<GoogleIdentity>((resolve, reject) => {
    const script = document.createElement("script")
    script.src = URL_SCRIPT
    script.async = true
    script.defer = true
    script.onload = () => {
      if (window.google) {
        resolve(window.google)
      } else {
        reject(new Error("el script de Google cargó pero no expuso window.google"))
      }
    }
    script.onerror = () => {
      // Se limpia para que un reintento (por ejemplo, después de que vuelva
      // la conexión) vuelva a intentar la carga en vez de quedar pegado al
      // rechazo anterior para siempre.
      cargando = null
      reject(new Error("no se pudo cargar el script de Google"))
    }
    document.head.appendChild(script)
  })

  return cargando
}

export type DatosDeLaCredencial = {
  email: string
  nombre: string
  apellido: string
}

/**
 * Lee el contenido del ID token SIN verificarlo, solo para prellenar el
 * formulario de registro con lo que la persona ya tiene en su cuenta de
 * Google.
 */
export function datosDeLaCredencial(credencial: string): DatosDeLaCredencial | null {
  const partes = credencial.split(".")
  if (partes.length !== 3) return null

  try {
    // base64url → base64, y se repone el padding que el formato omite.
    const base64 = partes[1].replace(/-/g, "+").replace(/_/g, "/")
    const conPadding = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), "=")
    // decodeURIComponent + escape es lo que hace que los acentos sobrevivan:
    // atob devuelve bytes, no caracteres, así que "Martín" llegaría roto.
    const json = decodeURIComponent(
      atob(conPadding)
        .split("")
        .map((c) => "%" + c.charCodeAt(0).toString(16).padStart(2, "0"))
        .join("")
    )
    const payload = JSON.parse(json) as {
      email?: string
      given_name?: string
      family_name?: string
    }
    return {
      email: payload.email ?? "",
      nombre: payload.given_name ?? "",
      apellido: payload.family_name ?? "",
    }
  } catch {
    return null
  }
}
