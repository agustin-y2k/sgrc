import { useMemo, useState } from "react"
import {
  useIsMutating,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { fechaDeLicenciaValida, licenciaValida } from "./validacionLicencia"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { EstadoBadge, TONO_LICENCIA } from "@/components/EstadoBadge"
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
import {
  aVencimientoDeclarado,
  CamposComunesDeLicencia,
  CamposDeVencimiento,
  desdeLicencia,
  LICENCIA_VACIA,
  SelectorDeEquipos,
  VENCIMIENTO_VACIO,
  type CamposLicencia,
} from "@/features/admin/FormularioLicencia"
import * as inventoryApi from "@/features/inventory/api"
import type { Licencia } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaLarga } from "@/lib/fechas"
import { contar, plural } from "@/lib/plural"

const LICENCIAS_KEY = ["licencias"]

/**
 * RF-03.11 a RF-03.14 — el contador de vencimiento de las licencias de
 * software.
 */

const ETIQUETA_ESTADO: Record<string, string> = {
  SIN_FECHA: "Falta cargar el vencimiento",
  VENCIDA: "Vencida",
  POR_VENCER: "Por vencer",
  VIGENTE: "Vigente",
}

/** El contador, en castellano. */
function textoDelContador(l: Licencia): string {
  if (l.diasRestantes == null) return "Sin fecha de vencimiento"
  const d = l.diasRestantes
  if (d > 1) return `Vence en ${d} días`
  if (d === 1) return "Vence mañana"
  if (d === 0) return "Vence hoy"
  if (d === -1) return "Venció ayer"
  return `Venció hace ${-d} días`
}

