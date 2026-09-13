import { useEffect, useState } from "react"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import { etiquetaDeCurso } from "@/features/academico/types"
import * as authApi from "@/features/auth/api"
import type { LugarParaRegistro } from "@/features/auth/types"

/**
 * Dónde trabaja y qué dicta, al registrarse (RF-01.3).
 *
 * **Es un `select` y no un campo con sugerencias.** La primera versión usaba
 * `<input list>`, que despliega lo cargado y además acepta cualquier texto —
 * elegante, y con un problema: no se ve. Quien abre el formulario ve un campo
 * de texto común, escribe «4to 2da» y nunca se entera de que había una lista.
 * Un desplegable se ve, y el texto libre pasa a estar donde se lo busca: al
 * final, después de una línea, en «Otro».
 *
 * El valor que viaja sigue siendo TEXTO, no una referencia: es lo que la
 * persona dice, para que el Admin sepa a qué asignarla —o que tiene que crear
 * el curso antes de poder hacerlo.
 */

/** Lo que se elige cuando no está en la lista. No puede ser el nombre de nada. */
export const OTRO = "__otro__"

/**
 * La lista llega de una ruta pública, porque esta pantalla se abre sin sesión.
 * Trae sólo nombres del ciclo activo (ver el handler).
 *
 * Con `useEffect` y no con react-query, igual que BotonGoogle y por la misma
 * razón: login y registro se abren ANTES de la sesión, fuera del armazón de la
 * aplicación y de su QueryClient.
 */
export function useLugaresDeRegistro() {
  const [lugares, setLugares] = useState<LugarParaRegistro[]>([])
  const [cargado, setCargado] = useState(false)

  useEffect(() => {
    // cancelado evita tocar el estado si el componente se desmontó mientras se
    // cargaba — pasa al navegar rápido entre login y registro.
    let cancelado = false
    authApi
      .opcionesDeRegistro()
      .then((data) => {
        if (cancelado) return
        setLugares(data.lugares)
        setCargado(true)
      })
      // Sin lista, los dos campos pasan a ser de texto libre y el registro anda
      // igual. Nadie se queda sin poder anotarse porque esta consulta falle.
      .catch(() => {
        if (!cancelado) setCargado(true)
      })
    return () => {
      cancelado = true
    }
  }, [])

  return { lugares, cargado }
}

/** El nombre con el que se muestra y se manda un lugar: «4°2 · Electromecánica». */
export function etiquetaDeLugar(l: LugarParaRegistro): string {
  return etiquetaDeCurso({ nombre: l.nombre, modalidad: l.modalidad })
}

/**
 * Un desplegable con lo que existe, una línea, y «Otro» al final.
 *
 * Cuando se elige «Otro» aparece debajo el campo de texto. Es un componente
 * porque el curso y la materia hacen exactamente lo mismo.
 */
