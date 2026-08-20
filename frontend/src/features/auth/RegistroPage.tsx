import { useState } from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { Link, useLocation } from "react-router"
import { z } from "zod"

import { PantallaDeAcceso } from "@/components/layout/PantallaDeAcceso"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { AvisoDeSpam } from "@/components/AvisoDeSpam"
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
import { InputPassword } from "@/components/ui/input-password"
import * as authApi from "@/features/auth/api"
import { BotonGoogle } from "@/features/auth/BotonGoogle"
import {
  DeclaracionDeCargo,
  camposDeclarados,
  loQueDictaEseCargo,
} from "@/features/auth/DeclaracionDeCargo"
import { RegistroConGoogle } from "@/features/auth/RegistroConGoogle"
import { getErrorMessage } from "@/lib/api-client"

// Espeja RegistroRequest de internal/auth/interfaces/http/dto.go / docs/08-api-spec.yaml.
// Lo que se declara (cargo, rol, curso y materia) vive en camposDeclarados,
// compartido con el registro por Google.
const registroSchema = z.object({
  nombre: z.string().min(1, "Requerido").max(100),
  apellido: z.string().min(1, "Requerido").max(100),
  email: z.string().email("Ingresá un email válido"),
  password: z.string().min(8, "Mínimo 8 caracteres"),
  ...camposDeclarados,
})

type RegistroValues = z.infer<typeof registroSchema>

export function RegistroPage() {
  const [error, setError] = useState<string | null>(null)
  const [enviado, setEnviado] = useState(false)
  const location = useLocation()

  // La credencial puede llegar de dos lados: de la pantalla de login (alguien
  // apretó "Iniciar sesión con Google" y todavía no tenía cuenta, ver
  // LoginPage) o del botón de esta misma pantalla.
  const credencialDelLogin = (location.state as { credencialDeGoogle?: string } | null)
    ?.credencialDeGoogle
  const [credencialDeGoogle, setCredencialDeGoogle] = useState<string | null>(
    credencialDelLogin ?? null
  )

  const form = useForm<RegistroValues>({
    resolver: zodResolver(registroSchema),
    defaultValues: {
      nombre: "",
      apellido: "",
      email: "",
      password: "",
      cursoSolicitado: "",
      materiaSolicitada: "",
      // El cargo y el rol arrancan sin elegir a propósito: son obligatorios y
      // no hay ninguna opción que sea el caso "normal" del que partir.
    },
  })

  async function onSubmit(values: RegistroValues) {
    setError(null)
    try {
      await authApi.registrar({
        ...values,
        ...loQueDictaEseCargo(values),
      })
      setEnviado(true)
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  if (enviado) {
    return (
      <PantallaDeAcceso>
        <Card>
          <CardHeader>
            <CardTitle>Cuenta creada</CardTitle>
            <CardDescription>
              Tu cuenta quedó pendiente de aprobación de un Admin. Vas a poder iniciar
              sesión una vez que te aprueben, y te avisamos por correo cuando pase.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3">
            <Button asChild className="w-full">
              <Link to="/login">Volver al login</Link>
            </Button>
            {/* Este es el primer correo que esta persona recibe del sistema,
                o sea el de mayor riesgo de terminar en spam. Y es el mejor
                momento para pedirle que marque el remitente: le sirve para
                todos los que vengan después. */}
            <AvisoDeSpam>El aviso puede caer en spam.</AvisoDeSpam>
          </CardContent>
        </Card>
      </PantallaDeAcceso>
    )
  }

  if (credencialDeGoogle) {
    return (
      <PantallaDeAcceso ancho="max-w-md">
        <RegistroConGoogle
          credencial={credencialDeGoogle}
          onRegistrado={() => setEnviado(true)}
        />
      </PantallaDeAcceso>
    )
  }

  return (
    <PantallaDeAcceso ancho="max-w-md">
      <Card>
        <CardHeader>
          <CardTitle>Crear cuenta</CardTitle>
          {/* RF-01.3 */}
          <CardDescription>
            Para el personal de la escuela. Un Admin tiene que aprobar la cuenta antes
            del primer ingreso.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            {/* noValidate: ver comentario en LoginPage.tsx — sin esto, la
                validación nativa de type="email" bloquea el submit antes de
                que react-hook-form/zod lo vean. */}
            <form
              noValidate
              onSubmit={form.handleSubmit(onSubmit)}
              className="grid gap-4"
            >
              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              {/* Apilados en un teléfono: dos campos de texto a 375px dejan
                  espacio para unos pocos caracteres visibles cada uno. */}
              <div className="grid gap-4 sm:grid-cols-2">
                <FormField
                  control={form.control}
                  name="nombre"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Nombre</FormLabel>
                      <FormControl>
                        <Input autoComplete="given-name" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="apellido"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Apellido</FormLabel>
                      <FormControl>
                        <Input autoComplete="family-name" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
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
                      <InputPassword autoComplete="new-password" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <DeclaracionDeCargo idPrefijo="registro" />

              <Button type="submit" disabled={form.formState.isSubmitting}>
                Crear cuenta
              </Button>
              {/* Con Google no hace falta elegir contraseña: el token trae
                  el nombre y el email ya verificados, y solo queda pedir
                  qué va a dictar (RegistroConGoogle). */}
              <BotonGoogle texto="signup_with" onCredential={setCredencialDeGoogle} />
              <p className="text-muted-foreground text-center text-sm">
                ¿Ya tenés cuenta?{" "}
                <Link to="/login" className="text-primary underline">
                  Iniciá sesión
                </Link>
              </p>
            </form>
          </Form>
        </CardContent>
      </Card>
    </PantallaDeAcceso>
  )
}
