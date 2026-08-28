import { useState } from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { Link, useNavigate } from "react-router"
import { z } from "zod"

import { PantallaDeAcceso } from "@/components/layout/PantallaDeAcceso"
import { AvisoDeSpam } from "@/components/AvisoDeSpam"
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
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { InputPassword } from "@/components/ui/input-password"
import * as authApi from "@/features/auth/api"
import { getErrorMessage } from "@/lib/api-client"

const pedirSchema = z.object({
  email: z.string().email("Ingresá un email válido"),
})

const restablecerSchema = z
  .object({
    codigo: z
      .string()
      .trim()
      .regex(/^\d{6}$/, "El código son 6 números"),
    passwordNueva: z.string().min(8, "Mínimo 8 caracteres"),
    repetirPassword: z.string(),
  })
  // Repetir la contraseña solo tiene sentido acá: en el login te enterás de
  // que la tipeaste mal en el intento siguiente, pero acá la estás eligiendo
  // a ciegas y con un código que se consume.
  .refine((v) => v.passwordNueva === v.repetirPassword, {
    message: "Las contraseñas no coinciden",
    path: ["repetirPassword"],
  })

type PedirValues = z.infer<typeof pedirSchema>
type RestablecerValues = z.infer<typeof restablecerSchema>

/**
 * Recuperación de contraseña por autoservicio (RF-01.10), en dos pasos dentro
 * de la misma pantalla.
 */
export function RecuperarPasswordPage() {
  const navigate = useNavigate()
  const [emailEnviado, setEmailEnviado] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const formPedir = useForm<PedirValues>({
    resolver: zodResolver(pedirSchema),
    defaultValues: { email: "" },
  })

  const formRestablecer = useForm<RestablecerValues>({
    resolver: zodResolver(restablecerSchema),
    defaultValues: { codigo: "", passwordNueva: "", repetirPassword: "" },
  })

  async function onPedirCodigo(values: PedirValues) {
    setError(null)
    try {
      await authApi.olvidePassword({ email: values.email })
      // Se pasa al paso 2 SIEMPRE, exista o no la cuenta: el backend responde
      // igual en los dos casos a propósito, y una pantalla que dijera "ese
      // email no está registrado" tiraría abajo todo el cuidado que se puso
      // del otro lado.
      setEmailEnviado(values.email)
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  async function onRestablecer(values: RestablecerValues) {
    setError(null)
    try {
      await authApi.restablecerPassword({
        email: emailEnviado!,
        codigo: values.codigo.trim(),
        passwordNueva: values.passwordNueva,
      })
      navigate("/login", {
        replace: true,
        state: { aviso: "Listo. Ya podés entrar con tu contraseña nueva." },
      })
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  function volverAEmpezar() {
    setError(null)
    formRestablecer.reset()
    setEmailEnviado(null)
  }

  return (
    <PantallaDeAcceso>
      <Card>
        <CardHeader>
          <CardTitle>
            {emailEnviado ? "Elegí una contraseña nueva" : "Recuperar contraseña"}
          </CardTitle>
          <CardDescription>
            {emailEnviado
              ? `Si ${emailEnviado} corresponde a una cuenta habilitada, te llegó un código.`
              : "Te mandamos un código a tu email para que puedas elegir una contraseña nueva."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {error && (
            <Alert variant="destructive" className="mb-4">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          {!emailEnviado ? (
            <Form {...formPedir}>
              {/* noValidate por lo mismo que en el login: la validación
                  nativa de type="email" bloquea el submit antes de que zod
                  llegue a mostrar nuestro mensaje. */}
              <form
                noValidate
                onSubmit={formPedir.handleSubmit(onPedirCodigo)}
                className="grid gap-4"
              >
                <FormField
                  control={formPedir.control}
                  name="email"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Email</FormLabel>
                      <FormControl>
                        <Input type="email" autoComplete="email" autoFocus {...field} />
                      </FormControl>
                      <FormDescription>
                        El mismo con el que te registraste en la escuela.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <Button
                  type="submit"
                  className="w-full"
                  disabled={formPedir.formState.isSubmitting}
                >
                  Enviarme el código
                </Button>
                <p className="text-muted-foreground text-center text-sm">
                  <Link to="/login" className="text-primary underline">
                    Volver al inicio de sesión
                  </Link>
                </p>
              </form>
            </Form>
          ) : (
            <Form {...formRestablecer}>
              <form
                noValidate
                onSubmit={formRestablecer.handleSubmit(onRestablecer)}
                className="grid gap-4"
              >
                <FormField
                  control={formRestablecer.control}
                  name="codigo"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Código</FormLabel>
                      <FormControl>
                        {/* inputMode="numeric" hace que en el celular
                            aparezca el teclado numérico, que es donde se
                            lee el mail. autoComplete="one-time-code" deja
                            que el sistema lo ofrezca solo. */}
                        <Input
                          inputMode="numeric"
                          autoComplete="one-time-code"
                          maxLength={6}
                          autoFocus
                          placeholder="123456"
                          className="text-center text-lg tracking-[0.4em]"
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        Los 6 números que te llegaron por mail. Vencen a los 15 minutos.
                      </FormDescription>
                      {/* El caso más urgente de los tres: el código vence a
                          los 15 minutos, así que buscarlo en spam es contra
                          reloj. */}
                      <AvisoDeSpam>
                        Si no lo ves, fijate en la carpeta de spam.
                      </AvisoDeSpam>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={formRestablecer.control}
                  name="passwordNueva"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Contraseña nueva</FormLabel>
                      <FormControl>
                        <InputPassword autoComplete="new-password" {...field} />
                      </FormControl>
                      <FormDescription>Mínimo 8 caracteres.</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={formRestablecer.control}
                  name="repetirPassword"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Repetir contraseña</FormLabel>
                      <FormControl>
                        <InputPassword autoComplete="new-password" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <Button
                  type="submit"
                  className="w-full"
                  disabled={formRestablecer.formState.isSubmitting}
                >
                  Cambiar mi contraseña
                </Button>
                {/* Salida para los dos casos que no son "me equivoqué al
                    tipear el código": escribí mal el email, o el código
                    venció mientras buscaba el mail. */}
                <Button type="button" variant="ghost" onClick={volverAEmpezar}>
                  Usar otro email o pedir un código nuevo
                </Button>
              </form>
            </Form>
          )}
        </CardContent>
      </Card>
    </PantallaDeAcceso>
  )
}
