import { Fragment, useState } from "react"
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
import { useAuth } from "@/features/auth/AuthContext"
import { useEsAngosto } from "@/lib/useEsAngosto"
import * as inventoryApi from "@/features/inventory/api"
import { CuentasDeEquipo } from "@/features/inventory/CuentasDeEquipo"
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
 */
function TablaDeEquipos({ equipos }: { equipos: Equipo[] }) {
  // RF-03.5: cualquier usuario autenticado puede reportar una falla, y esta
  // es la pantalla donde un docente ya está mirando los equipos.
  const [reportando, setReportando] = useState<Equipo | null>(null)
  // Las cuentas de un equipo (RF-03.22) se abren de a una: son varias líneas
  // por equipo y desplegarlas todas convertiría la tabla en un muro.
  const [viendoCuentas, setViendoCuentas] = useState<Equipo | null>(null)
  const { user } = useAuth()
  const esAdmin = user?.rol === "ADMIN"
  const angosto = useEsAngosto()

  // "Freezada" y "Software instalado" son datos de una computadora: en una
  // tabla de proyectores y cargadores serían dos columnas de guiones.
  const hayFreezado = equipos.some((equipo) => equipo.tipo === "PC")
  const haySoftware = equipos.some((equipo) => equipo.softwareInstalado)

  // Las acciones de un equipo, en un solo lugar: las dibujan la tabla y las
  // tarjetas, y si divergieran una de las dos formas ofrecería menos que la
  // otra sin que nadie lo note.
  function Acciones({ equipo }: { equipo: Equipo }) {
    return (
      <>
        {/* Un cargador no se reserva (RF-03.16), así que su calendario
            estaría siempre vacío: ofrecerlo sería un camino que no lleva a
            ningún lado. Reportar que no anda sí tiene sentido para todo lo
            que se presta. */}
        {equipo.reservable && (
          <Button asChild variant="outline" size="sm">
            <Link to={`/inventario/equipos/${equipo.id}/calendario`}>Ver calendario</Link>
          </Button>
        )}
        {/* Con qué usuario se entra a esta máquina (RF-03.22). Lo ve
            cualquier autenticado: la cuenta y su privilegio no son el
            secreto, y un docente parado frente a la notebook necesita
            saberlo. Qué contraseña se le revela lo decide el servidor,
            cuenta por cuenta.

            El botón aparece solo si hay algo anotado. El tipo es texto
            libre, así que el sistema no puede deducir que un cargador no
            tiene cuentas — pero sí sabe que no tiene ninguna, y eso alcanza.
            Para un Admin está siempre, porque si no, no habría cómo anotar
            la primera. */}
        {(esAdmin || equipo.tieneCuentas) && (
          <Button
            variant="outline"
            size="sm"
            aria-expanded={viendoCuentas?.id === equipo.id}
            onClick={() =>
              setViendoCuentas(viendoCuentas?.id === equipo.id ? null : equipo)
            }
          >
            Cómo entrar
          </Button>
        )}
        <Button variant="outline" size="sm" onClick={() => setReportando(equipo)}>
          Reportar problema
        </Button>
      </>
    )
  }

  // Lo que se abre desde los botones de un equipo. Va PEGADO al equipo en las
  // dos formas, no al final de la lista: en un carro de 31 máquinas, abrirlo
  // desde la primera lo dejaba treinta filas más abajo, fuera de la pantalla,
  // y desde el mostrador parecía que el botón no había hecho nada.
  //
  // Estuvo afuera hasta ahora porque un formulario adentro de una celda queda
  // ilegible en un teléfono. Eso dejó de ser un problema cuando la lista pasó
  // a ser tarjetas abajo de 640px: la tabla existe solo donde hay ancho.
  function PanelDelEquipo({ equipo }: { equipo: Equipo }) {
    if (reportando?.id !== equipo.id && viendoCuentas?.id !== equipo.id) return null
    return (
      <div className="grid gap-3">
        {reportando?.id === equipo.id && (
          <ReportarIncidencia equipo={equipo} onListo={() => setReportando(null)} />
        )}
        {viendoCuentas?.id === equipo.id && <CuentasDeEquipo equipo={equipo} />}
      </div>
    )
  }

  // Cuántas columnas tiene la tabla, para que el panel las cruce enteras.
  const columnas = 3 + (hayFreezado ? 1 : 0) + (haySoftware ? 1 : 0)

  return (
    <div>
      {/* En un teléfono, cada equipo es una tarjeta.

          La tabla scrollea de costado y no se pierde nada, pero las acciones
          —"Reportar problema" es EL motivo por el que un docente entra acá—
          quedan fuera de la pantalla y hay que descubrir un gesto lateral
          adentro de una tarjeta. Apiladas están a la vista.

          Es el mismo criterio que ya usa "Mis reservas", que nunca fue una
          tabla.

          Se elige UNA de las dos y no se tapa la otra con CSS: son
          estructuras distintas, y dibujar las dos deja cada equipo dos veces
          en el documento. */}
      {angosto ? (
        <div className="grid gap-2">
          {equipos.map((equipo) => (
            <div key={equipo.id} className="grid gap-2 rounded-md border p-3">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="font-medium">{equipo.etiqueta}</span>
                <EstadoDeEquipo estado={equipo.estado} />
                {hayFreezado && equipo.freezado && (
                  <span className="text-muted-foreground text-xs">Freezada</span>
                )}
              </div>
              {/* El software es el dato por el que un docente entra acá antes de
                elegir qué reservar (RF-03.7). Sin encabezado: en una tarjeta
                se lee como lo que es. */}
              {haySoftware && equipo.softwareInstalado && (
                <p className="text-muted-foreground text-sm">
                  {equipo.softwareInstalado}
                </p>
              )}
              <div className="flex flex-wrap gap-2">
                <Acciones equipo={equipo} />
              </div>
              <PanelDelEquipo equipo={equipo} />
            </div>
          ))}
        </div>
      ) : (
        /* De ahí para arriba, la tabla: con ancho de sobra compara mejor una
         fila con otra. Scrollea igual, por si el software de una máquina es
         largo. */
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
                <Fragment key={equipo.id}>
                  <TableRow>
                    <TableCell className="font-medium">{equipo.etiqueta}</TableCell>
                    <TableCell>
                      <EstadoDeEquipo estado={equipo.estado} />
                    </TableCell>
                    {hayFreezado && (
                      <TableCell>{equipo.freezado ? "Sí" : "No"}</TableCell>
                    )}
                    {haySoftware && (
                      <TableCell className="text-muted-foreground max-w-xs text-sm">
                        {equipo.softwareInstalado || "—"}
                      </TableCell>
                    )}
                    <TableCell className="text-right">
                      <div className="flex flex-wrap justify-end gap-2">
                        <Acciones equipo={equipo} />
                      </div>
                    </TableCell>
                  </TableRow>
                  {/* En su propia fila y cruzando todas las columnas: así el
                      panel queda debajo del equipo del que salió, y no
                      encajado en una celda. */}
                  {(reportando?.id === equipo.id || viendoCuentas?.id === equipo.id) && (
                    <TableRow className="hover:bg-transparent">
                      <TableCell colSpan={columnas} className="py-3">
                        <PanelDelEquipo equipo={equipo} />
                      </TableCell>
                    </TableRow>
                  )}
                </Fragment>
              ))}
            </TableBody>
          </Table>
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
        titulo="Las computadoras"
        descripcion="Cuáles hay en cada carro y qué programas tiene cada una. Tocá una para ver su calendario o para avisar que no anda. Más abajo están los otros equipos que se prestan."
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
