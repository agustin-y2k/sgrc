import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as adminApi from "@/features/admin/api"
import type { Carro, PC } from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * Alta (RF-03.2) y edición (RF-03.4/RF-03.10) de una PC.
 *
 * Es un solo componente para las dos cosas porque los campos son los mismos;
 * lo que cambia es qué se puede tocar. `identificador` y `numeroSerie` solo
 * aparecen al crear: el backend no los acepta en el PATCH (ver
 * editarPCRequest en internal/inventory/interfaces/http/dto.go), y ofrecer
 * un campo que se ignora en silencio es peor que no ofrecerlo.
 */

type CamposPC = {
  identificador: string
  numeroSerie: string
  freezado: boolean
  cpu: string
  ram: string
  sistemaOperativo: string
  softwareInstalado: string
}

const VACIO: CamposPC = {
  identificador: "",
  numeroSerie: "",
  freezado: false,
  cpu: "",
  ram: "",
  sistemaOperativo: "",
  softwareInstalado: "",
}

function desdePC(pc: PC): CamposPC {
  return {
    identificador: String(pc.identificador),
    numeroSerie: String(pc.numeroSerie),
    freezado: pc.freezado,
    cpu: pc.cpu ?? "",
    ram: pc.ram ?? "",
    sistemaOperativo: pc.sistemaOperativo ?? "",
    softwareInstalado: pc.softwareInstalado ?? "",
  }
}

/** Los campos de texto libre van sin valor en vez de con "" (RF-03.2). */
function opcional(valor: string): string | undefined {
  const limpio = valor.trim()
  return limpio === "" ? undefined : limpio
}

function CamposComunes({
  idPrefijo,
  valor,
  onChange,
}: {
  idPrefijo: string
  valor: CamposPC
  onChange: (v: CamposPC) => void
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

export function AltaDePC({ carroId }: { carroId: string }) {
  const queryClient = useQueryClient()
  const [campos, setCampos] = useState<CamposPC>(VACIO)

  const crear = useMutation({
    mutationFn: () =>
      adminApi.crearPC(carroId, {
        identificador: Number(campos.identificador),
        // El backend lo normaliza igual (a mayúsculas y sin espacios al
        // borde, ver domain.NormalizarNumeroSerie); acá se manda tal cual
        // se tipeó y la respuesta trae la forma canónica.
        numeroSerie: campos.numeroSerie.trim(),
        freezado: campos.freezado,
        cpu: opcional(campos.cpu),
        ram: opcional(campos.ram),
        sistemaOperativo: opcional(campos.sistemaOperativo),
        softwareInstalado: opcional(campos.softwareInstalado),
      }),
    onSuccess: async () => {
      setCampos(VACIO)
      await queryClient.invalidateQueries({ queryKey: ["pcs", carroId] })
    },
  })

  // El identificador es único dentro del carro y el número de serie en toda
  // la institución (RF-03.2). El backend rechaza el duplicado, así que acá
  // solo se chequea la forma.
  //
  // Y son dos formas distintas, aunque los dos campos se llamen "número":
  // el identificador es el que la escuela pinta en la máquina —"PC 1",
  // "PC 2"— y sí es un entero. El número de serie es el código de fábrica
  // de la etiqueta, y casi siempre trae letras ("5CD1234ABC"): exigirle
  // dígitos era lo que hacía imposible cargar la primera PC con el dato
  // real (ver migración 011).
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
      <p className="font-medium">Agregar una PC a este carro</p>

      {crear.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(crear.error)}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor={`alta-${carroId}-identificador`}>Identificador</Label>
          <Input
            id={`alta-${carroId}-identificador`}
            inputMode="numeric"
            value={campos.identificador}
            onChange={(e) => setCampos({ ...campos, identificador: e.target.value })}
            placeholder="El número pintado en la PC"
          />
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

      <CamposComunes idPrefijo={`alta-${carroId}`} valor={campos} onChange={setCampos} />

      {campos.identificador !== "" && !identificadorValido && (
        <p className="text-destructive text-sm">
          El identificador es un número entero: es el que está pintado en la máquina.
        </p>
      )}

      <div>
        <Button
          type="submit"
          disabled={!identificadorValido || !serieValida || crear.isPending}
        >
          {crear.isPending ? "Agregando…" : "Agregar PC"}
        </Button>
      </div>
    </form>
  )
}

export function EdicionDePC({
  pc,
  carros,
  onListo,
}: {
  pc: PC
  /** Para poder moverla a otro carro (RF-03.10). */
  carros: Carro[]
  onListo: () => void
}) {
  const queryClient = useQueryClient()
  const [campos, setCampos] = useState<CamposPC>(() => desdePC(pc))
  const [carroDestino, setCarroDestino] = useState(pc.carroId)

  const editar = useMutation({
    mutationFn: () =>
      adminApi.editarPC(pc.id, {
        // El carro solo viaja si cambió: mandarlo igual haría que el backend
        // revalide la unicidad del identificador contra el mismo carro sin
        // ninguna razón.
        carroId: carroDestino !== pc.carroId ? carroDestino : undefined,
        freezado: campos.freezado,
        cpu: opcional(campos.cpu),
        ram: opcional(campos.ram),
        sistemaOperativo: opcional(campos.sistemaOperativo),
        softwareInstalado: opcional(campos.softwareInstalado),
      }),
    onSuccess: async () => {
      // Se invalidan los dos carros: si la PC se movió, desaparece de uno y
      // aparece en el otro.
      await queryClient.invalidateQueries({ queryKey: ["pcs", pc.carroId] })
      if (carroDestino !== pc.carroId) {
        await queryClient.invalidateQueries({ queryKey: ["pcs", carroDestino] })
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
      <p className="font-medium">Editar PC {pc.identificador}</p>

      {editar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(editar.error)}</AlertDescription>
        </Alert>
      )}

      <p className="text-muted-foreground text-sm">
        El identificador ({pc.identificador}) y el número de serie ({pc.numeroSerie}) no
        se editan: identifican al equipo.
      </p>

      <CamposComunes idPrefijo={`edicion-${pc.id}`} valor={campos} onChange={setCampos} />

      <div className="grid gap-1.5">
        {/* RF-03.10: el identificador tiene que seguir siendo único dentro
            del carro destino, y de eso se encarga el backend — acá alcanza
            con mostrar el error si choca. */}
        <Label htmlFor={`edicion-${pc.id}-carro`}>Carro</Label>
        <Select
          id={`edicion-${pc.id}-carro`}
          value={carroDestino}
          onChange={(e) => setCarroDestino(e.target.value)}
        >
          {carros.map((c) => (
            <option key={c.id} value={c.id}>
              {c.nombre}
            </option>
          ))}
        </Select>
        {carroDestino !== pc.carroId && (
          <p className="text-muted-foreground text-sm">
            La PC se va a mover de carro. Sus reservas y su historial de incidencias no se
            tocan.
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
