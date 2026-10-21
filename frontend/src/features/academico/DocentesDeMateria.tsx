import { useEffect, useId, useRef, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as academicoApi from "@/features/academico/api"
import {
  etiquetaDeCurso,
  type Curso,
  type DocenteMateria,
  type Materia,
  type RolDocente,
} from "@/features/academico/types"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { useInvalidarAsignaciones } from "@/features/academico/useAsignaciones"
import * as adminApi from "@/features/admin/api"
import { getErrorMessage } from "@/lib/api-client"
import { todasLasPaginas } from "@/lib/todas-las-paginas"
import { contar, plural } from "@/lib/plural"

const ETIQUETA_ROL: Record<RolDocente, string> = {
  TITULAR: "Titular",
  SUPLENTE: "Suplente",
}

/** Docentes asignados a una materia (RF-02.6). */
export function DocentesDeMateria({
  materia,
  curso,
  soloLectura,
}: {
  materia: Materia
  curso: Curso
  soloLectura: boolean
}) {
  const queryClient = useQueryClient()
  const invalidarAsignaciones = useInvalidarAsignaciones()
  const [usuarioId, setUsuarioId] = useState("")
  const [rol, setRol] = useState<RolDocente>("TITULAR")
  const [resultado, setResultado] = useState("")
  const [quitando, setQuitando] = useState<{
    docente: DocenteMateria
    firma: string
  } | null>(null)
  const confirmacionDisponible = useRef(false)
  const disparador = useRef<HTMLButtonElement | null>(null)
  const devolverFoco = useRef(false)
  const tituloConfirmacion = useRef<HTMLHeadingElement>(null)
  const idConfirmacion = useId()
  useEffect(() => {
    if (quitando) tituloConfirmacion.current?.focus()
    else if (devolverFoco.current) {
      devolverFoco.current = false
      disparador.current?.focus()
    }
  }, [quitando])

  const docentesKey = ["docentes-materia", materia.id]
  const consultaDocentes = useQuery({
    queryKey: docentesKey,
    queryFn: () => academicoApi.listarDocentesDeMateria(materia.id),
  })

  // Solo cuentas APROBADAS: el backend rechaza asignar a cualquier otra.
  const consultaUsuarios = useQuery({
    queryKey: ["usuarios", "APROBADA"],
    queryFn: () =>
      todasLasPaginas((pagina) =>
        adminApi.listarUsuarios({
          estado: "APROBADA",
          pageSize: 200,
          ...(pagina > 1 ? { page: pagina } : {}),
        })
      ),
  })

  // Las dos: la lista de este panel y la del ciclo entero, que es la que
  // muestra los docentes en la fila de cada materia y en Usuarios. Sin la
  // segunda, asignar a alguien no se ve hasta recargar la página.
  const invalidar = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: docentesKey }),
      invalidarAsignaciones(),
      queryClient.invalidateQueries({ queryKey: ["mis-materias"] }),
      queryClient.invalidateQueries({ queryKey: ["materias-de-docente"] }),
    ])
  }

  const refrescarBaja = () =>
    Promise.all([
      invalidar(),
      ...[
        "reservas",
        "equipos-disponibles",
        "notificaciones",
        "reporte",
        "auditoria",
      ].map((key) => queryClient.invalidateQueries({ queryKey: [key] })),
    ])

  const asignar = useMutation({
    mutationFn: () => academicoApi.asignarDocente(materia.id, usuarioId, rol),
    onSuccess: async () => {
      setUsuarioId("")
      await invalidar()
    },
  })

  // Cambiar el rol es su propio endpoint y no "quitar y volver a asignar":
  // ese camino pasa por la cascada de RF-02.10 y, si es el único docente de
  // la materia, le cancela las reservas futuras.
  const cambiarRol = useMutation({
    mutationFn: ({ dm, rol }: { dm: DocenteMateria; rol: RolDocente }) =>
      academicoApi.cambiarRolDocente(materia.id, dm.id, rol),
    onSuccess: invalidar,
  })

  const remover = useMutation({
    mutationFn: (dm: DocenteMateria) =>
      academicoApi.removerDocenteMateria(materia.id, dm.id),
    onSuccess: async (respuesta, dm) => {
      setResultado(
        `Se quitó la asignación de ${nombreDe(dm.usuarioId)} de «${materia.nombre}» · ${etiquetaDeCurso(curso)}. ${
          respuesta.reservasCanceladas > 0
            ? `Se cancelaron ${contar(respuesta.reservasCanceladas, "reserva")} ${plural(respuesta.reservasCanceladas, "futura")} de esta materia, que quedó sin docente asignado.`
            : "No se cancelaron reservas futuras."
        }`
      )
      await refrescarBaja()
    },
    // Una respuesta perdida exige releer el estado, sin repetir el DELETE.
    onError: refrescarBaja,
  })

  if (consultaDocentes.error || consultaUsuarios.error)
    return (
      <ErrorDeConsulta
        mensaje="No pudimos consultar los docentes y sus cuentas."
        reintentando={consultaDocentes.isFetching || consultaUsuarios.isFetching}
        onReintentar={() => {
          void consultaDocentes.refetch()
          void consultaUsuarios.refetch()
        }}
      />
    )
  if (consultaDocentes.isPending || consultaUsuarios.isPending)
    return (
      <p role="status" className="text-muted-foreground text-sm">
        Cargando docentes…
      </p>
    )

  const asignados = consultaDocentes.data?.data ?? []
  const todos = consultaUsuarios.data?.data ?? []
  const yaAsignados = new Set(asignados.map((d) => d.usuarioId))
  const asignables = todos.filter((u) => !yaAsignados.has(u.id))
  const error = asignar.error ?? remover.error ?? cambiarRol.error
  const listo =
    consultaDocentes.isSuccess &&
    consultaUsuarios.isSuccess &&
    !consultaDocentes.isFetching &&
    !consultaUsuarios.isFetching
  const ocupado = asignar.isPending || cambiarRol.isPending || remover.isPending
  const bloquear = !listo || ocupado || quitando !== null || soloLectura

  function nombreDe(id: string) {
    const u = todos.find((x) => x.id === id)
    return u ? `${u.nombre} ${u.apellido}` : id
  }

  // Se cuentan todos los roles de esta materia concreta con cuenta aprobada.
  const activos = asignados.filter((dm) =>
    todos.some((u) => u.id === dm.usuarioId && u.estado === "APROBADA")
  )
  const firma = JSON.stringify([
    materia.id,
    materia.nombre,
    curso,
    asignados.map((dm) => [
      dm.id,
      dm.usuarioId,
      dm.rol,
      todos.find((u) => u.id === dm.usuarioId),
    ]),
  ])
  const docenteAQuitar = asignados.find((dm) => dm.id === quitando?.docente.id)
  const restantes = activos.filter((dm) => dm.id !== docenteAQuitar?.id).length
  const confirmacionVigente = !!docenteAQuitar && quitando?.firma === firma
  function volver() {
    confirmacionDisponible.current = false
    devolverFoco.current = true
    setQuitando(null)
  }
  function confirmar() {
    if (
      !listo ||
      ocupado ||
      soloLectura ||
      !confirmacionVigente ||
      !docenteAQuitar ||
      !confirmacionDisponible.current
    )
      return
    confirmacionDisponible.current = false
    setQuitando(null)
    remover.mutate(docenteAQuitar)
  }

  return (
    <div className="grid min-w-0 gap-2">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {resultado && (
        <p role="status" className="text-sm">
          {resultado}
        </p>
      )}
      {remover.isPending && (
        <p role="status" className="text-sm">
          Quitando docente…
        </p>
      )}

      {asignados.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          Sin docentes asignados: por ahora solo un Admin puede reservar esta materia.
        </p>
      ) : (
        <ul className="grid min-w-0 gap-1.5">
          {asignados.map((dm) => (
            <li
              key={dm.id}
              className="flex min-w-0 flex-wrap items-center justify-between gap-2"
            >
              <span className="text-sm">
                {nombreDe(dm.usuarioId)}{" "}
                {soloLectura && <Badge variant="outline">{ETIQUETA_ROL[dm.rol]}</Badge>}
              </span>
              {!soloLectura && (
                <span className="flex min-w-0 items-center gap-2">
                  {/* El nombre accesible va por aria-label y no por un <Label>
                      con texto: con varios docentes en la lista, repetir el
                      nombre de cada uno en un nodo de texto oculto lo duplica
                      en la pantalla para cualquiera que la lea por texto. */}
                  <Select
                    aria-label={`Rol de ${nombreDe(dm.usuarioId)}`}
                    className="w-auto"
                    value={dm.rol}
                    disabled={bloquear}
                    onChange={(e) =>
                      cambiarRol.mutate({ dm, rol: e.target.value as RolDocente })
                    }
                  >
                    <option value="TITULAR">{ETIQUETA_ROL.TITULAR}</option>
                    <option value="SUPLENTE">{ETIQUETA_ROL.SUPLENTE}</option>
                  </Select>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={bloquear}
                    onClick={(e) => {
                      disparador.current = e.currentTarget
                      confirmacionDisponible.current = true
                      remover.reset()
                      setResultado("")
                      setQuitando({ docente: dm, firma })
                    }}
                  >
                    Quitar
                  </Button>
                </span>
              )}
            </li>
          ))}
        </ul>
      )}

      {/* RF-02.10: quitar al último docente cancela las reservas futuras de
          la materia. Con otro asignado no se cancela nada. */}
      {!soloLectura && asignados.length === 1 && (
        <p className="text-muted-foreground text-xs">
          Es el único docente asignado: si lo quitás, se cancelan las reservas futuras de
          esta materia y se avisa a todos los Admin.
        </p>
      )}

      {!soloLectura && (
        <p className="text-muted-foreground text-xs">
          Puede haber varios titulares y suplentes. Las reservas se mantienen mientras
          quede algún docente asignado con cuenta aprobada a esta materia y curso.
        </p>
      )}
      {quitando && !soloLectura && (
        <div
          role="group"
          aria-labelledby={idConfirmacion}
          className="grid min-w-0 gap-3 rounded-md border p-3"
        >
          <h4
            id={idConfirmacion}
            ref={tituloConfirmacion}
            tabIndex={-1}
            className="font-medium"
          >
            Confirmar quitar docente
          </h4>
          <p className="text-sm wrap-anywhere">
            Vas a quitar a {nombreDe(quitando.docente.usuarioId)} (
            {ETIQUETA_ROL[quitando.docente.rol].toLowerCase()}) de «{materia.nombre}» ·{" "}
            {etiquetaDeCurso(curso)}.
          </p>
          {!confirmacionVigente ? (
            <p role="alert" className="text-sm">
              Las asignaciones cambiaron. Volvé y abrí otra vez la confirmación para
              revisar el efecto actual.
            </p>
          ) : restantes > 0 ? (
            <p className="text-sm">
              {restantes === 1 ? "Queda" : "Quedan"} {contar(restantes, "docente")} con
              cuenta aprobada en esta materia y curso, sean titulares o suplentes. Las
              reservas se mantienen.
            </p>
          ) : (
            <p className="text-destructive text-sm">
              Si confirmás y la materia queda sin ningún docente con cuenta aprobada, se
              cancelarán sus reservas futuras y se avisará a los administradores. Las
              materias de otros cursos conservan sus reservas.
            </p>
          )}
          <p className="text-muted-foreground text-xs">
            El sistema comprueba las asignaciones vigentes al guardar. Esta acción sólo
            quita esta asignación; la cuenta de la persona se conserva.
          </p>
          <div className="flex min-w-0 flex-wrap gap-2">
            <Button
              type="button"
              variant="destructive"
              size="sm"
              disabled={!listo || ocupado || !confirmacionVigente}
              onClick={confirmar}
            >
              Confirmar quitar docente
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={volver}>
              Volver
            </Button>
          </div>
        </div>
      )}

      {!soloLectura && (
        <form
          className="grid min-w-0 gap-2 sm:grid-cols-[1fr_auto_auto] sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            if (!bloquear && asignables.some((u) => u.id === usuarioId)) asignar.mutate()
          }}
        >
          <div className="grid min-w-0 gap-1.5">
            <Label htmlFor={`docente-${materia.id}`}>Asignar docente</Label>
            <Select
              id={`docente-${materia.id}`}
              value={usuarioId}
              disabled={bloquear}
              onChange={(e) => setUsuarioId(e.target.value)}
            >
              <option value="">Elegí una persona…</option>
              {asignables.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.nombre} {u.apellido} ({u.email})
                </option>
              ))}
            </Select>
          </div>
          <div className="grid min-w-0 gap-1.5">
            <Label htmlFor={`rol-${materia.id}`}>Rol</Label>
            <Select
              id={`rol-${materia.id}`}
              className="w-auto"
              value={rol}
              disabled={bloquear}
              onChange={(e) => setRol(e.target.value as RolDocente)}
            >
              <option value="TITULAR">Titular</option>
              <option value="SUPLENTE">Suplente</option>
            </Select>
          </div>
          <Button
            type="submit"
            size="sm"
            disabled={bloquear || !asignables.some((u) => u.id === usuarioId)}
          >
            Asignar
          </Button>
        </form>
      )}
    </div>
  )
}

/** Materias de un curso (RF-02.3). */
