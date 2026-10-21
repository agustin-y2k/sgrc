import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { EstadoBadge } from "@/components/EstadoBadge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as academicoApi from "@/features/academico/api"
import * as perfilApi from "@/features/perfil/api"
import {
  ETIQUETA_ESTADO_PEDIDO,
  materiaDelPedido,
  type PedidoDeMateria,
} from "@/features/perfil/types"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaLargaDeInstante } from "@/lib/fechas"

/** Los pedidos de docentes para dictar una materia más. */
export function PedidosDeMateriaPage() {
  const [soloPendientes, setSoloPendientes] = useState(true)

  const { data, error, isPending, isFetching, refetch } = useQuery({
    queryKey: ["pedidos-de-materia", soloPendientes],
    queryFn: () => perfilApi.listarPedidos(soloPendientes),
  })

  const pedidos = data?.data ?? []

  return (
    <div className="mx-auto grid max-w-3xl gap-4">
      <EncabezadoDePagina
        titulo="Pedidos para dictar una materia"
        descripcion="Aprobar deja a esa persona reservar computadoras para la materia. Conviene hablarlo antes con quien ya la da."
      />

      <div className="flex items-center gap-2">
        <Checkbox
          id="solo-pendientes"
          checked={soloPendientes}
          onCheckedChange={(v) => setSoloPendientes(v === true)}
        />
        <Label htmlFor="solo-pendientes">Ver solo los que faltan resolver</Label>
      </div>

      {error && (
        <ErrorDeConsulta
          mensaje="No pudimos consultar los pedidos de materia."
          reintentando={isFetching}
          onReintentar={() => void refetch()}
        />
      )}

      {isPending && <p role="status">Cargando pedidos…</p>}
      {!isPending && !error && pedidos.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground pt-6 text-sm">
            {soloPendientes ? "No hay pedidos sin resolver." : "Todavía no pidió nadie."}
          </CardContent>
        </Card>
      )}
      <fieldset disabled={!!error || isFetching} className="grid min-w-0 gap-3">
        {pedidos.map((p) => (
          <Pedido key={p.id} pedido={p} habilitado={!error && !isFetching} />
        ))}
      </fieldset>
    </div>
  )
}

