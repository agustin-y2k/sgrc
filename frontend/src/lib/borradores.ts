export const BORRADOR_RESERVA_KEY = "sgrc_borrador_reserva"

export function borrarBorradorReserva() {
  try {
    sessionStorage.removeItem(BORRADOR_RESERVA_KEY)
  } catch {
    /* El formulario sigue disponible si el navegador no permite guardar. */
  }
}

export function descartarBorradorDeOtraCuenta(usuarioId: string) {
  try {
    const texto = sessionStorage.getItem(BORRADOR_RESERVA_KEY)
    if (!texto) return
    const valor: unknown = JSON.parse(texto)
    if (
      !valor ||
      typeof valor !== "object" ||
      !("usuarioId" in valor) ||
      valor.usuarioId !== usuarioId
    )
      borrarBorradorReserva()
  } catch {
    borrarBorradorReserva()
  }
}
