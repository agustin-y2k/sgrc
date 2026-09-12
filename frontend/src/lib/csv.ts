/** Lectura y escritura de tablas en CSV. */

/** Punto y coma, no coma. Es lo que espera un Excel en español. */
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
 * Los separadores que se aceptan al LEER. Escribimos punto y coma, pero un
 * archivo que llega puede venir de cualquier lado: una planilla de Google
 * exporta con coma, un Excel en español con punto y coma, y un copiado desde
 * una tabla con tabulación. Rechazar el archivo por el separador equivocado es
 * mandar a alguien a convertirlo a mano sin decirle cómo.
 */
const SEPARADORES_ACEPTADOS = [";", ",", "\t"] as const

/**
 * Elige el separador contando cuál aparece más veces FUERA de comillas en la
 * primera línea. Contar adentro de las comillas es lo que hace fallar al
 * método ingenuo justo en el archivo que importa: una celda como
 * «"Lengua, Matemática, Física"» tiene más comas que el resto de la fila junta,
 * y un archivo separado por punto y coma se leería como separado por comas.
 */
function detectarSeparador(texto: string): string {
  const primeraLinea = texto.split(/\r?\n/, 1)[0] ?? ""

  let mejor = SEPARADOR
  let maximo = 0
  for (const candidato of SEPARADORES_ACEPTADOS) {
    let cuenta = 0
    let enComillas = false
    for (let i = 0; i < primeraLinea.length; i++) {
      const c = primeraLinea[i]
      if (c === '"') enComillas = !enComillas
      else if (c === candidato && !enComillas) cuenta++
    }
    if (cuenta > maximo) {
      maximo = cuenta
      mejor = candidato
    }
  }
  return mejor
}

/**
 * Parte un CSV en filas y celdas, respetando las comillas de RFC 4180: un
 * separador o un salto de línea entre comillas es contenido, y dos comillas
 * seguidas adentro son una comilla.
 *
 * Se recorre carácter por carácter y no con una expresión regular ni con
 * `split`: el caso que este archivo tiene que resolver —la lista de materias de
 * un curso en una sola celda entrecomillada— es exactamente el que rompe
 * cualquier `split` por el separador.
 *
 * Las filas totalmente vacías se descartan: un archivo que termina con un salto
 * de línea, que es lo normal, agregaría una fila fantasma al final.
 */
export function leerCSV(texto: string): string[][] {
  // El BOM que escribe Excel se colaría como parte del primer encabezado y lo
  // dejaría sin coincidir con ningún nombre de columna.
  const limpio = texto.replace(/^﻿/, "")
  const separador = detectarSeparador(limpio)

  const filas: string[][] = []
  let fila: string[] = []
  let celda = ""
  let enComillas = false

  const cerrarCelda = () => {
    fila.push(celda)
    celda = ""
  }
  const cerrarFila = () => {
    cerrarCelda()
    if (fila.some((c) => c.trim() !== "")) filas.push(fila)
    fila = []
  }

  for (let i = 0; i < limpio.length; i++) {
    const c = limpio[i]

    if (enComillas) {
      if (c === '"') {
        if (limpio[i + 1] === '"') {
          celda += '"'
          i++
        } else {
          enComillas = false
        }
      } else {
        celda += c
      }
      continue
    }

    if (c === '"') enComillas = true
    else if (c === separador) cerrarCelda()
    else if (c === "\n") cerrarFila()
    else if (c !== "\r") celda += c
  }
  // La última fila no termina en salto de línea si el archivo tampoco.
  if (celda !== "" || fila.length > 0) cerrarFila()

  return filas
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
