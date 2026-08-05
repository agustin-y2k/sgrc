/**
 * Descarga de una tabla como CSV, para las pantallas de reportes.
 *
 * Existe porque un reporte que no se puede sacar del sistema termina
 * saliendo igual, en una captura de pantalla pegada en un correo. Lo que se
 * pide de estos números —mandarlos a Dirección, adjuntarlos a un pedido a
 * la DGE, sumarlos con los de otro año— se hace en una planilla, y el
 * camino más corto hasta una planilla es un archivo.
 */

/**
 * Punto y coma, no coma.
 *
 * Excel elige el separador según la configuración regional, y en es-AR el
 * decimal es la coma: un CSV separado por comas se abre con todas las
 * columnas encimadas en la primera celda, que es exactamente el momento en
 * que alguien decide que la exportación no funciona. Con punto y coma se
 * abre bien, y LibreOffice lo detecta solo en cualquier caso.
 */
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

/**
 * Dispara la descarga de `filas` como un archivo llamado `nombre`.
 *
 * El BOM del principio no es decorativo: sin él, Excel lee el archivo como
 * Latin-1 y cualquier "Matemática" o "División" aparece con los acentos
 * rotos. Es el detalle que hace que la exportación se vea hecha o no.
 */
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
