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
import * as authApi from "@/features/auth/api"
import {
  DeclaracionDeCargo,
  camposDeclarados,
  loQueDictaEseCargo,
} from "@/features/auth/DeclaracionDeCargo"
import { getErrorMessage } from "@/lib/api-client"
import { datosDeLaCredencial } from "@/lib/google-identity"

/**
 * El segundo paso del registro con Google: lo único que el token de Google no
 * puede traer. Lo declarado es el mismo bloque que el registro con
 * contraseña (camposDeclarados), no una copia.
 */
const registroGoogleSchema = z.object({
  nombre: z.string().min(1, "Requerido").max(100),
  apellido: z.string().min(1, "Requerido").max(100),
  ...camposDeclarados,
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
      // El cargo y el rol arrancan sin elegir: son obligatorios y ninguna de
      // las dos opciones es el caso "normal" del que partir.
    },
  })

  async function onSubmit(values: RegistroGoogleValues) {
    setError(null)
    try {
      await authApi.registrarConGoogle({
        credential: credencial,
        nombre: values.nombre,
        apellido: values.apellido,
        cargoSolicitado: values.cargoSolicitado,
        rolSolicitado: values.rolSolicitado,
        ...loQueDictaEseCargo(values),
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
              Vas a entrar con <span className="font-medium">{datos.email}</span>. Faltan
              unos datos para que un Admin pueda aprobarte.
            </>
          ) : (
            "Faltan unos datos para que un Admin pueda aprobar tu cuenta."
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
            <DeclaracionDeCargo idPrefijo="registro-google" />

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