function FilaDeLicencia({
  licencia,
  marcada,
  onMarcar,
  sugerencias,
  consultaLista,
  ocupada,
}: {
  licencia: Licencia
  marcada: boolean
  onMarcar: (marcada: boolean) => void
  sugerencias: string[]
  consultaLista: boolean
  ocupada: boolean
}) {
  const queryClient = useQueryClient()
  const [editando, setEditando] = useState(false)
  const [borrando, setBorrando] = useState(false)
  const [campos, setCampos] = useState<CamposLicencia>(() => desdeLicencia(licencia))
  const [baseEdicion, setBaseEdicion] = useState(() =>
    JSON.stringify(desdeLicencia(licencia))
  )

  const invalidar = () => queryClient.invalidateQueries({ queryKey: LICENCIAS_KEY })

  const guardar = useMutation({
    mutationKey: LICENCIAS_KEY,
    mutationFn: () =>
      adminApi.editarLicencia(licencia.id, {
        nombre: campos.nombre.trim(),
        diasDuracion: Number(campos.diasDuracion),
        diasAviso: Number(campos.diasAviso),
        ...(campos.vencimiento.forma === "vence-el" &&
        campos.vencimiento.venceEl === licencia.fechaVencimiento
          ? {}
          : aVencimientoDeclarado(campos.vencimiento)),
      }),
    onSuccess: async () => {
      setEditando(false)
      await invalidar()
    },
  })

  const borrar = useMutation({
    mutationKey: LICENCIAS_KEY,
    mutationFn: () => adminApi.borrarLicencia(licencia.id),
    onSuccess: async () => {
      setBorrando(false)
      await invalidar()
    },
  })

  const renovar = useMutation({
    mutationKey: LICENCIAS_KEY,
    mutationFn: () => adminApi.renovarLicencias({ licenciaIds: [licencia.id] }),
    onSuccess: invalidar,
  })

  const error = guardar.error ?? borrar.error ?? renovar.error
  const sinFecha = licencia.fechaVencimiento == null
  const pendiente = ocupada || guardar.isPending || borrar.isPending || renovar.isPending
  const lista = consultaLista && !pendiente
  const edicionVigente = baseEdicion === JSON.stringify(desdeLicencia(licencia))

  return (
    <div className="grid gap-2 rounded-md border p-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 items-start gap-2">
          {/* La casilla solo tiene sentido si se puede renovar: una licencia
              sin fecha no se renueva, se carga. */}
          <input
            type="checkbox"
            className="mt-1"
            checked={marcada}
            disabled={sinFecha || !lista}
            aria-label={`Seleccionar ${licencia.nombre} de ${licencia.etiqueta}${licencia.carroNombre ? ` · ${licencia.carroNombre}` : ""}`}
            onChange={(e) => onMarcar(e.target.checked)}
          />
          <div className="min-w-0">
            <p className="font-medium">
              {licencia.nombre}{" "}
              <EstadoBadge tono={TONO_LICENCIA[licencia.estado] ?? "neutro"}>
                {ETIQUETA_ESTADO[licencia.estado] ?? licencia.estado}
              </EstadoBadge>
            </p>
            <p className="text-muted-foreground text-sm break-words">
              {licencia.etiqueta}
              {licencia.carroNombre && ` · ${licencia.carroNombre}`} ·{" "}
              {textoDelContador(licencia)}
              {licencia.fechaVencimiento &&
                ` (${formatearFechaLarga(licencia.fechaVencimiento)})`}
            </p>
            <p className="text-muted-foreground text-xs">
              Renovación de {licencia.diasDuracion} días · avisa{" "}
              {licencia.diasAviso === 0
                ? "el día que vence"
                : `${contar(licencia.diasAviso, "día")} antes`}
              {licencia.ultimaRenovacion &&
                ` · se renovó el ${formatearFechaLarga(licencia.ultimaRenovacion)}`}
            </p>
          </div>
        </div>

        {!editando && !borrando && (
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button
              size="sm"
              disabled={sinFecha || !lista}
              onClick={() => renovar.mutate()}
              title={
                sinFecha
                  ? "Primero cargá cuándo vence: renovar corre un contador que ya existe"
                  : undefined
              }
            >
              Renovar hoy
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={!lista}
              onClick={() => {
                guardar.reset()
                borrar.reset()
                renovar.reset()
                setCampos(desdeLicencia(licencia))
                setBaseEdicion(JSON.stringify(desdeLicencia(licencia)))
                setEditando(true)
              }}
            >
              Editar
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={!lista}
              onClick={() => {
                guardar.reset()
                borrar.reset()
                renovar.reset()
                setBorrando(true)
              }}
            >
              Quitar
            </Button>
          </div>
        )}
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {borrando && (
        <div className="grid gap-2 rounded-md border border-dashed p-3">
          <p className="text-sm">
            ¿Quitar la licencia de {licencia.nombre} de {licencia.etiqueta}? Deja de
            aparecer en la lista y de avisar. No toca la máquina.
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="destructive"
              size="sm"
              disabled={!lista}
              onClick={() => borrar.mutate()}
            >
              Quitar
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={pendiente}
              onClick={() => setBorrando(false)}
            >
              Cancelar
            </Button>
          </div>
        </div>
      )}

      {editando && (
        <form
          className="grid gap-3 rounded-md border border-dashed p-3"
          onSubmit={(e) => {
            e.preventDefault()
            if (lista && edicionVigente && licenciaValida(campos)) guardar.mutate()
          }}
        >
          <fieldset disabled={pendiente} className="contents">
            <CamposComunesDeLicencia
              idPrefijo={`editar-${licencia.id}`}
              valor={campos}
              onChange={setCampos}
              sugerencias={sugerencias}
            />
            <CamposDeVencimiento
              edicion
              idPrefijo={`editar-venc-${licencia.id}`}
              valor={campos.vencimiento}
              onChange={(vencimiento) => setCampos({ ...campos, vencimiento })}
            />
            {/* Cambiar los días de duración no mueve el vencimiento que ya
              está cargado: la licencia instalada hoy se compró con las
              condiciones viejas y sigue venciendo cuando vencía. Lo que
              cambia es cuánto va a durar la próxima renovación. */}
            <p className="text-muted-foreground text-xs">
              Cambiar los días de duración no mueve el vencimiento actual: aplica a la
              próxima renovación. Para recalcularlo ahora, elegí “Se renovó el…” con la
              fecha de la última renovación.
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                type="submit"
                size="sm"
                disabled={!lista || !edicionVigente || !licenciaValida(campos)}
              >
                Guardar
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  setCampos(desdeLicencia(licencia))
                  setEditando(false)
                }}
              >
                Cancelar
              </Button>
            </div>
          </fieldset>
          {!edicionVigente && (
            <p role="status">
              Esta licencia cambió mientras la editabas. Cancelá y volvé a abrir la
              edición para revisar sus datos actuales.
            </p>
          )}
        </form>
      )}
    </div>
  )
}

