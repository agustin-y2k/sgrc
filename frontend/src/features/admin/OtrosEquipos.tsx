import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { EstadoBadge } from "@/components/EstadoBadge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import * as adminApi from "@/features/admin/api"
import { AvisoDeCascada } from "@/features/admin/AvisoDeCascada"
import * as inventoryApi from "@/features/inventory/api"
import type { ResultadoCascada } from "@/features/admin/types"
import type { Equipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-03.15 — lo prestable que no está en ningún carro: un proyector, los
 * cargadores, las notebooks de otro modelo.
 */

const EQUIPOS_KEY = ["equipos-sueltos"]

function Alta({ tiposUsados, onListo }: { tiposUsados: string[]; onListo: () => void }) {
  const queryClient = useQueryClient()
  const [tipo, setTipo] = useState("")
  const [nombre, setNombre] = useState("")
  const [reservable, setReservable] = useState(false)

  const crear = useMutation({
    mutationFn: () =>
      adminApi.crearEquipoSuelto({ tipo: tipo.trim(), nombre: nombre.trim(), reservable }),
    onSuccess: async () => {
      setTipo("")
      setNombre("")
      setReservable(false)
      await queryClient.invalidateQueries({ queryKey: EQUIPOS_KEY })
      onListo()
    },
  })

  return (
    <form
      className="grid gap-3 rounded-md border border-dashed p-3"
      onSubmit={(e) => {
        e.preventDefault()
        crear.mutate()
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor="equipo-tipo">¿Qué es?</Label>
          <Input
            id="equipo-tipo"
            value={tipo}
            onChange={(e) => setTipo(e.target.value)}
            placeholder="Ej.: Proyector, Cargador, Notebook"
            list="tipos-de-equipo"
            required
          />
          {/* Texto libre con sugerencias: la lista de cosas que presta una
              escuela no es la misma que la de otra, y con una lista cerrada
              agregar "impresora 3D" pediría tocar el sistema. */}
          {tiposUsados.length > 0 && (
            <datalist id="tipos-de-equipo">
              {tiposUsados.map((t) => (
                <option key={t} value={t} />
              ))}
            </datalist>
          )}
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="equipo-nombre">¿Cómo lo llaman?</Label>
          <Input
            id="equipo-nombre"
            value={nombre}
            onChange={(e) => setNombre(e.target.value)}
            placeholder="Ej.: Cargador 1"
            required
          />
          {/* El nombre es lo único que lo distingue: dos filas llamadas
              "Cargador" serían indistinguibles justo donde hay que elegir
              cuál se está prestando. */}
          <p className="text-muted-foreground text-xs">
            Si hay más de uno igual, numeralos: Cargador 1, Cargador 2.
          </p>
        </div>
      </div>

      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          className="mt-1"
          checked={reservable}
          onChange={(e) => setReservable(e.target.checked)}
        />
        <span>
          Se puede reservar con anticipación
          <span className="text-muted-foreground block text-xs">
            Marcalo para un proyector, que alguien puede querer para una clase. Dejalo sin
            marcar para un cargador: se presta en el momento y aparecería como ruido cada
            vez que un docente va a reservar.
          </span>
        </span>
      </label>

      {crear.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(crear.error)}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={crear.isPending}>
          Agregar
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onListo}>
          Cancelar
        </Button>
      </div>
    </form>
  )
}

/**
 * Corregir lo que se cargó mal, o cambiar de idea sobre si algo se reserva.
 */
function Edicion({
  equipo,
  tiposUsados,
  onListo,
}: {
  equipo: Equipo
  tiposUsados: string[]
  onListo: () => void
}) {
  const queryClient = useQueryClient()
  const [tipo, setTipo] = useState(equipo.tipo ?? "")
  const [nombre, setNombre] = useState(equipo.nombre ?? "")
  const [reservable, setReservable] = useState(equipo.reservable ?? false)

  const guardar = useMutation({
    mutationFn: () =>
      adminApi.editarEquipo(equipo.id, {
        tipo: tipo.trim(),
        nombre: nombre.trim(),
        reservable,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: EQUIPOS_KEY })
      onListo()
    },
  })

  // Dejar de ser reservable no cancela nada: el backend solo saca al equipo
  // de la lista de libres de ahí en adelante.
  const dejaDeSerReservable = equipo.reservable && !reservable

  return (
    <form
      className="grid gap-3 rounded-md border border-dashed p-3"
      onSubmit={(e) => {
        e.preventDefault()
        guardar.mutate()
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor={`editar-tipo-${equipo.id}`}>¿Qué es?</Label>
          <Input
            id={`editar-tipo-${equipo.id}`}
            value={tipo}
            onChange={(e) => setTipo(e.target.value)}
            list="tipos-de-equipo"
            required
          />
          {tiposUsados.length > 0 && (
            <datalist id="tipos-de-equipo">
              {tiposUsados.map((t) => (
                <option key={t} value={t} />
              ))}
            </datalist>
          )}
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor={`editar-nombre-${equipo.id}`}>¿Cómo lo llaman?</Label>
          <Input
            id={`editar-nombre-${equipo.id}`}
            value={nombre}
            onChange={(e) => setNombre(e.target.value)}
            required
          />
        </div>
      </div>

      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          className="mt-1"
          checked={reservable}
          onChange={(e) => setReservable(e.target.checked)}
        />
        <span>Se puede reservar con anticipación</span>
      </label>

      {dejaDeSerReservable && (
        <p className="text-muted-foreground text-sm">
          Deja de aparecer cuando un docente arma una reserva. Las reservas que ya tenga
          siguen en pie: se siguen entregando normalmente.
        </p>
      )}

      {guardar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(guardar.error)}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={guardar.isPending}>
          Guardar
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onListo}>
          Cancelar
        </Button>
      </div>
    </form>
  )
}

export function OtrosEquipos() {
  const queryClient = useQueryClient()
  const [agregando, setAgregando] = useState(false)
  const [editando, setEditando] = useState<string | null>(null)
  const [dandoDeBaja, setDandoDeBaja] = useState<Equipo | null>(null)

  const [cascada, setCascada] = useState<ResultadoCascada | null>(null)

  const darDeBaja = useMutation({
    mutationFn: (equipo: Equipo) => adminApi.darDeBajaEquipo(equipo.id),
    onSuccess: async (resultado) => {
      setDandoDeBaja(null)
      setCascada(resultado)
      await queryClient.invalidateQueries({ queryKey: EQUIPOS_KEY })
    },
  })

  const { data, isLoading, error } = useQuery({
    queryKey: EQUIPOS_KEY,
    queryFn: () => inventoryApi.listarEquipos({ soloSueltos: true }),
  })

  const equipos = (data?.data ?? []).filter((e) => !e.dadoDeBaja)
  const tiposUsados = useMemo(
    () => [...new Set(equipos.map((e) => e.tipo))].sort(),
    [equipos]
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>Otros equipos</CardTitle>
        <CardDescription>
          Lo que se presta y no está en ningún carro: un proyector, cargadores, notebooks
          sueltas. Se entregan y se reciben en la misma pantalla que las computadoras.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {isLoading && <p className="text-muted-foreground text-sm">Cargando…</p>}
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(error)}</AlertDescription>
          </Alert>
        )}

        {!isLoading && equipos.length === 0 && (
          <p className="text-muted-foreground text-sm">
            No hay ningún equipo cargado todavía.
          </p>
        )}

        {darDeBaja.error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(darDeBaja.error)}</AlertDescription>
          </Alert>
        )}

        <AvisoDeCascada resultado={cascada} />

        {equipos.map((e) => {
          const editandoEste = editando === e.id
          const bajandoEste = dandoDeBaja?.id === e.id

          return (
            <div key={e.id} className="grid gap-2 rounded-md border p-3">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0">
                  <p className="font-medium">
                    {e.nombre}{" "}
                    {e.reservable ? (
                      <EstadoBadge tono="info">Se puede reservar</EstadoBadge>
                    ) : (
                      <EstadoBadge tono="neutro">Solo préstamo</EstadoBadge>
                    )}
                  </p>
                  <p className="text-muted-foreground text-sm">{e.tipo}</p>
                </div>
                {!editandoEste && !bajandoEste && (
                  <div className="flex shrink-0 flex-wrap gap-2">
                    <Button variant="outline" size="sm" onClick={() => setEditando(e.id)}>
                      Editar
                    </Button>
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() => setDandoDeBaja(e)}
                    >
                      Dar de baja
                    </Button>
                  </div>
                )}
              </div>

              {editandoEste && (
                <Edicion
                  equipo={e}
                  tiposUsados={tiposUsados}
                  onListo={() => setEditando(null)}
                />
              )}

              {bajandoEste && (
                <div className="grid gap-2 rounded-md border p-3">
                  <p className="text-destructive text-sm">
                    Dar de baja {e.nombre} lo saca del inventario y cancela sus reservas
                    futuras. Si está prestado, el sistema no deja darlo de baja: marcá
                    primero que volvió.
                  </p>
                  {/* El nombre se libera al dar de baja: un cargador que se
                      rompe y se reemplaza se va a seguir llamando igual. */}
                  <p className="text-muted-foreground text-sm">
                    El nombre queda libre para volver a usarlo en el equipo que lo
                    reemplace.
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      variant="destructive"
                      disabled={darDeBaja.isPending}
                      onClick={() => darDeBaja.mutate(e)}
                    >
                      Confirmar baja
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setDandoDeBaja(null)}
                    >
                      Volver
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )
        })}

        {/* Mientras se edita uno no se ofrece el alta: los dos formularios
            tienen los mismos campos, y verlos juntos no deja saber cuál se
            está completando. Mismo criterio que el inventario de un carro. */}
        {editando !== null ? null : agregando ? (
          <Alta tiposUsados={tiposUsados} onListo={() => setAgregando(false)} />
        ) : (
          <div>
            <Button variant="outline" size="sm" onClick={() => setAgregando(true)}>
              Agregar equipo
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
