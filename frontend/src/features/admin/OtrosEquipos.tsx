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
import * as inventoryApi from "@/features/inventory/api"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-03.15 — lo prestable que no está en ningún carro: un proyector, los
 * cargadores, las notebooks de otro modelo.
 *
 * Va en una sección aparte y no mezclado entre los carros porque no
 * pertenece a ninguno: meterlo en un carro llamado "Sueltos" sería volver a
 * la mentira que el modelo viene sacándose de encima.
 *
 * Puertas adentro son la misma entidad que las PCs, y eso no es un detalle
 * de implementación: es lo que hace que el proyector se preste, se reclame y
 * —si es reservable— se reserve, con exactamente los mismos flujos.
 */

const EQUIPOS_KEY = ["equipos-sueltos"]

function Alta({ tiposUsados, onListo }: { tiposUsados: string[]; onListo: () => void }) {
  const queryClient = useQueryClient()
  const [tipo, setTipo] = useState("")
  const [nombre, setNombre] = useState("")
  const [reservable, setReservable] = useState(false)

  const crear = useMutation({
    mutationFn: () =>
      adminApi.crearEquipo({ tipo: tipo.trim(), nombre: nombre.trim(), reservable }),
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

export function OtrosEquipos() {
  const [agregando, setAgregando] = useState(false)

  const { data, isLoading, error } = useQuery({
    queryKey: EQUIPOS_KEY,
    queryFn: inventoryApi.listarEquiposSueltos,
  })

  const equipos = (data?.data ?? []).filter((e) => !e.dadaDeBaja)
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

        {equipos.map((e) => (
          <div
            key={e.id}
            className="flex flex-wrap items-center justify-between gap-2 rounded-md border p-3"
          >
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
          </div>
        ))}

        {agregando ? (
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
