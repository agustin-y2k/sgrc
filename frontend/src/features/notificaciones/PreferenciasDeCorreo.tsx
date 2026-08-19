import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { AvisoDeSpam } from "@/components/AvisoDeSpam"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import * as notificacionesApi from "@/features/notificaciones/api"
import type {
  CategoriaEmail,
  GrupoDeCategoria,
  PreferenciaEmail,
} from "@/features/notificaciones/types"
import { getErrorMessage } from "@/lib/api-client"

// Sin exportar: nadie más consulta estas preferencias, y exportarla haría
// que este archivo dejara de ser solo un componente.
const PREFERENCIAS_EMAIL_KEY = ["notificaciones", "preferencias-email"]

/** El título de cada bloque y por qué está separado del de al lado. */
const GRUPOS: { grupo: GrupoDeCategoria; titulo: string; ayuda: string }[] = [
  {
    grupo: "CUENTA",
    titulo: "Tu cuenta",
    ayuda:
      "Estos salen siempre: son los únicos que no tienen un aviso equivalente acá adentro.",
  },
  {
    grupo: "PERSONAL",
    titulo: "Tus avisos",
    ayuda: "Los que hablan de tus reservas y de tus pedidos.",
  },
  {
    grupo: "ADMINISTRACION",
    titulo: "Administración",
    ayuda: "Los que le llegan a todo el equipo de administración.",
  },
]

/**
 * RF-05.13: cuáles de estos avisos llegan además por correo. Todos aparecen
 * en esta pantalla llueva o truene; lo que se elige acá es solamente si
 * además se manda un mail.
 */
export function PreferenciasDeCorreo() {
  const queryClient = useQueryClient()
  const [abierto, setAbierto] = useState(false)
  // seleccion en null = todavía no se tocó nada, vale lo que dice el servidor.
  const [seleccion, setSeleccion] = useState<CategoriaEmail[] | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: PREFERENCIAS_EMAIL_KEY,
    queryFn: notificacionesApi.listarPreferenciasEmail,
  })

  const guardar = useMutation({
    mutationFn: notificacionesApi.guardarPreferenciasEmail,
    onSuccess: (respuesta) => {
      queryClient.setQueryData(PREFERENCIAS_EMAIL_KEY, respuesta)
      setSeleccion(null)
    },
  })

  const preferencias = data?.data ?? []
  // Las fijas nunca viajan en la selección: el backend las rechaza, y
  // mandarlas sería pedir algo que no se puede pedir.
  const elegibles = preferencias.filter((p) => !p.fija)
  const guardadas = activasDe(elegibles)
  const elegidas = seleccion ?? guardadas
  const hayCambios = seleccion !== null && !mismasCategorias(seleccion, guardadas)

  const alternar = (categoria: CategoriaEmail, tildada: boolean) => {
    setSeleccion(
      tildada ? [...elegidas, categoria] : elegidas.filter((c) => c !== categoria)
    )
  }

  return (
    <Card className="mb-4">
      <CardContent className="grid gap-3 pt-6">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <p className="font-medium">Copias por correo</p>
            <p className="text-muted-foreground text-sm">
              {resumen(elegibles, isLoading)}
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={() => setAbierto(!abierto)}>
            {abierto ? "Cerrar" : "Elegir cuáles"}
          </Button>
        </div>

        {error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(error)}</AlertDescription>
          </Alert>
        )}

        {abierto && (
          <>
            {/* Lo primero que hay que entender antes de tocar una casilla:
                acá no se apaga ningún aviso, solo el mail. */}
            <p className="text-muted-foreground text-sm">
              Todos estos avisos te van a seguir apareciendo en esta pantalla. Lo que
              elegís acá es cuáles te llegan <strong>además</strong> por correo.
            </p>

            {GRUPOS.map(({ grupo, titulo, ayuda }) => {
              const delGrupo = preferencias.filter((p) => p.grupo === grupo)
              if (delGrupo.length === 0) return null

              return (
                <div key={grupo} className="grid gap-3 border-t pt-3">
                  <div>
                    <p className="text-sm font-medium">{titulo}</p>
                    <p className="text-muted-foreground text-xs">{ayuda}</p>
                  </div>

                  {delGrupo.map((p) => (
                    <div key={p.categoria} className="flex items-start gap-2">
                      <Checkbox
                        id={`correo-${p.categoria}`}
                        className="mt-1"
                        checked={p.fija || elegidas.includes(p.categoria)}
                        disabled={p.fija}
                        onCheckedChange={(v) => alternar(p.categoria, v === true)}
                      />
                      <div className="grid gap-0.5">
                        <div className="flex flex-wrap items-center gap-2">
                          <Label htmlFor={`correo-${p.categoria}`}>{p.etiqueta}</Label>
                          {p.fija && <Badge variant="secondary">Siempre</Badge>}
                        </div>
                        <p className="text-muted-foreground text-sm">{p.descripcion}</p>
                      </div>
                    </div>
                  ))}
                </div>
              )
            })}

            {guardar.error && (
              <Alert variant="destructive">
                <AlertDescription>{getErrorMessage(guardar.error)}</AlertDescription>
              </Alert>
            )}

            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                disabled={!hayCambios || guardar.isPending}
                onClick={() => guardar.mutate(elegidas)}
              >
                Guardar
              </Button>
              {hayCambios && (
                <span className="text-muted-foreground text-sm">
                  Tenés cambios sin guardar.
                </span>
              )}
            </div>

            <AvisoDeSpam>Los avisos de esta pantalla te llegan igual.</AvisoDeSpam>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function activasDe(preferencias: PreferenciaEmail[]): CategoriaEmail[] {
  return preferencias.filter((p) => p.activa).map((p) => p.categoria)
}

/** Comparación sin orden: las casillas se tildan en cualquier secuencia. */
function mismasCategorias(a: CategoriaEmail[], b: CategoriaEmail[]): boolean {
  return a.length === b.length && a.every((c) => b.includes(c))
}

/**
 * El resumen cuenta solo lo que se puede elegir: decir "6 de 17" contando las
 * de la cuenta haría parecer que se apagaron cosas que no se pueden apagar.
 */
function resumen(elegibles: PreferenciaEmail[], cargando: boolean): string {
  if (cargando) return "Cargando…"
  const activas = activasDe(elegibles).length
  if (activas === 0) {
    return "Solo te llegan por correo los avisos de tu cuenta. Elegí si querés alguno más."
  }
  return `Te llegan por correo ${activas} de ${elegibles.length} tipos de aviso, más los de tu cuenta.`
}
