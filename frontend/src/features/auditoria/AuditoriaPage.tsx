import { useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { Paginador } from "@/components/Paginador"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as auditoriaApi from "@/features/auditoria/api"
import {
  accionLegible,
  entidadLegible,
  type EntradaDeAuditoria,
  type FiltroDeAuditoria,
} from "@/features/auditoria/types"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaYHora } from "@/lib/fechas"
import { contar } from "@/lib/plural"

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

  const { data: opciones } = useQuery({
    queryKey: ["auditoria-opciones"],
    queryFn: auditoriaApi.opcionesDeAuditoria,
  })

  const { data, isLoading, error } = useQuery({
    queryKey: ["auditoria", filtro, pagina],
    queryFn: () => auditoriaApi.listarAuditoria(filtro, pagina),
  })

  // Cambiar un filtro vuelve a la primera página: quedarse en la 4 después de
  // acotar la búsqueda muestra una lista vacía que parece "no hay nada".
  const cambiar = (campo: keyof FiltroDeAuditoria, valor: string) => {
    setPagina(1)
    setFiltro((previo) => ({ ...previo, [campo]: valor || undefined }))
  }

  const entradas = data?.data ?? []
  const hayFiltro = Object.values(filtro).some(Boolean)

  return (
    <div className="mx-auto max-w-5xl">
      <EncabezadoDePagina
        titulo="Registro de auditoría"
        descripcion="Toda acción sensible del sistema queda registrada con quién la hizo, sobre qué, cuándo y desde qué dirección. No se puede editar ni borrar."
      />

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      <Card className="mb-4">
        <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div className="grid gap-1.5">
            <Label htmlFor="filtro-accion">Acción</Label>
            {/* Las opciones salen de lo que de verdad ocurrió en ESTA
                instalación, no del catálogo completo: ofrecer treinta acciones
                de las que la mitad nunca pasó convierte el selector en una
                lista de callejones sin salida. */}
            <Select
              id="filtro-accion"
              value={filtro.accion ?? ""}
              onChange={(e) => cambiar("accion", e.target.value)}
            >
              <option value="">Todas</option>
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
              onChange={(e) => cambiar("entidad", e.target.value)}
            >
              <option value="">Todo</option>
              {(opciones?.entidades ?? []).map((e) => (
                <option key={e} value={e}>
                  {entidadLegible(e)}
                </option>
              ))}
            </Select>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="filtro-desde">Desde</Label>
            <Input
              id="filtro-desde"
              type="date"
              value={filtro.desde ?? ""}
              onChange={(e) => cambiar("desde", e.target.value)}
            />
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="filtro-hasta">Hasta</Label>
            <Input
              id="filtro-hasta"
              type="date"
              value={filtro.hasta ?? ""}
              onChange={(e) => cambiar("hasta", e.target.value)}
            />
          </div>

          {hayFiltro && (
            <div className="sm:col-span-2 lg:col-span-4">
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

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}

      {!isLoading && entradas.length === 0 && (
        <Alert>
          <AlertDescription>
            {hayFiltro
              ? "No hay ninguna acción registrada que coincida con esos filtros."
              : "Todavía no hay ninguna acción registrada."}
          </AlertDescription>
        </Alert>
      )}

      {entradas.length > 0 && (
        <>
          <p className="text-muted-foreground mb-2 text-sm">
            {contar(data?.meta.total ?? 0, "acción", "acciones")} registradas.
          </p>
          <div className="grid gap-2">
            {entradas.map((entrada) => (
              <FilaDeAuditoria key={entrada.id} entrada={entrada} />
            ))}
          </div>
          {data && (
            <div className="mt-4">
              <Paginador
                meta={data.meta}
                onCambiarPagina={setPagina}
                etiqueta="acciones"
              />
            </div>
          )}
        </>
      )}
    </div>
  )
}

function FilaDeAuditoria({ entrada }: { entrada: EntradaDeAuditoria }) {
  const [detalleAbierto, setDetalleAbierto] = useState(false)
  const tieneDetalle = entrada.detalle !== undefined && entrada.detalle !== null

  return (
    <div className="grid gap-1 rounded-md border p-3">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <span className="font-medium">{accionLegible(entrada.accion)}</span>
        <span className="text-muted-foreground text-sm">
          {formatearFechaYHora(entrada.creadoEn)}
        </span>
      </div>

      <p className="text-muted-foreground text-sm">
        {/* Sin nombre, la cuenta se eliminó (RF-01.9). Se dice con todas las
            letras en vez de dejar el renglón a medias: que el actor ya no
            exista es parte de la respuesta, no un dato que falta. */}
        {entrada.actorNombre || "Una cuenta que después se eliminó"} ·{" "}
        {entidadLegible(entrada.entidad)}
        {entrada.ipOrigen && ` · desde ${entrada.ipOrigen}`}
      </p>

      {tieneDetalle && (
        <div>
          <Button
            variant="outline"
            size="sm"
            aria-expanded={detalleAbierto}
            onClick={() => setDetalleAbierto(!detalleAbierto)}
          >
            {detalleAbierto ? "Ocultar el detalle" : "Ver el detalle"}
          </Button>
          {detalleAbierto && (
            // Crudo a propósito: cada acción guarda lo suyo, con su propia
            // forma. Darle formato obligaría a conocer las treinta del catálogo
            // y a tocar esta pantalla con cada acción nueva — y lo que se
            // muestra mal es justo lo que se fue a buscar.
            <pre className="bg-muted mt-2 overflow-x-auto rounded-md p-2 text-xs">
              {JSON.stringify(entrada.detalle, null, 2)}
            </pre>
          )}
        </div>
      )}
    </div>
  )
}
