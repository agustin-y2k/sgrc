import { useQuery } from "@tanstack/react-query"
import { useLocation } from "react-router"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { EstadoBadge } from "@/components/EstadoBadge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { EscribirSugerencia } from "@/features/sugerencias/EscribirSugerencia"
import * as sugerenciasApi from "@/features/sugerencias/api"
import { ETIQUETA_TIPO } from "@/features/sugerencias/types"
import { formatearFechaLarga } from "@/lib/fechas"

/**
 * Donde alguien escribe al equipo de administración y lee lo que le
 * contestaron.
 *
 * Las dos cosas en la misma pantalla y no en dos: quien entra a escribir por
 * segunda vez tiene que poder ver qué pasó con la primera. Sin eso, el buzón
 * se siente como escribir al vacío, y dos mensajes sin respuesta alcanzan
 * para que nadie lo use más.
 */
export function MisMensajesPage() {
  const location = useLocation()
  const pantallaPrevia = (location.state as { desde?: string } | null)?.desde

  const { data } = useQuery({
    queryKey: ["sugerencias", "mias"],
    queryFn: sugerenciasApi.misSugerencias,
  })

  const mensajes = data?.data ?? []

  return (
    <div className="mx-auto grid max-w-3xl gap-4">
      <EncabezadoDePagina
        titulo="Escribinos"
        descripcion="Si algo del sistema no anda, o se te ocurre cómo mejorarlo, contalo acá."
      />

      <Card>
        <CardContent className="pt-6">
          <EscribirSugerencia pantallaPrevia={pantallaPrevia} />
        </CardContent>
      </Card>

      {mensajes.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Lo que escribiste</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-3">
            {mensajes.map((m) => (
              <div key={m.id} className="grid gap-1 rounded-lg border px-3 py-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-sm font-medium">{ETIQUETA_TIPO[m.tipo]}</span>
                  <EstadoBadge tono={m.estado === "RESUELTA" ? "exito" : "alerta"}>
                    {m.estado === "RESUELTA" ? "Contestado" : "Esperando respuesta"}
                  </EstadoBadge>
                </div>
                <p className="text-sm">{m.texto}</p>
                <p className="text-muted-foreground text-xs">
                  {formatearFechaLarga(m.creadaEn.slice(0, 10))}
                </p>
                {m.respuesta && (
                  <p className="bg-muted rounded-md px-3 py-2 text-sm">
                    Te contestaron: {m.respuesta}
                  </p>
                )}
              </div>
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
