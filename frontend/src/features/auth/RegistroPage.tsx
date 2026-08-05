import { useState } from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { Link } from "react-router"
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
import { getErrorMessage } from "@/lib/api-client"

// Espeja RegistroRequest de internal/auth/interfaces/http/dto.go / docs/08-api-spec.yaml.
const registroSchema = z.object({
  nombre: z.string().min(1, "Requerido").max(100),
  apellido: z.string().min(1, "Requerido").max(100),
  email: z.string().email("Ingresá un email válido"),
  password: z.string().min(8, "Mínimo 8 caracteres"),
  // RF-01.3 + RF-02.6: texto libre y opcional. Al registrarse la persona
  // todavía no está autenticada, así que no puede elegir de una lista — y
  // lo que va a dictar puede no existir todavía en el sistema.
  cursoSolicitado: z.string().max(100).optional(),
  materiaSolicitada: z.string().max(100).optional(),
})

type RegistroValues = z.infer<typeof registroSchema>

export function RegistroPage() {
  const [error, setError] = useState<string | null>(null)
  const [enviado, setEnviado] = useState(false)

  const form = useForm<RegistroValues>({
    resolver: zodResolver(registroSchema),
    defaultValues: {
      nombre: "",
      apellido: "",
      email: "",
      password: "",
      cursoSolicitado: "",
      materiaSolicitada: "",
    },
  })

  async function onSubmit(values: RegistroValues) {
    setError(null)
    try {
      await authApi.registrar({
        ...values,
        // Vacío se manda como ausente: "no lo declaró" y "lo dejó en blanco"
        // no son dos cosas distintas.
        cursoSolicitado: values.cursoSolicitado?.trim() || undefined,
        materiaSolicitada: values.materiaSolicitada?.trim() || undefined,
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
              sesión una vez que te aprueben.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button asChild className="w-full">
              <Link to="/login">Volver al login</Link>
            </Button>
          </CardContent>
        </Card>
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
            Para docentes. Un Admin tiene que aprobar la cuenta antes del primer ingreso.
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
                      <Input type="password" autoComplete="new-password" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <div className="border-t pt-4">
                <p className="mb-1 text-sm font-medium">¿Qué vas a dictar?</p>
                <p className="text-muted-foreground mb-3 text-xs">
                  Opcional, pero ayuda: es lo que el Admin va a mirar para asignarte a la
                  materia correcta al aprobar tu cuenta. Si el curso o la materia todavía
                  no existen, los crea.
                </p>
                <div className="grid gap-4">
                  <FormField
                    control={form.control}
                    name="cursoSolicitado"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Curso</FormLabel>
                        <FormControl>
                          <Input placeholder="Ej.: 5°A" {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name="materiaSolicitada"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Materia</FormLabel>
                        <FormControl>
                          <Input placeholder="Ej.: Programación" {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>

              <Button type="submit" disabled={form.formState.isSubmitting}>
                Crear cuenta
              </Button>
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
