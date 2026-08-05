import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { EstadoBadge, TONO_CUENTA } from "@/components/EstadoBadge"
import { Paginador } from "@/components/Paginador"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Select } from "@/components/ui/select"
import { useAuth } from "@/features/auth/AuthContext"
import type { Estado, Usuario } from "@/features/auth/types"
import { AltaDeAdmin } from "@/features/admin/AltaDeAdmin"
import * as adminApi from "@/features/admin/api"
import { getErrorMessage } from "@/lib/api-client"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"

const USUARIOS_KEY = ["admin", "usuarios"]

const ETIQUETA_ESTADO: Record<Estado, string> = {
  PENDIENTE: "Pendiente",
  APROBADA: "Aprobada",
  RECHAZADA: "Rechazada",
  BAJA: "Baja",
}

function EstadoDeCuenta({ estado }: { estado: Estado }) {
  return <EstadoBadge tono={TONO_CUENTA[estado]}>{ETIQUETA_ESTADO[estado]}</EstadoBadge>
}

type Confirmacion = {
  usuario: Usuario
  accion: "BAJA" | "ELIMINAR"
}

// RF-01/RF-02: panel de usuarios. Las acciones irreversibles (dar de baja,
// eliminar) piden confirmación explícita y explican la consecuencia — la
// baja es permanente (RF-02.9) y eliminar es un hard delete (RF-01.9).
export function UsuariosPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [filtroEstado, setFiltroEstado] = useState<Estado | "">("")
  const [pagina, setPagina] = useState(1)
  const [confirmando, setConfirmando] = useState<Confirmacion | null>(null)
  const [passwordTemporal, setPasswordTemporal] = useState<{
    usuario: string
    password: string
  } | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: [...USUARIOS_KEY, filtroEstado, pagina],
    queryFn: () =>
      adminApi.listarUsuarios({
        ...(filtroEstado ? { estado: filtroEstado } : {}),
        page: pagina,
      }),
  })

  const invalidar = () => queryClient.invalidateQueries({ queryKey: USUARIOS_KEY })

  const cambiarEstado = useMutation({
    mutationFn: ({ id, estado }: { id: string; estado: Estado }) =>
      adminApi.cambiarEstadoUsuario(id, estado),
    onSuccess: async () => {
      setConfirmando(null)
      await invalidar()
    },
  })

  const eliminar = useMutation({
    mutationFn: (id: string) => adminApi.eliminarUsuario(id),
    onSuccess: async () => {
      setConfirmando(null)
      await invalidar()
    },
  })

  const resetear = useMutation({
    mutationFn: (u: Usuario) => adminApi.resetearPassword(u.id),
    onSuccess: (res, u) => {
      // RF-01.6: no hay envío de mails, el Admin se la pasa a mano — así
      // que la temporal se muestra en pantalla una sola vez.
      setPasswordTemporal({
        usuario: `${u.nombre} ${u.apellido}`,
        password: res.passwordTemporal,
      })
    },
  })

  const usuarios = data?.data ?? []
  const errorActivo = error ?? cambiarEstado.error ?? eliminar.error ?? resetear.error

  return (
    <div className="mx-auto max-w-4xl">
      <EncabezadoDePagina
        titulo="Usuarios"
        descripcion="Todas las cuentas del sistema: estado, asignaciones y restablecimiento de contraseña."
        /* RF-01.4 */
        accion={<AltaDeAdmin usuariosKey={USUARIOS_KEY} />}
      />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <label className="text-sm" htmlFor="filtroEstado">
          Estado
        </label>
        <Select
          id="filtroEstado"
          className="w-auto"
          value={filtroEstado}
          onChange={(e) => {
            setFiltroEstado(e.target.value as Estado | "")
            // Cambiar el filtro cambia la colección: la página actual puede
            // caer más allá del final de la nueva.
            setPagina(1)
          }}
        >
          <option value="">Todos</option>
          <option value="PENDIENTE">Pendientes</option>
          <option value="APROBADA">Aprobadas</option>
          <option value="RECHAZADA">Rechazadas</option>
          <option value="BAJA">Dadas de baja</option>
        </Select>
      </div>

      {errorActivo && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(errorActivo)}</AlertDescription>
        </Alert>
      )}

      {passwordTemporal && (
        <Alert className="mb-4">
          <AlertDescription>
            Contraseña temporal de {passwordTemporal.usuario}:{" "}
            <code className="font-mono font-semibold">{passwordTemporal.password}</code>.
            Anotala y pasásela — no se vuelve a mostrar. Se le va a pedir que la cambie al
            entrar.{" "}
            <Button variant="outline" size="sm" onClick={() => setPasswordTemporal(null)}>
              Entendido
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}
      {!isLoading && usuarios.length === 0 && (
        <p className="text-muted-foreground">No hay usuarios con ese filtro.</p>
      )}

      <div className="grid gap-3">
        {usuarios.map((u) => {
          const esUnoMismo = u.id === user?.id
          const confirmandoEste = confirmando?.usuario.id === u.id
          const trabajando =
            (cambiarEstado.isPending && cambiarEstado.variables?.id === u.id) ||
            (eliminar.isPending && eliminar.variables === u.id)

          return (
            <Card key={u.id}>
              <CardContent className="grid gap-3 pt-4">
                {/* El estado va pegado al nombre y no contra el borde
                    derecho: en una pantalla ancha quedaban separados por
                    medio metro de blanco y había que ir y volver con la
                    vista para saber de quién era cada "Aprobada". */}
                <div>
                  <p className="flex flex-wrap items-center gap-x-2 gap-y-1 font-medium">
                    <span>
                      {u.nombre} {u.apellido}
                    </span>
                    <Badge variant="outline">{u.rol}</Badge>
                    <EstadoDeCuenta estado={u.estado} />
                    {esUnoMismo && (
                      <span className="text-muted-foreground text-sm font-normal">
                        (vos)
                      </span>
                    )}
                  </p>
                  <p className="text-muted-foreground text-sm break-all">{u.email}</p>
                </div>

                {!confirmandoEste && (
                  <div className="flex flex-wrap gap-2">
                    {u.estado === "PENDIENTE" && (
                      <Button
                        size="sm"
                        disabled={trabajando}
                        onClick={() =>
                          cambiarEstado.mutate({ id: u.id, estado: "APROBADA" })
                        }
                      >
                        Aprobar
                      </Button>
                    )}
                    {u.estado === "APROBADA" && (
                      <>
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={trabajando}
                          onClick={() => resetear.mutate(u)}
                        >
                          Resetear contraseña
                        </Button>
                        {/* Darse de baja a uno mismo dejaría al Admin fuera
                            del sistema en el acto; el backend además lo
                            rechaza si es el último (RF-01.8). */}
                        {!esUnoMismo && (
                          <Button
                            variant="destructive"
                            size="sm"
                            disabled={trabajando}
                            onClick={() => setConfirmando({ usuario: u, accion: "BAJA" })}
                          >
                            Dar de baja
                          </Button>
                        )}
                      </>
                    )}
                    {u.estado === "BAJA" && (
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={trabajando}
                        onClick={() => setConfirmando({ usuario: u, accion: "ELIMINAR" })}
                      >
                        Eliminar definitivamente
                      </Button>
                    )}
                  </div>
                )}

                {confirmandoEste && confirmando && (
                  <div className="grid gap-3 rounded-md border p-3">
                    <p className="text-destructive text-sm">
                      {confirmando.accion === "BAJA"
                        ? `Dar de baja a ${u.nombre} ${u.apellido} es permanente: no se puede reactivar la cuenta. Si sus materias quedan sin ningún otro docente, sus reservas futuras se cancelan.`
                        : `Eliminar la cuenta de ${u.nombre} ${u.apellido} la borra definitivamente y libera el email ${u.email} para un registro nuevo. Sus reservas e incidencias se conservan, pero pierden la referencia a la persona.`}
                    </p>
                    <div className="flex gap-2">
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={trabajando}
                        onClick={() =>
                          confirmando.accion === "BAJA"
                            ? cambiarEstado.mutate({ id: u.id, estado: "BAJA" })
                            : eliminar.mutate(u.id)
                        }
                      >
                        Confirmar
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={trabajando}
                        onClick={() => setConfirmando(null)}
                      >
                        Volver
                      </Button>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          )
        })}
      </div>

      {data && (
        <Paginador meta={data.meta} onCambiarPagina={setPagina} etiqueta="usuarios" />
      )}
    </div>
  )
}
