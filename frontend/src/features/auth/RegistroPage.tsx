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
import { Select } from "@/components/ui/select"
import * as authApi from "@/features/auth/api"
import { BotonGoogle } from "@/features/auth/BotonGoogle"
import { RegistroConGoogle } from "@/features/auth/RegistroConGoogle"
import { SelectorDeCursoSolicitado } from "@/features/auth/SelectorDeCursoSolicitado"
import { getErrorMessage } from "@/lib/api-client"

// Espeja RegistroRequest de internal/auth/interfaces/http/dto.go / docs/08-api-spec.yaml.
const registroSchema = z.object({
  nombre: z.string().min(1, "Requerido").max(100),
  apellido: z.string().min(1, "Requerido").max(100),
  email: z.string().email("Ingresá un email válido"),
  password: z.string().min(8, "Mínimo 8 caracteres"),
  // RF-01.3 + RF-02.6: los dos opcionales, y los dos siguen siendo texto
  // libre en el contrato — el curso puede no existir todavía en el sistema,
  // y quien se registra no está autenticado, así que no hay lista que
  // consultar. Lo que cambia es cómo se completan: el curso se arma con dos
  // desplegables (ver SelectorDeCursoSolicitado) para que llegue con el
  // nombre canónico "5°A" en vez de las cinco formas de escribirlo a mano,
  // y la materia sigue siendo un campo abierto, igual que cuando el Admin
  // crea una (MateriasDeCurso).
  cursoSolicitado: z.string().max(100).optional(),
  materiaSolicitada: z.string().max(100).optional(),
  // El rol sí tiene lista cerrada —es la misma de DocenteMateria— así que
  // es un desplegable y el backend lo valida. Vacío significa "no lo dijo".
  rolSolicitado: z.enum(["TITULAR", "SUPLENTE"]).or(z.literal("")).optional(),
})

type RegistroValues = z.infer<typeof registroSchema>

export function RegistroPage() {
  const [error, setError] = useState<string | null>(null)
  const [enviado, setEnviado] = useState(false)
  const location = useLocation()

  // La credencial puede llegar de dos lados: de la pantalla de login
  // (alguien apretó "Iniciar sesión con Google" y todavía no tenía cuenta,
  // ver LoginPage) o del botón de esta misma pantalla. En los dos casos el
  // token ya está en la mano, así que no hay que volver a pedírselo a
  // Google.
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
      rolSolicitado: "",
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
        rolSolicitado: values.rolSolicitado || undefined,
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
                        <FormControl>
                          <SelectorDeCursoSolicitado
                            idPrefijo="registro-curso"
                            value={field.value ?? ""}
                            onChange={field.onChange}
                          />
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
                  <FormField
                    control={form.control}
                    name="rolSolicitado"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Rol</FormLabel>
                        <FormControl>
                          <Select {...field} value={field.value ?? ""}>
                            <option value="">No lo sé todavía</option>
                            <option value="TITULAR">Titular</option>
                            <option value="SUPLENTE">Suplente</option>
                          </Select>
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
