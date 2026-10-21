import { invalidarInventario } from "./invalidarInventario"
import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

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
import { AltaDeEquipo, EdicionDeEquipo } from "@/features/admin/FormularioEquipo"
import { IncidenciasDeEquipo } from "@/features/admin/IncidenciasDeEquipo"
import { LicenciasDeEquipo } from "@/features/admin/LicenciasDeEquipo"
import { MarcasHuerfanas } from "@/features/admin/MarcasHuerfanas"
import { PreferenciaDeCarro } from "@/features/admin/PreferenciaDeCarro"
import { PreferenciasDeEquipo } from "@/features/admin/PreferenciasDeEquipo"
import { OtrosEquipos } from "@/features/admin/OtrosEquipos"
import { PrestamosDeEquipo } from "@/features/admin/PrestamosDeEquipo"
import * as inventoryApi from "@/features/inventory/api"
import { CuentasDeEquipo } from "@/features/inventory/CuentasDeEquipo"
import type { ResultadoCascada } from "@/features/admin/types"
import {
  ETIQUETA_ESTADO_EQUIPO,
  TRANSICIONES_DE_ESTADO,
} from "@/features/inventory/types"
import type { Carro, EstadoEquipo, Equipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { BusquedaDeInventario } from "@/features/admin/BusquedaDeInventario"
import { useEnfocarEquipo } from "@/features/admin/useEnfocarEquipo"
import { EstadoBadge, TONO_PC } from "@/components/EstadoBadge"

const CARROS_KEY = ["carros"]

type CambioEstado = {
  equipo: Equipo
  nuevoEstado: EstadoEquipo
  motivo: string
}

function EquiposAdmin({
  carroId,
  carros,
  equipoDestacado,
}: {
  carroId: string
  carros: Carro[]
  equipoDestacado?: string | null
}) {
  const queryClient = useQueryClient()
  const [cambiando, setCambiando] = useState<CambioEstado | null>(null)
  const [dandoDeBaja, setDandoDeBaja] = useState<Equipo | null>(null)
  const [editando, setEditando] = useState<string | null>(null)
  const [viendoIncidencias, setViendoIncidencias] = useState<string | null>(null)
  const [viendoLicencias, setViendoLicencias] = useState<string | null>(null)
  const [viendoPreferencias, setViendoPreferencias] = useState<string | null>(null)
  const [viendoEntregas, setViendoEntregas] = useState<string | null>(null)
  const [viendoCuentas, setViendoCuentas] = useState<string | null>(null)
  const [verRetirados, setVerRetirados] = useState(false)
  const [agregando, setAgregando] = useState(false)

  const { data, isPending, error, isFetching, refetch } = useQuery({
    queryKey: ["equipos", carroId],
    queryFn: () => inventoryApi.listarEquiposDeCarro(carroId),
  })
  useEnfocarEquipo(
    equipoDestacado,
    !error && !!data?.data.some((e) => e.id === equipoDestacado)
  )

  const invalidar = () => invalidarInventario(queryClient)

  // Las dos operaciones cancelan reservas de otros docentes, así que las dos
  // tienen que decir cuántas.
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

  const reactivar = useMutation({
    mutationFn: (equipo: Equipo) => adminApi.reactivarEquipo(equipo.id),
    onSuccess: invalidar,
  })

  if (error)
    return (
      <ErrorDeConsulta
        mensaje="No pudimos cargar los equipos de este carro."
        reintentando={isFetching}
        onReintentar={() => {
          void refetch()
        }}
      />
    )
  if (isPending)
    return (
      <p role="status" className="text-muted-foreground text-sm">
        Cargando equipos…
      </p>
    )

  const todos = data?.data ?? []
  const equipos = todos.filter((equipo) => !equipo.dadoDeBaja)
  const retirados = todos.filter((equipo) => equipo.dadoDeBaja)

  return (
    <div className="grid gap-3">
      {/* Los errores de estas dos operaciones se muestran DENTRO del recuadro
          de confirmación, al lado del botón que se apretó, y no acá arriba.
          Con un carro de treinta máquinas, un error sobre la número 27 salía
          fuera de la pantalla y "Confirmar cambio" parecía no hacer nada. */}
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
        const cuentasAbiertas = viendoCuentas === equipo.id

        return (
          <div
            key={equipo.id}
            id={`equipo-inventario-${equipo.id}`}
            tabIndex={-1}
            className="grid gap-2 rounded-md border p-3 focus:outline-2 focus:outline-primary"
          >
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
                  <EstadoBadge tono={TONO_PC[equipo.estado]}>
                    {ETIQUETA_ESTADO_EQUIPO[equipo.estado]}
                  </EstadoBadge>
                </p>
                <p className="text-muted-foreground text-sm break-words">
                  N° serie {equipo.numeroSerie}
                  {equipo.softwareInstalado && ` · ${equipo.softwareInstalado}`}
                </p>
              </div>
              {!cambiandoEsta && !bajandoEsta && !editandoEsta && (
                <details open={equipoDestacado === equipo.id}>
                  <summary
                    aria-label={`Gestionar ${equipo.etiqueta} · ${carros.find((c) => c.id === carroId)?.nombre ?? "Carro"}`}
                    className="cursor-pointer py-3 text-sm font-medium"
                  >
                    Gestionar equipo
                  </summary>
                  <div className="flex flex-wrap gap-2">
                    {/* Solo las transiciones que el backend acepta: desde fuera
                      de servicio no hay ninguna, y el botón "→ Disponible"
                      que había ahí devolvía siempre un error. */}
                    {TRANSICIONES_DE_ESTADO[equipo.estado].map((e) => (
                      <Button
                        key={e}
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          // Sin esto, el error de un intento anterior sobre OTRA
                          // máquina aparece en este recuadro recién abierto.
                          cambiarEstado.reset()
                          setCambiando({ equipo, nuevoEstado: e, motivo: "" })
                        }}
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
                    {/* Con qué cuenta se entra a esta máquina (RF-03.22). El
                      panel también está en Computadoras, que es donde lo
                      consulta un docente; acá está porque es donde un Admin
                      viene a CARGAR lo que se sabe de un equipo, y buscarlo
                      en la otra pantalla obliga a irse a mitad del alta. */}
                    <Button
                      variant="outline"
                      size="sm"
                      aria-expanded={cuentasAbiertas}
                      onClick={() => setViendoCuentas(cuentasAbiertas ? null : equipo.id)}
                    >
                      Cómo entrar
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
                      onClick={() =>
                        setViendoEntregas(entregasAbiertas ? null : equipo.id)
                      }
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
                      onClick={() => {
                        darDeBaja.reset()
                        setDandoDeBaja(equipo)
                      }}
                    >
                      Dar de baja
                    </Button>
                  </div>
                </details>
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
                {cambiarEstado.error && (
                  <Alert variant="destructive">
                    <AlertDescription>
                      {getErrorMessage(cambiarEstado.error)}
                    </AlertDescription>
                  </Alert>
                )}
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

            {cuentasAbiertas && <CuentasDeEquipo equipo={equipo} />}

            {bajandoEsta && (
              <div className="grid gap-2 rounded-md border p-3">
                <p className="text-destructive text-sm">
                  Dar de baja el equipo {equipo.identificador} la saca del inventario y
                  cancela sus reservas futuras. Su historial de incidencias se conserva.
                </p>
                {/* Los identificadores se liberan al dar de baja (migración
                    005): sin decirlo acá, quien da de baja una notebook para
                    volver a cargarla —con otro tipo, o fuera del carro— no
                    tiene forma de saber que puede. */}
                <p className="text-muted-foreground text-sm">
                  Su número de serie y su lugar en el carro quedan libres para volver a
                  usarlos en el equipo que la reemplace.
                </p>
                {darDeBaja.error && (
                  <Alert variant="destructive">
                    <AlertDescription>
                      {getErrorMessage(darDeBaja.error)}
                    </AlertDescription>
                  </Alert>
                )}
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

      {/* Lo dado de baja. Va plegado y al final porque no es parte del trabajo
          de todos los días: se abre el día que alguien se equivocó de fila.
          Antes no se mostraba en ninguna parte, y eso convertía una baja
          equivocada en algo que sólo se arreglaba entrando a la base. */}
      {retirados.length > 0 && (
        <div className="grid gap-2">
          <Button
            variant="ghost"
            size="sm"
            className="justify-self-start"
            aria-expanded={verRetirados}
            onClick={() => setVerRetirados(!verRetirados)}
          >
            {verRetirados ? "Ocultar" : "Ver"} los {retirados.length} equipos dados de
            baja
          </Button>

          {verRetirados &&
            retirados.map((equipo) => (
              <div
                key={equipo.id}
                className="text-muted-foreground flex flex-col gap-2 rounded-md border border-dashed p-3 sm:flex-row sm:items-center sm:justify-between"
              >
                <span className="min-w-0 break-words">
                  {equipo.nombre ?? `PC ${equipo.identificador}`}
                  {equipo.numeroSerie && ` · ${equipo.numeroSerie}`}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  className="shrink-0"
                  disabled={reactivar.isPending}
                  onClick={() => reactivar.mutate(equipo)}
                >
                  Devolver al inventario
                </Button>
              </div>
            ))}

          {/* El error va acá abajo y no arriba de todo: dice cuál de las tres
              cosas que la baja liberó —el número, el nombre o la serie— se la
              llevó otro equipo, y se lee al lado del botón que se apretó. */}
          {reactivar.error && (
            <Alert variant="destructive">
              <AlertDescription>{getErrorMessage(reactivar.error)}</AlertDescription>
            </Alert>
          )}
        </div>
      )}

      {/* Mientras se edita un equipo no se muestra: los dos formularios tienen
          los mismos campos, y verlos juntos no deja saber cuál se está
          completando. */}
      {editando === null && (
        <div className="grid gap-2">
          <Button
            type="button"
            variant="outline"
            className="h-11 justify-self-start"
            aria-expanded={agregando}
            onClick={() => setAgregando(!agregando)}
          >
            {agregando ? "Ocultar alta de equipo" : "Agregar un equipo al carro"}
          </Button>
          <div hidden={!agregando}>
            <AltaDeEquipo carroId={carroId} />
          </div>
        </div>
      )}
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
      await invalidarInventario(queryClient)
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
  const [carroEnPreferencia, setCarroEnPreferencia] = useState<string | null>(null)
  const [carroEnBaja, setCarroEnBaja] = useState<Carro | null>(null)
  const [nombreCarro, setNombreCarro] = useState("")
  const [descripcionCarro, setDescripcionCarro] = useState("")
  const [verRetirados, setVerRetirados] = useState(false)
  const [nuevoCarroAbierto, setNuevoCarroAbierto] = useState(false)
  const [equipoDestacado, setEquipoDestacado] = useState<string | null>(null)

  // Los retirados se piden sólo cuando se los quiere ver, y la clave de la
  // consulta los incluye: si no, al encender el interruptor react-query
  // devolvería la lista vieja de la caché y no pasaría nada.
  const { data, isPending, isSuccess, error, isFetching, refetch } = useQuery({
    queryKey: [...CARROS_KEY, { retirados: verRetirados }],
    queryFn: () =>
      verRetirados
        ? inventoryApi.listarCarrosConRetirados()
        : inventoryApi.listarCarros(),
  })

  // Retirar un carro. El 409 por "todavía tiene equipos adentro" es el caso
  // normal, no el excepcional: nadie vacía un carro antes de decidir sacarlo.
  const darDeBajaCarro = useMutation({
    mutationFn: (carro: Carro) => adminApi.darDeBajaCarro(carro.id),
    onSuccess: async () => {
      setCarroEnBaja(null)
      await invalidarInventario(queryClient)
    },
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
      await invalidarInventario(queryClient)
    },
  })

  const consultaLista = isSuccess && !error
  const todos = consultaLista ? data.data : []
  const carros = todos.filter((carro) => !carro.dadoDeBaja)
  const retirados = todos.filter((carro) => carro.dadoDeBaja)

  return (
    <div className="mx-auto max-w-4xl">
      <EncabezadoDePagina
        titulo="Gestión del inventario"
        descripcion="Alta y edición de carros y equipos, y seguimiento de las incidencias reportadas."
      />
      <BusquedaDeInventario
        carros={carros}
        carrosListos={consultaLista}
        onAbrir={(equipo) => {
          setEquipoDestacado(equipo.id)
          if (equipo.carroId) setCarroAbierto(equipo.carroId)
          const elemento = document.getElementById(`equipo-inventario-${equipo.id}`)
          elemento?.scrollIntoView?.({ block: "center", behavior: "smooth" })
          elemento?.focus({ preventScroll: true })
        }}
      />

      {error && (
        <div className="mb-4">
          <ErrorDeConsulta
            mensaje="No pudimos cargar los carros del inventario."
            reintentando={isFetching}
            onReintentar={() => {
              void refetch()
            }}
          />
        </div>
      )}
      {crearCarro.error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(crearCarro.error)}</AlertDescription>
        </Alert>
      )}

      <Card className="mb-4">
        <CardHeader>
          <Button
            type="button"
            variant="outline"
            className="h-11 justify-self-start"
            aria-expanded={nuevoCarroAbierto}
            onClick={() => setNuevoCarroAbierto(!nuevoCarroAbierto)}
          >
            {nuevoCarroAbierto ? "Ocultar alta de carro" : "Nuevo carro"}
          </Button>
        </CardHeader>
        <CardContent hidden={!nuevoCarroAbierto}>
          <form
            className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
            onSubmit={(e) => {
              e.preventDefault()
              if (consultaLista && nombreCarro.trim() && !crearCarro.isPending)
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
              disabled={
                !consultaLista || nombreCarro.trim() === "" || crearCarro.isPending
              }
            >
              Crear carro
            </Button>
          </form>
        </CardContent>
      </Card>

      {/* Va antes que el inventario y no al final: es lo único de esta pantalla
          que aparece por algo que pasó en OTRO módulo, y pide una decisión. Al
          final, debajo de una lista de carros que puede ser larga, sería un
          aviso que nadie ve. No ocupa nada cuando no hay huérfanas. */}
      <MarcasHuerfanas />

      {/* Los equipos que no están en ningún carro van en su propia sección,
          antes de la lista de carros: no pertenecen a ninguno, y meterlos en
          un carro llamado "Sueltos" sería volver a la mentira que el modelo
          viene sacándose de encima. */}
      <div className="mb-4">
        <OtrosEquipos equipoDestacado={equipoDestacado} />
      </div>

      {isPending && (
        <p role="status" className="text-muted-foreground">
          Cargando carros…
        </p>
      )}
      {consultaLista && carros.length === 0 && (
        <p className="text-muted-foreground text-sm">No hay carros activos cargados.</p>
      )}

      <div className="grid gap-3">
        {carros.map((carro) => {
          const abierto = carroAbierto === carro.id
          const editandoCarro = carroEnEdicion === carro.id
          const bajandoCarro = carroEnBaja?.id === carro.id
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
                  {!editandoCarro && !bajandoCarro && (
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
                        onClick={() =>
                          setCarroEnPreferencia(
                            carroEnPreferencia === carro.id ? null : carro.id
                          )
                        }
                      >
                        Preferencia del carro
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        aria-expanded={abierto}
                        onClick={() => {
                          setEquipoDestacado(null)
                          setCarroAbierto(abierto ? null : carro.id)
                        }}
                      >
                        {abierto ? "Ocultar equipos" : "Gestionar equipos"}
                      </Button>
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => {
                          // Sin el reset, el 409 de un carro anterior aparece
                          // dentro de este recuadro recién abierto.
                          darDeBajaCarro.reset()
                          setCarroEnBaja(carro)
                        }}
                      >
                        Dar de baja el carro
                      </Button>
                    </span>
                  )}
                </CardTitle>
                {carro.descripcion && !editandoCarro && !bajandoCarro && (
                  <CardDescription>{carro.descripcion}</CardDescription>
                )}
                {editandoCarro && (
                  <EdicionDeCarro carro={carro} onListo={() => setCarroEnEdicion(null)} />
                )}
                {carroEnPreferencia === carro.id && (
                  <PreferenciaDeCarro
                    carroId={carro.id}
                    onListo={() => setCarroEnPreferencia(null)}
                  />
                )}
                {bajandoCarro && (
                  <div className="grid gap-2 rounded-md border p-3">
                    <p className="text-destructive text-sm">
                      Dar de baja «{carro.nombre}» lo saca de circulación: deja de
                      aparecer en los listados y en el selector de a qué carro va un
                      equipo.
                    </p>
                    {/* Las dos cosas que no se ven y cambian la decisión: que
                        tiene que estar vacío —el error que más va a salir— y
                        que el nombre se libera, que es lo que permite que el
                        carro nuevo se llame igual que el que reemplaza. */}
                    <p className="text-muted-foreground text-sm">
                      Solo se puede si no le queda ningún equipo activo adentro. Su nombre
                      queda libre para el carro que lo reemplace, y se lo puede devolver a
                      circulación desde «Ver los carros retirados».
                    </p>
                    {darDeBajaCarro.error && (
                      <Alert variant="destructive">
                        <AlertDescription>
                          {getErrorMessage(darDeBajaCarro.error)}
                        </AlertDescription>
                      </Alert>
                    )}
                    <div className="flex gap-2">
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={darDeBajaCarro.isPending}
                        onClick={() => darDeBajaCarro.mutate(carro)}
                      >
                        Confirmar la baja del carro
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setCarroEnBaja(null)}
                      >
                        Volver
                      </Button>
                    </div>
                  </div>
                )}
              </CardHeader>
              {abierto && (
                <CardContent>
                  <EquiposAdmin
                    carroId={carro.id}
                    carros={carros}
                    equipoDestacado={equipoDestacado}
                  />
                </CardContent>
              )}
            </Card>
          )
        })}
      </div>

      <CarrosRetirados
        retirados={retirados}
        mostrando={verRetirados}
        consultaLista={consultaLista}
        onAlternar={() => setVerRetirados(!verRetirados)}
      />
    </div>
  )
}

// Los carros dados de baja, y la forma de traerlos de vuelta.
//
// Un carro retirado no aparece en ningún listado —el selector de "dónde va este
// equipo" se llena con la misma consulta—, así que sin esta sección no había
// manera de llegar a uno para reactivarlo.
//
// El interruptor se muestra SIEMPRE, incluso sin retirados, porque es la única
// pista de que los retirados existen en alguna parte: escondido detrás de
// "si hay alguno" no se lo encuentra el día que hace falta.
function CarrosRetirados({
  retirados,
  mostrando,
  consultaLista,
  onAlternar,
}: {
  retirados: Carro[]
  mostrando: boolean
  consultaLista: boolean
  onAlternar: () => void
}) {
  const queryClient = useQueryClient()

  const reactivar = useMutation({
    mutationFn: (carro: Carro) => adminApi.reactivarCarro(carro.id),
    onSuccess: () => invalidarInventario(queryClient),
  })

  return (
    <div className="mt-4 grid gap-2">
      <Button
        variant="ghost"
        size="sm"
        className="justify-self-start"
        aria-expanded={mostrando}
        onClick={onAlternar}
      >
        {mostrando ? "Ocultar los carros retirados" : "Ver los carros retirados"}
      </Button>

      {mostrando && consultaLista && retirados.length === 0 && (
        <p className="text-muted-foreground text-sm">No hay ningún carro retirado.</p>
      )}

      {mostrando &&
        consultaLista &&
        retirados.map((carro) => (
          <div
            key={carro.id}
            className="text-muted-foreground flex flex-col gap-2 rounded-md border border-dashed p-3 sm:flex-row sm:items-center sm:justify-between"
          >
            <span className="min-w-0 break-words">{carro.nombre}</span>
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              disabled={reactivar.isPending}
              onClick={() => reactivar.mutate(carro)}
            >
              Devolver a circulación
            </Button>
          </div>
        ))}

      {/* Falla si otro carro se quedó con el nombre mientras éste estaba
          retirado — que es justo para lo que la baja lo liberó. */}
      {reactivar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(reactivar.error)}</AlertDescription>
        </Alert>
      )}
    </div>
  )
}
