import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { EstadoBadge, TONO_CUENTA } from "@/components/EstadoBadge"
import { Paginador } from "@/components/Paginador"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Select } from "@/components/ui/select"
import type { AsignacionDocente } from "@/features/academico/types"
import { cursoDe, etiquetaDeCurso } from "@/features/academico/types"
import { useAsignacionesDelCicloActivo } from "@/features/academico/useAsignaciones"
import { useAuth } from "@/features/auth/AuthContext"
import { LoQueDeclaro } from "@/features/auth/LoQueDeclaro"
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

// PROMOVER y DEGRADAR piden confirmación como las otras dos aunque no
// destruyan nada: cambian quién puede tocar el inventario, el ciclo lectivo y
// las cuentas de los demás.
type Confirmacion = {
  usuario: Usuario
  accion: "BAJA" | "ELIMINAR" | "PROMOVER" | "DEGRADAR"
}

const TEXTO_CONFIRMACION: Record<Confirmacion["accion"], (u: Usuario) => string> = {
  BAJA: (u) =>
    `Dar de baja a ${u.nombre} ${u.apellido} es permanente: no se puede reactivar la cuenta. Si sus materias quedan sin ningún otro docente, sus reservas futuras se cancelan.`,
  ELIMINAR: (u) =>
    `Eliminar la cuenta de ${u.nombre} ${u.apellido} la borra definitivamente y libera el email ${u.email} para un registro nuevo. Sus reservas e incidencias se conservan, pero pierden la referencia a la persona. Lo que sí se borra: su hilo de soporte con las respuestas que se le dieron, sus avisos, sus pedidos de materia y —si es Admin— su horario de guardia.`,
  PROMOVER: (u) =>
    `${u.nombre} ${u.apellido} va a pasar a tener permisos de Admin: aprobar cuentas, editar el inventario y el ciclo lectivo, y dar de baja a otros usuarios. Conserva sus materias y sus reservas, y el cambio le aplica de inmediato, sin que tenga que volver a entrar.`,
  DEGRADAR: (u) =>
    `${u.nombre} ${u.apellido} deja de tener permisos de Admin y queda como docente. La cuenta sigue abierta: conserva sus materias, sus reservas y su forma de ingreso — lo único que pierde son las pantallas de administración, y deja de figurar en la lista de Admins con su horario de atención. Cualquier Admin puede volver a promoverlo.`,
}

// El plural del cartel de arrastre. Los mensajes del hilo no se cuentan
// aparte: se van con el hilo, y "1 hilo de soporte (4 mensajes)" dice en una
// sola frase lo que dos contadores sueltos hacen leer dos veces.
function describirArrastre(d: adminApi.ArrastreDeEliminacion): string {
  const partes: string[] = []
  if (d.hilosDeSoporte > 0) {
    const hilos = d.hilosDeSoporte === 1 ? "su hilo de soporte" : `sus ${d.hilosDeSoporte} hilos de soporte`
    partes.push(
      d.mensajesDeSoporte > 0 ? `${hilos} (${d.mensajesDeSoporte} mensajes)` : hilos
    )
  }
  if (d.bloquesDeGuardia > 0) {
    partes.push(
      d.bloquesDeGuardia === 1
        ? "1 tramo de su horario de guardia"
        : `los ${d.bloquesDeGuardia} tramos de su horario de guardia`
    )
  }
  if (d.pedidosDeMateria > 0) {
    partes.push(
      d.pedidosDeMateria === 1 ? "1 pedido de materia" : `${d.pedidosDeMateria} pedidos de materia`
    )
  }
  if (d.notificaciones > 0) {
    partes.push(
      d.notificaciones === 1 ? "1 aviso" : `${d.notificaciones} avisos`
    )
  }
  if (partes.length === 1) return partes[0]
  return `${partes.slice(0, -1).join(", ")} y ${partes[partes.length - 1]}`
}

const arrastreTieneAlgo = (d: adminApi.ArrastreDeEliminacion) =>
  d.hilosDeSoporte > 0 ||
  d.bloquesDeGuardia > 0 ||
  d.pedidosDeMateria > 0 ||
  d.notificaciones > 0

const esCambioDeRol = (accion: Confirmacion["accion"]) =>
  accion === "PROMOVER" || accion === "DEGRADAR"

