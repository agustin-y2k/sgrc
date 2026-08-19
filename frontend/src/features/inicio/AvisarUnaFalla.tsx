import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as inventoryApi from "@/features/inventory/api"
import { ReportarIncidencia } from "@/features/inventory/ReportarIncidencia"
import type { Equipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-03.5 desde la pantalla de inicio: avisar que una computadora no anda,
 * sin tener que encontrar antes el carro en el inventario.
 */
export function AvisarUnaFalla({ onCerrar }: { onCerrar: () => void }) {
  const [equipoId, setEquipoId] = useState("")

  const {
    data: equipos,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["equipos"],
    queryFn: () => inventoryApi.listarEquipos(),
  })

  // Los carros solo aportan el nombre del grupo. Si la consulta falla, la
  // lista sigue sirviendo: se agrupan igual, con el título genérico.
  const { data: carros } = useQuery({
    queryKey: ["carros"],
    queryFn: inventoryApi.listarCarros,
  })

  /** Los equipos por carro, en el orden en que están en el mueble. */
  const grupos = useMemo(() => {
    const nombreDeCarro = new Map((carros?.data ?? []).map((c) => [c.id, c.nombre]))
    const porCarro = new Map<string, { titulo: string; equipos: Equipo[] }>()

    for (const equipo of equipos?.data ?? []) {
      if (equipo.dadoDeBaja) continue
      // Lo que no está en ningún carro —un proyector, una notebook suelta—
      // se junta aparte en vez de quedar suelto entre los zócalos.
      const clave = equipo.carroId ?? ""
      const titulo = equipo.carroId
        ? (nombreDeCarro.get(equipo.carroId) ?? "Carro")
        : "Otros equipos"
      const grupo = porCarro.get(clave) ?? { titulo, equipos: [] }
      grupo.equipos.push(equipo)
      porCarro.set(clave, grupo)
    }

    for (const grupo of porCarro.values()) {
      grupo.equipos.sort(
        (a, b) =>
          (a.identificador ?? 0) - (b.identificador ?? 0) ||
          a.etiqueta.localeCompare(b.etiqueta)
      )
    }

    // Los sueltos al final: lo habitual es reportar una del carro.
    return [...porCarro.entries()]
      .sort(([a], [b]) => (a === "" ? 1 : b === "" ? -1 : a.localeCompare(b)))
      .map(([clave, grupo]) => ({ clave, ...grupo }))
  }, [equipos, carros])

  const elegido = (equipos?.data ?? []).find((equipo) => equipo.id === equipoId)

  return (
    <div className="bg-superficie grid gap-3 rounded-xl border p-4">
      <div>
        <p className="font-medium">Avisar que una computadora no anda</p>
        <p className="text-muted-foreground text-sm">
          Elegí cuál es y contá qué le pasa. El aviso le llega a los Admin.
        </p>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-1.5">
        <Label htmlFor="equipo-con-falla">¿Cuál es?</Label>
        <Select
          id="equipo-con-falla"
          value={equipoId}
          onChange={(e) => setEquipoId(e.target.value)}
          disabled={isLoading || grupos.length === 0}
        >
          <option value="">{isLoading ? "Cargando…" : "Elegí la computadora"}</option>
          {grupos.map((grupo) => (
            <optgroup key={grupo.clave} label={grupo.titulo}>
              {grupo.equipos.map((equipo) => (
                <option key={equipo.id} value={equipo.id}>
                  {equipo.etiqueta}
                </option>
              ))}
            </optgroup>
          ))}
        </Select>
      </div>

      {/* El formulario aparece recién con la máquina elegida: mostrarlo antes
          es pedir que se describa la falla de algo que todavía no se dijo
          cuál es. `key` para que cambiar de máquina no arrastre el texto ya
          escrito para otra. */}
      {elegido && (
        <ReportarIncidencia
          key={elegido.id}
          equipo={elegido}
          onListo={() => {
            setEquipoId("")
            onCerrar()
          }}
        />
      )}

      {!elegido && (
        <div>
          <Button
            variant="outline"
            size="sm"
            className="h-11 px-4 sm:h-9"
            onClick={onCerrar}
          >
            Volver
          </Button>
        </div>
      )}
    </div>
  )
}
