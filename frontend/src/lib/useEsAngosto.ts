import { useEffect, useState } from "react"

/**
 * Ancho a partir del cual entra una tabla cómoda. Es el mismo valor que el
 * breakpoint `sm` de Tailwind: si se cambia uno hay que cambiar el otro, y
 * está acá para que se vea que son el mismo número y no dos por casualidad.
 */
const ANCHO_SM = 640

/**
 * Si la pantalla es angosta (un teléfono).
 *
 * Existe para elegir entre DOS ESTRUCTURAS distintas, no para esconder una con
 * CSS. Dibujar las dos y tapar una con `sm:hidden` deja cada equipo dos veces
 * en el documento: pesa el doble, y en un test —donde no hay CSS que oculte
 * nada— cada búsqueda por texto encuentra dos.
 *
 * Para todo lo que sea el MISMO contenido acomodado distinto, las clases de
 * Tailwind siguen siendo el camino; esto es para cuando cambia la estructura.
 */
export function useEsAngosto(): boolean {
  const [angosto, setAngosto] = useState(() => consultar())

  useEffect(() => {
    const mq = window.matchMedia?.(`(max-width: ${ANCHO_SM - 1}px)`)
    if (!mq) return
    const alCambiar = (e: MediaQueryListEvent) => setAngosto(e.matches)
    mq.addEventListener("change", alCambiar)
    // Por si el ancho cambió entre el primer render y este efecto.
    setAngosto(mq.matches)
    return () => mq.removeEventListener("change", alCambiar)
  }, [])

  return angosto
}

// Fuera del hook para que el estado inicial no dependa de que matchMedia
// exista: en un entorno de test que no lo define, se asume pantalla ancha.
function consultar(): boolean {
  return window.matchMedia?.(`(max-width: ${ANCHO_SM - 1}px)`).matches ?? false
}
