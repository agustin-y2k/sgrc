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
import { AvisoDeCascada } from "@/features/admin/AvisoDeCascada"
import { AltaDeEquipo, EdicionDeEquipo } from "@/features/admin/FormularioEquipo"
import { IncidenciasDeEquipo } from "@/features/admin/IncidenciasDeEquipo"
import { LicenciasDeEquipo } from "@/features/admin/LicenciasDeEquipo"
import { PreferenciasDeEquipo } from "@/features/admin/PreferenciasDeEquipo"
import { OtrosEquipos } from "@/features/admin/OtrosEquipos"
import { PrestamosDeEquipo } from "@/features/admin/PrestamosDeEquipo"
import * as inventoryApi from "@/features/inventory/api"
import type { ResultadoCascada } from "@/features/admin/types"
import { ETIQUETA_ESTADO_EQUIPO } from "@/features/inventory/types"
import type { Carro, EstadoEquipo, Equipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"

const CARROS_KEY = ["carros"]

type CambioEstado = {
  equipo: Equipo
  nuevoEstado: EstadoEquipo
  motivo: string
}

function EquiposAdmin({ carroId, carros }: { carroId: string; carros: Carro[] }) {
  const queryClient = useQueryClient()
  const [cambiando, setCambiando] = useState<CambioEstado | null>(null)
  const [dandoDeBaja, setDandoDeBaja] = useState<Equipo | null>(null)
  const [editando, setEditando] = useState<string | null>(null)
  const [viendoIncidencias, setViendoIncidencias] = useState<string | null>(null)
  const [viendoLicencias, setViendoLicencias] = useState<string | null>(null)
  const [viendoPreferencias, setViendoPreferencias] = useState<string | null>(null)
  const [viendoEntregas, setViendoEntregas] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ["equipos", carroId],
    queryFn: () => inventoryApi.listarEquiposDeCarro(carroId),
  })

  const invalidar = () =>
    queryClient.invalidateQueries({ queryKey: ["equipos", carroId] })

  // Las dos operaciones cancelan reservas de otros docentes, así que las dos
  // tienen que decir cuántas. Se avisa DESPUÉS y con el número real: antes de
  // apretar solo se puede advertir que va a pasar, no cuánto.
  const [cascada, setCascada] = useState<ResultadoCascada | null>(null)

  const cambiarEstado = useMutation({
    mutationFn: ({ equipo, nuevoEstado, motivo }: CambioEstado) =>
      adminApi.cambiarEstadoEquipo(equipo.id, nuevoEstado, motivo.trim() || undefined),
    onSuccess: async (resultado) => {
      setCambiando(null)
      setCascada(resultado)
      await invalidar()
    },
  })

  const darDeBaja = useMutation({
    mutationFn: (equipo: Equipo) => adminApi.darDeBajaEquipo(equipo.id),
    onSuccess: async (resultado) => {
      setDandoDeBaja(null)
      setCascada(resultado)
      await invalidar()
    },
  })

  if (isLoading) return <p className="text-muted-foreground text-sm">Cargando equipos…</p>

  const equipos = (data?.data ?? []).filter((equipo) => !equipo.dadoDeBaja)
  const error = cambiarEstado.error ?? darDeBaja.error

  return (
    <div className="grid gap-3">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      <AvisoDeCascada resultado={cascada} />

      {equipos.length === 0 && (
        <p className="text-muted-foreground text-sm">
          Este carro no tiene equipos activos. Agregá el primero con el formulario de
          abajo: sin equipos cargados nadie puede reservar.
        </p>
      )}

      {equipos.map((equipo) => {
        const cambiandoEsta = cambiando?.equipo.id === equipo.id
        const bajandoEsta = dandoDeBaja?.id === equipo.id
        const editandoEsta = editando === equipo.id
        const incidenciasAbiertas = viendoIncidencias === equipo.id
        const licenciasAbiertas = viendoLicencias === equipo.id
        const preferenciasAbiertas = viendoPreferencias === equipo.id
        const entregasAbiertas = viendoEntregas === equipo.id

        return (
          <div key={equipo.id} className="grid gap-2 rounded-md border p-3">
            {/* Identificador y número de serie arriba, la fila de botones
                abajo, SIEMPRE — no lado a lado a partir de `sm`.

                Antes lo eran, y con cinco botones entraba. Hoy son ocho y el
                renglón de botones mide más que la tarjeta: como no podía
                encogerse, se quedaba con todo el ancho y al texto le tocaba
                un carácter, así que "N° serie 5CD1..." bajaba en vertical
                letra por letra. Al lado de una fila de botones que crece con
                cada función nueva, ningún texto está a salvo. */}
            <div className="grid gap-2">
              <div className="min-w-0">
                <p className="font-medium">
                  {equipo.etiqueta}{" "}
                  <Badge
                    variant={equipo.estado === "DISPONIBLE" ? "secondary" : "destructive"}
                  >
                    {ETIQUETA_ESTADO_EQUIPO[equipo.estado]}
                  </Badge>
                </p>
                <p className="text-muted-foreground text-sm break-words">
                  N° serie {equipo.numeroSerie}
                  {equipo.softwareInstalado && ` · ${equipo.softwareInstalado}`}
                </p>
              </div>
              {!cambiandoEsta && !bajandoEsta && !editandoEsta && (
                <div className="flex flex-wrap gap-2">
                  {(
                    [
                      "DISPONIBLE",
                      "EN_MANTENIMIENTO",
                      "FUERA_DE_SERVICIO",
                    ] as EstadoEquipo[]
                  )
                    .filter((e) => e !== equipo.estado)
                    .map((e) => (
                      <Button
                        key={e}
                        variant="outline"
                        size="sm"
                        onClick={() =>
                          setCambiando({ equipo, nuevoEstado: e, motivo: "" })
                        }
                      >
                        → {ETIQUETA_ESTADO_EQUIPO[e]}
                      </Button>
                    ))}
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setEditando(equipo.id)}
                  >
                    Editar
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    aria-expanded={incidenciasAbiertas}
                    onClick={() =>
                      setViendoIncidencias(incidenciasAbiertas ? null : equipo.id)
                    }
                  >
                    Incidencias
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    aria-expanded={licenciasAbiertas}
                    onClick={() =>
                      setViendoLicencias(licenciasAbiertas ? null : equipo.id)
                    }
                  >
                    Licencias
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    aria-expanded={entregasAbiertas}
                    onClick={() => setViendoEntregas(entregasAbiertas ? null : equipo.id)}
                  >
                    Entregas
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    aria-expanded={preferenciasAbiertas}
                    onClick={() =>
                      setViendoPreferencias(preferenciasAbiertas ? null : equipo.id)
                    }
                  >
                    Preferencias
                  </Button>
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => setDandoDeBaja(equipo)}
                  >
                    Dar de baja
                  </Button>
                </div>
              )}
            </div>

            {/* RF-03.8: sacar un equipo de DISPONIBLE cancela sus reservas
                futuras, y RF-03.8 aclara que al volver a DISPONIBLE NO se
                restauran. Por eso se avisa antes, no después. */}
            {cambiandoEsta && cambiando && (
              <div className="grid gap-2 rounded-md border p-3">
                {cambiando.nuevoEstado !== "DISPONIBLE" ? (
                  <p className="text-destructive text-sm">
                    Pasar el equipo {equipo.identificador} a{" "}
                    {ETIQUETA_ESTADO_EQUIPO[cambiando.nuevoEstado].toLowerCase()} cancela
                    todas sus reservas futuras y avisa a cada docente. Si más adelante
                    vuelve a estar disponible, esas reservas no se restauran solas.
                  </p>
                ) : (
                  <p className="text-muted-foreground text-sm">
                    El equipo vuelve a estar disponible para reservar. Las reservas que se
                    cancelaron mientras no lo estaba no se recuperan.
                  </p>
                )}
                <div className="grid gap-1.5">
                  <Label htmlFor={`motivo-${equipo.id}`}>Motivo (opcional)</Label>
                  <Input
                    id={`motivo-${equipo.id}`}
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
              <EdicionDeEquipo
                equipo={equipo}
                carros={carros}
                onListo={() => setEditando(null)}
              />
            )}

            {incidenciasAbiertas && <IncidenciasDeEquipo equipoId={equipo.id} />}

            {licenciasAbiertas && <LicenciasDeEquipo equipoId={equipo.id} />}

            {entregasAbiertas && <PrestamosDeEquipo equipoId={equipo.id} />}

            {preferenciasAbiertas && <PreferenciasDeEquipo equipoId={equipo.id} />}

            {bajandoEsta && (
              <div className="grid gap-2 rounded-md border p-3">
                <p className="text-destructive text-sm">
                  Dar de baja el equipo {equipo.identificador} la saca del inventario y
                  cancela sus reservas futuras. Su historial de incidencias se conserva.
                </p>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="destructive"
                    disabled={darDeBaja.isPending}
                    onClick={() => darDeBaja.mutate(equipo)}
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

      {/* Mientras se edita un equipo no se muestra: los dos formularios tienen
          los mismos campos, y verlos juntos no deja saber cuál se está
          completando. */}
      {editando === null && <AltaDeEquipo carroId={carroId} />}
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

// RF-03: carros, equipos e incidencias. Es la pantalla desde la que se arma el
// inventario de cero — sin equipos cargados acá, nadie puede reservar nada.
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
        descripcion="Alta y edición de carros y equipos, y seguimiento de las incidencias reportadas."
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

      {/* Los equipos que no están en ningún carro van en su propia sección,
          antes de la lista de carros: no pertenecen a ninguno, y meterlos en
          un carro llamado "Sueltos" sería volver a la mentira que el modelo
          viene sacándose de encima. */}
      <div className="mb-4">
        <OtrosEquipos />
      </div>

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
                        {abierto ? "Ocultar equipos" : "Gestionar equipos"}
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
                  <EquiposAdmin carroId={carro.id} carros={carros} />
                </CardContent>
              )}
            </Card>
          )
        })}
      </div>
    </div>
  )
}
