/**
 * Si al Admin hay que pedirle que declare la jornada de la escuela.
 *
 * Se le pide en CADA inicio de sesión mientras la escuela no tenga ningún
 * tramo cargado. Es una molestia deliberada: quien quiere probar el sistema
 * puede trabajar sin horario declarado —no hay restricción y todo funciona—
 * pero el sistema no deja de recordarle que le falta la decisión de la que
 * dependen las reservas de toda la escuela.
 *
 * Se descarta por SESIÓN y no para siempre. Guardarlo para siempre convertiría
 * "todavía no lo decidí" en "ya lo decidí", que es justamente la confusión que
 * hace que alguien descubra el problema con doscientas reservas cargadas.
 */

const CLAVE = "sgrc:pedido-de-jornada-descartado"

/**
 * sessionStorage y no memoria: recargar la página con F5 no es volver a
 * entrar, y volver a plantar el cuestionario ahí sería castigar un refresh.
 *
 * Todo va envuelto porque el acceso mismo puede tirar: en una ventana privada,
 * o con el almacenamiento del sitio bloqueado, leerlo lanza. Ante la duda se
 * pregunta, que es el lado seguro.
 */
export function pedidoDescartado(): boolean {
  try {
    return sessionStorage.getItem(CLAVE) === "si"
  } catch {
    return false
  }
}

export function descartarPedido() {
  try {
    sessionStorage.setItem(CLAVE, "si")
  } catch {
    // Sin almacenamiento el pedido va a reaparecer al navegar. Es molesto y
    // es el lado correcto de fallar: nunca esconde la pregunta.
  }
}

/** Al cerrar sesión se olvida, para que el próximo inicio vuelva a pedirlo. */
export function olvidarPedidoDescartado() {
  try {
    sessionStorage.removeItem(CLAVE)
  } catch {
    // Ídem: si no se pudo guardar, tampoco hay nada que borrar.
  }
}
