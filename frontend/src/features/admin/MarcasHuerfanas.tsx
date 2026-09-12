import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import * as adminApi from "@/features/admin/api"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-03.21 — las marcas de preferencia que dejaron de cruzar con alguna materia.
 *
 * Una marca se guarda por NOMBRE de materia, así que renombrar «Matemática» a
 * «Matemáticas» desde Académico no rompe nada visible: las máquinas marcadas
 * simplemente dejan de aparecer primero, y el síntoma —«el orden cambió»— no
 * lleva a la causa. Esta sección es el único lugar del sistema desde donde se
 * las puede ver y sacar.
 *
 * A diferencia de los carros retirados, acá NO hay interruptor permanente: cero
 * huérfanas es el estado normal y sano, y una tarjeta fija diciendo que no pasa
 * nada sería ruido en la pantalla que más se usa del módulo. Aparece cuando hay
 * algo que hacer, que es cuando la pregunta existe.
 */
export function MarcasHuerfanas() {
  const queryClient = useQueryClient()

  const { data, error } = useQuery({
    queryKey: ["preferencias", "huerfanas"],
    queryFn: () => adminApi.listarPreferenciasHuerfanas(),
  })

  const quitar = useMutation({
    mutationFn: (id: string) => adminApi.borrarPreferencia(id),
    // La clave corta alcanza a esta lista y a la de cada equipo: una marca que
    // se saca de acá también desaparece de la ficha de su máquina.
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["preferencias"] }),
  })

  const huerfanas = data?.data ?? []

  // Fallar acá no puede tapar el inventario: quien entró a esta pantalla vino a
  // gestionar carros y equipos, no a revisar marcas. Se calla y sigue.
  if (error || huerfanas.length === 0) return null

  return (
    <Card className="mb-4">
      <CardHeader>
        <CardTitle>Marcas que quedaron sin materia</CardTitle>
        <CardDescription>
          Estas máquinas están marcadas como preferentes para una materia que ya no existe
          con ese nombre — normalmente porque se la renombró desde Académico. La marca no
          hace nada: el equipo aparece en el orden de siempre. Quitala, o volvé a marcar
          la máquina con el nombre nuevo desde su ficha.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-2">
        {quitar.error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(quitar.error)}</AlertDescription>
          </Alert>
        )}

        {huerfanas.map((h) => (
          <div
            key={h.id}
            className="flex flex-col gap-2 rounded-md border border-dashed p-3 sm:flex-row sm:items-center sm:justify-between"
          >
            <div className="min-w-0">
              <p className="min-w-0 break-words text-sm font-medium">{h.alcance}</p>
              {/* Qué máquina es, con su carro adelante: "PC 3" hay una por
                  carro, así que la etiqueta sola no identifica nada. */}
              <p className="text-muted-foreground text-xs">
                {h.carroNombre
                  ? `${h.carroNombre} · ${h.equipoEtiqueta}`
                  : h.equipoEtiqueta}{" "}
                <Badge variant="outline">Prioridad {h.prioridad}</Badge>{" "}
                {h.equipoDadoDeBaja && <Badge variant="outline">Equipo retirado</Badge>}
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              disabled={quitar.isPending}
              onClick={() => quitar.mutate(h.id)}
            >
              Quitar
            </Button>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