/**
 * El panel para cargar una licencia en varios equipos a la vez.
 *
 * Quien decide si está abierto es la PÁGINA, no este componente, y no es un
 * detalle de estilo: el botón vive en la acción del encabezado, que es un slot
 * `shrink-0` pensado para un control chico. Cuando este panel se renderizaba
 * ahí adentro, se negaba a encoger y aplastaba el título y la descripción de
 * la pantalla hasta dejarlos en una palabra por renglón. Ahora el encabezado
 * recibe el botón y el panel se dibuja debajo, a lo ancho de la página.
 */
function AltaDeLicencias({
  sugerencias,
  onCerrar,
}: {
  sugerencias: string[]
  onCerrar: () => void
}) {
  const queryClient = useQueryClient()
  const [campos, setCampos] = useState<CamposLicencia>(LICENCIA_VACIA)
  const [seleccionadas, setSeleccionadas] = useState<Set<string>>(new Set())
  const [resumen, setResumen] = useState<string | null>(null)

  const consultaCarros = useQuery({
    queryKey: ["carros"],
    queryFn: inventoryApi.listarCarros,
  })

  // El inventario completo en una sola consulta, y no los equipos de cada
  // carro por separado.
  const consultaEquipos = useQuery({
    queryKey: ["equipos"],
    queryFn: () => inventoryApi.listarEquipos(),
  })

  const carros = consultaCarros.data
  const todosLosEquipos = consultaEquipos.data
  const consultaLista = [consultaCarros, consultaEquipos].every(
    (q) => q.isSuccess && !q.error && !q.isFetching
  )
  const errorConsulta = consultaCarros.error ?? consultaEquipos.error
  const equipos = useMemo(() => {
    const nombreDeCarro = new Map((carros?.data ?? []).map((c) => [c.id, c.nombre]))
    return (todosLosEquipos?.data ?? [])
      .filter((equipo) => !equipo.dadoDeBaja)
      .map((equipo) => ({
        id: equipo.id,
        etiqueta: equipo.etiqueta,
        // Vacío en los sueltos: no cuelgan de ningún carro, y el formulario
        // omite el paréntesis en vez de mostrarlo vacío (RF-03.17).
        carroNombre: (equipo.carroId && nombreDeCarro.get(equipo.carroId)) || "",
      }))
  }, [carros, todosLosEquipos])

  const seleccionValida =
    seleccionadas.size > 0 &&
    [...seleccionadas].every((id) => equipos.some((e) => e.id === id))
  const listo = consultaLista && seleccionValida && licenciaValida(campos)

  const crear = useMutation({
    mutationKey: LICENCIAS_KEY,
    mutationFn: () =>
      adminApi.crearLicencias({
        equipoIds: [...seleccionadas],
        nombre: campos.nombre.trim(),
        diasDuracion: Number(campos.diasDuracion),
        diasAviso: Number(campos.diasAviso),
        ...aVencimientoDeclarado(campos.vencimiento),
      }),
    onSuccess: async (respuesta) => {
      const yaEstaban = respuesta.equiposQueYaLaTenian?.length ?? 0
      setResumen(
        yaEstaban === 0
          ? `Se cargó en ${contar(respuesta.creadas.length, "equipo")}.`
          : `Se cargó en ${contar(respuesta.creadas.length, "equipo")}. ${yaEstaban} ya la tenían y se dejaron como estaban.`
      )
      setCampos({ ...LICENCIA_VACIA, vencimiento: VENCIMIENTO_VACIO })
      setSeleccionadas(new Set())
      await queryClient.invalidateQueries({ queryKey: LICENCIAS_KEY })
    },
  })

  return (
    <Card className="mb-6">
      <CardHeader>
        <CardTitle>Cargar una licencia</CardTitle>
        <CardDescription>
          Se carga el mismo software en todos los equipos que elijas, de uno o varios
          carros. Cada máquina lleva su propio contador: si alguna queda sin renovar, se
          ve.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (listo && !crear.isPending) {
              setResumen(null)
              crear.mutate()
            }
          }}
        >
          <fieldset disabled={crear.isPending} className="contents">
            <CamposComunesDeLicencia
              idPrefijo="alta-licencia"
              valor={campos}
              onChange={setCampos}
              sugerencias={sugerencias}
            />
            {errorConsulta ? (
              <ErrorDeConsulta
                mensaje="No pudimos verificar el inventario para cargar licencias."
                reintentando={consultaCarros.isFetching || consultaEquipos.isFetching}
                onReintentar={() => {
                  void consultaCarros.refetch()
                  void consultaEquipos.refetch()
                }}
              />
            ) : !consultaLista ? (
              <p role="status">Consultando inventario…</p>
            ) : (
              <SelectorDeEquipos
                equipos={equipos}
                seleccionadas={seleccionadas}
                onChange={setSeleccionadas}
              />
            )}
            {seleccionadas.size > 0 && (
              <Button
                type="button"
                variant="outline"
                onClick={() => setSeleccionadas(new Set())}
              >
                Limpiar selección de equipos
              </Button>
            )}
            {consultaLista && seleccionadas.size > 0 && !seleccionValida && (
              <p role="status">
                Cambió el inventario. Limpiá la selección y revisá los equipos
                disponibles.
              </p>
            )}
            <CamposDeVencimiento
              idPrefijo="alta-venc"
              valor={campos.vencimiento}
              onChange={(vencimiento) => setCampos({ ...campos, vencimiento })}
            />

            {crear.error && (
              <Alert variant="destructive">
                <AlertDescription>{getErrorMessage(crear.error)}</AlertDescription>
              </Alert>
            )}
            {resumen && (
              <Alert>
                <AlertDescription>{resumen}</AlertDescription>
              </Alert>
            )}

            <div className="flex flex-wrap gap-2">
              <Button type="submit" disabled={crear.isPending || !listo}>
                Cargar en {contar(seleccionadas.size, "equipo")}
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setResumen(null)
                  onCerrar()
                }}
              >
                Cerrar
              </Button>
            </div>
          </fieldset>
        </form>
      </CardContent>
    </Card>
  )
}

