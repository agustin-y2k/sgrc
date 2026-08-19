/**
 * Plurales escritos como se hablan.
 *
 * El sistema decía "1 equipo(s) entregado(s)" y "Hoy ya pasaron 1 clase(s)".
 * Esa notación es una forma de no elegir: le pasa al lector el trabajo de
 * resolver la frase. En una pantalla que atiende alguien apurado, con
 * alguien más esperando del otro lado del mostrador, se lee como un sistema
 * a medio terminar — y quien recién empieza a usarlo no sabe si el paréntesis
 * significa algo que él no entiende.
 *
 * El caso difícil del castellano —"1 clase" contra "0 clases", que en inglés
 * se resuelve igual pero en otras lenguas no— está cubierto: solo el 1 es
 * singular.
 */

/** La palabra sola: `plural(2, "clase")` → "clases". */
export function plural(n: number, singular: string, formaPlural?: string) {
  if (n === 1) return singular
  return formaPlural ?? `${singular}s`
}

/**
 * El número y la palabra: `contar(2, "clase")` → "2 clases".
 *
 * Para las palabras que no pluralizan agregando una "s" se pasa la forma
 * completa: `contar(n, "lápiz", "lápices")`.
 */
export function contar(n: number, singular: string, formaPlural?: string) {
  return `${n} ${plural(n, singular, formaPlural)}`
}
