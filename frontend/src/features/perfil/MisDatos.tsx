import { zodResolver } from "@hookform/resolvers/zod"
import { Pencil } from "lucide-react"
import { useState } from "react"
import { useForm } from "react-hook-form"
import { z } from "zod"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import * as perfilApi from "@/features/perfil/api"
import { getErrorMessage } from "@/lib/api-client"
import { setToken } from "@/lib/token-store"

// El mismo largo que el registro y que la columna VARCHAR(100) — ver
// domain.LargoMaxNombre en internal/auth/domain/usuario.go.
const misDatosSchema = z.object({
  nombre: z.string().min(1, "Requerido").max(100),
  apellido: z.string().min(1, "Requerido").max(100),
})

type MisDatosValues = z.infer<typeof misDatosSchema>

/**
 * El nombre con el que figurás en todo el sistema, con un botón para
 * cambiarlo.
 *
 * Hasta ahora el nombre se escribía una sola vez, al registrarse, y quien lo
 * había tipeado mal —o se había casado, o usaba su segundo nombre— tenía que
 * pedírselo a un Admin que tampoco tenía cómo cambiarlo. Es autoservicio y no
 * pasa por ninguna aprobación, igual que la foto.
 */
export function MisDatos({ usuario }: { usuario: Usuario }) {
  const { refetchUser } = useAuth()
  const [editando, setEditando] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const form = useForm<MisDatosValues>({
    resolver: zodResolver(misDatosSchema),
    defaultValues: { nombre: usuario.nombre, apellido: usuario.apellido },
  })

  function empezarAEditar() {
    // El formulario se resetea al abrirse y no al cerrarse: si quedó a medio
    // escribir de la vez anterior, lo que se ve al volver a abrirlo es el
    // nombre que está guardado.
    form.reset({ nombre: usuario.nombre, apellido: usuario.apellido })
    setError(null)
    setEditando(true)
  }

  async function onSubmit(values: MisDatosValues) {
    setError(null)
    try {
      const { token } = await perfilApi.actualizarMisDatos(values)
      // El token viejo dice el nombre viejo en los claims. Hoy ningún endpoint
      // lo lee, pero se reemplaza igual para que no queden dos versiones de
      // cómo se llama la persona dando vueltas.
      setToken(token)
      // refetchUser relee /me: con eso se actualizan solos el saludo del
      // inicio y el nombre del menú de arriba.
      await refetchUser()
      setEditando(false)
    } catch (err) {
      setError(getErrorMessage(err))
    }
  }

  if (!editando) {
    return (
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <p className="text-lg font-medium">
            {usuario.nombre} {usuario.apellido}
          </p>
          <p className="text-muted-foreground text-sm">{usuario.email}</p>
          <p className="text-muted-foreground text-sm">
            {usuario.rol === "ADMIN" ? "Administración" : "Docente"}
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-11 px-4 sm:h-9"
          onClick={empezarAEditar}
        >
          <Pencil className="mr-2 size-4" aria-hidden="true" />
          Cambiar mi nombre
        </Button>
      </div>
    )
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="grid gap-4">
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {/* Apilados en un teléfono, igual que en el registro. */}
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

        <p className="text-muted-foreground text-sm">
          Es con el que te ven el resto de los docentes y el equipo de administración en
          las reservas y en las entregas.
        </p>

        <div className="flex flex-wrap gap-2">
          <Button
            type="submit"
            size="sm"
            className="h-11 px-4 sm:h-9"
            disabled={form.formState.isSubmitting}
          >
            {form.formState.isSubmitting ? "Guardando…" : "Guardar"}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-11 px-4 sm:h-9"
            disabled={form.formState.isSubmitting}
            onClick={() => setEditando(false)}
          >
            Cancelar
          </Button>
        </div>
      </form>
    </Form>
  )
}
