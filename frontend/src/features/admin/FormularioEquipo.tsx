import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as adminApi from "@/features/admin/api"
import type { Carro, Equipo } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

/** Alta (RF-03.2) y edición (RF-03.4/RF-03.10) de un equipo. */

/**
 * Los datos de la máquina. Están aparte porque no son de "un equipo de carro"
 * sino de "una computadora": una notebook suelta tiene los mismos, y la
 * pantalla de Otros equipos reusa estos campos cuando el equipo está marcado
 * como computadora (RF-03.15).
 */
export type FichaTecnica = {
  freezado: boolean
  cpu: string
  ram: string
  sistemaOperativo: string
  softwareInstalado: string
}

export const FICHA_VACIA: FichaTecnica = {
  freezado: false,
  cpu: "",
  ram: "",
  sistemaOperativo: "",
  softwareInstalado: "",
}

type CamposEquipo = FichaTecnica & {
  identificador: string
  numeroSerie: string
}

const VACIO: CamposEquipo = {
  identificador: "",
  numeroSerie: "",
  freezado: false,
  cpu: "",
  ram: "",
  sistemaOperativo: "",
  softwareInstalado: "",
}

function desdeEquipo(equipo: Equipo): CamposEquipo {
  return {
    identificador: String(equipo.identificador),
    numeroSerie: String(equipo.numeroSerie),
    freezado: equipo.freezado,
    cpu: equipo.cpu ?? "",
    ram: equipo.ram ?? "",
    sistemaOperativo: equipo.sistemaOperativo ?? "",
    softwareInstalado: equipo.softwareInstalado ?? "",
  }
}

/** Los campos de texto libre van sin valor en vez de con "" (RF-03.2). */
function opcional(valor: string): string | undefined {
  const limpio = valor.trim()
  return limpio === "" ? undefined : limpio
}

/**
 * Genérico sobre `T extends FichaTecnica` para que sirva igual con los campos
 * de un equipo de carro —que llevan además identificador y serie— y con los de
 * una computadora suelta: el spread conserva lo que cada uno traiga de más.
 */
export function CamposDeLaMaquina<T extends FichaTecnica>({
  idPrefijo,
  valor,
  onChange,
}: {
  idPrefijo: string
  valor: T
  onChange: (v: T) => void
}) {
  return (
    <>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor={`${idPrefijo}-cpu`}>CPU</Label>
          <Input
            id={`${idPrefijo}-cpu`}
            value={valor.cpu}
            onChange={(e) => onChange({ ...valor, cpu: e.target.value })}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor={`${idPrefijo}-ram`}>RAM</Label>
          <Input
            id={`${idPrefijo}-ram`}
            value={valor.ram}
            onChange={(e) => onChange({ ...valor, ram: e.target.value })}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor={`${idPrefijo}-so`}>Sistema operativo</Label>
          <Input
            id={`${idPrefijo}-so`}
            value={valor.sistemaOperativo}
            onChange={(e) => onChange({ ...valor, sistemaOperativo: e.target.value })}
          />
        </div>
        <div className="grid gap-1.5">
          {/* RF-03.7: es el dato por el que un docente entra al inventario
              antes de elegir qué reservar, así que tenerlo al día importa
              más que el resto de la ficha técnica. */}
          <Label htmlFor={`${idPrefijo}-software`}>Software instalado</Label>
          <Input
            id={`${idPrefijo}-software`}
            value={valor.softwareInstalado}
            onChange={(e) => onChange({ ...valor, softwareInstalado: e.target.value })}
            placeholder="Ej.: AutoCAD 2027, Office 2021"
          />
        </div>
      </div>

      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={valor.freezado}
          onChange={(e) => onChange({ ...valor, freezado: e.target.checked })}
        />
        Freezada (los cambios se descartan al reiniciar)
      </label>
    </>
  )
}