/** La barra de renovación masiva: aparece solo cuando hay algo marcado. */
function RenovacionMasiva({
  ids,
  lista,
  onListo,
}: {
  ids: string[]
  lista: boolean
  onListo: (resumen: string) => void
}) {
  const queryClient = useQueryClient()
  const [renovadaEl, setRenovadaEl] = useState("")

  const renovar = useMutation({
    mutationKey: LICENCIAS_KEY,
    mutationFn: () =>
      adminApi.renovarLicencias({
        licenciaIds: ids,
        renovadaEl: renovadaEl || undefined,
      }),
    onSuccess: async (respuesta) => {
      const sinFecha = respuesta.sinFechaPrevia?.length ?? 0
      const resumen =
        sinFecha === 0
          ? `Se renovaron ${contar(respuesta.renovadas.length, "licencia")}.`
          : `Se renovaron ${respuesta.renovadas.length}. ${sinFecha} no se pudieron: primero hay que cargarles el vencimiento.`
      await queryClient.invalidateQueries({ queryKey: LICENCIAS_KEY })
      onListo(resumen)
    },
  })

  return (
    <Card>
      <CardContent className="grid gap-3">
        <p className="text-sm font-medium">
          {contar(ids.length, "licencia")} {plural(ids.length, "seleccionada")}
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="renovadas-el">Fecha en que se renovaron</Label>
            <Input
              id="renovadas-el"
              type="date"
              disabled={renovar.isPending}
              value={renovadaEl}
              onChange={(e) => setRenovadaEl(e.target.value)}
            />
            {/* Vacío = hoy, que es el caso normal. Con fecha, es el del
                olvido: se renovaron el martes y se cargan el jueves. */}
            <p className="text-muted-foreground text-xs">
              Vacío significa hoy. Si las renovaste otro día, poné esa fecha: el contador
              arranca ahí.
            </p>
          </div>
        </div>
        {renovar.error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(renovar.error)}</AlertDescription>
          </Alert>
        )}
        <div className="flex flex-wrap gap-2">
          <Button
            className="h-auto min-h-9 whitespace-normal"
            disabled={
              !lista ||
              renovar.isPending ||
              (!!renovadaEl && !fechaDeLicenciaValida(renovadaEl))
            }
            onClick={() => {
              if (
                lista &&
                !renovar.isPending &&
                (!renovadaEl || fechaDeLicenciaValida(renovadaEl))
              )
                renovar.mutate()
            }}
          >
            Renovar las {ids.length} seleccionadas
          </Button>
          <Button
            variant="outline"
            disabled={renovar.isPending}
            onClick={() => onListo("")}
          >
            Limpiar selección de licencias
          </Button>
        </div>
        {!lista && !renovar.isPending && (
          <p role="status">
            La selección cambió o no se pudo verificar. Limpiá la selección y revisá las
            licencias.
          </p>
        )}
      </CardContent>
    </Card>
  )
}

