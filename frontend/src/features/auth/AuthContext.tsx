import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
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
  login: (email: string, password: string) => Promise<{ debeCambiarPassword: boolean }>
  /** Ingreso con el ID token que devolvió Google. */
  loginConGoogle: (credential: string) => Promise<{ debeCambiarPassword: boolean }>
  logout: () => void
  refetchUser: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<Usuario | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [errorDeSesion, setErrorDeSesion] = useState<string | null>(null)
  const [motivoDeCierre, setMotivoDeCierre] = useState<string | null>(null)

  // Si había una sesión abierta EN ESTA VISITA. Es un ref y no un estado
  // porque lo lee el manejador de abajo, que se registra una sola vez y se
  // quedaría con el valor del primer render.
  //
  // Se marca donde la sesión se abre (loadUser) y no en un useEffect sobre
  // `user`: un efecto pasivo corre DESPUÉS del commit, así que quedaba una
  // ventana en la que la pantalla ya mostraba a la persona y esto seguía en
  // false. Un 401 que cayera justo ahí cerraba la sesión sin explicar por qué.
  const habiaSesionAbierta = useRef(false)

  // El backend puede rechazar el token en cualquier request, no solo en el
  // GET /me del arranque: la cuenta se dio de baja (RF-02.8), o alguien
  // cambió su contraseña y eso cerró las sesiones abiertas (RF-01.11).
  useEffect(() => {
    return registrarManejadorDeSesionRechazada((mensaje) => {
      clearToken()
      setUser(null)
      // errorDeSesion se limpia a propósito: eso es "no pude verificar la
      // sesión, reintentá", y acá el backend sí contestó.
      setErrorDeSesion(null)
      // El motivo se muestra solo si la persona ESTABA usando el sistema y la
      // sacaron: ahí sí necesita saber por qué desapareció lo que estaba
      // haciendo. Volver después de un rato y encontrar el token vencido no
      // es un problema que haya que explicarle a nadie —es lo que pasa
      // siempre—, y un cartel rojo de "token inválido o sesión expirada" al
      // abrir la aplicación en el teléfono se lee como un error del sistema.
      // Ahí va directo a la pantalla de ingreso, sin decir nada.
      if (habiaSesionAbierta.current) setMotivoDeCierre(mensaje)
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
      habiaSesionAbierta.current = true
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

  const login = useCallback(
    async (email: string, password: string) =>
      abrirSesion(await authApi.login({ email, password })),
    [abrirSesion]
  )

  const loginConGoogle = useCallback(
    async (credential: string) => abrirSesion(await authApi.loginConGoogle(credential)),
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
