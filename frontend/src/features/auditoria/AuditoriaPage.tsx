import { useEffect, useId, useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { Paginador } from "@/components/Paginador"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as adminApi from "@/features/admin/api"
import * as auditoriaApi from "@/features/auditoria/api"
import {
  accionLegible,
  entidadEnFrase,
  entidadDeFiltro,
  entidadLegible,
  type EntradaDeAuditoria,
  type FiltroDeAuditoria,
} from "@/features/auditoria/types"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { errorRangoFechas } from "@/lib/rango-fechas"
import { todasLasPaginas } from "@/lib/todas-las-paginas"
import { formatearFechaYHora } from "@/lib/fechas"
import { contar } from "@/lib/plural"
import { actorLegible, detalleLegible, resumenDelDetalle } from "./detalle"
import { DetalleDeAuditoria } from "./DetalleDeAuditoria"

/**
 * El registro de auditoría (docs/09-seguridad-rbac.md §5).
 *
 * Existe porque el registro se escribía desde siempre y no había forma de
 * leerlo: la única manera era entrar a la base con psql. Un registro que sólo
 * puede consultar quien tiene acceso de administrador de base de datos no está
 * disponible para el Admin que tiene la pregunta, y la pregunta —«¿quién borró
 * esto?»— aparece justo cuando ya no se puede reconstruir de memoria.
 */

const FILTRO_VACIO: FiltroDeAuditoria = {}

export function AuditoriaPage() {
  const [filtro, setFiltro] = useState<FiltroDeAuditoria>(FILTRO_VACIO)
  const [pagina, setPagina] = useState(1)
  const [actorElegido, setActorElegido] = useState<EntradaDeAuditoria>()
  const errorFechas = errorRangoFechas(filtro.desde, filtro.hasta, 366)

  const consultaOpciones = useQuery({
    queryKey: ["auditoria-opciones"],
    queryFn: auditoriaApi.opcionesDeAuditoria,
  })

  // Quién: se recorren todas las páginas para no omitir personas.
  // Se pide acá y no se escribe el id a mano porque el filtro del backend es
  // por id, y nadie tiene a mano el UUID de una persona.
  const consultaUsuarios = useQuery({
    queryKey: ["usuarios", "para-auditoria"],
    queryFn: () =>
      todasLasPaginas((pagina) =>
        adminApi.listarUsuarios({
          pageSize: 200,
          ...(pagina > 1 ? { page: pagina } : {}),
        })
      ),
  })

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: ["auditoria", filtro, pagina],
    queryFn: () => auditoriaApi.listarAuditoria(filtro, pagina),
    enabled: !errorFechas,
  })

  const opciones = consultaOpciones.data
  const usuarios = consultaUsuarios.data
  const opcionesListas = consultaOpciones.isSuccess && !consultaOpciones.error
  const usuariosListos = consultaUsuarios.isSuccess && !consultaUsuarios.error
  useEffect(() => {
    if (!data || error || isFetching || errorFechas) return
    const ultima = Math.max(1, Math.ceil(data.meta.total / data.meta.pageSize))
    if (pagina > ultima) setPagina(ultima)
  }, [data, error, isFetching, errorFechas, pagina])

  // Cambiar un filtro vuelve a la primera página: quedarse en la 4 después de
  // acotar la búsqueda muestra una lista vacía que parece "no hay nada".
  const cambiar = (campo: keyof FiltroDeAuditoria, valor: string) => {
    setPagina(1)
    setFiltro((previo) => ({
      ...previo,
      ...(campo === "entidad" ? { entidadId: undefined } : {}),
      [campo]: valor || undefined,
    }))
  }

  const entradas = data?.data ?? []
  const hayFiltro = Object.values(filtro).some(Boolean)

  return (
    <div className="mx-auto max-w-5xl">
      <EncabezadoDePagina
        titulo="Registro de auditoría"
        descripcion="Consultá quién cambió un equipo, una reserva o una cuenta y cuándo ocurrió. Este historial ayuda a investigar errores y revisar el uso de los permisos; no se puede editar ni borrar."
      />

      <p className="text-muted-foreground mb-4 text-sm">
        Una acción y los cambios que produjo pueden aparecer en entradas separadas. Los
        nombres de las fichas se muestran como figuran hoy.
      </p>

      {errorFechas && (
        <Alert variant="destructive" className="mb-4" id="error-fechas-auditoria">
          <AlertDescription>{errorFechas}</AlertDescription>
        </Alert>
      )}
      {!errorFechas && error && (
        <div className="mb-4">
          <ErrorDeConsulta
            mensaje="No pudimos consultar el registro de auditoría."
            reintentando={isFetching}
            onReintentar={() => void refetch()}
          />
        </div>
      )}
      {consultaOpciones.error && (
        <div className="mb-4">
          <ErrorDeConsulta
            mensaje="No pudimos consultar las acciones y los tipos de ficha para filtrar."
            reintentando={consultaOpciones.isFetching}
            onReintentar={() => void consultaOpciones.refetch()}
          />
        </div>
      )}
      {consultaUsuarios.error && (
        <div className="mb-4">
          <ErrorDeConsulta
            mensaje="No pudimos consultar las personas para filtrar."
            reintentando={consultaUsuarios.isFetching}
            onReintentar={() => void consultaUsuarios.refetch()}
          />
        </div>
      )}

      <Card className="mb-4">
        <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <div className="grid gap-1.5">
            <Label htmlFor="filtro-accion">Acción</Label>
            {/* Las opciones salen de lo que de verdad ocurrió en ESTA
                instalación, no del catálogo completo: ofrecer treinta acciones
                de las que la mitad nunca pasó convierte el selector en una
                lista de callejones sin salida. */}
            <Select
              id="filtro-accion"
              value={filtro.accion ?? ""}
              disabled={!opcionesListas}
              onChange={(e) => cambiar("accion", e.target.value)}
            >
              <option value="">
                {consultaOpciones.isPending
                  ? "Cargando acciones…"
                  : !opcionesListas
                    ? "Acciones no disponibles"
                    : "Todas"}
              </option>
              {(opciones?.acciones ?? []).map((a) => (
                <option key={a} value={a}>
                  {accionLegible(a)}
                </option>
              ))}
            </Select>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="filtro-entidad">Sobre qué</Label>
            <Select
              id="filtro-entidad"
              value={filtro.entidad ?? ""}
              disabled={!opcionesListas}
              onChange={(e) => cambiar("entidad", e.target.value)}
            >
              <option value="">
                {consultaOpciones.isPending
                  ? "Cargando tipos de ficha…"
                  : !opcionesListas
                    ? "Tipos no disponibles"
                    : "Todo"}
              </option>
              {(opciones?.entidades ?? []).map((e) => (
                <option key={e} value={e}>
                  {entidadLegible(e)}
                </option>
              ))}
            </Select>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="filtro-usuario">Quién</Label>
            <Select
              id="filtro-usuario"
              value={filtro.usuarioId ?? ""}
              disabled={!usuariosListos}
              onChange={(e) => cambiar("usuarioId", e.target.value)}
            >
              <option value="">
                {consultaUsuarios.isPending
                  ? "Cargando personas…"
                  : !usuariosListos
                    ? "Personas no disponibles"
                    : "Cualquiera"}
              </option>
              {filtro.usuarioId &&
                !(usuarios?.data ?? []).some((u) => u.id === filtro.usuarioId) && (
                  <option value={filtro.usuarioId}>
                    {actorElegido && actorElegido.actorId === filtro.usuarioId
                      ? actorLegible(actorElegido)
                      : "Cuenta que ya no figura en usuarios"}
                  </option>
                )}
              {(usuarios?.data ?? []).map((u) => (
                <option key={u.id} value={u.id}>
                  {u.apellido}, {u.nombre}
                </option>
              ))}
            </Select>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="filtro-desde">Desde</Label>
            <Input
              id="filtro-desde"
              type="date"
              max="9999-12-31"
              aria-invalid={!!errorFechas}
              aria-describedby={errorFechas ? "error-fechas-auditoria" : undefined}
              value={filtro.desde ?? ""}
              onChange={(e) => cambiar("desde", e.target.value)}
            />
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="filtro-hasta">Hasta</Label>
            <Input
              id="filtro-hasta"
              type="date"
              max="9999-12-31"
              aria-invalid={!!errorFechas}
              aria-describedby={errorFechas ? "error-fechas-auditoria" : undefined}
              value={filtro.hasta ?? ""}
              onChange={(e) => cambiar("hasta", e.target.value)}
            />
          </div>

          {/* El id de una ficha no se escribe: se llega acá desde el botón de
              una fila. Por eso no hay campo, pero sí hace falta decir que está
              puesto — si no, la lista aparece acotada sin explicación. */}
          {filtro.entidadId && (
            <div className="text-muted-foreground flex flex-wrap items-center gap-2 text-sm sm:col-span-2 lg:col-span-5">
              <span>
                Mostrando solo lo que le pasó a{" "}
                {filtro.entidad
                  ? entidadEnFrase(filtro.entidad).replace(/^est[ea] /, "")
                  : "una ficha"}{" "}
                en particular.
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => cambiar("entidadId", "")}
              >
                Ver todas
              </Button>
            </div>
          )}

          {hayFiltro && (
            <div className="sm:col-span-2 lg:col-span-5">
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setFiltro(FILTRO_VACIO)
                  setPagina(1)
                }}
              >
                Quitar los filtros
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {!errorFechas && isLoading && <p className="text-muted-foreground">Cargando…</p>}

      {!errorFechas && !isLoading && !error && entradas.length === 0 && (
        <Alert>
          <AlertDescription>
            {hayFiltro
              ? "No hay ninguna acción registrada que coincida con esos filtros."
              : "Todavía no hay ninguna acción registrada."}
          </AlertDescription>
        </Alert>
      )}

      {!errorFechas && !error && entradas.length > 0 && (
        <>
          <p className="text-muted-foreground mb-2 text-sm">
            {contar(data?.meta.total ?? 0, "registro", "registros")} encontrados.
          </p>
          <div className="grid gap-2">
            {entradas.map((entrada) => (
              <FilaDeAuditoria
                key={entrada.id}
                entrada={entrada}
                filtradaPorFicha={filtro.entidadId !== undefined}
                filtradaPorActor={filtro.usuarioId === entrada.actorId}
                onVerElActor={() => {
                  setActorElegido(entrada)
                  setPagina(1)
                  setFiltro((previo) => ({ ...previo, usuarioId: entrada.actorId }))
                }}
                onVerLaFicha={() => {
                  // La entidad va junto con el id: el mismo UUID no se repite
                  // entre tablas, pero decir cuál es hace que el aviso de
                  // arriba pueda nombrarla.
                  setPagina(1)
                  setFiltro((previo) => ({
                    ...previo,
                    entidad: entidadDeFiltro(entrada.entidad),
                    entidadId: entrada.entidadId,
                  }))
                }}
              />
            ))}
          </div>
          {data && (
            <div className="mt-4">
              <Paginador
                meta={data.meta}
                onCambiarPagina={setPagina}
                etiqueta="registros"
              />
            </div>
          )}
        </>
      )}
    </div>
  )
}

