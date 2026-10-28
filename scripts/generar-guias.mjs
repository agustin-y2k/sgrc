#!/usr/bin/env node
import { spawn } from "node:child_process"
import { createHash, randomBytes } from "node:crypto"
import { cp, mkdir, mkdtemp, readFile, readdir, writeFile } from "node:fs/promises"
import { createServer } from "node:net"
import { tmpdir } from "node:os"
import { dirname, join, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import { anotarHTML, prepararAnotaciones } from "../docs/guias/generar/anotaciones.mjs"

const raiz = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const soloPDF = process.argv.length === 3 && process.argv[2] === "--solo-pdf"
if (process.argv.length > 2 && !soloPDF)
  throw new Error("Uso: generar-guias.mjs [--solo-pdf]")
const salida = await mkdtemp(join(tmpdir(), "sgrc-guias-"))
const publicacion = join(salida, "publicacion")
await mkdir(join(publicacion, "imagenes"), { recursive: true })
const proyecto = `sgrc-guias-${randomBytes(5).toString("hex")}`
const composeFile = join(salida, "compose.json")
const clave = () => randomBytes(32).toString("hex")
const env = { ...process.env, TZ: "America/Argentina/Buenos_Aires" }
for (const nombre of Object.keys(env))
  if (
    nombre.startsWith("GUIA_") ||
    nombre.startsWith("E2E_") ||
    nombre === "CARPETA_CAPTURAS"
  )
    delete env[nombre]
Object.assign(env, {
  GUIA_ADMIN_EMAIL: "administracion@escuela.test",
  GUIA_ADMIN_PASSWORD: `Guia1!${randomBytes(20).toString("hex")}`,
  GUIA_DOCENTE_EMAIL: "ana.gomez@escuela.test",
  GUIA_DOCENTE_PASSWORD: `Docente1!${randomBytes(20).toString("hex")}`,
  SALIDA: salida,
  GUIA_IMAGENES: join(publicacion, "imagenes"),
})
let hijo
let interrumpido = false
for (const senal of ["SIGINT", "SIGTERM"])
  process.on(senal, () => {
    interrumpido = true
    hijo?.kill("SIGTERM")
  })
async function ejecutar(comando, args, cwd = raiz) {
  if (interrumpido) throw new Error("Generación interrumpida")
  await new Promise((ok, fallo) => {
    hijo = spawn(comando, args, { cwd, env, stdio: "inherit" })
    hijo.once("error", fallo)
    hijo.once("exit", (codigo) => {
      hijo = undefined
      codigo === 0 ? ok() : fallo(new Error(`${comando}: salida ${codigo}`))
    })
  })
}
const compose = (...args) =>
  ejecutar("docker", ["compose", "-p", proyecto, "-f", composeFile, ...args], salida)
async function puertoLibre() {
  const servidor = createServer()
  await new Promise((ok, fallo) => {
    servidor.once("error", fallo)
    servidor.listen(0, "127.0.0.1", ok)
  })
  const puerto = servidor.address().port
  await new Promise((ok, fallo) => servidor.close((e) => (e ? fallo(e) : ok())))
  return puerto
}
console.log(`Generación de guías: ${salida}`)
const guias = [
  ["guia-docentes.html", "SGRC-guia-docentes.pdf"],
  ["guia-admins.html", "SGRC-guia-administradores.pdf"],
]
let capturas
try {
  if (!soloPDF) {
    const puerto = await puertoLibre()
    const origen = `http://127.0.0.1:${puerto}`
    env.GUIA_BASE_URL = origen
    const backend = `${proyecto}-backend:local`,
      frontend = `${proyecto}-frontend:local`
    const datos = {
      POSTGRES_HOST: "postgres",
      POSTGRES_PORT: "5432",
      POSTGRES_DB: "guias",
      POSTGRES_USER: "guias",
      POSTGRES_PASSWORD: clave(),
      JWT_SECRET: clave(),
      JWT_ACCESS_TTL: "24h",
      JWT_REMEMBER_TTL: "720h",
      OUTBOX_SECRET: clave(),
      CUENTAS_SECRET: clave(),
      SEED_ADMIN_EMAIL: env.GUIA_ADMIN_EMAIL,
      SEED_ADMIN_PASSWORD: env.GUIA_ADMIN_PASSWORD,
      FRONTEND_ORIGIN: origen,
      APP_TIMEZONE: env.TZ,
      APP_PORT: "8080",
      TRUSTED_PROXIES: "172.31.45.0/24",
      SMTP_HOST: "",
      GOOGLE_CLIENT_ID: "",
    }
    await writeFile(
      composeFile,
      JSON.stringify({
        services: {
          postgres: {
            image: "postgres:16-alpine",
            tmpfs: ["/var/lib/postgresql/data"],
            environment: {
              POSTGRES_DB: datos.POSTGRES_DB,
              POSTGRES_USER: datos.POSTGRES_USER,
              POSTGRES_PASSWORD: datos.POSTGRES_PASSWORD,
            },
            healthcheck: {
              test: ["CMD-SHELL", "pg_isready -h 127.0.0.1 -U guias -d guias"],
              interval: "2s",
              timeout: "3s",
              retries: 30,
            },
          },
          "sgrc-app": {
            image: backend,
            environment: datos,
            depends_on: { postgres: { condition: "service_healthy" } },
            healthcheck: {
              test: ["CMD", "/sgrc-app", "healthcheck"],
              interval: "2s",
              timeout: "5s",
              retries: 30,
            },
          },
          frontend: {
            image: frontend,
            ports: [`127.0.0.1:${puerto}:80`],
            depends_on: { "sgrc-app": { condition: "service_healthy" } },
            healthcheck: {
              test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1/health"],
              interval: "2s",
              timeout: "5s",
              retries: 30,
            },
          },
        },
        networks: {
          default: { ipam: { config: [{ subnet: "172.31.45.0/24" }] } },
        },
      }),
      { mode: 0o600 }
    )
    await ejecutar("docker", ["build", "-t", backend, "."])
    await ejecutar("docker", ["build", "-t", frontend, "frontend"])
    await compose("up", "-d", "--wait", "--wait-timeout", "120")
    await ejecutar("node", ["docs/guias/generar/capturar-actual.mjs"])
    capturas = prepararAnotaciones(
      JSON.parse(await readFile(join(salida, "capturas.json"), "utf8"))
    )
  } else {
    await cp(join(raiz, "docs/guias/imagenes"), join(publicacion, "imagenes"), {
      recursive: true,
    })
    const generacion = JSON.parse(
      await readFile(join(raiz, "docs/guias/generacion.json"), "utf8")
    )
    capturas = prepararAnotaciones({ capturas: generacion.imagenes })
    for (const captura of capturas.capturas) {
      const contenido = await readFile(join(publicacion, "imagenes", captura.archivo))
      if (createHash("sha256").update(contenido).digest("hex") !== captura.sha256)
        throw new Error(
          `Las marcas no corresponden a ${captura.archivo}; ejecutar make capturas`
        )
    }
  }
  // Se preparan ambos documentos antes de sustituir los archivos publicados.
  for (const [html, pdf] of guias) {
    let contenido = await readFile(join(raiz, "docs/guias", html), "utf8")
    if (!soloPDF) {
      const fecha = new Intl.DateTimeFormat("es-AR", {
        timeZone: env.TZ,
        day: "numeric",
        month: "long",
        year: "numeric",
      }).format(new Date(capturas.capturas.at(-1).fecha))
      const marcador = /(<span data-fecha-capturas>)[^<]*(<\/span>)/g
      if ([...contenido.matchAll(marcador)].length !== 1)
        throw new Error(`Falta la fecha de capturas en ${html}`)
      contenido = contenido.replace(marcador, (_, inicio, fin) => inicio + fecha + fin)
    }
    contenido = anotarHTML(contenido, capturas)
    await writeFile(join(publicacion, html), contenido)
    await ejecutar("node", [
      "docs/guias/generar/hacer-pdf.mjs",
      join(publicacion, html),
      join(publicacion, pdf),
    ])
    await ejecutar("pdftotext", [join(publicacion, pdf), join(salida, pdf + ".txt")])
    const texto = await readFile(join(salida, pdf + ".txt"), "utf8")
    if (
      !texto.includes("Las palabras que vas a ver") ||
      texto.split("\f").length - 1 < 30
    )
      throw new Error(`El PDF ${pdf} no contiene el final de la guía o quedó truncado`)
  }
  for (const [html] of guias)
    await cp(join(publicacion, html), join(raiz, "docs/guias", html))
  if (!soloPDF)
    for (const nombre of await readdir(join(publicacion, "imagenes")))
      await cp(
        join(publicacion, "imagenes", nombre),
        join(raiz, "docs/guias/imagenes", nombre)
      )
  for (const [, pdf] of guias)
    await cp(join(publicacion, pdf), join(raiz, "docs/guias", pdf))
  if (!soloPDF) {
    const imagenes = []
    for (const captura of capturas.capturas) {
      const contenido = await readFile(join(publicacion, "imagenes", captura.archivo))
      imagenes.push({
        ...captura,
        sha256: createHash("sha256").update(contenido).digest("hex"),
      })
    }
    await writeFile(
      join(raiz, "docs/guias/generacion.json"),
      JSON.stringify(
        {
          version: capturas.version,
          edicion: "Árbol de trabajo; ver CHANGELOG.md para cambios sin release",
          esquema: Math.max(
            ...(await readdir(join(raiz, "migrations"))).map((n) =>
              Number(n.match(/^(\d+)_.*\.sql$/)?.[1] ?? 0)
            )
          ),
          generado: new Date().toISOString(),
          herramienta: "Playwright / Chromium",
          entorno:
            "Efímero, con datos ficticios y sin configuraciones de la instalación local",
          imagenes,
        },
        null,
        2
      ) + "\n"
    )
  }
  console.log("Ambas guías publicadas. Evidencia de generación:", salida)
} finally {
  if (!soloPDF) {
    const previa = interrumpido
    interrumpido = false
    try {
      await compose("down", "--remove-orphans")
    } finally {
      interrumpido = previa
    }
  }
}
