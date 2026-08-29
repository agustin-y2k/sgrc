/**
 * Para buscar sin que importen tildes ni mayúsculas: en el mostrador —y en el
 * apuro de armar una reserva antes de que empiece la clase— se escribe
 * "programacion" o "edutec", y que no aparezca "Programación" o "Carro EDUTEC"
 * se lee como que el equipo no está cargado.
 */
export function sinTildes(texto: string): string {
  return texto
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase()
}

/**
 * A partir de cuántos elementos vale la pena mostrar un buscador. Con un carro
 * cargado ya se pasa; con tres cosas sueltas una caja de búsqueda es ruido.
 *
 * Vive acá y no en cada pantalla para que las tres listas de equipos —la
 * entrega, la reserva y el bloqueo— aparezcan y desaparezcan con el mismo
 * criterio.
 */
export const MINIMO_PARA_BUSCAR = 8
