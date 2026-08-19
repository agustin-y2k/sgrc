import { useState } from "react"

import { urlDeFoto } from "@/features/perfil/api"
import { cn } from "@/lib/utils"

/**
 * La cara de una persona: su foto si subió una, y si no las iniciales.
 *
 * Las iniciales no son un placeholder mientras carga: son el estado normal
 * de casi todas las cuentas. Subir una foto es opcional a propósito —nadie
 * tiene que hacerlo para poder trabajar— así que el caso sin foto tiene que
 * verse bien, no verse "incompleto".
 *
 * Si la imagen falla (la borraron desde otra pantalla, la sesión venció, la
 * red se cortó) se vuelve a las iniciales en silencio. Un cuadrado roto con
 * el ícono de imagen partida al lado del nombre de alguien es peor que no
 * mostrar nada.
 */
export function Avatar({
  usuarioId,
  nombre,
  apellido,
  tieneFoto = true,
  version,
  className,
}: {
  usuarioId: string
  nombre: string
  apellido: string
  /**
   * Falso cuando ya se sabe que no hay foto, para no pedir una imagen que
   * va a dar 404. En las pantallas que no lo saben se deja en true: el
   * fallback cubre el caso.
   */
  tieneFoto?: boolean
  /** Cambia cuando la foto cambia, para saltear la caché del navegador. */
  version?: string
  className?: string
}) {
  const [fallo, setFallo] = useState(false)
  const iniciales = (nombre[0] ?? "") + (apellido[0] ?? "")

  const clases = cn(
    "bg-accent text-accent-foreground grid size-7 shrink-0 place-items-center overflow-hidden rounded-full text-xs font-semibold",
    className
  )

  if (!tieneFoto || fallo) {
    return (
      <span aria-hidden="true" className={clases}>
        {iniciales}
      </span>
    )
  }

  return (
    <span aria-hidden="true" className={clases}>
      <img
        src={urlDeFoto(usuarioId, version)}
        alt=""
        className="size-full object-cover"
        onError={() => setFallo(true)}
      />
    </span>
  )
}
