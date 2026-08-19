import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "react-router"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import * as authApi from "@/features/auth/api"
import type { Estado } from "@/features/auth/types"
import { getErrorMessage } from "@/lib/api-client"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"

const PENDIENTES_QUERY_KEY = ["usuarios", "PENDIENTE"]

// Admin: aprobación de cuentas PENDIENTE (RF-01.3).
export function AprobacionPage() {
  const queryClient = useQueryClient()
  // Id de la cuenta cuyo rechazo está esperando confirmación.
  const [confirmandoRechazo, setConfirmandoRechazo] = useState<string | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: PENDIENTES_QUERY_KEY,
    queryFn: () => authApi.listarUsuarios({ estado: "PENDIENTE" }),
  })

  const cambiarEstado = useMutation({
    mutationFn: ({ id, estado }: { id: string; estado: Estado }) =>
      authApi.cambiarEstado(id, { estado: estado as "APROBADA" | "RECHAZADA" }),
    onSuccess: () => {
      setConfirmandoRechazo(null)
      return queryClient.invalidateQueries({ queryKey: PENDIENTES_QUERY_KEY })
    },
  })

  const pendientes = data?.data ?? []

  return (
    <div className="mx-auto max-w-2xl">
      <EncabezadoDePagina
        titulo="Cuentas pendientes"
        descripcion="Docentes que se registraron y esperan aprobación. Al aprobar, asignales el curso y la materia que pidieron."
      />

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}
      {cambiarEstado.error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(cambiarEstado.error)}</AlertDescription>
        </Alert>
      )}

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}

      {!isLoading && pendientes.length === 0 && (
        <p className="text-muted-foreground">No hay cuentas pendientes.</p>
      )}

      <div className="grid gap-3">
        {pendientes.map((u) => {
          const enCurso = cambiarEstado.isPending && cambiarEstado.variables?.id === u.id
          const pidiendoConfirmacion = confirmandoRechazo === u.id

          return (
            <Card key={u.id}>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  {u.nombre} {u.apellido}
                  <Badge variant="default">{u.rol}</Badge>
                </CardTitle>
                <CardDescription>{u.email}</CardDescription>
              </CardHeader>
              <CardContent>
                {/* RF-01.3: lo que declaró al registrarse. Es lo que evita
                    tener que preguntarle por fuera del sistema a qué materia
                    y curso corresponde asignarlo (RF-02.6) — y si no existen
                    todavía, que el Admin sepa que los tiene que crear. */}
                {(u.cursoSolicitado || u.materiaSolicitada || u.rolSolicitado) && (
                  <div className="bg-muted/40 mb-3 rounded-md border p-3 text-sm">
                    <p className="mb-1 font-medium">Pidió dictar</p>
                    <p>
                      {u.materiaSolicitada || "—"}
                      {u.cursoSolicitado ? ` · ${u.cursoSolicitado}` : ""}
                      {u.rolSolicitado
                        ? ` · como ${u.rolSolicitado === "TITULAR" ? "titular" : "suplente"}`
                        : ""}
                    </p>
                    <p className="text-muted-foreground mt-1 text-xs">
                      Es lo que escribió al registrarse, no una referencia: puede que el
                      curso o la materia todavía no existan. Se asigna desde{" "}
                      <Link to="/admin/academico" className="underline">
                        Académico
                      </Link>{" "}
                      después de aprobarlo.
                    </p>
                  </div>
                )}
                {/* Rechazar no se deshace: RECHAZADA es un estado terminal y
                    no transiciona a ningún lado (ver PuedeTransicionarA en
                    internal/auth/domain/usuario.go). Lo que sí se puede es
                    eliminar la cuenta desde el panel de usuarios (RF-01.9), y
                    la confirmación lo dice: sin esa salida, un rechazo por
                    error dejaba el email tomado para siempre. */}
                {pidiendoConfirmacion ? (
                  <div className="grid gap-3">
                    <p className="text-destructive text-sm">
                      Rechazar a {u.nombre} {u.apellido} no se puede deshacer: la cuenta
                      no se reactiva, y con ella rechazada esa persona no va a poder
                      volver a registrarse con {u.email}. Si fue un error, eliminá la
                      cuenta desde{" "}
                      <Link to="/admin/usuarios" className="underline">
                        Usuarios
                      </Link>{" "}
                      para liberar el email.
                    </p>
                    <div className="flex gap-2">
                      <Button
                        variant="destructive"
                        disabled={enCurso}
                        onClick={() =>
                          cambiarEstado.mutate({ id: u.id, estado: "RECHAZADA" })
                        }
                      >
                        Confirmar rechazo
                      </Button>
                      <Button
                        variant="outline"
                        disabled={enCurso}
                        onClick={() => setConfirmandoRechazo(null)}
                      >
                        Cancelar
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="flex gap-2">
                    <Button
                      disabled={enCurso}
                      onClick={() =>
                        cambiarEstado.mutate({ id: u.id, estado: "APROBADA" })
                      }
                    >
                      Aprobar
                    </Button>
                    <Button
                      variant="destructive"
                      disabled={enCurso}
                      onClick={() => setConfirmandoRechazo(u.id)}
                    >
                      Rechazar
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          )
        })}
      </div>
    </div>
  )
}