export function AltaDeEquipo({ carroId }: { carroId: string }) {
  const queryClient = useQueryClient()
  const [campos, setCampos] = useState<CamposEquipo>(VACIO)

  const crear = useMutation({
    mutationFn: () =>
      adminApi.crearEquipoDeCarro(carroId, {
        identificador: Number(campos.identificador),
        // El backend lo normaliza igual (a mayúsculas y sin espacios al
        // borde, ver domain.NormalizarNumeroSerie); acá se manda tal cual se
        // tipeó y la respuesta trae la forma canónica.
        numeroSerie: campos.numeroSerie.trim(),
        freezado: campos.freezado,
        cpu: opcional(campos.cpu),
        ram: opcional(campos.ram),
        sistemaOperativo: opcional(campos.sistemaOperativo),
        softwareInstalado: opcional(campos.softwareInstalado),
      }),
    onSuccess: async () => {
      setCampos(VACIO)
      await queryClient.invalidateQueries({ queryKey: ["equipos", carroId] })
    },
  })

  // El identificador es único dentro del carro y el número de serie en toda
  // la institución (RF-03.2).
  const identificadorValido = /^\d+$/.test(campos.identificador.trim())
  const serieValida = campos.numeroSerie.trim() !== ""

  return (
    <form
      className="grid gap-3 rounded-md border p-3"
      onSubmit={(e) => {
        e.preventDefault()
        crear.mutate()
      }}
    >
      <p className="font-medium">Agregar un equipo a este carro</p>

      {crear.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(crear.error)}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          {/* "Número de máquina" y no "Identificador": lo segundo no dice
              qué escribir, y quien completa el formulario tiene la máquina
              adelante con el número pintado. La aclaración va debajo y
              siempre visible, no solo cuando el campo ya está mal: sirve
              para no equivocarse, no para enterarse después. */}
          <Label htmlFor={`alta-${carroId}-identificador`}>Número de máquina</Label>
          <Input
            id={`alta-${carroId}-identificador`}
            inputMode="numeric"
            aria-describedby={`alta-${carroId}-identificador-ayuda`}
            value={campos.identificador}
            onChange={(e) => setCampos({ ...campos, identificador: e.target.value })}
            placeholder="Ej. 1"
          />
          <p
            id={`alta-${carroId}-identificador-ayuda`}
            className="text-muted-foreground text-sm"
          >
            El número pintado en la máquina, que es el del zócalo que ocupa en el carro.
            Va solo el número: 1, 2, 3…
          </p>
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor={`alta-${carroId}-serie`}>Número de serie</Label>
          {/* Sin inputMode="numeric": en un teléfono eso abre el teclado
              numérico, y el código de fábrica lleva letras. */}
          <Input
            id={`alta-${carroId}-serie`}
            autoCapitalize="characters"
            autoCorrect="off"
            spellCheck={false}
            value={campos.numeroSerie}
            onChange={(e) => setCampos({ ...campos, numeroSerie: e.target.value })}
            placeholder="El de la etiqueta, ej. 5CD1234ABC"
          />
        </div>
      </div>

      <CamposDeLaMaquina
        idPrefijo={`alta-${carroId}`}
        valor={campos}
        onChange={setCampos}
      />

      {campos.identificador !== "" && !identificadorValido && (
        <p className="text-destructive text-sm">
          El número de máquina va sin letras: escribí solo el número pintado, por ejemplo
          1. El código con letras es el número de serie, que va en el campo de al lado.
        </p>
      )}

      <div>
        <Button
          type="submit"
          disabled={!identificadorValido || !serieValida || crear.isPending}
        >
          {crear.isPending ? "Agregando…" : "Agregar al carro"}
        </Button>
      </div>
    </form>
  )
}

export function EdicionDeEquipo({
  equipo,
  carros,
  onListo,
}: {
  equipo: Equipo
  /** Para poder moverla a otro carro (RF-03.10). */
  carros: Carro[]
  onListo: () => void
}) {
  const queryClient = useQueryClient()
  const [campos, setCampos] = useState<CamposEquipo>(() => desdeEquipo(equipo))
  const [carroDestino, setCarroDestino] = useState(equipo.carroId)

  const editar = useMutation({
    mutationFn: () =>
      adminApi.editarEquipo(equipo.id, {
        // El carro solo viaja si cambió: mandarlo igual haría que el backend
        // revalide la unicidad del identificador contra el mismo carro sin
        // ninguna razón.
        carroId: carroDestino !== equipo.carroId ? carroDestino : undefined,
        freezado: campos.freezado,
        cpu: opcional(campos.cpu),
        ram: opcional(campos.ram),
        sistemaOperativo: opcional(campos.sistemaOperativo),
        softwareInstalado: opcional(campos.softwareInstalado),
      }),
    onSuccess: async () => {
      // Se invalidan los dos carros: si el equipo se movió, desaparece de uno y
      // aparece en el otro.
      await queryClient.invalidateQueries({ queryKey: ["equipos", equipo.carroId] })
      if (carroDestino !== equipo.carroId) {
        await queryClient.invalidateQueries({ queryKey: ["equipos", carroDestino] })
      }
      onListo()
    },
  })

  return (
    <form
      className="grid gap-3 rounded-md border p-3"
      onSubmit={(e) => {
        e.preventDefault()
        editar.mutate()
      }}
    >
      <p className="font-medium">Editar {equipo.etiqueta}</p>

      {editar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(editar.error)}</AlertDescription>
        </Alert>
      )}

      <p className="text-muted-foreground text-sm">
        El número de máquina ({equipo.identificador}) y el número de serie (
        {equipo.numeroSerie}) no se editan: identifican al equipo.
      </p>

      <CamposDeLaMaquina
        idPrefijo={`edicion-${equipo.id}`}
        valor={campos}
        onChange={setCampos}
      />

      <div className="grid gap-1.5">
        {/* RF-03.10: el identificador tiene que seguir siendo único dentro
            del carro destino, y de eso se encarga el backend — acá alcanza
            con mostrar el error si choca. */}
        <Label htmlFor={`edicion-${equipo.id}-carro`}>Carro</Label>
        <Select
          id={`edicion-${equipo.id}-carro`}
          value={carroDestino}
          onChange={(e) => setCarroDestino(e.target.value)}
        >
          {carros.map((c) => (
            <option key={c.id} value={c.id}>
              {c.nombre}
            </option>
          ))}
        </Select>
        {carroDestino !== equipo.carroId && (
          <p className="text-muted-foreground text-sm">
            El equipo se va a mover de carro. Sus reservas y su historial de incidencias
            no se tocan.
          </p>
        )}
      </div>

      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={editar.isPending}>
          {editar.isPending ? "Guardando…" : "Guardar cambios"}
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onListo}>
          Volver
        </Button>
      </div>
    </form>
  )
}
