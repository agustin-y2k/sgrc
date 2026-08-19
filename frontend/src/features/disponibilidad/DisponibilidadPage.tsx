import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { SelectorDeHora } from "@/components/SelectorDeHora"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import { useAuth } from "@/features/auth/AuthContext"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { DIAS_SEMANA, etiquetaDia, ordenarPorDia } from "@/features/disponibilidad/types"
import type {
  AdminDisponibilidad,
  BloqueHorario,
  DiaSemana,
  Excepcion,
  TipoExcepcion,
} from "@/features/disponibilidad/types"
import { getErrorMessage } from "@/lib/api-client"
import { EncabezadoDePagina } from "@/components/EncabezadoDePagina"

/** Una sola raíz: cambiar mi horario también cambia lo que ve el resto. */
const DISPONIBILIDAD_KEY = ["disponibilidad"]
const ADMINS_KEY = [...DISPONIBILIDAD_KEY, "admins"]
const MI_HORARIO_KEY = [...DISPONIBILIDAD_KEY, "mi-horario"]

/**
 * "disponibleAhora" lo calcula el backend contra la hora del servidor en el
 * momento de la consulta, así que la respuesta se vuelve vieja sola: a las
 * 09:59 dice "disponible" y a las 10:01 ya no.
 */
const REFRESCO_MS = 60_000

function estadoDeExcepcion(e: Excepcion): string {
  if (e.tipo === "NO_DISPONIBLE") return "Hoy no viene"
  return `Hoy con horario distinto: ${e.horaInicio}–${e.horaFin}`
}

function HorarioSemanal({ bloques }: { bloques: BloqueHorario[] }) {
  if (bloques.length === 0) {
    return <p className="text-muted-foreground text-sm">Sin horario cargado.</p>
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {ordenarPorDia(bloques).map((b) => (
        <Badge key={b.id} variant="outline">
          {etiquetaDia(b.diaSemana)} {b.horaInicio}–{b.horaFin}
        </Badge>
      ))}
    </div>
  )
}

function TarjetaDeAdmin({ admin }: { admin: AdminDisponibilidad }) {
  return (
    <Card>
      <CardContent className="grid gap-2 pt-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="font-medium">
            {admin.nombre} {admin.apellido}
          </p>
          {admin.disponibleAhora ? (
            <Badge>Disponible ahora</Badge>
          ) : (
            <Badge variant="secondary">No disponible</Badge>
          )}
        </div>

        {/* La excepción explica por qué el estado no coincide con el
            horario de abajo — sin esto, un Admin que hoy no viene se ve
            como "no disponible" en pleno horario habitual y parece un
            error de la pantalla. */}
        {admin.excepcionHoy && (
          <p className="text-sm">
            {estadoDeExcepcion(admin.excepcionHoy)}
            {admin.excepcionHoy.motivo && ` — ${admin.excepcionHoy.motivo}`}
          </p>
        )}

        <HorarioSemanal bloques={admin.horarioSemanal} />
      </CardContent>
    </Card>
  )
}

/** Campos de un bloque, compartidos por el alta y la edición. */
type FormBloque = { diaSemana: DiaSemana; horaInicio: string; horaFin: string }

const BLOQUE_VACIO: FormBloque = {
  diaSemana: "LUNES",
  horaInicio: "08:00",
  horaFin: "12:00",
}

function CamposDeBloque({
  valor,
  onChange,
  idPrefijo,
}: {
  valor: FormBloque
  onChange: (v: FormBloque) => void
  idPrefijo: string
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <div className="grid gap-2">
        <Label htmlFor={`${idPrefijo}-dia`}>Día</Label>
        <Select
          id={`${idPrefijo}-dia`}
          value={valor.diaSemana}
          onChange={(e) => onChange({ ...valor, diaSemana: e.target.value as DiaSemana })}
        >
          {DIAS_SEMANA.map((d) => (
            <option key={d.valor} value={d.valor}>
              {d.etiqueta}
            </option>
          ))}
        </Select>
      </div>
      <SelectorDeHora
        id={`${idPrefijo}-inicio`}
        etiqueta="Desde"
        valor={valor.horaInicio}
        onCambio={(v) => onChange({ ...valor, horaInicio: v })}
      />
      <SelectorDeHora
        id={`${idPrefijo}-fin`}
        etiqueta="Hasta"
        valor={valor.horaFin}
        onCambio={(v) => onChange({ ...valor, horaFin: v })}
      />
    </div>
  )
}

