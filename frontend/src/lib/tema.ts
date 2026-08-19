/** Claro u oscuro. */

export type Tema = "claro" | "oscuro"

export const CLAVE_TEMA = "sgrc-tema"

/** Lo que el usuario eligió a mano, o null si nunca tocó el interruptor. */
export function temaElegido(): Tema | null {
  try {
    const guardado = localStorage.getItem(CLAVE_TEMA)
    return guardado === "claro" || guardado === "oscuro" ? guardado : null
  } catch {
    // localStorage puede tirar excepción con las cookies bloqueadas o en
    // navegación privada de algunos navegadores.
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
