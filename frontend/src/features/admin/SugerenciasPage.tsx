import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { EstadoBadge } from "@/components/EstadoBadge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import * as sugerenciasApi from "@/features/sugerencias/api"
import { ETIQUETA_TIPO, type Sugerencia } from "@/features/sugerencias/types"
import { getErrorMessage } from "@/lib/api-client"
import { formatearFechaLarga } from "@/lib/fechas"

/** Lo que la gente escribió sobre el sistema, para leerlo y contestarlo. */
export function SugerenciasPage() {
  const [soloAbiertas, setSoloAbiertas] = useState(true)

  const { data, error } = useQuery({
    queryKey: ["sugerencias", "todas", soloAbiertas],
    queryFn: () => sugerenciasApi.listar(soloAbiertas),
  })

  const mensajes = data?.data ?? []

  return (
    <div className="mx-auto grid max-w-3xl gap-4">
      <EncabezadoDePagina
        titulo="Lo que nos escribieron"
        descripcion="Problemas y sugerencias sobre el sistema. Contestar cierra el mensaje y le llega un aviso a quien lo escribió."
      />

      <div className="flex items-center gap-2">
        <Checkbox
          id="solo-abiertas"
          checked={soloAbiertas}
          onCheckedChange={(v) => setSoloAbiertas(v === true)}
        />
        <Label htmlFor="solo-abiertas">Ver solo lo que falta contestar</Label>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {mensajes.length === 0 ? (
        <Card>
          <CardContent className="text-muted-foreground pt-6 text-sm">
            {soloAbiertas
              ? "No hay nada sin contestar."
              : "Todavía no escribió nadie."}
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-3">
          {mensajes.map((m) => (
            <MensajeRecibido key={m.id} mensaje={m} />
          ))}
        </div>
      )}
    </div>
  )
}

function MensajeRecibido({ mensaje }: { mensaje: Sugerencia }) {
  const qc = useQueryClient()
  const [respondiendo, setRespondiendo] = useState(false)
  const [respuesta, setRespuesta] = useState("")
  const [error, setError] = useState("")

  const responder = useMutation({
    mutationFn: () => sugerenciasApi.responder(mensaje.id, respuesta),
    onSuccess: () => {
      setError("")
      setRespondiendo(false)
      setRespuesta("")
      qc.invalidateQueries({ queryKey: ["sugerencias"] })
    },
    onError: (e) => setError(getErrorMessage(e)),
  })

  return (
    <Card>
      <CardContent className="grid gap-2 pt-6">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="font-medium">{ETIQUETA_TIPO[mensaje.tipo]}</span>
          <EstadoBadge tono={mensaje.estado === "RESUELTA" ? "exito" : "alerta"}>
            {mensaje.estado === "RESUELTA" ? "Contestado" : "Sin contestar"}
          </EstadoBadge>
        </div>

        <p className="text-sm">{mensaje.texto}</p>

        {/* Desde dónde y con qué versión: es lo que permite ir a mirar sin
            tener que buscar a la persona para preguntarle qué estaba
            haciendo. */}
        <p className="text-muted-foreground text-xs">
          {formatearFechaLarga(mensaje.creadaEn.slice(0, 10))}
          {mensaje.pantalla && ` · desde ${mensaje.pantalla}`}
          {mensaje.version && ` · versión ${mensaje.version}`}
        </p>

        {mensaje.respuesta && (
          <p className="bg-muted rounded-md px-3 py-2 text-sm">
            Se contestó: {mensaje.respuesta}
          </p>
        )}

        {mensaje.estado === "ABIERTA" &&
          (respondiendo ? (
            <div className="grid gap-2">
              <Label htmlFor={`respuesta-${mensaje.id}`}>Tu respuesta</Label>
              <textarea
                id={`respuesta-${mensaje.id}`}
                rows={3}
                value={respuesta}
                onChange={(e) => setRespuesta(e.target.value)}
                placeholder="Ej.: Era la jornada: el jueves no estaba declarado. Ya lo cargamos, probá de nuevo."
                className="border-input focus-visible:border-ring focus-visible:ring-ring/50 w-full rounded-lg border bg-transparent px-2.5 py-2 text-base transition-colors outline-none focus-visible:ring-3 md:text-sm"
              />
              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              <div className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  className="h-11 px-4 sm:h-9"
                  disabled={respuesta.trim() === "" || responder.isPending}
                  onClick={() => responder.mutate()}
                >
                  Contestar y cerrar
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-11 px-4 sm:h-9"
                  onClick={() => setRespondiendo(false)}
                >
                  Volver
                </Button>
              </div>
            </div>
          ) : (
            <div>
              <Button
                variant="outline"
                size="sm"
                className="h-11 px-4 sm:h-9"
                onClick={() => setRespondiendo(true)}
              >
                Contestar
              </Button>
            </div>
          ))}
      </CardContent>
    </Card>
  )
}

