// Mismo rango que el dominio académico del backend.
export const ANIO_CICLO_MINIMO = 2000
export const ANIO_CICLO_MAXIMO = 2100

export function esAnioDeCicloValido(anio: number): boolean {
  return Number.isInteger(anio) && anio >= ANIO_CICLO_MINIMO && anio <= ANIO_CICLO_MAXIMO
}

export function parsearAnioDeCiclo(texto: string): number | undefined {
  const limpio = texto.trim()
  if (!/^\d{4}$/.test(limpio)) return undefined
  const anio = Number(limpio)
  return esAnioDeCicloValido(anio) ? anio : undefined
}