function FilaDeAuditoria({
  entrada,
  filtradaPorFicha,
  onVerLaFicha,
  filtradaPorActor,
  onVerElActor,
}: {
  entrada: EntradaDeAuditoria
  /** Ya se está mirando una ficha sola: ofrecerlo de nuevo no lleva a ningún lado. */
  filtradaPorFicha: boolean
  onVerLaFicha: () => void
  filtradaPorActor: boolean
  onVerElActor: () => void
}) {
  const [detalleAbierto, setDetalleAbierto] = useState(false)
  const detalleId = useId()
  const detalle = detalleLegible(entrada)
  const resumen = resumenDelDetalle(detalle)
  const tieneDetalle = entrada.detalle !== undefined && entrada.detalle !== null

  return (
    <div
      role="article"
      aria-label={`${detalle.titulo}: ${entrada.entidadNombre ?? entidadLegible(entrada.entidad)}`}
      className="grid min-w-0 gap-1 rounded-md border p-3 [overflow-wrap:anywhere]"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <span className="font-medium">{detalle.titulo}</span>
        <span className="text-muted-foreground text-sm">
          {formatearFechaYHora(entrada.creadoEn)}
        </span>
      </div>

      <p className="text-muted-foreground text-sm">
        {actorLegible(entrada)} · {entidadLegible(entrada.entidad)}
        {entrada.ipOrigen && (
          <span title="Dirección de la conexión; una red compartida puede mostrar la misma para varias personas.">{` · desde ${entrada.ipOrigen}`}</span>
        )}
      </p>

      {entrada.entidadNombre && (
        <p className="text-sm font-medium wrap-anywhere">{entrada.entidadNombre}</p>
      )}
      {resumen && <p className="text-sm wrap-anywhere">{resumen}</p>}
      {entrada.actorId === "00000000-0000-0000-0000-000000000000" && (
        <p className="text-muted-foreground text-xs">
          Puede corresponder a una tarea automática o a un cambio realizado fuera de la
          aplicación.
        </p>
      )}

      <div className="flex flex-wrap gap-2">
        {!filtradaPorActor && (
          <Button
            variant="outline"
            size="sm"
            className="h-auto min-h-11 max-w-full whitespace-normal text-left sm:min-h-7"
            onClick={onVerElActor}
          >
            {entrada.actorId === "00000000-0000-0000-0000-000000000000"
              ? "Ver acciones sin usuario identificado"
              : "Ver acciones de esta cuenta"}
          </Button>
        )}
        {/* «Todo lo que le pasó a esta cosa» es media razón de ser de un
            registro de auditoría, y el backend ya la sabía contestar. Se llega
            desde la fila y no desde un campo porque lo que el filtro pide es un
            UUID, que nadie tiene a mano ni reconoce si lo ve. */}
        {entrada.entidadId && !filtradaPorFicha && (
          <Button
            variant="outline"
            size="sm"
            className="h-auto min-h-11 max-w-full whitespace-normal text-left sm:min-h-7"
            onClick={onVerLaFicha}
          >
            Ver todo lo de {entidadEnFrase(entrada.entidad)}
          </Button>
        )}
      </div>

      {tieneDetalle && (
        <div>
          <Button
            variant="outline"
            size="sm"
            aria-expanded={detalleAbierto}
            aria-controls={detalleAbierto ? detalleId : undefined}
            onClick={() => setDetalleAbierto(!detalleAbierto)}
          >
            {detalleAbierto ? "Ocultar el detalle" : "Ver el detalle"}
          </Button>
          {detalleAbierto && (
            <div id={detalleId}>
              <DetalleDeAuditoria detalle={detalle} />
            </div>
          )}
        </div>
      )}
    </div>
  )
}
