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
  return (
    <EstadoBadge tono={TONO_PC[estado]}>{ETIQUETA_ESTADO_EQUIPO[estado]}</EstadoBadge>
  )
}

/**
 * La tabla con la que un docente mira un conjunto de equipos: qué son, cómo
 * están, y las dos cosas que puede hacer con cada uno.
 *
 * Está separada de quién los busca porque la usan dos secciones —los equipos
 * de un carro y los que no están en ninguno— y son la misma tabla. Con dos
 * copias, la primera vez que se agregue una columna o se cambie un rótulo
 * las dos empiezan a mostrar cosas distintas de la misma máquina.
 */
function TablaDeEquipos({ equipos }: { equipos: Equipo[] }) {
  // RF-03.5: cualquier usuario autenticado puede reportar una falla, y esta
  // es la pantalla donde un docente ya está mirando los equipos.
  const [reportando, setReportando] = useState<Equipo | null>(null)

  // "Freezada" y "Software instalado" son datos de una computadora: en una
  // tabla de proyectores y cargadores serían dos columnas de guiones.
  //
  // Cada una se decide por separado y mirando el dato, no el tipo. El alta de
  // un equipo suelto hoy solo pide tipo, nombre y si se reserva, así que una
  // notebook fuera de un carro es tipo PC pero no tiene software cargado:
  // preguntando por el tipo, esa tabla mostraría una columna entera vacía.
  // Si algún día el alta acepta el dato, la columna aparece sola.
  const hayFreezado = equipos.some((equipo) => equipo.tipo === "PC")
  const haySoftware = equipos.some((equipo) => equipo.softwareInstalado)

  return (
    // La tabla scrollea sola en pantallas angostas en vez de desbordar la
    // página (RNF-07: diseño responsive).
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Equipo</TableHead>
            <TableHead>Estado</TableHead>
            {hayFreezado && <TableHead>Freezada</TableHead>}
            {/* RF-03.7: el software instalado es justamente el dato por el
                que un docente entra acá antes de elegir qué reservar. */}
            {haySoftware && <TableHead>Software instalado</TableHead>}
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
              {hayFreezado && <TableCell>{equipo.freezado ? "Sí" : "No"}</TableCell>}
              {haySoftware && (
                <TableCell className="text-muted-foreground max-w-xs text-sm">
                  {equipo.softwareInstalado || "—"}
                </TableCell>
              )}
              <TableCell className="text-right">
                <div className="flex flex-wrap justify-end gap-2">
                  {/* Un cargador no se reserva (RF-03.16), así que su
                      calendario estaría siempre vacío: ofrecerlo sería un
                      camino que no lleva a ningún lado. Reportar que no anda
                      sí tiene sentido para todo lo que se presta. */}
                  {equipo.reservable && (
                    <Button asChild variant="outline" size="sm">
                      <Link to={`/inventario/equipos/${equipo.id}/calendario`}>
                        Ver calendario
                      </Link>
                    </Button>
                  )}
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setReportando(equipo)}
                  >
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

function EquiposDelCarro({ carroId }: { carroId: string }) {
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
      <p className="text-muted-foreground text-sm">
        Este carro no tiene equipos activos.
      </p>
    )
  }

  return <TablaDeEquipos equipos={equipos} />
}

/**
 * RF-03.15 — lo prestable que no está en ningún carro: un proyector, los
 * cargadores, las notebooks de otro modelo.
 *
 * Faltaba de esta pantalla, y el agujero era concreto: el proyector se puede
 * reservar —aparece en la lista al armar una reserva— pero acá no existía,
 * así que un docente no tenía desde dónde mirar si está libre el jueves ni
 * avisar que no enciende. Toda falla de un equipo suelto tenía que pasar por
 * un Admin, que es exactamente lo que RF-03.5 quiere evitar.
 *
 * Va después de los carros y no antes: un docente entra a esta pantalla a
 * ver qué computadoras hay, y los carros son esa respuesta. El panel del
 * Admin los pone primero porque ahí son una tarea de mantenimiento más.
 *
 * La sección entera desaparece si la institución no presta nada suelto: un
 * cartel de "no hay" sería contarle a la mayoría de las escuelas algo que no
 * necesitan saber.
 */
function OtrosEquipos() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["equipos", "sueltos"],
    queryFn: () => inventoryApi.listarEquipos({ soloSueltos: true }),
  })

  const equipos = (data?.data ?? []).filter((equipo) => !equipo.dadoDeBaja)
  if (isLoading || error || equipos.length === 0) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle>Otros equipos</CardTitle>
        <CardDescription>
          Lo que la escuela presta y no está en ningún carro.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <TablaDeEquipos equipos={equipos} />
      </CardContent>
    </Card>
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

      {/* Sin carros la pantalla no está necesariamente vacía: una escuela
          puede prestar solo un proyector y dos notebooks sueltas, que salen
          en la sección de abajo. Por eso el cartel habla de los carros y no
          del inventario entero. */}
      {!isLoading && carros.length === 0 && (
        <p className="text-muted-foreground mb-3">Todavía no hay carros cargados.</p>
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

        <OtrosEquipos />
      </div>
    </div>
  )
}
