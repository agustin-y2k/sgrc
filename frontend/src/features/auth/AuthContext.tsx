import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react"
import * as authApi from "@/features/auth/api"
import type { LoginResponse, Usuario } from "@/features/auth/types"
import { ApiError, registrarManejadorDeSesionRechazada } from "@/lib/api-client"
import { olvidarPedidoDescartado } from "@/features/admin/pedidoDeJornada"
import { clearToken, getToken, setToken } from "@/lib/token-store"

type AuthContextValue = {
  user: Usuario | null
  isLoading: boolean
  /**
   * Se llena solo cuando la sesión no se pudo verificar por una falla de
   * red/servidor (no por un token inválido).
   */
  errorDeSesion: string | null
  /**
   * Por qué se cerró la sesión, cuando la cerró el backend y no la persona.
   */
  motivoDeCierre: string | null
  /**
   * `recordarme` es la casilla "Mantener la sesión iniciada": el backend firma
   * un token de vigencia larga en vez de la normal. Omitido = sesión normal.
   */
  login: (
    email: string,
    password: string,
    recordarme?: boolean
  ) => Promise<{ debeCambiarPassword: boolean }>
  /** Ingreso con el ID token que devolvió Google, con la misma casilla. */
  loginConGoogle: (
    credential: string,
    recordarme?: boolean
  ) => Promise<{ debeCambiarPassword: boolean }>
  logout: () => void
  refetchUser: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<Usuario | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [errorDeSesion, setErrorDeSesion] = useState<string | null>(null)
  const [motivoDeCierre, setMotivoDeCierre] = useState<string | null>(null)

  // El backend puede rechazar el token en cualquier request, no solo en el
  // GET /me del arranque: la cuenta se dio de baja (RF-02.8), o alguien
  // cambió su contraseña y eso cerró las sesiones abiertas (RF-01.11).
  useEffect(() => {
    return registrarManejadorDeSesionRechazada((mensaje, motivo) => {
      clearToken()
      setUser(null)
      // errorDeSesion se limpia a propósito: eso es "no pude verificar la
      // sesión, reintentá", y acá el backend sí contestó.
      setErrorDeSesion(null)
      // Que una sesión venza NO se anuncia. Es el final normal de toda sesión
      // —le pasa a todo el mundo, todos los días, y no hay nada que la persona
      // pueda hacer distinto— así que un cartel ahí solo consigue que volver
      // al sistema parezca un error del sistema. Va directo a la pantalla de
      // ingreso, callado.
      //
      // Que la sesión la haya cerrado ALGUIEN sí se explica: una cuenta dada
      // de baja (RF-02.8) o una contraseña cambiada (RF-01.11) dejan a la
      // persona reintentando sin entender por qué no entra. El motivo lo dice
      // el backend en un header, no el texto del mensaje ni una suposición
      // nuestra sobre si estaba usando el sistema.
      if (motivo === "revocada" || motivo === "password-cambiada") {
        setMotivoDeCierre(mensaje)
        return
      }
      // "invalida" y "desconocido" caen acá: un token falsificado no merece
      // explicación, y un motivo que no reconocemos se trata como el caso
      // normal en vez de mostrar un cartel que no sabemos si corresponde.
      // Antes de este header la condición era "¿estaba usando el sistema?", y
      // por eso una pestaña abierta toda la noche amanecía con "token inválido
      // o expirado" en la pantalla.
    })
  }, [])

  // Al bootear, si hay un token guardado, valida que siga siendo válido
  // contra el backend (no hay lista de revocación local — GET /me es la única
  // forma de confirmarlo) e hidrata el usuario completo.
  const loadUser = useCallback(async () => {
    if (!getToken()) {
      setUser(null)
      setErrorDeSesion(null)
      setIsLoading(false)
      return
    }
    try {
      const usuario = await authApi.me()
      setUser(usuario)
      setErrorDeSesion(null)
    } catch (err) {
      const tokenRechazado =
        err instanceof ApiError && (err.status === 401 || err.status === 403)
      if (tokenRechazado) {
        clearToken()
        setUser(null)
        setErrorDeSesion(null)
      } else {
        // Se conserva el token a propósito: la sesión probablemente sigue
        // siendo válida, lo que falló es la conexión.
        setUser(null)
        setErrorDeSesion("No se pudo verificar tu sesión. Revisá tu conexión.")
      }
      throw err
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    // El error ya quedó reflejado en el estado; acá se traga para no
    // generar una promesa rechazada sin manejar en el arranque.
    loadUser().catch(() => {})
  }, [loadUser])

  // abrirSesion es lo que los dos caminos de ingreso (contraseña y Google)
  // tienen en común: el backend devuelve el mismo LoginResponse en ambos, así
  // que a partir de acá la sesión es idéntica.
  const abrirSesion = useCallback(
    async (res: LoginResponse) => {
      if (!res.token) {
        // El backend nunca devuelve 200 sin token — Login() en
        // internal/auth/application/service.go rechaza con 403 antes de
        // firmar si la cuenta no está APROBADA. Si esto dispara, algo cambió
        // del lado del backend sin avisar acá.
        throw new Error("login sin token en la respuesta")
      }
      setToken(res.token)
      // El motivo del cierre anterior deja de tener sentido apenas se entra
      // de nuevo: si no, el cartel "tu sesión se cerró porque…" quedaría
      // colgado en el login después de un ingreso exitoso.
      setMotivoDeCierre(null)
      // Si loadUser falla acá, el error se propaga hasta LoginPage y se le
      // muestra al usuario: tragarlo haría que el login pareciera exitoso y
      // dejara a la persona de vuelta en /login sin decir por qué.
      await loadUser()
      return { debeCambiarPassword: res.debeCambiarPassword }
    },
    [loadUser]
  )

  // recordarme llega desde la casilla de la pantalla de ingreso y solo afecta
  // cuánto vive el token que firma el backend. Por omisión, false: en una
  // computadora compartida de la escuela una sesión de un mes es exactamente
  // lo que no queremos dejar abierto sin que nadie lo haya pedido.
  const login = useCallback(
    async (email: string, password: string, recordarme = false) =>
      abrirSesion(await authApi.login({ email, password, recordarme })),
    [abrirSesion]
  )

  const loginConGoogle = useCallback(
    async (credential: string, recordarme = false) =>
      abrirSesion(await authApi.loginConGoogle(credential, recordarme)),
    [abrirSesion]
  )

  const logout = useCallback(() => {
    clearToken()
    // El pedido de declarar la jornada vuelve en el próximo inicio: haberlo
    // postergado vale para esta sesión, no para la cuenta.
    olvidarPedidoDescartado()
    setUser(null)
    setErrorDeSesion(null)
    // Salir por decisión propia no necesita explicación en el login.
    setMotivoDeCierre(null)
  }, [])

  const value = useMemo(
    () => ({
      user,
      isLoading,
      errorDeSesion,
      motivoDeCierre,
      login,
      loginConGoogle,
      logout,
      refetchUser: loadUser,
    }),
    [
      user,
      isLoading,
      errorDeSesion,
      motivoDeCierre,
      login,
      loginConGoogle,
      logout,
      loadUser,
    ]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error("useAuth debe usarse dentro de <AuthProvider>")
  }
  return ctx
}