/**
 * Qué dicta una persona, con el curso al lado: "Matemática · 1°4° (titular)".
 *
 * El curso va SIEMPRE pegado a la materia. "Matemática" a secas no distingue
 * la de 1°4° de la de 5°A, y son materias distintas con docentes distintos.
 */
function QueDicta({
  usuario: u,
  asignaciones,
}: {
  usuario: Usuario
  asignaciones: AsignacionDocente[]
}) {
  const suyas = asignaciones
    .filter((a) => a.usuarioId === u.id)
    .sort((a, b) => a.cursoNombre.localeCompare(b.cursoNombre, "es"))

  if (suyas.length === 0) {
    // De un Admin no se dice nada: administrar el sistema no implica dar
    // clase, y "no tiene materias" sería ruido en la mayoría de las fichas.
    // De un docente aprobado sí, porque sin materia no puede reservar nada y
    // eso es algo para resolver.
    if (u.rol !== "DOCENTE" || u.estado !== "APROBADA") return null
    return (
      <p className="text-muted-foreground text-sm">
        Sin materias asignadas: todavía no puede reservar.
      </p>
    )
  }

  return (
    <p className="text-muted-foreground text-sm">
      Dicta{" "}
      {suyas
        .map(
          (a) =>
            `${a.materiaNombre} · ${etiquetaDeCurso(cursoDe(a))} (${a.rol === "TITULAR" ? "titular" : "suplente"})`
        )
        .join(", ")}
    </p>
  )
}

