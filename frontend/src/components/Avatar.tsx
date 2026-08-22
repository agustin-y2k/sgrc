import { useFotoDePerfil } from "@/features/perfil/useFoto"
import { cn } from "@/lib/utils"

/** La cara de una persona: su foto si subió una, y si no las iniciales. */
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
   * Falso cuando ya se sabe que no hay foto, para ni siquiera pedirla. En las
   * pantallas que no lo saben se deja en true: el 404 se cachea y se dibujan
   * las iniciales.
   */
  tieneFoto?: boolean
  /** Cambia cuando la foto cambia, para saltear la caché del navegador. */
  version?: string
  className?: string
}) {
  const foto = useFotoDePerfil(usuarioId, version, tieneFoto)
  const iniciales = (nombre[0] ?? "") + (apellido[0] ?? "")

  const clases = cn(
    "bg-accent text-accent-foreground grid size-7 shrink-0 place-items-center overflow-hidden rounded-full text-xs font-semibold",
    className
  )

  // Las iniciales son el caso normal, no un fallback de emergencia: la
  // mayoría de la gente no sube foto.
  if (foto === null) {
    return (
      <span aria-hidden="true" className={clases}>
        {iniciales}
      </span>
    )
  }

  return (
    <span aria-hidden="true" className={clases}>
      <img src={foto} alt="" className="size-full object-cover" />
    </span>
  )
}
