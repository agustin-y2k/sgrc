import { useEffect, useId, useState } from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { Link, useLocation, useNavigate } from "react-router"
import { z } from "zod"

import { PantallaDeAcceso } from "@/components/layout/PantallaDeAcceso"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
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
import { Label } from "@/components/ui/label"
import { InputPassword } from "@/components/ui/input-password"
import * as authApi from "@/features/auth/api"
import { useAuth } from "@/features/auth/AuthContext"
import { BotonGoogle } from "@/features/auth/BotonGoogle"
import { ApiError, getErrorMessage } from "@/lib/api-client"

const loginSchema = z.object({
  email: z.string().email("Ingresá un email válido"),
  password: z.string().min(1, "Ingresá tu contraseña"),
  // Cuál de las dos casillas de "mantener la sesión iniciada" está marcada:
  // la del formulario de contraseña, la del botón de Google, o ninguna.
  //
  // Es UN campo con tres valores y no dos booleanos, y eso es lo que hace
  // que la regla "marcar una desmarca la otra" no se pueda romper: no hay
  // dos estados que puedan quedar desincronizados, hay uno solo. Sin
  // validación, porque desde la pantalla no se puede dejar en un valor que
  // no sea alguno de estos tres.
  recordarme: z.enum(["ninguna", "password", "google"]),
})

type LoginValues = z.infer<typeof loginSchema>
type Recordarme = LoginValues["recordarme"]

/**
 * Una de las dos casillas de "mantener la sesión iniciada": la que va con el
 * formulario de contraseña y la que va con el botón de Google.
 *
 * Son dos y no una porque con una sola —arriba del botón de contraseña, con
 * el de Google abajo del separador— no hay forma de saber si vale para los
 * dos caminos o solo para el de arriba. Cada una está pegada a su botón.
 *
 * Marcar una desmarca la otra y eso no lo hace ningún código: las dos leen y
 * escriben el MISMO campo del formulario, así que la exclusión es la forma
 * del dato, no una regla que alguien tenga que mantener.
 *
 * El precio de esa exclusión: la casilla vale para el botón que tiene al
 * lado y no para el otro. Quien marque esta y termine entrando por el otro
 * camino recibe la sesión corta. Por eso el aviso se repite en las dos —
 * quien baja derecho a Google no leyó el de arriba— y por eso ninguna está
 * suelta en el medio de la pantalla.
 */
function CasillaDeSesion({
  via,
  valor,
  alCambiar,
}: {
  via: Exclude<Recordarme, "ninguna">
  valor: Recordarme
  alCambiar: (valor: Recordarme) => void
}) {
  // useId y no un id fijo: el <label> tiene que apuntar a SU casilla, y en
  // la pantalla hay dos.
  const id = useId()

  return (
    <div className="flex items-start gap-2">
      <Checkbox
        id={id}
        className="mt-1"
        checked={valor === via}
        onCheckedChange={(v) => alCambiar(v === true ? via : "ninguna")}
      />
      <div className="grid gap-0.5">
        {/* "con Google" se ve, no es solo para lectores de pantalla. Dos
            casillas con el mismo rótulo son dos casillas idénticas: quien
            usa un lector escucha "mantener la sesión iniciada" dos veces sin
            saber en qué se diferencian, y quien mira la pantalla lee dos
            veces lo mismo y concluye que da igual cuál marcar —que es
            exactamente lo contrario de lo que pasa—. La de arriba no lleva
            aclaración porque no la necesita: está pegada al formulario de
            contraseña y arriba de su botón. */}
        <Label htmlFor={id} className="font-normal">
          Mantener la sesión iniciada{via === "google" ? " con Google" : ""}
        </Label>
        {/* La advertencia no es decorativa: el token queda guardado en ESE
            navegador hasta que venza, y la única forma de cortarlo a
            distancia es cambiar la contraseña de la cuenta. En una máquina
            del laboratorio eso es dejarle la sesión abierta al siguiente que
            se siente. */}
        <p className="text-muted-foreground text-xs">
          Solo en tu computadora o tu teléfono. No la marques en una máquina compartida de
          la escuela.
        </p>
      </div>
    </div>
  )
}

export function LoginPage() {
  const { login, loginConGoogle, motivoDeCierre } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [error, setError] = useState<string | null>(null)

  // Mensaje que deja la pantalla de recuperación al terminar ("ya podés
  // entrar con tu contraseña nueva").
  const aviso = (location.state as { aviso?: string } | null)?.aviso

  // El enlace de "olvidé mi contraseña" solo aparece si el despliegue puede
  // mandar mails: sin SMTP el backend responde 503 y la pantalla llevaría a
  // un callejón sin salida.
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
    defaultValues: { email: "", password: "", recordarme: "ninguna" },
  })

  // Las dos casillas salen de acá: `watch` para que la pantalla se redibuje
  // cuando cambia, `setValue` para escribir. Marcar una desmarca la otra sin
  // ningún código que lo haga, porque el valor es uno solo.
  const recordarme = form.watch("recordarme")
  const cambiarRecordarme = (valor: Recordarme) => form.setValue("recordarme", valor)

  async function onSubmit(values: LoginValues) {
    setError(null)
    try {
      const { debeCambiarPassword } = await login(
        values.email,
        values.password,
        values.recordarme === "password"
      )
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
  // tiene cuenta en el sistema.
  async function onCredencialDeGoogle(credencial: string) {
    setError(null)
    try {
      // La casilla que manda acá es la de Google, no la de arriba: cada una
      // vale para el botón que tiene al lado.
      const { debeCambiarPassword } = await loginConGoogle(
        credencial,
        form.getValues("recordarme") === "google"
      )
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
                      <InputPassword autoComplete="current-password" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <CasillaDeSesion
                via="password"
                valor={recordarme}
                alCambiar={cambiarRecordarme}
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
              <BotonGoogle onCredential={onCredencialDeGoogle}>
                <CasillaDeSesion
                  via="google"
                  valor={recordarme}
                  alCambiar={cambiarRecordarme}
                />
              </BotonGoogle>
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
