import { useEffect, useState } from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { Link, useLocation, useNavigate } from "react-router"
import { z } from "zod"

import { PantallaDeAcceso } from "@/components/layout/PantallaDeAcceso"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import * as authApi from "@/features/auth/api"
import { useAuth } from "@/features/auth/AuthContext"
import { BotonGoogle } from "@/features/auth/BotonGoogle"
import { ApiError, getErrorMessage } from "@/lib/api-client"

const loginSchema = z.object({
  email: z.string().email("Ingresá un email válido"),
  password: z.string().min(1, "Ingresá tu contraseña"),
})

type LoginValues = z.infer<typeof loginSchema>

export function LoginPage() {
  const { login, loginConGoogle, motivoDeCierre } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [error, setError] = useState<string | null>(null)

  // Mensaje que deja la pantalla de recuperación al terminar ("ya podés
  // entrar con tu contraseña nueva"). Va en el state de la navegación y no
  // en la URL para que no quede en el historial ni se muestre de nuevo al
  // recargar.
  const aviso = (location.state as { aviso?: string } | null)?.aviso

  // El enlace de "olvidé mi contraseña" solo aparece si el despliegue puede
  // mandar mails: sin SMTP el backend responde 503 y la pantalla llevaría a
  // un callejón sin salida. Mismo criterio que el botón de Google.
  //
  // Mientras la consulta no vuelve no se dibuja: es un parpadeo de menos
  // que mostrarlo y esconderlo. Si falla, tampoco — que no aparezca un
  // enlace es mejor que ofrecer algo que no funciona.
  const [recuperacionDisponible, setRecuperacionDisponible] = useState(false)
  useEffect(() => {
    let cancelado = false
    authApi
      .configPublica()
      .then(({ recuperacionPorEmail }) => {
        if (!cancelado) setRecuperacionDisponible(Boolean(recuperacionPorEmail))
      })
      .catch(() => {})
    return () => {
      cancelado = true
    }
  }, [])

  // <ProtectedRoute> guarda acá la ruta que el usuario quiso abrir sin
  // sesión, para devolverlo ahí después de loguearse en vez de dejarlo
  // siempre en el home.
  const destinoOriginal = (location.state as { from?: { pathname?: string } } | null)
    ?.from?.pathname

  const form = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { email: "", password: "" },
  })

  async function onSubmit(values: LoginValues) {
    setError(null)
    try {
      const { debeCambiarPassword } = await login(values.email, values.password)
      // El cambio de contraseña forzado (RF-01.6) gana sobre el destino
      // original: hasta que no la cambie no puede operar nada más.
      navigate(debeCambiarPassword ? "/cambiar-password" : (destinoOriginal ?? "/"), {
        replace: true,
      })
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  // Google devolvió un token válido, pero eso todavía no dice si la persona
  // tiene cuenta en el sistema. El 404 es la respuesta normal la primera
  // vez: significa "el token está bien, la cuenta no existe", y de ahí se
  // sigue al registro llevando el mismo token, para no hacer apretar el
  // botón de Google dos veces.
  async function onCredencialDeGoogle(credencial: string) {
    setError(null)
    try {
      const { debeCambiarPassword } = await loginConGoogle(credencial)
      navigate(debeCambiarPassword ? "/cambiar-password" : (destinoOriginal ?? "/"), {
        replace: true,
      })
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        navigate("/registro", { state: { credencialDeGoogle: credencial } })
        return
      }
      setError(getErrorMessage(err))
    }
  }

  return (
    <PantallaDeAcceso>
      <Card>
        <CardHeader>
          <CardTitle>Iniciar sesión</CardTitle>
          <CardDescription>Con el email que registraste en la escuela.</CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            {/* noValidate: sin esto, la validación nativa de type="email"
                bloquea el evento submit antes de que react-hook-form/zod lo
                vean, y nuestro mensaje nunca se muestra. */}
            <form
              noValidate
              onSubmit={form.handleSubmit(onSubmit)}
              className="grid gap-4"
            >
              {aviso && !error && (
                <Alert>
                  <AlertDescription>{aviso}</AlertDescription>
                </Alert>
              )}
              {/* El backend cerró la sesión (cuenta dada de baja, o cambio
                  de contraseña que invalidó los tokens abiertos). Sin este
                  cartel, la persona aparece en el login sin ninguna
                  explicación de por qué la echaron. */}
              {motivoDeCierre && !error && (
                <Alert>
                  <AlertDescription>{motivoDeCierre}</AlertDescription>
                </Alert>
              )}
              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              <FormField
                control={form.control}
                name="email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Email</FormLabel>
                    <FormControl>
                      <Input type="email" autoComplete="email" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="password"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Contraseña</FormLabel>
                    <FormControl>
                      <Input type="password" autoComplete="current-password" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Button
                type="submit"
                className="w-full"
                disabled={form.formState.isSubmitting}
              >
                Iniciar sesión
              </Button>
              {/* Va dentro del form pero después del botón: si el
                  despliegue no tiene Google configurado, no se dibuja nada
                  y el formulario queda exactamente como antes. */}
              <BotonGoogle onCredential={onCredencialDeGoogle} />
              {recuperacionDisponible && (
                <p className="text-muted-foreground text-center text-sm">
                  <Link to="/recuperar-password" className="text-primary underline">
                    Olvidé mi contraseña
                  </Link>
                </p>
              )}
              <p className="text-muted-foreground text-center text-sm">
                ¿No tenés cuenta?{" "}
                <Link to="/registro" className="text-primary underline">
                  Registrate
                </Link>
              </p>
            </form>
          </Form>
        </CardContent>
      </Card>
    </PantallaDeAcceso>
  )
}
