import type { LucideIcon } from "lucide-react"
import { Link } from "react-router"

/**
 * Un atajo de la pantalla de inicio: un ícono, qué se hace ahí y para qué
 * sirve, en una tarjeta que se toca entera.
 *
 * Reemplaza a la lista de botones grises que había antes, y los cambios no
 * son decorativos:
 *
 * - El rótulo es una acción en infinitivo ("Ver las computadoras"), no el
 *   nombre de una sección del sistema ("Inventario"). Alguien que no sabe
 *   cómo está dividido el sistema no puede elegir entre nombres de módulos,
 *   pero sí entre cosas que quiere hacer.
 * - Debajo va una línea que dice para qué sirve. Sin ella, el docente entra
 *   a averiguarlo, que es exactamente la navegación a ciegas que hay que
 *   evitar.
 * - El área de toque es toda la tarjeta y no un renglón de texto: en un
 *   teléfono, un enlace de una línea es un blanco chico.
 */
export function AccesoDirecto({
  icono: Icono,
  titulo,
  ayuda,
  a,
  onClick,
}: {
  icono: LucideIcon
  titulo: string
  ayuda: string
  /** A dónde lleva. Si no está, es un botón que abre algo en esta pantalla. */
  a?: string
  onClick?: () => void
}) {
  const clases =
    "bg-superficie hover:border-primary/40 hover:bg-muted focus-visible:ring-ring flex w-full items-start gap-3 rounded-xl border p-4 text-left transition-colors focus-visible:ring-2 focus-visible:outline-none"

  const contenido = (
    <>
      <span
        aria-hidden="true"
        className="bg-accent text-accent-foreground grid size-10 shrink-0 place-items-center rounded-lg"
      >
        <Icono className="size-5" />
      </span>
      <span className="min-w-0">
        <span className="block font-medium">{titulo}</span>
        <span className="text-muted-foreground block text-sm">{ayuda}</span>
      </span>
    </>
  )

  if (a) {
    return (
      <Link to={a} className={clases}>
        {contenido}
      </Link>
    )
  }

  return (
    <button type="button" onClick={onClick} className={clases}>
      {contenido}
    </button>
  )
}
