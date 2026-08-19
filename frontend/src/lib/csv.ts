/** Descarga de una tabla como CSV, para las pantallas de reportes. */

/** Punto y coma, no coma. */
const SEPARADOR = ";"

function escapar(valor: string | number): string {
  const texto = String(valor)
  // Comillas dobles alrededor solo si hace falta; las de adentro se duplican.
  return /[";\r\n]/.test(texto) ? `"${texto.replace(/"/g, '""')}"` : texto
}

export function filasACSV(filas: (string | number)[][]): string {
  // CRLF: es lo que espera Excel, y lo que dice el RFC 4180.
  return filas.map((fila) => fila.map(escapar).join(SEPARADOR)).join("\r\n")
}

/** Dispara la descarga de `filas` como un archivo llamado `nombre`. */
export function descargarCSV(nombre: string, filas: (string | number)[][]): void {
  const blob = new Blob(["﻿", filasACSV(filas)], {
    type: "text/csv;charset=utf-8",
  })
  const url = URL.createObjectURL(blob)
  const enlace = document.createElement("a")
  enlace.href = url
  enlace.download = nombre.endsWith(".csv") ? nombre : `${nombre}.csv`
  enlace.click()
  URL.revokeObjectURL(url)
}