/** RF-07.1/07.3 — el patrón semanal propio del Admin autenticado. */
function MiHorario() {
  const queryClient = useQueryClient()
  const [nuevo, setNuevo] = useState<FormBloque>(BLOQUE_VACIO)
  const [editandoId, setEditandoId] = useState<string | null>(null)
  const [edicion, setEdicion] = useState<FormBloque>(BLOQUE_VACIO)

  const { data, isLoading, error } = useQuery({
    queryKey: MI_HORARIO_KEY,
    queryFn: () => disponibilidadApi.miHorario(),
  })

  const invalidar = () => queryClient.invalidateQueries({ queryKey: DISPONIBILIDAD_KEY })

  const agregar = useMutation({
    mutationFn: (b: FormBloque) =>
      disponibilidadApi.agregarBloque(b.diaSemana, b.horaInicio, b.horaFin),
    onSuccess: async () => {
      setNuevo(BLOQUE_VACIO)
      await invalidar()
    },
  })

  const editar = useMutation({
    mutationFn: ({ id, ...b }: FormBloque & { id: string }) =>
      disponibilidadApi.editarBloque(id, b),
    onSuccess: async () => {
      setEditandoId(null)
      await invalidar()
    },
  })

  // La lambda no sobra: react-query invoca mutationFn con (variables,
  // contexto), así que pasar la función de la API pelada le agrega un segundo
  // argumento con el QueryClient adentro.
  const eliminar = useMutation({
    mutationFn: (id: string) => disponibilidadApi.eliminarBloque(id),
    onSuccess: invalidar,
  })

  const bloques = ordenarPorDia(data?.data ?? [])
  const errorAccion = agregar.error ?? editar.error ?? eliminar.error

  // El rango lo valida el backend (hora_fin > hora_inicio, en domain y en un
  // CHECK), pero avisar antes evita mandar un request que ya sabemos que va a
  // fallar.
  const rangoNuevoInvalido = nuevo.horaFin <= nuevo.horaInicio
  const rangoEdicionInvalido = edicion.horaFin <= edicion.horaInicio

  function empezarAEditar(b: BloqueHorario) {
    setEditandoId(b.id)
    setEdicion({
      diaSemana: b.diaSemana,
      horaInicio: b.horaInicio,
      horaFin: b.horaFin,
    })
  }

  return (
    <section className="mt-8 grid gap-3">
      <h2 className="text-xl font-semibold">Mi horario</h2>
      <p className="text-muted-foreground text-sm">
        Los cambios valen desde ahora en adelante, para todas las semanas: no hay que
        cargarlo semana por semana.
      </p>

      {(error || errorAccion) && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error ?? errorAccion)}</AlertDescription>
        </Alert>
      )}

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}

      {!isLoading && bloques.length === 0 && (
        <p className="text-muted-foreground">
          Todavía no cargaste ningún bloque: figurás como no disponible siempre.
        </p>
      )}

      <div className="grid gap-2">
        {bloques.map((b) =>
          editandoId === b.id ? (
            <Card key={b.id}>
              <CardContent className="grid gap-3 pt-4">
                <CamposDeBloque
                  valor={edicion}
                  onChange={setEdicion}
                  idPrefijo={`editar-${b.id}`}
                />
                {rangoEdicionInvalido && (
                  <p className="text-destructive text-sm">
                    La hora de fin tiene que ser posterior a la de inicio.
                  </p>
                )}
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    disabled={rangoEdicionInvalido || editar.isPending}
                    onClick={() => editar.mutate({ id: b.id, ...edicion })}
                  >
                    Guardar
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setEditandoId(null)}>
                    Cancelar
                  </Button>
                </div>
              </CardContent>
            </Card>
          ) : (
            <Card key={b.id}>
              <CardContent className="flex flex-wrap items-center justify-between gap-2 pt-4">
                <span>
                  {etiquetaDia(b.diaSemana)} · {b.horaInicio}–{b.horaFin}
                </span>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => empezarAEditar(b)}>
                    Editar
                  </Button>
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={eliminar.isPending}
                    onClick={() => eliminar.mutate(b.id)}
                  >
                    Eliminar
                  </Button>
                </div>
              </CardContent>
            </Card>
          )
        )}
      </div>

      <Card>
        <CardContent className="grid gap-3 pt-4">
          <p className="font-medium">Agregar un bloque</p>
          <CamposDeBloque valor={nuevo} onChange={setNuevo} idPrefijo="nuevo" />
          {rangoNuevoInvalido && (
            <p className="text-destructive text-sm">
              La hora de fin tiene que ser posterior a la de inicio.
            </p>
          )}
          <div>
            <Button
              size="sm"
              disabled={rangoNuevoInvalido || agregar.isPending}
              onClick={() => agregar.mutate(nuevo)}
            >
              Agregar
            </Button>
          </div>
        </CardContent>
      </Card>
    </section>
  )
}