export function LicenciasPage() {
  const [marcadas, setMarcadas] = useState<Set<string>>(new Set())
  const [altaAbierta, setAltaAbierta] = useState(false)
  const [resumen, setResumen] = useState("")
  const renovando = useIsMutating({ mutationKey: LICENCIAS_KEY }) > 0

  const { data, isLoading, isSuccess, isFetching, error, refetch } = useQuery({
    queryKey: LICENCIAS_KEY,
    queryFn: adminApi.listarLicencias,
  })

  const licencias = data?.data ?? []
  const consultaLista = isSuccess && !error && !isFetching
  const seleccionValida =
    marcadas.size > 0 &&
    [...marcadas].every((id) =>
      licencias.some((l) => l.id === id && l.fechaVencimiento != null)
    )

  // Los nombres ya cargados, para que el formulario los ofrezca y no se
  // termine con "AutoCAD 2027" en un equipo y "Autocad 2027" en otra: serían
  // dos programas distintos en la lista, con dos contadores que nadie
  // relaciona.
  const sugerencias = useMemo(
    () => [...new Set((data?.data ?? []).map((l) => l.nombre))].sort(),
    [data]
  )

  const sinFecha = licencias.filter((l) => l.estado === "SIN_FECHA").length
  const vencidas = licencias.filter((l) => l.estado === "VENCIDA").length
  const porVencer = licencias.filter((l) => l.estado === "POR_VENCER").length

  const alternar = (id: string, marcada: boolean) => {
    const nueva = new Set(marcadas)
    if (marcada) nueva.add(id)
    else nueva.delete(id)
    setMarcadas(nueva)
  }

  return (
    <div>
      <EncabezadoDePagina
        titulo="Licencias de software"
        descripcion="Qué software con vencimiento hay en cada equipo y cuántos días le quedan. Configurá la anticipación de los avisos para cada licencia."
        accion={
          !altaAbierta && (
            <Button variant="outline" onClick={() => setAltaAbierta(true)}>
              Cargar una licencia
            </Button>
          )
        }
      />

      {altaAbierta && (
        <AltaDeLicencias
          sugerencias={sugerencias}
          onCerrar={() => setAltaAbierta(false)}
        />
      )}

      {error && (
        <ErrorDeConsulta
          mensaje="No pudimos consultar las licencias."
          reintentando={isFetching}
          onReintentar={() => void refetch()}
        />
      )}
      {resumen && (
        <Alert className="mb-4">
          <AlertDescription>{resumen}</AlertDescription>
        </Alert>
      )}
      {marcadas.size > 0 && (
        <RenovacionMasiva
          ids={[...marcadas]}
          lista={consultaLista && seleccionValida && !renovando}
          onListo={(mensaje) => {
            setResumen(mensaje)
            setMarcadas(new Set())
          }}
        />
      )}

      {isLoading && <p className="text-muted-foreground text-sm">Cargando licencias…</p>}

      {!isLoading && !error && licencias.length === 0 && (
        <Card>
          <CardContent>
            <p className="text-muted-foreground text-sm">
              Todavía no hay licencias cargadas. Si no sabés cuándo vence alguna, cargala
              igual sin fecha: queda marcada como pendiente de verificar y no avisa nada
              hasta que la completes.
            </p>
          </CardContent>
        </Card>
      )}

      {!error && licencias.length > 0 && (
        <div className="grid gap-4">
          {/* El resumen arriba: lo primero que se quiere saber al entrar es
              si hay algo que resolver hoy, no la lista entera. */}
          {(sinFecha > 0 || vencidas > 0 || porVencer > 0) && (
            <Alert>
              <AlertDescription>
                {[
                  vencidas > 0 && `${contar(vencidas, "vencida")}`,
                  porVencer > 0 && `${porVencer} por vencer`,
                  sinFecha > 0 && `${sinFecha} sin fecha cargada`,
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </AlertDescription>
            </Alert>
          )}

          {/* El orden lo decide el backend: primero las que no tienen fecha
              (hay que ir a mirar la máquina), después de la más vencida a la
              que más le falta. No se reordena acá para que la pantalla y el
              criterio de los avisos no puedan discrepar. */}
          <div className="grid gap-3">
            {licencias.map((l) => (
              <FilaDeLicencia
                key={l.id}
                licencia={l}
                marcada={marcadas.has(l.id)}
                onMarcar={(m) => alternar(l.id, m)}
                sugerencias={sugerencias}
                consultaLista={consultaLista}
                ocupada={renovando}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
