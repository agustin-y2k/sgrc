import { useState } from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { Link } from "react-router"
import { z } from "zod"

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
import { Select } from "@/components/ui/select"
import * as authApi from "@/features/auth/api"
import { SelectorDeCursoSolicitado } from "@/features/auth/SelectorDeCursoSolicitado"
import { getErrorMessage } from "@/lib/api-client"
import { datosDeLaCredencial } from "@/lib/google-identity"

/**
 * El segundo paso del registro con Google: lo único que el token de Google
 * no puede traer.
 *
 * Que exista este paso es el motivo por el que el ingreso con Google son
 * dos endpoints y no uno. Al aprobar una cuenta, el Admin necesita saber
 * qué curso y qué materia le corresponde (RF-01.3 / RF-02.6); sin eso
 * recibiría solicitudes sin ningún dato para saber a qué asignarlas.
 *
 * El email no se puede editar: viene firmado dentro del token y es lo
 * único que el backend va a mirar. Mostrarlo como un campo editable sería
 * ofrecer un cambio que no tiene ningún efecto.
 */
const registroGoogleSchema = z.object({
  nombre: z.string().min(1, "Requerido").max(100),
  apellido: z.string().min(1, "Requerido").max(100),
  cursoSolicitado: z.string().max(100).optional(),
  materiaSolicitada: z.string().max(100).optional(),
  rolSolicitado: z.enum(["TITULAR", "SUPLENTE"]).or(z.literal("")).optional(),
})

type RegistroGoogleValues = z.infer<typeof registroGoogleSchema>

export function RegistroConGoogle({
  credencial,
  onRegistrado,
}: {
  credencial: string
  onRegistrado: () => void
}) {
  const [error, setError] = useState<string | null>(null)

  // Lectura sin verificar, solo para prellenar: la verificación de verdad
  // la hace el backend contra las claves de Google. Ver datosDeLaCredencial.
  const datos = datosDeLaCredencial(credencial)

  const form = useForm<RegistroGoogleValues>({
    resolver: zodResolver(registroGoogleSchema),
    defaultValues: {
      nombre: datos?.nombre ?? "",
      apellido: datos?.apellido ?? "",
      cursoSolicitado: "",
      materiaSolicitada: "",
      rolSolicitado: "",
    },
  })

  async function onSubmit(values: RegistroGoogleValues) {
    setError(null)
    try {
      await authApi.registrarConGoogle({
        credential: credencial,
        nombre: values.nombre,
        apellido: values.apellido,
        // Vacío se manda como ausente, igual que en el registro con
        // contraseña: "no lo declaró" y "lo dejó en blanco" no son dos
        // cosas distintas.
        cursoSolicitado: values.cursoSolicitado?.trim() || undefined,
        materiaSolicitada: values.materiaSolicitada?.trim() || undefined,
        rolSolicitado: values.rolSolicitado || undefined,
      })
      onRegistrado()
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Completá tu registro</CardTitle>
        <CardDescription>
          {datos?.email ? (
            <>
              Vas a entrar con <span className="font-medium">{datos.email}</span>. Falta
              un dato para que un Admin pueda aprobarte.
            </>
          ) : (
            "Falta un dato para que un Admin pueda aprobar tu cuenta."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form noValidate onSubmit={form.handleSubmit(onSubmit)} className="grid gap-4">
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
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
            <div className="border-t pt-4">
              <p className="mb-1 text-sm font-medium">¿Qué vas a dictar?</p>
              <p className="text-muted-foreground mb-3 text-xs">
                Opcional, pero ayuda: es lo que el Admin va a mirar para asignarte a la
                materia correcta al aprobar tu cuenta. Si el curso o la materia todavía no
                existen, los crea.
              </p>
              <div className="grid gap-4">
                <FormField
                  control={form.control}
                  name="cursoSolicitado"
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <SelectorDeCursoSolicitado
                          idPrefijo="registro-google-curso"
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
  )
}
