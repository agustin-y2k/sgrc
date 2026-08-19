import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { EstadoBadge } from "@/components/EstadoBadge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { useAuth } from "@/features/auth/AuthContext"
import { EscribirSugerencia } from "@/features/sugerencias/EscribirSugerencia"
import * as sugerenciasApi from "@/features/sugerencias/api"
import { ETIQUETA_TIPO, type Sugerencia } from "@/features/sugerencias/types"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaYHora } from "@/lib/fechas"

/**
 * Las conversaciones con el equipo de administración, del lado que toque:
 * un docente ve las suyas y escribe, un Admin las ve todas y contesta.
 *
 * Vive acá adentro y no en una pantalla aparte para que haya UN solo lugar
 * donde mirar si alguien está esperando algo. Y para que contestar no
 * signifique abrir Gmail.
 */
export function PanelDeSoporte({
  abiertoDeEntrada = false,
}: {
  abiertoDeEntrada?: boolean
}) {
  const { user } = useAuth()
  const esAdmin = user?.rol === "ADMIN"
  const [abierto, setAbierto] = useState(abiertoDeEntrada)
  const [escribiendo, setEscribiendo] = useState(abiertoDeEntrada && !esAdmin)
  const [soloPendientes, setSoloPendientes] = useState(true)

  const { data, error, isLoading } = useQuery({
    queryKey: esAdmin
      ? ["sugerencias", "todas", soloPendientes]
      : ["sugerencias", "mias"],
    queryFn: () =>
      esAdmin ? sugerenciasApi.listar(soloPendientes) : sugerenciasApi.misSugerencias(),
  })

  const hilos = data?.data ?? []

  return (
    <Card className="mb-4">
      <CardContent className="grid gap-3 pt-6">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <p className="font-medium">
              {esAdmin ? "Pedidos de ayuda" : "Ayuda y mensajes"}
            </p>
            <p className="text-muted-foreground text-sm">
              {resumen(hilos, esAdmin, isLoading)}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {!esAdmin && (
              <Button
                size="sm"
                onClick={() => {
                  setEscribiendo(!escribiendo)
                  setAbierto(true)
                }}
              >
                {escribiendo ? "Cerrar" : "Pedir ayuda"}
              </Button>
            )}
            <Button variant="outline" size="sm" onClick={() => setAbierto(!abierto)}>
              {abierto ? "Cerrar" : "Ver conversaciones"}
            </Button>
          </div>
        </div>

        {error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(error)}</AlertDescription>
          </Alert>
        )}

        {escribiendo && !esAdmin && (
          <div className="border-t pt-3">
            <EscribirSugerencia onEnviada={() => setEscribiendo(false)} />
          </div>
        )}

        {abierto && (
          <div className="grid gap-3 border-t pt-3">
            {esAdmin && (
              <div className="flex items-center gap-2">
                <Checkbox
                  id="solo-pendientes"
                  checked={soloPendientes}
                  onCheckedChange={(v) => setSoloPendientes(v === true)}
                />
                <Label htmlFor="solo-pendientes">Ver solo lo que falta contestar</Label>
              </div>
            )}

            {hilos.length === 0 && (
              <p className="text-muted-foreground text-sm">
                {esAdmin
                  ? "No hay conversaciones para mostrar."
                  : "Todavía no escribiste nada."}
              </p>
            )}

            {hilos.map((h) => (
              <Hilo key={h.id} hilo={h} esAdmin={esAdmin} />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

/** Una conversación con sus mensajes y la caja para seguir contestando. */
function Hilo({ hilo, esAdmin }: { hilo: Sugerencia; esAdmin: boolean }) {
  const qc = useQueryClient()
  const [texto, setTexto] = useState("")
  // Se abre solo lo que espera respuesta: en una bandeja de quince, tener
  // todos los hilos desplegados es lo mismo que no tener ninguno.
  const [desplegado, setDesplegado] = useState(hilo.esperaRespuesta)

  const invalidar = () => qc.invalidateQueries({ queryKey: ["sugerencias"] })

  const responder = useMutation({
    mutationFn: () => sugerenciasApi.responder(hilo.id, texto),
    onSuccess: () => {
      setTexto("")
      invalidar()
    },
  })

  const resolver = useMutation({
    mutationFn: () => sugerenciasApi.resolver(hilo.id),
    onSuccess: invalidar,
  })

  return (
    <div className="grid gap-2 rounded-lg border px-3 py-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="grid gap-0.5">
          <span className="font-medium">{hilo.asunto}</span>
          <span className="text-muted-foreground text-xs">
            {ETIQUETA_TIPO[hilo.tipo]} · {formatearFechaYHora(hilo.ultimaActividadEn)}
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {hilo.estado === "RESUELTA" ? (
            <EstadoBadge tono="exito">Resuelta</EstadoBadge>
          ) : hilo.esperaRespuesta ? (
            <EstadoBadge tono="alerta">
              {esAdmin ? "Falta contestar" : "Esperando respuesta"}
            </EstadoBadge>
          ) : (
            <EstadoBadge tono="neutro">Te contestaron</EstadoBadge>
          )}
          <Button variant="outline" size="sm" onClick={() => setDesplegado(!desplegado)}>
            {desplegado ? "Ocultar" : "Ver"}
          </Button>
        </div>
      </div>

      {desplegado && (
        <>
          <div className="grid gap-2">
            {hilo.mensajes.map((m) => (
              <div
                key={m.id}
                className={
                  m.deAdmin
                    ? "bg-muted rounded-md px-3 py-2 text-sm"
                    : "rounded-md border px-3 py-2 text-sm"
                }
              >
                <p className="text-muted-foreground mb-1 text-xs">
                  {m.deAdmin ? "Administración" : esAdmin ? "Quien escribió" : "Vos"} ·{" "}
                  {formatearFechaYHora(m.escritoEn)}
                </p>
                {/* whitespace-pre-line: la gente escribe en párrafos y sin
                    esto todo llega en un bloque. */}
                <p className="whitespace-pre-line">{m.texto}</p>
              </div>
            ))}
          </div>

          {hilo.pantalla && esAdmin && (
            <p className="text-muted-foreground text-xs">
              Lo escribió desde {hilo.pantalla}
              {hilo.version ? ` (versión ${hilo.version})` : ""}
            </p>
          )}

          <div className="grid gap-2">
            <Label htmlFor={`respuesta-${hilo.id}`} className="sr-only">
              Escribir en esta conversación
            </Label>
            <textarea
              id={`respuesta-${hilo.id}`}
              rows={3}
              value={texto}
              onChange={(e) => setTexto(e.target.value)}
              placeholder={esAdmin ? "Contestale…" : "Seguí la conversación acá mismo…"}
              className="border-input focus-visible:border-ring focus-visible:ring-ring/50 w-full rounded-lg border bg-transparent px-2.5 py-2 text-base transition-colors outline-none focus-visible:ring-3 md:text-sm"
            />
            {(responder.error || resolver.error) && (
              <Alert variant="destructive">
                <AlertDescription>
                  {getErrorMessage(responder.error ?? resolver.error)}
                </AlertDescription>
              </Alert>
            )}
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                disabled={texto.trim() === "" || responder.isPending}
                onClick={() => responder.mutate()}
              >
                {responder.isPending ? "Mandando…" : "Responder"}
              </Button>
              {/* Cerrar es un acto aparte de contestar: se contesta muchas
                  veces y se cierra una, cuando el tema terminó de verdad. */}
              {esAdmin && hilo.estado === "ABIERTA" && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={resolver.isPending}
                  onClick={() => resolver.mutate()}
                >
                  Dar por resuelta
                </Button>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  )
}

function resumen(hilos: Sugerencia[], esAdmin: boolean, cargando: boolean): string {
  if (cargando) return "Cargando…"

  if (esAdmin) {
    const pendientes = hilos.filter((h) => h.esperaRespuesta).length
    if (pendientes === 0) return "No hay nadie esperando respuesta."
    return pendientes === 1
      ? "Hay 1 conversación esperando respuesta."
      : `Hay ${pendientes} conversaciones esperando respuesta.`
  }

  const contestadas = hilos.filter(
    (h) => !h.esperaRespuesta && h.estado === "ABIERTA"
  ).length
  if (contestadas > 0) {
    return contestadas === 1
      ? "Te contestaron 1 conversación."
      : `Te contestaron ${contestadas} conversaciones.`
  }
  if (hilos.length === 0) {
    return "Si necesitás una mano con algo, escribinos y te contestamos por acá."
  }
  return "Tus conversaciones con el equipo de administración."
}
