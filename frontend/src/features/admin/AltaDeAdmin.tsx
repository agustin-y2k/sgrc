import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import * as adminApi from "@/features/admin/api"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-01.4 — un Admin crea otro Admin, que queda APROBADA de inmediato.
 *
 * Sin esto la institución se quedaba con la única cuenta administrativa que
 * siembra el arranque (SEED_ADMIN_EMAIL): el autorregistro crea DOCENTE y no
 * hay ningún endpoint que cambie el rol de una cuenta existente, así que la
 * segunda persona que administre el sistema tendría que compartir la cuenta
 * de la primera.
 */

const MIN_PASSWORD = 8

export function AltaDeAdmin({ usuariosKey }: { usuariosKey: unknown[] }) {
  const queryClient = useQueryClient()
  const [abierto, setAbierto] = useState(false)
  const [nombre, setNombre] = useState("")
  const [apellido, setApellido] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [creado, setCreado] = useState<string | null>(null)

  const limpiar = () => {
    setNombre("")
    setApellido("")
    setEmail("")
    setPassword("")
  }

  const crear = useMutation({
    mutationFn: () =>
      adminApi.crearAdmin({
        nombre: nombre.trim(),
        apellido: apellido.trim(),
        email: email.trim(),
        password,
      }),
    onSuccess: async () => {
      setCreado(`${nombre.trim()} ${apellido.trim()}`)
      limpiar()
      await queryClient.invalidateQueries({ queryKey: usuariosKey })
    },
  })

  if (!abierto) {
    return (
      <Button variant="outline" onClick={() => setAbierto(true)}>
        Crear otro Admin
      </Button>
    )
  }

  const completo =
    nombre.trim() !== "" &&
    apellido.trim() !== "" &&
    email.trim() !== "" &&
    password.length >= MIN_PASSWORD

  return (
    <Card className="mb-4">
      <CardHeader>
        <CardTitle>Nuevo Admin</CardTitle>
      </CardHeader>
      <CardContent>
        {crear.error && (
          <Alert variant="destructive" className="mb-3">
            <AlertDescription>{getErrorMessage(crear.error)}</AlertDescription>
          </Alert>
        )}

        {creado && (
          <Alert className="mb-3">
            <AlertDescription>
              {creado} ya puede entrar con esa contraseña. No hay envío de mails: se la
              tenés que pasar vos.
            </AlertDescription>
          </Alert>
        )}

        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault()
            crear.mutate()
          }}
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="admin-nombre">Nombre</Label>
              <Input
                id="admin-nombre"
                value={nombre}
                onChange={(e) => setNombre(e.target.value)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="admin-apellido">Apellido</Label>
              <Input
                id="admin-apellido"
                value={apellido}
                onChange={(e) => setApellido(e.target.value)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="admin-email">Email</Label>
              <Input
                id="admin-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="admin-password">Contraseña inicial</Label>
              <Input
                id="admin-password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
          </div>

          {password !== "" && password.length < MIN_PASSWORD && (
            <p className="text-destructive text-sm">
              La contraseña necesita al menos {MIN_PASSWORD} caracteres.
            </p>
          )}

          {/* A diferencia del autorregistro (RF-01.3), esta cuenta no pasa
              por PENDIENTE: queda aprobada y puede entrar enseguida. */}
          <p className="text-muted-foreground text-sm">
            Queda aprobada de entrada, con permisos de Admin y sin pasar por la lista de
            pendientes.
          </p>

          <div className="flex gap-2">
            <Button type="submit" disabled={!completo || crear.isPending}>
              {crear.isPending ? "Creando…" : "Crear Admin"}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setAbierto(false)
                setCreado(null)
                limpiar()
              }}
            >
              Cerrar
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
