/** Plurales escritos como se hablan. */

/** La palabra sola: `plural(2, "clase")` → "clases". */
export function plural(n: number, singular: string, formaPlural?: string) {
  if (n === 1) return singular
  return formaPlural ?? `${singular}s`
}

/** El número y la palabra: `contar(2, "clase")` → "2 clases". */
export function contar(n: number, singular: string, formaPlural?: string) {
  return `${n} ${plural(n, singular, formaPlural)}`
}
