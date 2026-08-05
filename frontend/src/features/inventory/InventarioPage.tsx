import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "react-router"

import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { EstadoBadge, TONO_PC } from "@/components/EstadoBadge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import * as inventoryApi from "@/features/inventory/api"
import { ReportarIncidencia } from "@/features/inventory/ReportarIncidencia"
import type { EstadoPC, PC } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

const ETIQUETA_ESTADO: Record<EstadoPC, string> = {
  DISPONIBLE: "Disponible",
  EN_MANTENIMIENTO: "En mantenimiento",
  FUERA_DE_SERVICIO: "Fuera de servicio",
}

function EstadoDePC({ estado }: { estado: EstadoPC }) {
  return <EstadoBadge tono={TONO_PC[estado]}>{ETIQUETA_ESTADO[estado]}</EstadoBadge>
}

function PCsDelCarro({ carroId }: { carroId: string }) {
  // RF-03.5: cualquier usuario autenticado puede reportar una falla, y esta
  // es la pantalla donde un docente ya está mirando las PCs.
  const [reportando, setReportando] = useState<PC | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: ["pcs", carroId],
    queryFn: () => inventoryApi.listarPCsDeCarro(carroId),
  })

  if (isLoading) return <p className="text-muted-foreground text-sm">Cargando PCs…</p>
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{getErrorMessage(error)}</AlertDescription>
      </Alert>
    )
  }

  const pcs = (data?.data ?? []).filter((pc) => !pc.dadaDeBaja)
  if (pcs.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">Este carro no tiene PCs activas.</p>
    )
  }

  return (
    // La tabla scrollea sola en pantallas angostas en vez de desbordar la
    // página (RNF-07: diseño responsive).
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>PC</TableHead>
            <TableHead>Estado</TableHead>
            <TableHead>Freezada</TableHead>
            {/* RF-03.7: el software instalado es justamente el dato por el
                que un docente entra acá antes de elegir qué reservar. */}
            <TableHead>Software instalado</TableHead>
            <TableHead className="text-right">Acciones</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {pcs.map((pc) => (
            <TableRow key={pc.id}>
              <TableCell className="font-medium">PC {pc.identificador}</TableCell>
              <TableCell>
                <EstadoDePC estado={pc.estado} />
              </TableCell>
              <TableCell>{pc.freezado ? "Sí" : "No"}</TableCell>
              <TableCell className="text-muted-foreground max-w-xs text-sm">
                {pc.softwareInstalado || "—"}
              </TableCell>
              <TableCell className="text-right">
                <div className="flex flex-wrap justify-end gap-2">
                  <Button asChild variant="outline" size="sm">
                    <Link to={`/inventario/pcs/${pc.id}/calendario`}>Ver calendario</Link>
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setReportando(pc)}>
                    Reportar problema
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      {/* Fuera de la tabla a propósito: un formulario dentro de una celda
          queda ilegible en pantallas angostas, que es justo desde donde un
          docente reporta una PC que no anda. */}
      {reportando && (
        <div className="mt-3">
          <ReportarIncidencia pc={reportando} onListo={() => setReportando(null)} />
        </div>
      )}
    </div>
  )
}

// RF-03.7 + RF-04.4: cualquier usuario autenticado puede recorrer carros y
// PCs, y desde acá abrir el calendario de cada una.
export function InventarioPage() {
  const [carroAbierto, setCarroAbierto] = useState<string | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: ["carros"],
    queryFn: inventoryApi.listarCarros,
  })

  const carros = data?.data ?? []

  return (
    <div className="mx-auto max-w-4xl">
      <EncabezadoDePagina
        titulo="Inventario"
        descripcion="Carros y PCs de la institución. Abrí una PC para ver su calendario, o reportá una falla."
      />

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}

      {!isLoading && carros.length === 0 && (
        <p className="text-muted-foreground">Todavía no hay carros cargados.</p>
      )}

      <div className="grid gap-3">
        {carros.map((carro) => {
          const abierto = carroAbierto === carro.id
          return (
            <Card key={carro.id}>
              <CardHeader>
                <CardTitle className="flex items-center justify-between gap-2">
                  <span>{carro.nombre}</span>
                  <Button
                    variant="outline"
                    size="sm"
                    aria-expanded={abierto}
                    onClick={() => setCarroAbierto(abierto ? null : carro.id)}
                  >
                    {abierto ? "Ocultar PCs" : "Ver PCs"}
                  </Button>
                </CardTitle>
                {carro.descripcion && (
                  <CardDescription>{carro.descripcion}</CardDescription>
                )}
              </CardHeader>
              {abierto && (
                <CardContent>
                  <PCsDelCarro carroId={carro.id} />
                </CardContent>
              )}
            </Card>
          )
        })}
      </div>
    </div>
  )
}
