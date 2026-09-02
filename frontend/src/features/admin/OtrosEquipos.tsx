import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import * as adminApi from "@/features/admin/api"
import { AvisoDeCascada } from "@/features/admin/AvisoDeCascada"
import * as inventoryApi from "@/features/inventory/api"
import { CuentasDeEquipo } from "@/features/inventory/CuentasDeEquipo"
import type { ResultadoCascada } from "@/features/admin/types"
import {
  ETIQUETA_ESTADO_EQUIPO,
  TRANSICIONES_DE_ESTADO,
} from "@/features/inventory/types"
import type { Equipo, EstadoEquipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-03.15 — lo prestable que no está en ningún carro: un proyector, los
 * cargadores, las notebooks de otro modelo.
 */

const EQUIPOS_KEY = ["equipos-sueltos"]

type CambioEstado = {
  equipo: Equipo
  nuevoEstado: EstadoEquipo
  motivo: string
}

function Alta({ tiposUsados, onListo }: { tiposUsados: string[]; onListo: () => void }) {
  const queryClient = useQueryClient()
  const [tipo, setTipo] = useState("")
  const [nombre, setNombre] = useState("")
  const [numeroSerie, setNumeroSerie] = useState("")
  const [reservable, setReservable] = useState(false)

  const crear = useMutation({
    mutationFn: () =>
      adminApi.crearEquipoSuelto({
        tipo: tipo.trim(),
        nombre: nombre.trim(),
        numeroSerie: numeroSerie.trim(),
        reservable,
      }),
    onSuccess: async () => {
      setTipo("")
      setNombre("")
      setNumeroSerie("")
      setReservable(false)
      await queryClient.invalidateQueries({ queryKey: EQUIPOS_KEY })
      onListo()
    },
  })

  return (
    <form
      className="grid gap-3 rounded-md border border-dashed p-3"
      onSubmit={(e) => {
        e.preventDefault()
        crear.mutate()
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor="equipo-tipo">¿Qué es?</Label>
          <Input
            id="equipo-tipo"
            value={tipo}
            onChange={(e) => setTipo(e.target.value)}
            placeholder="Ej.: Proyector, Cargador, Notebook"
            list="tipos-de-equipo"
            required
          />
          {/* Texto libre con sugerencias: la lista de cosas que presta una
              escuela no es la misma que la de otra, y con una lista cerrada
              agregar "impresora 3D" pediría tocar el sistema. */}
          {tiposUsados.length > 0 && (
            <datalist id="tipos-de-equipo">
              {tiposUsados.map((t) => (
                <option key={t} value={t} />
              ))}
            </datalist>
          )}
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="equipo-nombre">¿Cómo lo llaman?</Label>
          <Input
            id="equipo-nombre"
            value={nombre}
            onChange={(e) => setNombre(e.target.value)}
            placeholder="Ej.: Cargador 1"
            required
          />
          {/* El nombre es lo único que lo distingue: dos filas llamadas
              "Cargador" serían indistinguibles justo donde hay que elegir
              cuál se está prestando. */}
          <p className="text-muted-foreground text-xs">
            Si hay más de uno igual, numeralos: Cargador 1, Cargador 2.
          </p>
        </div>
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="equipo-numero-serie">
          Número de serie <span className="text-muted-foreground">(si tiene)</span>
        </Label>
        <Input
          id="equipo-numero-serie"
          value={numeroSerie}
          onChange={(e) => setNumeroSerie(e.target.value)}
          placeholder="El de la etiqueta de fábrica"
        />
        {/* Opcional para todo, no solo para las notebooks: un proyector tiene
            serie —y es de lo que más se extravía— y un cargador no tiene
            ninguna. Exigirlo obligaría a inventar valores, que es peor que
            dejarlo vacío. */}
        <p className="text-muted-foreground text-xs">
          Dejalo vacío si el equipo no tiene. Es el número de fábrica, el que sirve para
          reclamarlo si se pierde.
        </p>
      </div>

      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          className="mt-1"
          checked={reservable}
          onChange={(e) => setReservable(e.target.checked)}
        />
        <span>
          Se puede reservar con anticipación
          <span className="text-muted-foreground block text-xs">
            Marcalo para un proyector, que alguien puede querer para una clase. Dejalo sin
            marcar para un cargador: se presta en el momento y aparecería como ruido cada
            vez que un docente va a reservar.
          </span>
        </span>
      </label>

      {crear.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(crear.error)}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={crear.isPending}>
          Agregar
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onListo}>
          Cancelar
        </Button>
      </div>
    </form>
  )
}

/**
 * Corregir lo que se cargó mal, o cambiar de idea sobre si algo se reserva.
 */
function Edicion({
  equipo,
  tiposUsados,
  onListo,
}: {
  equipo: Equipo
  tiposUsados: string[]
  onListo: () => void
}) {
  const queryClient = useQueryClient()
  const [tipo, setTipo] = useState(equipo.tipo ?? "")
  const [nombre, setNombre] = useState(equipo.nombre ?? "")
  const [numeroSerie, setNumeroSerie] = useState(equipo.numeroSerie ?? "")
  const [reservable, setReservable] = useState(equipo.reservable ?? false)

  const guardar = useMutation({
    mutationFn: () =>
      adminApi.editarEquipo(equipo.id, {
        tipo: tipo.trim(),
        nombre: nombre.trim(),
        numeroSerie: numeroSerie.trim(),
        reservable,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: EQUIPOS_KEY })
      onListo()
    },
  })

  // Dejar de ser reservable no cancela nada: el backend solo saca al equipo
  // de la lista de libres de ahí en adelante.
  const dejaDeSerReservable = equipo.reservable && !reservable

  return (
    <form
      className="grid gap-3 rounded-md border border-dashed p-3"
      onSubmit={(e) => {
        e.preventDefault()
        guardar.mutate()
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor={`editar-tipo-${equipo.id}`}>¿Qué es?</Label>
          <Input
            id={`editar-tipo-${equipo.id}`}
            value={tipo}
            onChange={(e) => setTipo(e.target.value)}
            list="tipos-de-equipo"
            required
          />
          {tiposUsados.length > 0 && (
            <datalist id="tipos-de-equipo">
              {tiposUsados.map((t) => (
                <option key={t} value={t} />
              ))}
            </datalist>
          )}
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor={`editar-nombre-${equipo.id}`}>¿Cómo lo llaman?</Label>
          <Input
            id={`editar-nombre-${equipo.id}`}
            value={nombre}
            onChange={(e) => setNombre(e.target.value)}
            required
          />
        </div>
      </div>

      {/* Editable y no solo cargable en el alta: los equipos que ya estaban
          en el sistema no tienen serie, y sin esto habría que darlos de baja
          y recrearlos —perdiendo su historial— solo para anotarla. */}
      <div className="grid gap-1.5">
        <Label htmlFor={`editar-serie-${equipo.id}`}>
          Número de serie <span className="text-muted-foreground">(si tiene)</span>
        </Label>
        <Input
          id={`editar-serie-${equipo.id}`}
          value={numeroSerie}
          onChange={(e) => setNumeroSerie(e.target.value)}
          placeholder="El de la etiqueta de fábrica"
        />
      </div>

      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          className="mt-1"
          checked={reservable}
          onChange={(e) => setReservable(e.target.checked)}
        />
        <span>Se puede reservar con anticipación</span>
      </label>

      {dejaDeSerReservable && (
        <p className="text-muted-foreground text-sm">
          Deja de aparecer cuando un docente arma una reserva. Las reservas que ya tenga
          siguen en pie: se siguen entregando normalmente.
        </p>
      )}

      {guardar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(guardar.error)}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={guardar.isPending}>
          Guardar
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onListo}>
          Cancelar
        </Button>
      </div>
    </form>
  )
}

export function OtrosEquipos() {
  const queryClient = useQueryClient()
  const [agregando, setAgregando] = useState(false)
  const [editando, setEditando] = useState<string | null>(null)
  const [dandoDeBaja, setDandoDeBaja] = useState<Equipo | null>(null)
  const [viendoCuentas, setViendoCuentas] = useState<string | null>(null)
  const [cambiando, setCambiando] = useState<CambioEstado | null>(null)

  const [cascada, setCascada] = useState<ResultadoCascada | null>(null)

  // RF-03.8 — lo mismo que para una computadora de carro: un proyector
  // también se rompe, y antes lo único que se podía hacer con él era darlo de
  // baja, que además borra su historial de la vista y libera su nombre.
  const cambiarEstado = useMutation({
    mutationFn: ({ equipo, nuevoEstado, motivo }: CambioEstado) =>
      adminApi.cambiarEstadoEquipo(equipo.id, nuevoEstado, motivo.trim() || undefined),
    onSuccess: async (resultado) => {
      setCambiando(null)
      setCascada(resultado)
      await queryClient.invalidateQueries({ queryKey: EQUIPOS_KEY })
    },
  })

  const darDeBaja = useMutation({
    mutationFn: (equipo: Equipo) => adminApi.darDeBajaEquipo(equipo.id),
    onSuccess: async (resultado) => {
      setDandoDeBaja(null)
      setCascada(resultado)
      await queryClient.invalidateQueries({ queryKey: EQUIPOS_KEY })
    },
  })

  const { data, isLoading, error } = useQuery({
    queryKey: EQUIPOS_KEY,
    queryFn: () => inventoryApi.listarEquipos({ soloSueltos: true }),
  })

  const equipos = (data?.data ?? []).filter((e) => !e.dadoDeBaja)
  const tiposUsados = useMemo(
    () => [...new Set(equipos.map((e) => e.tipo))].sort(),
    [equipos]
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>Otros equipos</CardTitle>
        <CardDescription>
          Lo que se presta y no está en ningún carro: un proyector, cargadores, notebooks
          sueltas. Se entregan y se reciben en la misma pantalla que las computadoras.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {isLoading && <p className="text-muted-foreground text-sm">Cargando…</p>}
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{getErrorMessage(error)}</AlertDescription>
          </Alert>
        )}

        {!isLoading && equipos.length === 0 && (
          <p className="text-muted-foreground text-sm">
            No hay ningún equipo cargado todavía.
          </p>
        )}

        {/* Los errores de cambiar el estado y de dar de baja se muestran
            DENTRO del recuadro de confirmación, al lado del botón que se
            apretó: acá arriba, con la lista larga, quedaban fuera de la
            pantalla y el botón parecía no hacer nada. */}
        <AvisoDeCascada resultado={cascada} />

        {equipos.map((e) => {
          const editandoEste = editando === e.id
          const bajandoEste = dandoDeBaja?.id === e.id
          const cuentasAbiertas = viendoCuentas === e.id
          const cambiandoEste = cambiando?.equipo.id === e.id

          return (
            <div key={e.id} className="grid gap-2 rounded-md border p-3">
              {/* Nombre arriba y botones abajo SIEMPRE, no lado a lado a
                  partir de `sm`. Es el mismo problema que ya tenía el
                  inventario de un carro: la fila de botones es `shrink-0` y
                  crece con cada función nueva —hoy son cinco, con los dos
                  cambios de estado—, así que al nombre y sus etiquetas les
                  tocaba un pedazo cada vez más angosto y "Solo préstamo"
                  bajaba a un renglón propio. */}
              <div className="grid gap-2">
                <div className="min-w-0">
                  <p className="flex flex-wrap items-center gap-1.5 font-medium">
                    {e.nombre}
                    {/* El estado primero: dice si el equipo se puede usar hoy.
                        Que además se reserve o no es la regla de siempre. */}
                    <EstadoBadge tono={TONO_PC[e.estado]}>
                      {ETIQUETA_ESTADO_EQUIPO[e.estado]}
                    </EstadoBadge>
                    {e.reservable ? (
                      <EstadoBadge tono="info">Se puede reservar</EstadoBadge>
                    ) : (
                      <EstadoBadge tono="neutro">Solo préstamo</EstadoBadge>
                    )}
                  </p>
                  <p className="text-muted-foreground text-sm">
                    {e.tipo}
                    {/* Se muestra solo si lo tiene: una línea "Serie: —" en cada
                        cargador es ruido en la lista que más se mira. */}
                    {e.numeroSerie && (
                      <>
                        {" · "}
                        <span className="font-mono">{e.numeroSerie}</span>
                      </>
                    )}
                  </p>
                </div>
                {!editandoEste && !bajandoEste && !cambiandoEste && (
                  <div className="flex flex-wrap gap-2">
                    {/* Solo las transiciones que el backend acepta: fuera de
                        servicio es terminal, y para eso está Dar de baja. */}
                    {TRANSICIONES_DE_ESTADO[e.estado].map((destino) => (
                      <Button
                        key={destino}
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          // Sin esto, el error de un intento anterior sobre
                          // OTRO equipo aparece en este recuadro recién
                          // abierto.
                          cambiarEstado.reset()
                          setCambiando({ equipo: e, nuevoEstado: destino, motivo: "" })
                        }}
                      >
                        → {ETIQUETA_ESTADO_EQUIPO[destino]}
                      </Button>
                    ))}
                    <Button variant="outline" size="sm" onClick={() => setEditando(e.id)}>
                      Editar
                    </Button>
                    {/* Con qué cuenta se entra a este equipo (RF-03.22). Una
                        notebook suelta es justamente la que alguien se lleva y
                        abre lejos del laboratorio. */}
                    <Button
                      variant="outline"
                      size="sm"
                      aria-expanded={cuentasAbiertas}
                      onClick={() => setViendoCuentas(cuentasAbiertas ? null : e.id)}
                    >
                      Cómo entrar
                    </Button>
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() => {
                        darDeBaja.reset()
                        setDandoDeBaja(e)
                      }}
                    >
                      Dar de baja
                    </Button>
                  </div>
                )}
              </div>

              {/* RF-03.8: sacar un equipo de disponible cancela sus reservas
                  futuras, y volver a disponible NO las restaura. Por eso se
                  avisa antes y no después. */}
              {cambiandoEste && cambiando && (
                <div className="grid gap-2 rounded-md border p-3">
                  {cambiando.nuevoEstado !== "DISPONIBLE" ? (
                    <p className="text-destructive text-sm">
                      Pasar {e.nombre} a{" "}
                      {ETIQUETA_ESTADO_EQUIPO[cambiando.nuevoEstado].toLowerCase()}{" "}
                      cancela sus reservas futuras y avisa a cada docente. Deja de poder
                      entregarse: para llevarlo al técnico, registrá una salida a
                      reparación desde Entregas.
                    </p>
                  ) : (
                    <p className="text-muted-foreground text-sm">
                      El equipo vuelve a poder reservarse y entregarse. Las reservas que
                      se cancelaron mientras no lo estaba no se recuperan.
                    </p>
                  )}
                  <div className="grid gap-1.5">
                    <Label htmlFor={`motivo-suelto-${e.id}`}>Motivo (opcional)</Label>
                    <Input
                      id={`motivo-suelto-${e.id}`}
                      value={cambiando.motivo}
                      onChange={(ev) =>
                        setCambiando({ ...cambiando, motivo: ev.target.value })
                      }
                      placeholder="Se incluye en el aviso al docente"
                    />
                  </div>
                  {cambiarEstado.error && (
                    <Alert variant="destructive">
                      <AlertDescription>
                        {getErrorMessage(cambiarEstado.error)}
                      </AlertDescription>
                    </Alert>
                  )}
                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      variant="destructive"
                      disabled={cambiarEstado.isPending}
                      onClick={() => cambiarEstado.mutate(cambiando)}
                    >
                      Confirmar cambio
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setCambiando(null)}
                    >
                      Volver
                    </Button>
                  </div>
                </div>
              )}

              {editandoEste && (
                <Edicion
                  equipo={e}
                  tiposUsados={tiposUsados}
                  onListo={() => setEditando(null)}
                />
              )}

              {cuentasAbiertas && <CuentasDeEquipo equipo={e} />}

              {bajandoEste && (
                <div className="grid gap-2 rounded-md border p-3">
                  <p className="text-destructive text-sm">
                    Dar de baja {e.nombre} lo saca del inventario y cancela sus reservas
                    futuras. Si está prestado, el sistema no deja darlo de baja: marcá
                    primero que volvió.
                  </p>
                  {/* El nombre y la serie se liberan al dar de baja: un
                      cargador que se rompe y se reemplaza se va a seguir
                      llamando igual, y la misma máquina se puede volver a
                      cargar con la serie que trae de fábrica. */}
                  <p className="text-muted-foreground text-sm">
                    El nombre y el número de serie quedan libres para volver a usarlos en
                    el equipo que lo reemplace.
                  </p>
                  {darDeBaja.error && (
                    <Alert variant="destructive">
                      <AlertDescription>
                        {getErrorMessage(darDeBaja.error)}
                      </AlertDescription>
                    </Alert>
                  )}
                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      variant="destructive"
                      disabled={darDeBaja.isPending}
                      onClick={() => darDeBaja.mutate(e)}
                    >
                      Confirmar baja
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setDandoDeBaja(null)}
                    >
                      Volver
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )
        })}

        {/* Mientras se edita uno no se ofrece el alta: los dos formularios
            tienen los mismos campos, y verlos juntos no deja saber cuál se
            está completando. Mismo criterio que el inventario de un carro. */}
        {editando !== null ? null : agregando ? (
          <Alta tiposUsados={tiposUsados} onListo={() => setAgregando(false)} />
        ) : (
          <div>
            <Button variant="outline" size="sm" onClick={() => setAgregando(true)}>
              Agregar equipo
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
