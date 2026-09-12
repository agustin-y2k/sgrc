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

/**
 * Recorta las puntas y reduce toda corrida de espacios interna a uno solo.
 *
 * Recortar las puntas no alcanza: «Educación Física» y «Educación  Física» se
 * ven IDÉNTICAS en pantalla y son dos textos distintos. Un doble espacio lo
 * produce una celda de planilla, un copiado de un PDF o un dedo que rebotó, y
 * es invisible para quien lo carga y para quien lo revisa después.
 *
 * `\s` cubre también el espacio duro (U+00A0) que viaja al copiar desde una
 * página web, que de los tres casos es el más difícil de ver.
 *
 * Espeja a domain.CanonizarEspacios del backend, que es quien decide de verdad
 * cómo se guarda el texto.
 */
export function canonizarEspacios(texto: string): string {
  return texto.trim().replace(/\s+/g, " ")
}

/**
 * La forma en que se comparan dos nombres para decidir si son EL MISMO: sin
 * tildes, sin mayúsculas y con los espacios canonizados.
 *
 * Es la pregunta «¿esta materia ya está cargada?», distinta de «¿se escriben
 * igual?». Usa las mismas dos reglas que el backend: la normalización de la
 * columna generada y la canonización de espacios de la validación.
 */
export function claveDeNombre(texto: string): string {
  return sinTildes(canonizarEspacios(texto))
}