// RF-01/RF-02: panel de usuarios.
export function UsuariosPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [filtroEstado, setFiltroEstado] = useState<Estado | "">("")
  const [pagina, setPagina] = useState(1)
  const [altaAbierta, setAltaAbierta] = useState(false)

  // Qué dicta cada quien, del ciclo activo: una consulta para toda la página,
  // no una por usuario.
  const asignaciones = useAsignacionesDelCicloActivo()
  const [confirmando, setConfirmando] = useState<Confirmacion | null>(null)
  const [passwordTemporal, setPasswordTemporal] = useState<{
    usuario: string
    password: string
  } | null>(null)
  // Qué se llevó el último borrado definitivo. Sólo se muestra si se llevó
  // algo: el caso normal es que no, y un cartel que aparece siempre deja de
  // leerse justo cuando tiene algo que decir.
  const [arrastre, setArrastre] = useState<{
    usuario: string
    detalle: adminApi.ArrastreDeEliminacion
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
    mutationFn: (u: Usuario) => adminApi.eliminarUsuario(u.id),
    onSuccess: async (detalle, u) => {
      setConfirmando(null)
      // El borrado dispara diez cascadas. Dos importan y no se pueden deshacer:
      // el hilo de soporte se va con las respuestas que escribió el propio
      // Admin, y el horario de guardia decide si el barrido actúa (RF-07.6).
      // El `detalle &&` no sobra: si el servidor contesta 200 con el cuerpo
      // vacío —una versión anterior, un proxy que lo recorta— el borrado igual
      // tiene que terminar bien.
      if (detalle && arrastreTieneAlgo(detalle)) {
        setArrastre({ usuario: `${u.nombre} ${u.apellido}`, detalle })
      }
      await invalidar()
    },
  })

  const promover = useMutation({
    mutationFn: (id: string) => adminApi.promoverAAdmin(id),
    onSuccess: async () => {
      setConfirmando(null)
      await invalidar()
    },
  })

  const degradar = useMutation({
    mutationFn: (id: string) => adminApi.degradarADocente(id),
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
  const errorActivo =
    error ??
    cambiarEstado.error ??
    eliminar.error ??
    resetear.error ??
    promover.error ??
    degradar.error

  return (
    <div className="mx-auto max-w-4xl">
      <EncabezadoDePagina
        titulo="Usuarios"
        descripcion="Todas las cuentas del sistema: estado, asignaciones y restablecimiento de contraseña."
        /* RF-01.4 */
        accion={
          !altaAbierta && (
            <Button variant="outline" onClick={() => setAltaAbierta(true)}>
              Crear otro Admin
            </Button>
          )
        }
      />

      {altaAbierta && (
        <AltaDeAdmin usuariosKey={USUARIOS_KEY} onCerrar={() => setAltaAbierta(false)} />
      )}

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

      {arrastre && (
        <Alert className="mb-4">
          <AlertDescription>
            Se borró la cuenta de {arrastre.usuario} y con ella{" "}
            {describirArrastre(arrastre.detalle)}. Eso no se puede deshacer.{" "}
            <Button variant="outline" size="sm" onClick={() => setArrastre(null)}>
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
            (eliminar.isPending && eliminar.variables?.id === u.id) ||
            (promover.isPending && promover.variables === u.id) ||
            (degradar.isPending && degradar.variables === u.id)

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
                  {/* Qué dicta, del ciclo activo. Sin esto, la única forma de
                      saber si una persona tiene materias era ir a Académico y
                      abrirlas una por una desde el otro lado. */}
                  <QueDicta usuario={u} asignaciones={asignaciones} />
                </div>

                {/* Lo que declaró al registrarse, con el mismo detalle que
                    en Aprobación. Este atajo dejaba aprobar sin verlo: el
                    botón decide sobre una persona de la que no se mostraba
                    nada más que el nombre, y el curso y la materia que pidió
                    son justamente lo que hay que mirar antes. */}
                {u.estado === "PENDIENTE" && <LoQueDeclaro usuario={u} />}

                {!confirmandoEste && (
                  <div className="flex flex-wrap gap-2">
                    {u.estado === "PENDIENTE" && (
                      <>
                        <Button
                          size="sm"
                          disabled={trabajando}
                          onClick={() =>
                            cambiarEstado.mutate({ id: u.id, estado: "APROBADA" })
                          }
                        >
                          Aprobar
                        </Button>
                        {/* Rechazar estaba solo en Aprobación. Que un estado
                            ofrezca la mitad de sus salidas obliga a irse a
                            otra pantalla para terminar lo que se empezó acá. */}
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={trabajando}
                          onClick={() =>
                            cambiarEstado.mutate({ id: u.id, estado: "RECHAZADA" })
                          }
                        >
                          Rechazar
                        </Button>
                      </>
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
                        {/* Solo sobre docentes: el backend rechaza promover
                            a alguien que ya es Admin, y ofrecer un botón que
                            siempre falla es peor que no ofrecerlo. */}
                        {u.rol === "DOCENTE" && (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={trabajando}
                            onClick={() =>
                              setConfirmando({ usuario: u, accion: "PROMOVER" })
                            }
                          >
                            Promover a admin
                          </Button>
                        )}
                        {/* La inversa, y con la misma restricción de la
                            baja: sobre uno mismo no se ofrece. Quien se
                            quitara los permisos perdería en el acto esta
                            pantalla y dependería de otro Admin para volver
                            atrás (el backend lo rechaza igual). */}
                        {u.rol === "ADMIN" && !esUnoMismo && (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={trabajando}
                            onClick={() =>
                              setConfirmando({ usuario: u, accion: "DEGRADAR" })
                            }
                          >
                            Quitar permisos de admin
                          </Button>
                        )}
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
                    {/* Los dos estados terminales (RF-01.9). RECHAZADA está
                        acá porque es el único modo de deshacer un rechazo
                        equivocado: esa cuenta no transiciona a ningún lado,
                        así que sin eliminar, el email quedaría tomado para
                        siempre. */}
                    {(u.estado === "BAJA" || u.estado === "RECHAZADA") && (
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
                    {/* Los dos cambios de rol no destruyen nada, así que no
                        van en rojo: el rojo es para lo que borra. Pero sí
                        piden confirmación, porque cambian quién puede tocar
                        el sistema y quien los sufre no puede deshacerlos. */}
                    <p
                      className={
                        esCambioDeRol(confirmando.accion)
                          ? "text-sm"
                          : "text-destructive text-sm"
                      }
                    >
                      {TEXTO_CONFIRMACION[confirmando.accion](u)}
                    </p>
                    <div className="flex gap-2">
                      <Button
                        variant={
                          esCambioDeRol(confirmando.accion) ? "default" : "destructive"
                        }
                        size="sm"
                        disabled={trabajando}
                        onClick={() => {
                          if (confirmando.accion === "BAJA") {
                            cambiarEstado.mutate({ id: u.id, estado: "BAJA" })
                          } else if (confirmando.accion === "PROMOVER") {
                            promover.mutate(u.id)
                          } else if (confirmando.accion === "DEGRADAR") {
                            degradar.mutate(u.id)
                          } else {
                            eliminar.mutate(u)
                          }
                        }}
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
