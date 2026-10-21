import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { EstadoBadge, TONO_PC } from "@/components/EstadoBadge"
import * as inventoryApi from "@/features/inventory/api"
import {
  ETIQUETA_ESTADO_EQUIPO,
  type Carro,
  type Equipo,
} from "@/features/inventory/types"
import { sinTildes } from "@/lib/texto"
import { contar, plural } from "@/lib/plural"

export function BusquedaDeInventario({
  carros,
  onAbrir,
  carrosListos,
}: {
  carros: Carro[]
  onAbrir: (equipo: Equipo) => void
  carrosListos: boolean
}) {
  const [texto, setTexto] = useState("")
  const [estado, setEstado] = useState("")
  const buscando = !!texto.trim() || !!estado
  const consulta = useQuery({
    queryKey: ["equipos", "busqueda-global"],
    queryFn: () => inventoryApi.listarEquipos(),
    enabled: buscando && carrosListos,
  })
  const nombres = new Map(carros.map((c) => [c.id, c.nombre]))
  const filtro = sinTildes(texto.trim())
  const encontrados = (consulta.data?.data ?? []).filter(
    (e) =>
      !e.dadoDeBaja &&
      (!estado || e.estado === estado) &&
      sinTildes(
        `${e.etiqueta} ${e.nombre ?? ""} ${nombres.get(e.carroId ?? "") ?? ""} ${e.numeroSerie ?? ""} ${e.softwareInstalado ?? ""}`
      ).includes(filtro)
  )
  return (
    <Card className="mb-4">
      <CardHeader>
        <CardTitle>Encontrar un equipo</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="buscar-inventario">Equipo, carro, serie o software</Label>
            <Input
              id="buscar-inventario"
              value={texto}
              disabled={!carrosListos}
              onChange={(e) => setTexto(e.target.value)}
              placeholder="Ej.: PC 12, EDUTEC o AutoCAD"
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="estado-inventario">Estado del equipo</Label>
            <Select
              id="estado-inventario"
              value={estado}
              disabled={!carrosListos}
              onChange={(e) => setEstado(e.target.value)}
            >
              <option value="">Todos los estados</option>
              {Object.entries(ETIQUETA_ESTADO_EQUIPO).map(([valor, etiqueta]) => (
                <option key={valor} value={valor}>
                  {etiqueta}
                </option>
              ))}
            </Select>
          </div>
        </div>
        {!carrosListos ? (
          <p className="text-muted-foreground text-sm">
            La búsqueda necesita el listado de carros. Esperá la carga o reintentá su
            consulta si falló.
          </p>
        ) : !buscando ? (
          <p className="text-muted-foreground text-sm">
            Buscá en todos los carros y equipos sueltos. Podés combinar el texto con un
            estado.
          </p>
        ) : consulta.error ? (
          <ErrorDeConsulta
            mensaje="No pudimos consultar el inventario para la búsqueda."
            reintentando={consulta.isFetching}
            onReintentar={() => {
              void consulta.refetch()
            }}
          />
        ) : consulta.isPending ? (
          <p role="status" className="text-muted-foreground text-sm">
            Buscando equipos…
          </p>
        ) : (
          <>
            <p role="status" className="text-muted-foreground text-sm">
              {contar(encontrados.length, "equipo")}{" "}
              {plural(encontrados.length, "encontrado")}.
            </p>
            {encontrados.length === 0 && (
              <p className="text-sm">
                Ningún equipo coincide. Probá otro texto o estado.
              </p>
            )}
            <div className="grid max-h-96 gap-2 overflow-y-auto rounded-md">
              {encontrados.map((e) => (
                <div key={e.id} className="grid gap-2 rounded-md border p-3">
                  <p className="flex flex-wrap items-center gap-2 font-medium">
                    {e.etiqueta}
                    <EstadoBadge tono={TONO_PC[e.estado]}>
                      {ETIQUETA_ESTADO_EQUIPO[e.estado]}
                    </EstadoBadge>
                  </p>
                  <p className="text-muted-foreground text-sm break-words">
                    {e.carroId
                      ? (nombres.get(e.carroId) ?? "Carro sin consultar")
                      : "Equipo suelto"}{" "}
                    · Serie {e.numeroSerie || "sin registrar"}
                    {e.softwareInstalado ? ` · ${e.softwareInstalado}` : ""}
                  </p>
                  <Button
                    type="button"
                    variant="outline"
                    className="h-11 justify-self-start"
                    onClick={() => onAbrir(e)}
                  >
                    Abrir ficha de {e.etiqueta}
                  </Button>
                </div>
              ))}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
