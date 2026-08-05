/**
 * Claro u oscuro.
 *
 * Por defecto manda la preferencia del sistema operativo: alguien que tiene
 * el teléfono en oscuro no debería tener que configurar nada acá tampoco. El
 * interruptor de la barra es para el caso en que esa preferencia no sirva —
 * el laboratorio con las persianas abiertas al mediodía, o al revés— y una
 * vez que se usa, la elección manda sobre el sistema y se recuerda.
 *
 * Ojo: la lógica de decidir el tema inicial está duplicada en el script
 * inline de `index.html`. Es a propósito y es el precio de no mostrar un
 * fogonazo blanco antes de que cargue el bundle. Si cambia CLAVE o el nombre
 * de la clase, hay que tocar los dos lados.
 */

export type Tema = "claro" | "oscuro"

export const CLAVE_TEMA = "sgrc-tema"

/** Lo que el usuario eligió a mano, o null si nunca tocó el interruptor. */
export function temaElegido(): Tema | null {
  try {
    const guardado = localStorage.getItem(CLAVE_TEMA)
    return guardado === "claro" || guardado === "oscuro" ? guardado : null
  } catch {
    // localStorage puede tirar excepción con las cookies bloqueadas o en
    // navegación privada de algunos navegadores. Sin preferencia guardada
    // el sistema decide, que es un buen resultado igual.
    return null
  }
}

export function temaDelSistema(): Tema {
  return typeof matchMedia === "function" &&
    matchMedia("(prefers-color-scheme: dark)").matches
    ? "oscuro"
    : "claro"
}

export function temaEfectivo(): Tema {
  return temaElegido() ?? temaDelSistema()
}

/** Prende o apaga la clase que activa la paleta oscura de index.css. */
export function aplicarTema(tema: Tema): void {
  document.documentElement.classList.toggle("dark", tema === "oscuro")
  // Para que el navegador pinte con el tema correcto lo que no controlamos:
  // las barras de scroll y los selectores de fecha nativos, que en oscuro
  // aparecían blancos en medio de un formulario oscuro.
  document.documentElement.style.colorScheme = tema === "oscuro" ? "dark" : "light"
}

export function guardarTema(tema: Tema): void {
  try {
    localStorage.setItem(CLAVE_TEMA, tema)
  } catch {
    // Si no se puede guardar, el tema igual vale para esta sesión.
  }
}