function DesplegableConOtro({
  id,
  rotulo,
  ayuda,
  opciones,
  valor,
  onChange,
  placeholderOtro,
  rotuloVacio,
}: {
  id: string
  rotulo: string
  ayuda: string
  opciones: string[]
  /** El texto elegido o escrito. Vacío = todavía no eligió. */
  valor: string
  onChange: (valor: string) => void
  placeholderOtro: string
  rotuloVacio: string
}) {
  // `esOtro` no se deduce del valor: alguien puede escribir a mano algo que
  // también está en la lista, y en ese caso el desplegable saltaría solo a esa
  // opción mientras escribe. Es estado propio.
  const [esOtro, setEsOtro] = useState(false)
  const enLista = opciones.includes(valor)

  return (
    <div className="grid gap-1.5">
      <Label htmlFor={id}>{rotulo}</Label>
      <Select
        id={id}
        value={esOtro ? OTRO : enLista ? valor : ""}
        onChange={(e) => {
          const elegido = e.target.value
          if (elegido === OTRO) {
            setEsOtro(true)
            // Se limpia: lo que había era una opción de la lista, y dejarla
            // puesta haría que el campo de texto naciera con algo que la
            // persona no escribió.
            onChange("")
            return
          }
          setEsOtro(false)
          onChange(elegido)
        }}
      >
        <option value="">{rotuloVacio}</option>
        {opciones.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
        {/* La línea separa lo que la escuela tiene cargado de la salida de
            emergencia. `disabled` porque no es una opción: es un renglón. */}
        <option disabled>──────────</option>
        <option value={OTRO}>Otro (escribirlo)</option>
      </Select>

      {esOtro && (
        <Input
          id={`${id}-otro`}
          aria-label={`${rotulo}: escribilo`}
          value={valor}
          maxLength={100}
          placeholder={placeholderOtro}
          onChange={(e) => onChange(e.target.value)}
        />
      )}

      <p className="text-muted-foreground text-xs">{ayuda}</p>
    </div>
  )
}

export function SelectorDeCursoSolicitado({
  idPrefijo,
  value,
  onChange,
  lugares,
  cargado,
}: {
  idPrefijo: string
  value: string
  onChange: (curso: string) => void
  lugares: LugarParaRegistro[]
  cargado: boolean
}) {
  const opciones = lugares.map(etiquetaDeLugar)

  // Sin lista —porque la consulta falló o porque la escuela todavía no cargó
  // nada— el campo es de texto a secas. Un desplegable con una sola opción
  // («Otro») sería una puerta con un cartel que dice «puerta».
  if (cargado && opciones.length === 0) {
    return (
      <div className="grid gap-1.5">
        <Label htmlFor={`${idPrefijo}-curso`}>Curso o lugar donde trabajás</Label>
        <Input
          id={`${idPrefijo}-curso`}
          value={value}
          maxLength={100}
          placeholder="Ej.: 4°2"
          onChange={(e) => onChange(e.target.value)}
        />
        <p className="text-muted-foreground text-xs">Como lo nombra tu escuela.</p>
      </div>
    )
  }

  return (
    <DesplegableConOtro
      id={`${idPrefijo}-curso`}
      rotulo="Curso o lugar donde trabajás"
      rotuloVacio={cargado ? "Elegí una opción…" : "Cargando…"}
      ayuda="Si no está en la lista, elegí «Otro» y escribilo — Dirección, Biblioteca, Preceptoría."
      opciones={opciones}
      valor={value}
      onChange={onChange}
      placeholderOtro="Ej.: Biblioteca"
    />
  )
}

export function SelectorDeMateriaSolicitada({
  idPrefijo,
  value,
  onChange,
  lugarElegido,
}: {
  idPrefijo: string
  value: string
  onChange: (materia: string) => void
  /** El lugar de la lista que se eligió, o null si escribió uno propio. */
  lugarElegido: LugarParaRegistro | null
}) {
  // Un lugar que no dicta materias (RF-02.13): preguntar cuál sería volver a la
  // ceremonia que ese modelo saca. Se completa solo con el nombre del lugar,
  // que es para lo que se reserva ahí.
  useEffect(() => {
    if (lugarElegido?.tipo === "ESPACIO" && value !== lugarElegido.nombre) {
      onChange(lugarElegido.nombre)
    }
  }, [lugarElegido, value, onChange])

  if (lugarElegido?.tipo === "ESPACIO") {
    return (
      <p className="text-muted-foreground text-sm">
        En <b>{lugarElegido.nombre}</b> no se dicta una materia: se reserva para el
        lugar. No hay nada más que completar acá.
      </p>
    )
  }

  // Curso escrito a mano: no existe todavía, así que no hay materias que
  // ofrecer. Texto libre directo, sin un desplegable vacío de por medio.
  if (lugarElegido === null) {
    return (
      <div className="grid gap-1.5">
        <Label htmlFor={`${idPrefijo}-materia`}>Materia</Label>
        <Input
          id={`${idPrefijo}-materia`}
          value={value}
          maxLength={100}
          placeholder="Ej.: Programación"
          onChange={(e) => onChange(e.target.value)}
        />
        <p className="text-muted-foreground text-xs">
          Como ese curso todavía no está cargado, escribí la materia.
        </p>
      </div>
    )
  }

  return (
    <DesplegableConOtro
      id={`${idPrefijo}-materia`}
      rotulo="Materia"
      rotuloVacio="Elegí una materia…"
      ayuda="Son las que ese curso tiene cargadas. Si falta la tuya, elegí «Otro» y escribila."
      opciones={lugarElegido.materias}
      valor={value}
      onChange={onChange}
      placeholderOtro="Ej.: Programación"
    />
  )
}
