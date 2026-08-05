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
import type { Usuario } from "@/features/auth/types"
import { ApiError } from "@/lib/api-client"
import { clearToken, getToken, setToken } from "@/lib/token-store"

type AuthContextValue = {
  user: Usuario | null
  isLoading: boolean
  /**
   * Se llena solo cuando la sesión no se pudo verificar por una falla de
   * red/servidor (no por un token inválido). El token sigue guardado y la
   * operación se puede reintentar — ver <ProtectedRoute>.
   */
  errorDeSesion: string | null
  login: (email: string, password: string) => Promise<{ debeCambiarPassword: boolean }>
  logout: () => void
  refetchUser: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<Usuario | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [errorDeSesion, setErrorDeSesion] = useState<string | null>(null)

  // Al bootear, si hay un token guardado, valida que siga siendo válido
  // contra el backend (no hay lista de revocación local — GET /me es la
  // única forma de confirmarlo) e hidrata el usuario completo.
  //
  // Distinguir 401/403 de una falla de red es importante: un token vencido
  // SÍ tiene que cerrar la sesión, pero un backend momentáneamente caído no
  // — si borráramos el token ahí, un blip de red desloguearía al usuario y
  // lo obligaría a escribir la contraseña de nuevo sin ninguna explicación.
  const loadUser = useCallback(async () => {
    if (!getToken()) {
      setUser(null)
      setErrorDeSesion(null)
      setIsLoading(false)
      return
    }
    try {
      setUser(await authApi.me())
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

  const login = useCallback(
    async (email: string, password: string) => {
      const res = await authApi.login({ email, password })
      if (!res.token) {
        // El backend nunca devuelve 200 sin token — Login() en
        // internal/auth/application/service.go rechaza con 403 antes de
        // firmar si la cuenta no está APROBADA. Si esto dispara, algo
        // cambió del lado del backend sin avisar acá.
        throw new Error("login sin token en la respuesta")
      }
      setToken(res.token)
      // Si loadUser falla acá, el error se propaga hasta LoginPage y se le
      // muestra al usuario — antes el login parecía exitoso pero lo dejaba
      // de vuelta en /login sin decir por qué.
      await loadUser()
      return { debeCambiarPassword: res.debeCambiarPassword }
    },
    [loadUser]
  )

  const logout = useCallback(() => {
    clearToken()
    setUser(null)
    setErrorDeSesion(null)
  }, [])

  const value = useMemo(
    () => ({ user, isLoading, errorDeSesion, login, logout, refetchUser: loadUser }),
    [user, isLoading, errorDeSesion, login, logout, loadUser]
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