function Pedido({
  pedido,
  habilitado,
}: {
  pedido: PedidoDeMateria
  habilitado: boolean
}) {
  const qc = useQueryClient()
  const [panel, setPanel] = useState<"aprobar" | "rechazar" | null>(null)
  const [respuesta, setRespuesta] = useState("")
  const [cursoId, setCursoId] = useState("")
  const [rol, setRol] = useState("")
  const [error, setError] = useState("")

  // Los cursos hacen falta solo para crear una materia que no existe.
  const consultaCiclos = useQuery({
    queryKey: ["ciclos"],
    queryFn: academicoApi.listarCiclos,
    enabled: pedido.esMateriaNueva && panel === "aprobar",
  })
  const cicloActivo = consultaCiclos.data?.data.find((c) => c.activo)
  const consultaCursos = useQuery({
    queryKey: ["cursos", cicloActivo?.id],
    queryFn: () => academicoApi.listarCursos(cicloActivo!.id),
    enabled: !!cicloActivo && panel === "aprobar" && pedido.esMateriaNueva,
  })

  // Quiénes la dan hoy: es con quien hay que hablar antes de decidir.
  const consultaDocentes = useQuery({
    queryKey: ["docentes-de-materia", pedido.materiaId],
    queryFn: () => academicoApi.listarDocentesDeMateria(pedido.materiaId!),
    enabled: !!pedido.materiaId,
  })

  const resolver = useMutation({
    mutationFn: (aprobar: boolean) => {
      if (!habilitado || (aprobar && pedido.esMateriaNueva && !cursoValido))
        throw new Error("Volvé a consultar los datos antes de confirmar.")
      return perfilApi.resolverPedido(pedido.id, {
        aprobar,
        respuesta,
        cursoId: cursoId || undefined,
        rol: rol || undefined,
      })
    },
    onSuccess: () => {
      setError("")
      setPanel(null)
      setRespuesta("")
      qc.invalidateQueries({ queryKey: ["pedidos-de-materia"] })
    },
    onError: (e) => setError(getErrorMessage(e)),
  })

  const catalogosListos =
    !!cicloActivo &&
    !consultaCiclos.error &&
    !consultaCiclos.isFetching &&
    !!consultaCursos.data &&
    !consultaCursos.error &&
    !consultaCursos.isFetching
  const cursoValido =
    catalogosListos && !!consultaCursos.data?.data.some((c) => c.id === cursoId)
  const faltaCurso = pedido.esMateriaNueva && !cursoValido
  const docentes = consultaDocentes.data

  return (
    <Card>
      <CardContent className="grid gap-2 pt-6">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="font-medium">{materiaDelPedido(pedido)}</span>
          <EstadoBadge
            tono={
              pedido.estado === "APROBADO"
                ? "exito"
                : pedido.estado === "RECHAZADO"
                  ? "peligro"
                  : "alerta"
            }
          >
            {ETIQUETA_ESTADO_PEDIDO[pedido.estado]}
          </EstadoBadge>
        </div>

        {pedido.docenteNombre && (
          <p className="text-sm">
            Lo pide <span className="font-medium">{pedido.docenteNombre}</span>.
          </p>
        )}

        <p className="text-sm">Lo explica así: «{pedido.motivo}»</p>

        <p className="text-muted-foreground text-xs">
          Pedido el {formatearFechaLargaDeInstante(pedido.creadoEn)}
        </p>

        {pedido.esMateriaNueva && (
          <p className="text-muted-foreground text-sm">
            Esa materia todavía no existe: al aprobar se crea con ese nombre.
          </p>
        )}

        {consultaDocentes.error && (
          <ErrorDeConsulta
            mensaje="No pudimos consultar quiénes dictan esta materia."
            reintentando={consultaDocentes.isFetching}
            onReintentar={() => void consultaDocentes.refetch()}
          />
        )}
        {!consultaDocentes.error && (docentes?.data.length ?? 0) > 0 && (
          <p className="text-muted-foreground text-sm">
            Hoy esa materia la dan{" "}
            {docentes!.data.length === 1
              ? "1 docente"
              : `${docentes!.data.length} docentes`}
            , que ya recibieron el aviso de este pedido.
          </p>
        )}

        {pedido.respuesta && (
          <p className="bg-muted rounded-md px-3 py-2 text-sm">
            Se contestó: {pedido.respuesta}
          </p>
        )}

        {pedido.estado === "PENDIENTE" &&
          (panel === null ? (
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                className="h-11 px-4 sm:h-9"
                onClick={() => setPanel("aprobar")}
              >
                Aprobar
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="h-11 px-4 sm:h-9"
                onClick={() => setPanel("rechazar")}
              >
                No aprobar
              </Button>
            </div>
          ) : (
            <div className="grid gap-2">
              {panel === "aprobar" && pedido.esMateriaNueva && (
                <div className="grid gap-1.5">
                  <Label htmlFor={`curso-${pedido.id}`}>¿En qué curso se crea?</Label>
                  {consultaCiclos.isPending && <p role="status">Cargando ciclos…</p>}
                  {consultaCiclos.error && (
                    <ErrorDeConsulta
                      mensaje="No pudimos consultar el ciclo activo."
                      reintentando={consultaCiclos.isFetching}
                      onReintentar={() => void consultaCiclos.refetch()}
                    />
                  )}
                  {!consultaCiclos.isPending && !consultaCiclos.error && !cicloActivo && (
                    <p>
                      No hay un ciclo activo. Crealo en Ciclos, cursos y materias antes de
                      aprobar.
                    </p>
                  )}
                  {cicloActivo && consultaCursos.isPending && (
                    <p role="status">Cargando cursos…</p>
                  )}
                  {consultaCursos.error && (
                    <ErrorDeConsulta
                      mensaje="No pudimos consultar los cursos."
                      reintentando={consultaCursos.isFetching}
                      onReintentar={() => void consultaCursos.refetch()}
                    />
                  )}
                  {catalogosListos && consultaCursos.data?.data.length === 0 && (
                    <p>
                      El ciclo activo todavía no tiene cursos. Cargá uno antes de aprobar.
                    </p>
                  )}
                  <Select
                    disabled={!catalogosListos || resolver.isPending}
                    id={`curso-${pedido.id}`}
                    value={cursoId}
                    onChange={(e) => setCursoId(e.target.value)}
                  >
                    <option value="">Elegí el curso…</option>
                    {(consultaCursos.data?.data ?? []).map((c) => (
                      <option key={c.id} value={c.id}>
                        {c.nombre}
                      </option>
                    ))}
                  </Select>
                </div>
              )}

              {panel === "aprobar" && (
                <div className="grid gap-1.5">
                  <Label htmlFor={`rol-${pedido.id}`}>¿Con qué rol queda?</Label>
                  <Select
                    id={`rol-${pedido.id}`}
                    value={rol}
                    onChange={(e) => setRol(e.target.value)}
                  >
                    {/* Vacío = lo decide el sistema: titular si nadie la da,
                        suplente si ya la da alguien. El rol no cambia lo que
                        puede hacer, pero es el dato que después alguien lee
                        para saber quién es quién. */}
                    <option value="">Como corresponda (según quién la dé hoy)</option>
                    <option value="TITULAR">Titular</option>
                    <option value="SUPLENTE">Suplente</option>
                  </Select>
                </div>
              )}

              <div className="grid gap-1.5">
                <Label htmlFor={`respuesta-${pedido.id}`}>
                  {panel === "rechazar"
                    ? "¿Por qué no? (obligatorio)"
                    : "Algo para decirle (opcional)"}
                </Label>
                <Input
                  id={`respuesta-${pedido.id}`}
                  value={respuesta}
                  onChange={(e) => setRespuesta(e.target.value)}
                  placeholder={
                    panel === "rechazar"
                      ? "Ej.: Hablé con dirección: la materia queda a cargo de quien la da hoy."
                      : "Ej.: Hablado con dirección, comparten el turno."
                  }
                />
              </div>

              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}

              <div className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant={panel === "rechazar" ? "destructive" : "default"}
                  className="h-11 px-4 sm:h-9"
                  disabled={
                    resolver.isPending ||
                    (panel === "rechazar" && respuesta.trim() === "") ||
                    (panel === "aprobar" && faltaCurso)
                  }
                  onClick={() => resolver.mutate(panel === "aprobar")}
                >
                  {panel === "aprobar" ? "Confirmar y habilitar" : "Confirmar el rechazo"}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-11 px-4 sm:h-9"
                  onClick={() => setPanel(null)}
                >
                  Volver
                </Button>
              </div>
            </div>
          ))}
      </CardContent>
    </Card>
  )
}
