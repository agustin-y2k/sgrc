import { useState } from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { useNavigate } from "react-router"
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
import { useAuth } from "@/features/auth/AuthContext"
import { getErrorMessage } from "@/lib/api-client"
import { setToken } from "@/lib/token-store"

const cambiarPasswordSchema = z.object({
  passwordActual: z.string().min(1, "Requerido"),
  passwordNueva: z.string().min(8, "Mínimo 8 caracteres"),
})

type CambiarPasswordValues = z.infer<typeof cambiarPasswordSchema>

// Cubre tanto el cambio forzado tras un reset de Admin (RF-01.6, redirigido
// acá por <ProtectedRoute>) como el cambio voluntario en cualquier momento
// (RF-01.7) — mismo formulario, mismo endpoint.
export function CambiarPasswordPage() {
  const { refetchUser } = useAuth()
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)

  const form = useForm<CambiarPasswordValues>({
    resolver: zodResolver(cambiarPasswordSchema),
    defaultValues: { passwordActual: "", passwordNueva: "" },
  })

  async function onSubmit(values: CambiarPasswordValues) {
    setError(null)
    try {
      // El token nuevo va primero: el viejo lleva debeCambiarPassword=true
      // en los claims y el backend rechaza con 403 todo lo que no sea /me o
      // este endpoint, así que el refetchUser de abajo fallaría con el
      // anterior (RF-01.6).
      const { token } = await authApi.cambiarPassword(values)
      setToken(token)
      await refetchUser()
      navigate("/", { replace: true })
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  return (
    <div className="flex justify-center p-4 pt-12">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Cambiar contraseña</CardTitle>
          <CardDescription>
            Ingresá tu contraseña actual y la nueva contraseña.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="grid gap-4">
              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              <FormField
                control={form.control}
                name="passwordActual"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Contraseña actual</FormLabel>
                    <FormControl>
                      <Input type="password" autoComplete="current-password" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="passwordNueva"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Contraseña nueva</FormLabel>
                    <FormControl>
                      <Input type="password" autoComplete="new-password" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Button type="submit" disabled={form.formState.isSubmitting}>
                Cambiar contraseña
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  )
}