/**
 * RF-07.4/07.5 — lo puntual: un día distinto al patrón, o el atajo para
 * marcarse ausente ahora mismo.
 */
function MisExcepciones() {
  const queryClient = useQueryClient()
  const [fecha, setFecha] = useState("")
  const [tipo, setTipo] = useState<TipoExcepcion>("NO_DISPONIBLE")
  const [horaInicio, setHoraInicio] = useState("08:00")
  const [horaFin, setHoraFin] = useState("12:00")
  const [motivo, setMotivo] = useState("")
  const [confirmacion, setConfirmacion] = useState<string | null>(null)

  const invalidar = () => queryClient.invalidateQueries({ queryKey: DISPONIBILIDAD_KEY })

  const noDisponibleAhora = useMutation({
    mutationFn: () => disponibilidadApi.marcarNoDisponibleAhora(),
    onSuccess: async () => {
      setConfirmacion("Quedaste marcado como no disponible por el resto del día.")
      await invalidar()
    },
  })

  const cargar = useMutation({
    mutationFn: (e: Parameters<typeof disponibilidadApi.cargarExcepcion>[0]) =>
      disponibilidadApi.cargarExcepcion(e),
    onSuccess: async () => {
      setConfirmacion("Excepción guardada.")
      setFecha("")
      setMotivo("")
      await invalidar()
    },
  })

  const rangoInvalido = tipo === "HORARIO_MODIFICADO" && horaFin <= horaInicio
  const errorAccion = noDisponibleAhora.error ?? cargar.error

  function guardar() {
    setConfirmacion(null)
    cargar.mutate({
      fecha,
      tipo,
      // NO_DISPONIBLE no lleva horario: mandarlo igual lo rechaza el
      // backend con 400 (chk_excepcion_horario_coherente).
      ...(tipo === "HORARIO_MODIFICADO" ? { horaInicio, horaFin } : {}),
      ...(motivo.trim() !== "" ? { motivo: motivo.trim() } : {}),
    })
  }

  return (
    <section className="mt-8 grid gap-3">
      <h2 className="text-xl font-semibold">Excepciones</h2>

      {errorAccion && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(errorAccion)}</AlertDescription>
        </Alert>
      )}
      {confirmacion && !errorAccion && (
        <Alert>
          <AlertDescription>{confirmacion}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardContent className="grid gap-3 pt-4">
          <p className="font-medium">Hoy no estoy</p>
          <p className="text-muted-foreground text-sm">
            Un paso, para cuando ya empezó el día: llegaste tarde, te tuviste que ir, no
            vas a poder venir. Vale solo para hoy y no toca tu horario habitual.
          </p>
          <div>
            <Button
              variant="outline"
              size="sm"
              disabled={noDisponibleAhora.isPending}
              onClick={() => {
                setConfirmacion(null)
                noDisponibleAhora.mutate()
              }}
            >
              No estoy disponible ahora
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="grid gap-3 pt-4">
          <p className="font-medium">Un día puntual distinto</p>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="excepcion-fecha">Fecha</Label>
              <Input
                id="excepcion-fecha"
                type="date"
                value={fecha}
                onChange={(e) => setFecha(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="excepcion-tipo">Qué pasa ese día</Label>
              <Select
                id="excepcion-tipo"
                value={tipo}
                onChange={(e) => setTipo(e.target.value as TipoExcepcion)}
              >
                <option value="NO_DISPONIBLE">No vengo</option>
                <option value="HORARIO_MODIFICADO">Vengo en otro horario</option>
              </Select>
            </div>
          </div>

          {tipo === "HORARIO_MODIFICADO" && (
            <div className="grid gap-3 sm:grid-cols-2">
              <SelectorDeHora
                id="excepcion-inicio"
                etiqueta="Desde"
                valor={horaInicio}
                onCambio={setHoraInicio}
              />
              <SelectorDeHora
                id="excepcion-fin"
                etiqueta="Hasta"
                valor={horaFin}
                onCambio={setHoraFin}
              />
            </div>
          )}

          <div className="grid gap-2">
            <Label htmlFor="excepcion-motivo">Motivo (opcional)</Label>
            <Input
              id="excepcion-motivo"
              value={motivo}
              onChange={(e) => setMotivo(e.target.value)}
              placeholder="Capacitación, licencia…"
            />
          </div>

          {rangoInvalido && (
            <p className="text-destructive text-sm">
              La hora de fin tiene que ser posterior a la de inicio.
            </p>
          )}

          {/* Cargar dos veces la misma fecha reemplaza la anterior en vez de
              acumular (UNIQUE por usuario y fecha en la base). */}
          <p className="text-muted-foreground text-sm">
            Si ya habías cargado una excepción para esa fecha, esta la reemplaza.
          </p>

          <div>
            <Button
              size="sm"
              disabled={fecha === "" || rangoInvalido || cargar.isPending}
              onClick={guardar}
            >
              Guardar excepción
            </Button>
          </div>
        </CardContent>
      </Card>
    </section>
  )
}

/**
 * RF-07 — quién de los Admins está en el laboratorio ahora y con qué horario
 * habitual.
 */
export function DisponibilidadPage() {
  const { user } = useAuth()

  const { data, isLoading, error } = useQuery({
    queryKey: ADMINS_KEY,
    queryFn: () => disponibilidadApi.listarDisponibilidadDeAdmins(),
    refetchInterval: REFRESCO_MS,
  })

  const admins = data?.data ?? []

  return (
    <div className="mx-auto max-w-3xl">
      {/* RF-07.6: importa decirlo. Alguien podría suponer que si ningún
          Admin figura disponible no se puede reservar, y no es así. */}
      <EncabezadoDePagina
        titulo="Disponibilidad de Admins"
        descripcion="Es informativo: sirve para saber a quién buscar en el laboratorio. No cambia en nada lo que podés hacer en el sistema."
      />

      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {isLoading && <p className="text-muted-foreground">Cargando…</p>}

      {!isLoading && admins.length === 0 && (
        <p className="text-muted-foreground">No hay ningún Admin aprobado.</p>
      )}

      <div className="grid gap-3">
        {admins.map((a) => (
          <TarjetaDeAdmin key={a.usuarioId} admin={a} />
        ))}
      </div>

      {user?.rol === "ADMIN" && (
        <>
          <MiHorario />
          <MisExcepciones />
        </>
      )}
    </div>
  )
}
