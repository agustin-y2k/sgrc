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
import { ETIQUETA_ESTADO_EQUIPO } from "@/features/inventory/types"
import type { EstadoEquipo, Equipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"


function EstadoDeEquipo({ estado }: { estado: EstadoEquipo }) {
  return <EstadoBadge tono={TONO_PC[estado]}>{ETIQUETA_ESTADO_EQUIPO[estado]}</EstadoBadge>
}

function EquiposDelCarro({ carroId }: { carroId: string }) {
  // RF-03.5: cualquier usuario autenticado puede reportar una falla, y esta
  // es la pantalla donde un docente ya está mirando los equipos.
  const [reportando, setReportando] = useState<Equipo | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: ["equipos", carroId],
    queryFn: () => inventoryApi.listarEquiposDeCarro(carroId),
  })

  if (isLoading) return <p className="text-muted-foreground text-sm">Cargando equipos…</p>
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{getErrorMessage(error)}</AlertDescription>
      </Alert>
    )
  }

  const equipos = (data?.data ?? []).filter((equipo) => !equipo.dadoDeBaja)
  if (equipos.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">Este carro no tiene equipos activas.</p>
    )
  }

  return (
    // La tabla scrollea sola en pantallas angostas en vez de desbordar la
    // página (RNF-07: diseño responsive).
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Equipo</TableHead>
            <TableHead>Estado</TableHead>
            <TableHead>Freezada</TableHead>
            {/* RF-03.7: el software instalado es justamente el dato por el
                que un docente entra acá antes de elegir qué reservar. */}
            <TableHead>Software instalado</TableHead>
            <TableHead className="text-right">Acciones</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {equipos.map((equipo) => (
            <TableRow key={equipo.id}>
              <TableCell className="font-medium">{equipo.etiqueta}</TableCell>
              <TableCell>
                <EstadoDeEquipo estado={equipo.estado} />
              </TableCell>
              <TableCell>{equipo.freezado ? "Sí" : "No"}</TableCell>
              <TableCell className="text-muted-foreground max-w-xs text-sm">
                {equipo.softwareInstalado || "—"}
              </TableCell>
              <TableCell className="text-right">
                <div className="flex flex-wrap justify-end gap-2">
                  <Button asChild variant="outline" size="sm">
                    <Link to={`/inventario/equipos/${equipo.id}/calendario`}>Ver calendario</Link>
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setReportando(equipo)}>
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
          docente reporta un equipo que no anda. */}
      {reportando && (
        <div className="mt-3">
          <ReportarIncidencia equipo={reportando} onListo={() => setReportando(null)} />
        </div>
      )}
    </div>
  )
}

// RF-03.7 + RF-04.4: cualquier usuario autenticado puede recorrer carros y
// Equipos, y desde acá abrir el calendario de cada una.
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
        descripcion="Carros y equipos de la institución. Abrí un equipo para ver su calendario, o reportá una falla."
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
                    {abierto ? "Ocultar equipos" : "Ver equipos"}
                  </Button>
                </CardTitle>
                {carro.descripcion && (
                  <CardDescription>{carro.descripcion}</CardDescription>
                )}
              </CardHeader>
              {abierto && (
                <CardContent>
                  <EquiposDelCarro carroId={carro.id} />
                </CardContent>
              )}
            </Card>
          )
        })}
      </div>
    </div>
  )
}
