import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"
import { ErrorDeConsulta } from "@/components/ErrorDeConsulta"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { getErrorMessage } from "@/lib/api-client"

import {
  consultarConfiguracion,
  guardarConfiguracion,
  type ConfiguracionOperacion,
} from "./configuracion-api"
const CAMPOS = [
  {
    clave: "retiroGraciaMinutos",
    etiqueta: "Espera para retirar una reserva (minutos)",
    min: 1,
    max: 1440,
    ayuda:
      "Se cuenta desde el inicio de la clase. Cumplido el plazo, una PC sin retirar puede liberarse si hay mostrador atendido. Una entrega registrada, aunque ya se haya devuelto, protege esa reserva. Si la clase termina antes de cumplirse el plazo, no se libera por este motivo.",
  },
  {
    clave: "recordatorioAntelacionMinutos",
    etiqueta: "Antelación del recordatorio de reserva (minutos)",
    min: 1,
    max: 1440,
    ayuda:
      "Cuánto antes de la clase se prepara el recordatorio. El envío depende de que el docente haya activado esa preferencia.",
  },
  {
    clave: "cierreGraciaMinutos",
    etiqueta: "Espera después del cierre de la escuela (minutos)",
    min: 0,
    max: 720,
    ayuda:
      "Tiempo entre el fin de la jornada declarada y el aviso a administradores por equipos que siguen afuera. Cero permite avisar desde el cierre.",
  },
  {
    clave: "cierreHoraSinJornada",
    etiqueta: "Hora del aviso si no hay jornada declarada",
    min: 0,
    max: 23,
    ayuda:
      "Hora local, de 0 a 23. Se usa sólo cuando la institución todavía no declaró su jornada; si la declaró, manda el horario de ese día.",
  },
  {
    clave: "licenciasHoraAviso",
    etiqueta: "Hora desde la que se avisan licencias",
    min: 0,
    max: 23,
    ayuda:
      "Hora local, de 0 a 23. Los días de anticipación y la duración se editan en cada licencia.",
  },
] as const

export function ConfiguracionPage() {
  const client = useQueryClient()
  const consulta = useQuery({
    queryKey: ["configuracion-operacion"],
    queryFn: consultarConfiguracion,
  })
  const [borrador, setBorrador] = useState<ConfiguracionOperacion | null>(null)
  const [guardada, setGuardada] = useState(false)
  const datos = borrador ?? consulta.data
  const guardar = useMutation({
    mutationFn: (datos: ConfiguracionOperacion) => guardarConfiguracion(datos),
    onSuccess: (actual) => {
      client.setQueryData(["configuracion-operacion"], actual)
      setBorrador(null)
      setGuardada(true)
    },
  })
  const valido =
    !!datos &&
    CAMPOS.every(
      (c) =>
        Number.isInteger(datos[c.clave]) &&
        datos[c.clave] >= c.min &&
        datos[c.clave] <= c.max
    )
  return (
    <div className="mx-auto max-w-3xl">
      <EncabezadoDePagina
        titulo="Configuración del sistema"
        descripcion="Plazos y avisos para el trabajo diario de la institución."
      />
      {consulta.error && (
        <ErrorDeConsulta
          mensaje="No pudimos consultar la configuración."
          reintentando={consulta.isFetching}
          onReintentar={() => void consulta.refetch()}
        />
      )}
      {consulta.isPending && <p role="status">Consultando configuración…</p>}
      {datos && !consulta.error && (
        <Card>
          <CardHeader>
            <CardTitle>Reservas y avisos automáticos</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground mb-4 text-sm">
              Los cambios se aplican a las próximas comprobaciones automáticas, incluidas
              las reservas pendientes. No revierten liberaciones anteriores ni repiten
              avisos ya enviados. Las comprobaciones de reservas se ejecutan cada cinco
              minutos y las de licencias según el intervalo de la instalación; el aviso
              puede aparecer en la siguiente pasada.
            </p>
            <form
              className="grid gap-5"
              onSubmit={(e) => {
                e.preventDefault()
                if (valido && borrador && !guardar.isPending) guardar.mutate(datos)
              }}
            >
              {CAMPOS.map((c) => (
                <div key={c.clave} className="grid gap-1.5">
                  <Label htmlFor={c.clave}>{c.etiqueta}</Label>
                  <Input
                    id={c.clave}
                    type="number"
                    required
                    min={c.min}
                    max={c.max}
                    step={1}
                    value={Number.isNaN(datos[c.clave]) ? "" : datos[c.clave]}
                    aria-describedby={`${c.clave}-ayuda`}
                    disabled={guardar.isPending}
                    onChange={(e) => {
                      setBorrador({ ...datos, [c.clave]: e.target.valueAsNumber })
                      setGuardada(false)
                      guardar.reset()
                    }}
                  />
                  <p id={`${c.clave}-ayuda`} className="text-muted-foreground text-sm">
                    {c.ayuda} Valores permitidos: {c.min} a {c.max}.
                  </p>
                </div>
              ))}
              {guardar.error && (
                <Alert variant="destructive">
                  <AlertDescription>
                    {getErrorMessage(guardar.error)}. Podés recargar para revisar la
                    configuración vigente; se descartará tu edición.
                  </AlertDescription>
                </Alert>
              )}
              {guardada && (
                <p role="status">
                  Configuración guardada. Los próximos controles usarán estos valores.
                </p>
              )}
              <div className="flex flex-wrap gap-2">
                <Button
                  type="submit"
                  disabled={
                    !borrador || !valido || guardar.isPending || consulta.isFetching
                  }
                >
                  {guardar.isPending ? "Guardando…" : "Guardar configuración"}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={guardar.isPending || consulta.isFetching}
                  onClick={() => {
                    setBorrador(null)
                    setGuardada(false)
                    guardar.reset()
                    void consulta.refetch()
                  }}
                >
                  Recargar configuración
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}
      <p className="text-muted-foreground mt-4 text-sm">
        La jornada y los horarios de atención se gestionan en sus pantallas. Los
        vencimientos de licencias se editan por equipo. Las sesiones y los códigos de
        recuperación conservan los límites de seguridad de la instalación.
      </p>
    </div>
  )
}
