import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import * as adminApi from "@/features/admin/api"
import { AltaDePC, EdicionDePC } from "@/features/admin/FormularioPC"
import { IncidenciasDePC } from "@/features/admin/IncidenciasDePC"
import * as inventoryApi from "@/features/inventory/api"
import type { Carro, EstadoPC, PC } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"

const CARROS_KEY = ["carros"]

const ETIQUETA_ESTADO: Record<EstadoPC, string> = {
  DISPONIBLE: "Disponible",
  EN_MANTENIMIENTO: "En mantenimiento",
  FUERA_DE_SERVICIO: "Fuera de servicio",
}

type CambioEstado = {
  pc: PC
  nuevoEstado: EstadoPC
  motivo: string
}

function PCsAdmin({ carroId, carros }: { carroId: string; carros: Carro[] }) {
  const queryClient = useQueryClient()
  const [cambiando, setCambiando] = useState<CambioEstado | null>(null)
  const [dandoDeBaja, setDandoDeBaja] = useState<PC | null>(null)
  const [editando, setEditando] = useState<string | null>(null)
  const [viendoIncidencias, setViendoIncidencias] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ["pcs", carroId],
    queryFn: () => inventoryApi.listarPCsDeCarro(carroId),
  })

  const invalidar = () => queryClient.invalidateQueries({ queryKey: ["pcs", carroId] })

  const cambiarEstado = useMutation({
    mutationFn: ({ pc, nuevoEstado, motivo }: CambioEstado) =>
      adminApi.cambiarEstadoPC(pc.id, nuevoEstado, motivo.trim() || undefined),
    onSuccess: async () => {
      setCambiando(null)
      await invalidar()
    },
  })

  const darDeBaja = useMutation({
    mutationFn: (pc: PC) => adminApi.darDeBajaPC(pc.id),
    onSuccess: async () => {
      setDandoDeBaja(null)
      await invalidar()
    },
  })

  if (isLoading) return <p className="text-muted-foreground text-sm">Cargando PCs…</p>

  const pcs = (data?.data ?? []).filter((pc) => !pc.dadaDeBaja)
  const error = cambiarEstado.error ?? darDeBaja.error

  return (
    <div className="grid gap-3">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {pcs.length === 0 && (
        <p className="text-muted-foreground text-sm">
          Este carro no tiene PCs activas. Agregá la primera con el formulario de abajo:
          sin PCs cargadas nadie puede reservar.
        </p>
      )}

      {pcs.map((pc) => {
        const cambiandoEsta = cambiando?.pc.id === pc.id
        const bajandoEsta = dandoDeBaja?.id === pc.id
        const editandoEsta = editando === pc.id
        const incidenciasAbiertas = viendoIncidencias === pc.id

        return (
          <div key={pc.id} className="grid gap-2 rounded-md border p-3">
            {/* Misma razón que en la cabecera del carro: identificador y
                número de serie arriba, la fila de botones abajo. Acá son
                cinco, así que en un teléfono se envuelven igual — pero
                empiezan siempre en el mismo borde izquierdo y no donde
                termine el texto de arriba. */}
            <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0">
                <p className="font-medium">
                  PC {pc.identificador}{" "}
                  <Badge
                    variant={pc.estado === "DISPONIBLE" ? "secondary" : "destructive"}
                  >
                    {ETIQUETA_ESTADO[pc.estado]}
                  </Badge>
                </p>
                <p className="text-muted-foreground text-sm break-words">
                  N° serie {pc.numeroSerie}
                  {pc.softwareInstalado && ` · ${pc.softwareInstalado}`}
                </p>
              </div>
              {!cambiandoEsta && !bajandoEsta && !editandoEsta && (
                <div className="flex shrink-0 flex-wrap gap-2">
                  {(["DISPONIBLE", "EN_MANTENIMIENTO", "FUERA_DE_SERVICIO"] as EstadoPC[])
                    .filter((e) => e !== pc.estado)
                    .map((e) => (
                      <Button
                        key={e}
                        variant="outline"
                        size="sm"
                        onClick={() => setCambiando({ pc, nuevoEstado: e, motivo: "" })}
                      >
                        → {ETIQUETA_ESTADO[e]}
                      </Button>
                    ))}
                  <Button variant="outline" size="sm" onClick={() => setEditando(pc.id)}>
                    Editar
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    aria-expanded={incidenciasAbiertas}
                    onClick={() =>
                      setViendoIncidencias(incidenciasAbiertas ? null : pc.id)
                    }
                  >
                    Incidencias
                  </Button>
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => setDandoDeBaja(pc)}
                  >
                    Dar de baja
                  </Button>
                </div>
              )}
            </div>

            {/* RF-03.8: sacar una PC de DISPONIBLE cancela sus reservas
                futuras, y RF-03.8 aclara que al volver a DISPONIBLE NO se
                restauran. Por eso se avisa antes, no después. */}
            {cambiandoEsta && cambiando && (
              <div className="grid gap-2 rounded-md border p-3">
                {cambiando.nuevoEstado !== "DISPONIBLE" ? (
                  <p className="text-destructive text-sm">
                    Pasar la PC {pc.identificador} a{" "}
                    {ETIQUETA_ESTADO[cambiando.nuevoEstado].toLowerCase()} cancela todas
                    sus reservas futuras y avisa a cada docente. Si más adelante vuelve a
                    estar disponible, esas reservas no se restauran solas.
                  </p>
                ) : (
                  <p className="text-muted-foreground text-sm">
                    La PC vuelve a estar disponible para reservar. Las reservas que se
                    cancelaron mientras no lo estaba no se recuperan.
                  </p>
                )}
                <div className="grid gap-1.5">
                  <Label htmlFor={`motivo-${pc.id}`}>Motivo (opcional)</Label>
                  <Input
                    id={`motivo-${pc.id}`}
                    value={cambiando.motivo}
                    onChange={(e) =>
                      setCambiando({ ...cambiando, motivo: e.target.value })
                    }
                    placeholder="Se incluye en el aviso al docente"
                  />
                </div>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="destructive"
                    disabled={cambiarEstado.isPending}
                    onClick={() => cambiarEstado.mutate(cambiando)}
                  >
                    Confirmar cambio
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setCambiando(null)}>
                    Volver
                  </Button>
                </div>
              </div>
            )}

            {editandoEsta && (
              <EdicionDePC pc={pc} carros={carros} onListo={() => setEditando(null)} />
            )}

            {incidenciasAbiertas && <IncidenciasDePC pcId={pc.id} />}

            {bajandoEsta && (
              <div className="grid gap-2 rounded-md border p-3">
                <p className="text-destructive text-sm">
                  Dar de baja la PC {pc.identificador} la saca del inventario y cancela
                  sus reservas futuras. Su historial de incidencias se conserva.
                </p>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="destructive"
                    disabled={darDeBaja.isPending}
                    onClick={() => darDeBaja.mutate(pc)}
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

      {/* Mientras se edita una PC no se muestra: los dos formularios tienen
          los mismos campos, y verlos juntos no deja saber cuál se está
          completando. */}
      {editando === null && <AltaDePC carroId={carroId} />}
    </div>
  )
}

// RF-03.1 — renombrar un carro o corregir su descripción.
function EdicionDeCarro({ carro, onListo }: { carro: Carro; onListo: () => void }) {
  const queryClient = useQueryClient()
  const [nombre, setNombre] = useState(carro.nombre)
  const [descripcion, setDescripcion] = useState(carro.descripcion ?? "")

  const editar = useMutation({
    mutationFn: () =>
      adminApi.editarCarro(carro.id, {
        nombre: nombre.trim(),
        descripcion: descripcion.trim(),
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: CARROS_KEY })
      onListo()
    },
  })

  return (
    <form
      className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
      onSubmit={(e) => {
        e.preventDefault()
        editar.mutate()
      }}
    >
      {editar.error && (
        <Alert variant="destructive" className="sm:col-span-3">
          <AlertDescription>{getErrorMessage(editar.error)}</AlertDescription>
        </Alert>
      )}
      <div className="grid gap-1.5">
        <Label htmlFor={`carro-${carro.id}-nombre`}>Nombre del carro</Label>
        <Input
          id={`carro-${carro.id}-nombre`}
          value={nombre}
          onChange={(e) => setNombre(e.target.value)}
        />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor={`carro-${carro.id}-desc`}>Descripción del carro</Label>
        <Input
          id={`carro-${carro.id}-desc`}
          value={descripcion}
          onChange={(e) => setDescripcion(e.target.value)}
        />
      </div>
      <div className="flex gap-2">
        <Button
          type="submit"
          size="sm"
          disabled={nombre.trim() === "" || editar.isPending}
        >
          Guardar
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onListo}>
          Volver
        </Button>
      </div>
    </form>
  )
}

// RF-03: carros, PCs e incidencias. Es la pantalla desde la que se arma el
// inventario de cero — sin PCs cargadas acá, nadie puede reservar nada.
export function InventarioAdminPage() {
  const queryClient = useQueryClient()
  const [carroAbierto, setCarroAbierto] = useState<string | null>(null)
  const [carroEnEdicion, setCarroEnEdicion] = useState<string | null>(null)
  const [nombreCarro, setNombreCarro] = useState("")
  const [descripcionCarro, setDescripcionCarro] = useState("")

  const { data, isLoading, error } = useQuery({
    queryKey: CARROS_KEY,
    queryFn: inventoryApi.listarCarros,
  })

  const crearCarro = useMutation({
    mutationFn: () =>
      adminApi.crearCarro({
        nombre: nombreCarro,
        descripcion: descripcionCarro || undefined,
      }),
    onSuccess: async () => {
      setNombreCarro("")
      setDescripcionCarro("")
      await queryClient.invalidateQueries({ queryKey: CARROS_KEY })
    },
  })

  const carros = data?.data ?? []

  return (
    <div className="mx-auto max-w-4xl">
      <EncabezadoDePagina
        titulo="Gestión del inventario"
        descripcion="Alta y edición de carros y PCs, y seguimiento de las incidencias reportadas."
      />

      {(error || crearCarro.error) && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>
            {getErrorMessage(error ?? crearCarro.error)}
          </AlertDescription>
        </Alert>
      )}

      <Card className="mb-4">
        <CardHeader>
          <CardTitle>Nuevo carro</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
            onSubmit={(e) => {
              e.preventDefault()
              crearCarro.mutate()
            }}
          >
            <div className="grid gap-1.5">
              <Label htmlFor="nombreCarro">Nombre</Label>
              <Input
                id="nombreCarro"
                value={nombreCarro}
                onChange={(e) => setNombreCarro(e.target.value)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="descCarro">Descripción</Label>
              <Input
                id="descCarro"
                value={descripcionCarro}
                onChange={(e) => setDescripcionCarro(e.target.value)}
              />
            </div>
            <Button
              type="submit"
              disabled={nombreCarro.trim() === "" || crearCarro.isPending}
            >
              Crear carro
            </Button>
          </form>
        </CardContent>
      </Card>

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}

      <div className="grid gap-3">
        {carros.map((carro) => {
          const abierto = carroAbierto === carro.id
          const editandoCarro = carroEnEdicion === carro.id
          return (
            <Card key={carro.id}>
              <CardHeader>
                {/* Nombre arriba y botones abajo en el teléfono, y recién
                    en la misma línea cuando hay ancho. Con `flex-wrap` los
                    dos en la misma fila, un nombre largo —"Carro EDUTEC"—
                    empujaba los botones a la línea siguiente y, por el
                    `justify-between`, los dejaba pegados a la derecha: cada
                    tarjeta ponía "Editar carro" en un lugar distinto según
                    cuánto medía su nombre. `min-w-0` + `break-words` es lo
                    que impide que un nombre sin espacios desborde la
                    tarjeta en vez de cortarse. */}
                <CardTitle className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                  <span className="min-w-0 break-words">{carro.nombre}</span>
                  {!editandoCarro && (
                    <span className="flex shrink-0 flex-wrap gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setCarroEnEdicion(carro.id)}
                      >
                        Editar carro
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        aria-expanded={abierto}
                        onClick={() => setCarroAbierto(abierto ? null : carro.id)}
                      >
                        {abierto ? "Ocultar PCs" : "Gestionar PCs"}
                      </Button>
                    </span>
                  )}
                </CardTitle>
                {carro.descripcion && !editandoCarro && (
                  <CardDescription>{carro.descripcion}</CardDescription>
                )}
                {editandoCarro && (
                  <EdicionDeCarro carro={carro} onListo={() => setCarroEnEdicion(null)} />
                )}
              </CardHeader>
              {abierto && (
                <CardContent>
                  <PCsAdmin carroId={carro.id} carros={carros} />
                </CardContent>
              )}
            </Card>
          )
        })}
      </div>
    </div>
  )
}
