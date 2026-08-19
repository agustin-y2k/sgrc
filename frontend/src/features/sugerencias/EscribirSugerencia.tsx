import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useLocation } from "react-router"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import * as sugerenciasApi from "@/features/sugerencias/api"
import type { TipoDeMensaje } from "@/features/sugerencias/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * El formulario para contar que algo no anda o proponer un cambio.
 *
 * Está pensado para alguien que ya se sintió torpe usando el sistema: no
 * pide categoría, ni prioridad, ni pasos para reproducir. Dos botones y un
 * cuadro de texto. Lo que hace falta para entender el reporte —desde qué
 * pantalla se escribió, con qué versión— lo agrega la aplicación sola.
 *
 * `pantallaPrevia` existe porque este formulario vive en su propia página:
 * la ruta que importa es la que la persona estaba mirando cuando decidió
 * escribir, no `/mis-mensajes`.
 */
export function EscribirSugerencia({
  pantallaPrevia,
  onEnviada,
}: {
  pantallaPrevia?: string
  onEnviada?: () => void
}) {
  const qc = useQueryClient()
  const location = useLocation()
  const [tipo, setTipo] = useState<TipoDeMensaje>("PROBLEMA")
  const [texto, setTexto] = useState("")
  const [error, setError] = useState("")
  const [enviada, setEnviada] = useState(false)

  const escribir = useMutation({
    mutationFn: () =>
      sugerenciasApi.escribir(tipo, texto, pantallaPrevia ?? location.pathname),
    onSuccess: () => {
      setError("")
      setTexto("")
      setEnviada(true)
      qc.invalidateQueries({ queryKey: ["sugerencias", "mias"] })
      onEnviada?.()
    },
    onError: (e) => setError(getErrorMessage(e)),
  })

  return (
    <div className="grid gap-3">
      {enviada && (
        <Alert>
          <AlertDescription>
            Llegó. Lo van a leer los Admin y te contestan por acá y por correo.
          </AlertDescription>
        </Alert>
      )}

      <div className="grid gap-1.5">
        <Label>¿Qué querés contarnos?</Label>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant={tipo === "PROBLEMA" ? "default" : "outline"}
            size="sm"
            className="h-11 px-4 sm:h-9"
            onClick={() => setTipo("PROBLEMA")}
          >
            Algo no anda
          </Button>
          <Button
            type="button"
            variant={tipo === "SUGERENCIA" ? "default" : "outline"}
            size="sm"
            className="h-11 px-4 sm:h-9"
            onClick={() => setTipo("SUGERENCIA")}
          >
            Se me ocurre una idea
          </Button>
        </div>
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="texto-sugerencia">Contalo con tus palabras</Label>
        {/* Un textarea y no un input de una línea: lo que se cuenta acá son
            varias frases, y un renglón que se desplaza mientras se escribe
            hace perder el hilo de lo que uno venía diciendo. */}
        <textarea
          id="texto-sugerencia"
          rows={5}
          value={texto}
          onChange={(e) => setTexto(e.target.value)}
          placeholder="Ej.: Cuando quiero reservar para el jueves no me aparece ninguna computadora, y sé que hay libres."
          className="border-input focus-visible:border-ring focus-visible:ring-ring/50 w-full rounded-lg border bg-transparent px-2.5 py-2 text-base transition-colors outline-none focus-visible:ring-3 md:text-sm"
        />
        <p className="text-muted-foreground text-sm">
          No hace falta que sepas por qué pasa. Con contar qué querías hacer y qué viste
          alcanza.
        </p>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div>
        <Button
          type="button"
          size="sm"
          className="h-11 px-4 sm:h-9"
          disabled={texto.trim() === "" || escribir.isPending}
          onClick={() => escribir.mutate()}
        >
          {escribir.isPending ? "Mandando…" : "Mandar"}
        </Button>
      </div>
    </div>
  )
}
