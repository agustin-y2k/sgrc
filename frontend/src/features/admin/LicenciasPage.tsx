import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

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
}: {
  licencia: Licencia
  marcada: boolean
  onMarcar: (marcada: boolean) => void
  sugerencias: string[]
}) {
  const queryClient = useQueryClient()
  const [editando, setEditando] = useState(false)
  const [borrando, setBorrando] = useState(false)
  const [campos, setCampos] = useState<CamposLicencia>(() => desdeLicencia(licencia))

  const invalidar = () => queryClient.invalidateQueries({ queryKey: LICENCIAS_KEY })

  const guardar = useMutation({
    mutationFn: () =>
      adminApi.editarLicencia(licencia.id, {
        nombre: campos.nombre.trim(),
        diasDuracion: Number(campos.diasDuracion),
        diasAviso: Number(campos.diasAviso),
        ...aVencimientoDeclarado(campos.vencimiento),
      }),
    onSuccess: async () => {
      setEditando(false)
      await invalidar()
    },
  })

  const borrar = useMutation({
    mutationFn: () => adminApi.borrarLicencia(licencia.id),
    onSuccess: async () => {
      setBorrando(false)
      await invalidar()
    },
  })

  const renovar = useMutation({
    mutationFn: () => adminApi.renovarLicencias({ licenciaIds: [licencia.id] }),
    onSuccess: invalidar,
  })

  const error = guardar.error ?? borrar.error ?? renovar.error
  const sinFecha = licencia.fechaVencimiento == null

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
            disabled={sinFecha}
            aria-label={`Seleccionar ${licencia.nombre} de ${licencia.etiqueta}`}
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
              disabled={sinFecha || renovar.isPending}
              onClick={() => renovar.mutate()}
              title={
                sinFecha
                  ? "Primero cargá cuándo vence: renovar corre un contador que ya existe"
                  : undefined
              }
            >
              Renovar hoy
            </Button>
            <Button variant="outline" size="sm" onClick={() => setEditando(true)}>
              Editar
            </Button>
            <Button variant="outline" size="sm" onClick={() => setBorrando(true)}>
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
              disabled={borrar.isPending}
              onClick={() => borrar.mutate()}
            >
              Quitar
            </Button>
            <Button variant="outline" size="sm" onClick={() => setBorrando(false)}>
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
            guardar.mutate()
          }}
        >
          <CamposComunesDeLicencia
            idPrefijo={`editar-${licencia.id}`}
            valor={campos}
            onChange={setCampos}
            sugerencias={sugerencias}
          />
          <CamposDeVencimiento
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
            <Button type="submit" size="sm" disabled={guardar.isPending}>
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
        </form>
      )}
    </div>
  )
}

function AltaDeLicencias({ sugerencias }: { sugerencias: string[] }) {
  const queryClient = useQueryClient()
  const [abierto, setAbierto] = useState(false)
  const [campos, setCampos] = useState<CamposLicencia>(LICENCIA_VACIA)
  const [seleccionadas, setSeleccionadas] = useState<Set<string>>(new Set())
  const [resumen, setResumen] = useState<string | null>(null)

  const { data: carros } = useQuery({
    queryKey: ["carros"],
    queryFn: inventoryApi.listarCarros,
  })

  // El inventario completo en una sola consulta, y no los equipos de cada
  // carro por separado.
  const { data: todosLosEquipos } = useQuery({
    queryKey: ["equipos"],
    queryFn: () => inventoryApi.listarEquipos(),
  })

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

  const crear = useMutation({
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

  if (!abierto) {
    return (
      <Button variant="outline" onClick={() => setAbierto(true)}>
        Cargar una licencia
      </Button>
    )
  }

  return (
    <Card>
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
            setResumen(null)
            crear.mutate()
          }}
        >
          <CamposComunesDeLicencia
            idPrefijo="alta-licencia"
            valor={campos}
            onChange={setCampos}
            sugerencias={sugerencias}
          />
          <SelectorDeEquipos
            equipos={equipos}
            seleccionadas={seleccionadas}
            onChange={setSeleccionadas}
          />
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
            <Button type="submit" disabled={crear.isPending || seleccionadas.size === 0}>
              Cargar en {contar(seleccionadas.size, "equipo")}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setAbierto(false)
                setResumen(null)
              }}
            >
              Cerrar
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

/** La barra de renovación masiva: aparece solo cuando hay algo marcado. */
function RenovacionMasiva({ ids, onListo }: { ids: string[]; onListo: () => void }) {
  const queryClient = useQueryClient()
  const [renovadaEl, setRenovadaEl] = useState("")
  const [resumen, setResumen] = useState<string | null>(null)

  const renovar = useMutation({
    mutationFn: () =>
      adminApi.renovarLicencias({
        licenciaIds: ids,
        renovadaEl: renovadaEl || undefined,
      }),
    onSuccess: async (respuesta) => {
      const sinFecha = respuesta.sinFechaPrevia?.length ?? 0
      setResumen(
        sinFecha === 0
          ? `Se renovaron ${contar(respuesta.renovadas.length, "licencia")}.`
          : `Se renovaron ${respuesta.renovadas.length}. ${sinFecha} no se pudieron: primero hay que cargarles el vencimiento.`
      )
      await queryClient.invalidateQueries({ queryKey: LICENCIAS_KEY })
      onListo()
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
        {resumen && (
          <Alert>
            <AlertDescription>{resumen}</AlertDescription>
          </Alert>
        )}
        <div>
          <Button disabled={renovar.isPending} onClick={() => renovar.mutate()}>
            Renovar las {ids.length} seleccionadas
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

export function LicenciasPage() {
  const [marcadas, setMarcadas] = useState<Set<string>>(new Set())

  const { data, isLoading, error } = useQuery({
    queryKey: LICENCIAS_KEY,
    queryFn: adminApi.listarLicencias,
  })

  const licencias = data?.data ?? []

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
        descripcion="Qué software con vencimiento hay en cada equipo y cuántos días le quedan. El día antes de que venza —y el día que vence— les llega un mail a todos los administradores."
        accion={<AltaDeLicencias sugerencias={sugerencias} />}
      />

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {isLoading && <p className="text-muted-foreground text-sm">Cargando licencias…</p>}

      {!isLoading && licencias.length === 0 && (
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

      {licencias.length > 0 && (
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

          {marcadas.size > 0 && (
            <RenovacionMasiva
              ids={[...marcadas]}
              onListo={() => setMarcadas(new Set())}
            />
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
              />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
