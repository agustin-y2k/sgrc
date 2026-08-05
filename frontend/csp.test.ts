import { createHash } from "node:crypto"
import { readFileSync } from "node:fs"

/**
 * La CSP de nginx (nginx-seguridad.conf) autoriza el script inline de
 * index.html —el que aplica el tema antes de pintar— por su hash SHA-256,
 * en vez de abrir `script-src` con 'unsafe-inline'.
 *
 * El problema del hash es cómo falla: si alguien edita ese script y no
 * actualiza la CSP, el navegador lo bloquea y lo único que se nota es que
 * vuelve el fogonazo blanco al recargar en modo oscuro. Nadie relaciona eso
 * con un header, y menos meses después. Este test hace ruidosa esa
 * desincronización.
 *
 * Se lee el index.html de la fuente y no el de dist/: Vite copia el script
 * inline byte a byte (verificado comparando los dos hashes), así que no hace
 * falta compilar para correr este test.
 */

const rutaHTML = "index.html"
const rutaCSP = "nginx-seguridad.conf"

function scriptInlineDelHTML(): string {
  const html = readFileSync(rutaHTML, "utf8")
  // El resto de los <script> del index llevan src; el inline es el único
  // que abre sin atributos.
  const encontrados = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)]

  if (encontrados.length !== 1) {
    throw new Error(
      `${rutaHTML}: se esperaba exactamente 1 script inline y hay ${encontrados.length}. ` +
        `Si se agregó otro, la CSP necesita también su hash (ver ${rutaCSP}).`
    )
  }
  return encontrados[0][1]
}

function hashCSP(contenido: string): string {
  return "sha256-" + createHash("sha256").update(contenido, "utf8").digest("base64")
}

/**
 * El valor de la directiva, no el archivo entero: arriba del `add_header`
 * hay un comentario que explica cada parte de la política y menciona
 * literalmente cosas como "unsafe-inline". Buscar sobre el texto crudo hacía
 * que el test leyera la explicación en vez de la política — y diera por
 * abierto algo que en realidad está cerrado.
 */
function politica(): string {
  const csp = readFileSync(rutaCSP, "utf8")
  const directiva = /^\s*add_header Content-Security-Policy\s+"([^"]*)"/m.exec(csp)

  if (!directiva) {
    throw new Error(`${rutaCSP}: no se encontró el add_header de Content-Security-Policy`)
  }
  return directiva[1]
}

describe("CSP", () => {
  it("autoriza el script inline de index.html por su hash exacto", () => {
    const esperado = hashCSP(scriptInlineDelHTML())

    expect(
      politica().includes(`'${esperado}'`),
      `El hash del script inline de ${rutaHTML} cambió.\n` +
        `Actualizá script-src en ${rutaCSP} con:\n\n    '${esperado}'\n\n` +
        `Sin esto el navegador bloquea el script y vuelve el fogonazo blanco ` +
        `al recargar en modo oscuro.`
    ).toBe(true)
  })

  // Un 'unsafe-inline' en script-src dejaría pasar cualquier script que una
  // inyección logre meter en el HTML, que es exactamente lo que la CSP está
  // para impedir. En style-src sí está, y es deliberado (ver el comentario
  // en nginx-seguridad.conf).
  it("no abre script-src con unsafe-inline", () => {
    const scriptSrc = /script-src ([^;]*)/.exec(politica())?.[1] ?? ""

    expect(scriptSrc).not.toBe("")
    expect(scriptSrc).not.toContain("unsafe-inline")
    expect(scriptSrc).not.toContain("unsafe-eval")
  })

  // El botón de "Iniciar sesión con Google" es lo único que la SPA carga de
  // un tercero: un script y un iframe de accounts.google.com. Si la CSP deja
  // de permitirlos, el botón desaparece sin más síntoma que su ausencia.
  it("permite lo que el botón de Google necesita", () => {
    const csp = politica()

    expect(csp).toContain("https://accounts.google.com/gsi/client") // el script
    expect(csp).toContain("frame-src https://accounts.google.com/gsi/") // el botón
    expect(csp).toContain("connect-src 'self' https://accounts.google.com/gsi/")
  })
})
