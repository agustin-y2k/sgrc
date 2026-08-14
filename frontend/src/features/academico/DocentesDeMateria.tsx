import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as academicoApi from "@/features/academico/api"
import type { DocenteMateria, Materia, RolDocente } from "@/features/academico/types"
import * as adminApi from "@/features/admin/api"
import { getErrorMessage } from "@/lib/api-client"

const ETIQUETA_ROL: Record<RolDocente, string> = {
  TITULAR: "Titular",
  SUPLENTE: "Suplente",
}

/**
 * Docentes asignados a una materia (RF-02.6). Sin al menos uno, la materia
 * existe pero nadie puede reservar sobre ella salvo un Admin (RF-04.1).
 *
 * El endpoint de academic devuelve solo `usuarioId`, así que el nombre se
 * resuelve acá cruzando contra la lista de usuarios de auth.
 */
export function DocentesDeMateria({
  materia,
  soloLectura,
}: {
  materia: Materia
  soloLectura: boolean
}) {
  const queryClient = useQueryClient()
  const [usuarioId, setUsuarioId] = useState("")
  const [rol, setRol] = useState<RolDocente>("TITULAR")
  const [ultimaCascada, setUltimaCascada] = useState<number | null>(null)

  const docentesKey = ["docentes-materia", materia.id]
  const { data, isLoading } = useQuery({
    queryKey: docentesKey,
    queryFn: () => academicoApi.listarDocentesDeMateria(materia.id),
  })

  // Solo cuentas APROBADAS: el backend rechaza asignar a cualquier otra.
  const { data: usuarios } = useQuery({
    queryKey: ["usuarios", "APROBADA"],
    queryFn: () => adminApi.listarUsuarios({ estado: "APROBADA" }),
  })

  const invalidar = () => queryClient.invalidateQueries({ queryKey: docentesKey })

  const asignar = useMutation({
    mutationFn: () => academicoApi.asignarDocente(materia.id, usuarioId, rol),
    onSuccess: async () => {
      setUsuarioId("")
      await invalidar()
    },
  })

  // Cambiar el rol es su propio endpoint y no "quitar y volver a asignar":
  // ese camino pasa por la cascada de RF-02.10 y, si es el único docente de
  // la materia, le cancela las reservas futuras. Corregir un rol mal cargado
  // no puede costar las clases ya reservadas.
  const cambiarRol = useMutation({
    mutationFn: ({ dm, rol }: { dm: DocenteMateria; rol: RolDocente }) =>
      academicoApi.cambiarRolDocente(materia.id, dm.id, rol),
    onSuccess: invalidar,
  })

  const remover = useMutation({
    mutationFn: (dm: DocenteMateria) =>
      academicoApi.removerDocenteMateria(materia.id, dm.id),
    onSuccess: async (resultado) => {
      // La cascada es destructiva y silenciosa: sin esto, el Admin quita al
      // último docente y no se entera de que canceló clases (el aviso de
      // RF-02.8 le llega a la campana, no a esta pantalla).
      setUltimaCascada(resultado.reservasCanceladas)
      await invalidar()
    },
  })

  if (isLoading)
    return <p className="text-muted-foreground text-sm">Cargando docentes…</p>

  const asignados = data?.data ?? []
  const todos = usuarios?.data ?? []
  const yaAsignados = new Set(asignados.map((d) => d.usuarioId))
  const asignables = todos.filter((u) => !yaAsignados.has(u.id))
  const error = asignar.error ?? remover.error ?? cambiarRol.error

  const nombreDe = (id: string) => {
    const u = todos.find((x) => x.id === id)
    return u ? `${u.nombre} ${u.apellido}` : id
  }

  return (
    <div className="grid gap-2">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {ultimaCascada !== null && ultimaCascada > 0 && (
        <Alert>
          <AlertDescription>
            Se cancelaron {ultimaCascada} reserva(s) futuras de esta materia, que quedó
            sin docente asignado.
          </AlertDescription>
        </Alert>
      )}

      {asignados.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          Sin docentes asignados: por ahora solo un Admin puede reservar esta materia.
        </p>
      ) : (
        <ul className="grid gap-1.5">
          {asignados.map((dm) => (
            <li key={dm.id} className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-sm">
                {nombreDe(dm.usuarioId)}{" "}
                {soloLectura && <Badge variant="outline">{ETIQUETA_ROL[dm.rol]}</Badge>}
              </span>
              {!soloLectura && (
                <span className="flex items-center gap-2">
                  {/* El nombre accesible va por aria-label y no por un <Label>
                      con texto: con varios docentes en la lista, repetir el
                      nombre de cada uno en un nodo de texto oculto lo duplica
                      en la pantalla para cualquiera que la lea por texto. */}
                  <Select
                    aria-label={`Rol de ${nombreDe(dm.usuarioId)}`}
                    className="w-auto"
                    value={dm.rol}
                    disabled={cambiarRol.isPending}
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
                    disabled={remover.isPending}
                    onClick={() => remover.mutate(dm)}
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
        <form
          className="grid gap-2 sm:grid-cols-[1fr_auto_auto] sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            asignar.mutate()
          }}
        >
          <div className="grid gap-1.5">
            <Label htmlFor={`docente-${materia.id}`}>Asignar docente</Label>
            <Select
              id={`docente-${materia.id}`}
              value={usuarioId}
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
          <div className="grid gap-1.5">
            <Label htmlFor={`rol-${materia.id}`}>Rol</Label>
            <Select
              id={`rol-${materia.id}`}
              className="w-auto"
              value={rol}
              onChange={(e) => setRol(e.target.value as RolDocente)}
            >
              <option value="TITULAR">Titular</option>
              <option value="SUPLENTE">Suplente</option>
            </Select>
          </div>
          <Button
            type="submit"
            size="sm"
            disabled={usuarioId === "" || asignar.isPending}
          >
            Asignar
          </Button>
        </form>
      )}
    </div>
  )
}

/**
 * Materias de un curso (RF-02.3). Es el nivel más profundo del árbol y el
 * que realmente importa: sin materias no hay nada que reservar.
 */
