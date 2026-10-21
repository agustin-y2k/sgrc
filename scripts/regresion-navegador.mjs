#!/usr/bin/env node
// Regresión local con imágenes comunes y una base efímera nueva por bloque.
// No lee configuraciones locales ni utiliza el proyecto Docker habitual.
import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const raiz = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const bloques = [
  {
    nombre: "docentes",
    sembrar: true,
    bandera: "E2E_DOCENTES_AISLADOS",
    archivos: ["flujos-docentes-reales"],
  },
  {
    nombre: "reservas",
    bandera: "E2E_RESERVAS_AISLADAS",
    archivos: ["flujos-reservas-reales"],
  },
  {
    nombre: "entregas",
    bandera: "E2E_ENTREGAS_AISLADAS",
    archivos: ["flujos-entregas-reales"],
  },
  {
    nombre: "cierre",
    bandera: "E2E_CIERRE_AISLADO",
    archivos: ["flujos-cierre-anual-reales"],
  },
  {
    nombre: "inventario",
    bandera: "E2E_INVENTARIO_AISLADO",
    archivos: ["flujos-inventario-reales"],
  },
  {
    nombre: "soporte",
    bandera: "E2E_SOPORTE_AISLADO",
    archivos: ["flujos-soporte-reales"],
  },
  {
    nombre: "reportes",
    bandera: "E2E_REPORTES_AISLADO",
    archivos: ["flujos-reportes-auditoria-reales"],
  },
  {
    nombre: "usabilidad",
    sembrar: true,
    archivos: [
      "ux-prioridades",
      "ux-recorrido",
      "ux-consultas",
      "ux-cierre-reservas",
      "ux-continuidad",
      "ux-series-calendario",
      "ux-soporte",
    ],
  },
  {
    nombre: "basicos",
    sembrar: true,
    clase: true,
    archivos: ["login", "reserva", "responsive", "tactil", "tema"],
  },
  {
    nombre: "transversal",
    sembrar: true,
    bandera: "E2E_REVISION_AISLADA",
    archivos: ["revision-frontend"],
  },
  {
    nombre: "vistas",
    sembrar: true,
    bandera: "E2E_REVISION_AISLADA",
    archivos: ["revision-vistas", "revision-final"],
  },
];

const argumentos = process.argv.slice(2);
if (
  argumentos.length &&
  (argumentos.length !== 2 || argumentos[0] !== "--solo")
) {
  throw new Error(
    "Uso: node scripts/regresion-navegador.mjs [--solo docentes,reservas,entregas,cierre,inventario,soporte,reportes,usabilidad,basicos,transversal,vistas]",
  );
}
const nombres = argumentos.length
  ? argumentos[1].split(",")
  : bloques.map((b) => b.nombre);
if (nombres.some((n) => !bloques.some((b) => b.nombre === n)))
  throw new Error("Bloque desconocido");
const seleccion = bloques.filter((b) => nombres.includes(b.nombre));
const carpeta = await mkdtemp(join(tmpdir(), "sgrc-regresion-"));
const proyecto = `sgrc-regresion-${randomBytes(5).toString("hex")}`;
const archivoCompose = join(carpeta, "compose.json");
const origen = `http://127.0.0.1:${await puertoLibre()}`;
const clave = () => randomBytes(32).toString("hex");
const adminPassword = `Revision1!${clave()}`;
const docentePassword = `Docente1!${clave()}`;
const adminEmail = "admin@revision.test",
  docenteEmail = "docente@revision.test";
const imagenBackend = `${proyecto}-backend:local`,
  imagenFrontend = `${proyecto}-frontend:local`;
const entorno = { ...process.env, TZ: "America/Argentina/Buenos_Aires" };
for (const campo of Object.keys(entorno))
  if (campo.startsWith("E2E_") || campo === "CARPETA_CAPTURAS")
    delete entorno[campo];
Object.assign(entorno, {
  E2E_BASE_URL: origen,
  E2E_ADMIN_EMAIL: adminEmail,
  E2E_ADMIN_PASSWORD: adminPassword,
  E2E_DOCENTE_EMAIL: docenteEmail,
  E2E_DOCENTE_PASSWORD: docentePassword,
  E2E_OTRO_DOCENTE_EMAIL: "otro-docente@revision.test",
  E2E_OTRO_DOCENTE_PASSWORD: docentePassword,
});
const datos = {
  POSTGRES_HOST: "postgres",
  POSTGRES_PORT: "5432",
  POSTGRES_DB: "regresion",
  POSTGRES_USER: "revision",
  POSTGRES_PASSWORD: clave(),
  JWT_SECRET: clave(),
  JWT_ACCESS_TTL: "24h",
  JWT_REMEMBER_TTL: "720h",
  OUTBOX_SECRET: clave(),
  CUENTAS_SECRET: clave(),
  SEED_ADMIN_EMAIL: adminEmail,
  SEED_ADMIN_PASSWORD: adminPassword,
  FRONTEND_ORIGIN: origen,
  APP_TIMEZONE: entorno.TZ,
  APP_PORT: "8080",
  TRUSTED_PROXIES: "172.31.44.0/24",
  SMTP_HOST: "",
  GOOGLE_CLIENT_ID: "",
};
await writeFile(
  archivoCompose,
  JSON.stringify({
    services: {
      postgres: {
        image: "postgres:16-alpine",
        environment: {
          POSTGRES_DB: datos.POSTGRES_DB,
          POSTGRES_USER: datos.POSTGRES_USER,
          POSTGRES_PASSWORD: datos.POSTGRES_PASSWORD,
        },
        tmpfs: ["/var/lib/postgresql/data"],
        healthcheck: {
          test: [
            "CMD-SHELL",
            "pg_isready -h 127.0.0.1 -U revision -d regresion",
          ],
          interval: "2s",
          timeout: "3s",
          retries: 30,
        },
      },
      "sgrc-app": {
        image: imagenBackend,
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
        image: imagenFrontend,
        ports: [`127.0.0.1:${new URL(origen).port}:80`],
        depends_on: { "sgrc-app": { condition: "service_healthy" } },
        healthcheck: {
          test: [
            "CMD",
            "wget",
            "-q",
            "-O",
            "/dev/null",
            "http://127.0.0.1/health",
          ],
          interval: "2s",
          timeout: "5s",
          retries: 30,
        },
      },
    },
    networks: { default: { ipam: { config: [{ subnet: "172.31.44.0/24" }] } } },
  }),
  { mode: 0o600 },
);
const informe = {
  inicio: new Date().toISOString(),
  proyecto,
  origen,
  imagenes: {},
  bloques: [],
};
const guardar = () =>
  writeFile(join(carpeta, "resultado.json"), JSON.stringify(informe, null, 2), {
    mode: 0o600,
  });
let hijo;
let interrumpido = false;
for (const senal of ["SIGINT", "SIGTERM"])
  process.on(senal, () => {
    interrumpido = true;
    hijo?.kill("SIGTERM");
  });

async function ejecutar(
  comando,
  args,
  { cwd = raiz, env = entorno, capturar = false } = {},
) {
  if (interrumpido) throw new Error("Ejecución interrumpida");
  let salida = "";
  return new Promise((ok, fallo) => {
    hijo = spawn(comando, args, {
      cwd,
      env,
      stdio: capturar ? ["ignore", "pipe", "inherit"] : "inherit",
    });
    if (capturar)
      hijo.stdout.on("data", (dato) => {
        salida += dato;
      });
    hijo.once("error", fallo);
    hijo.once("exit", (codigo) => {
      hijo = undefined;
      codigo === 0
        ? ok(salida.trim())
        : fallo(new Error(`${comando}: salida ${codigo}`));
    });
  });
}
const compose = (...args) =>
  ejecutar(
    "docker",
    ["compose", "-p", proyecto, "-f", archivoCompose, ...args],
    { cwd: carpeta },
  );
async function limpiar() {
  // También se ejecuta tras un fallo. El proyecto es único y sólo contiene tmpfs.
  const anterior = interrumpido;
  interrumpido = false;
  try {
    await compose("down", "--remove-orphans");
  } finally {
    interrumpido = anterior;
  }
}

async function api(metodo, ruta, cuerpo, token = "") {
  const respuesta = await fetch(origen + ruta, {
    method: metodo,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    ...(cuerpo === undefined ? {} : { body: JSON.stringify(cuerpo) }),
  });
  if (!respuesta.ok)
    throw new Error(`${metodo} ${ruta}: HTTP ${respuesta.status}`);
  return respuesta.headers.get("content-type")?.includes("application/json")
    ? respuesta.json()
    : undefined;
}

async function sembrar(clase) {
  const formatoFecha = new Intl.DateTimeFormat("sv-SE", {
    timeZone: entorno.TZ,
  });
  const { token } = await api("POST", "/api/auth/login", {
    email: adminEmail,
    password: adminPassword,
  });
  for (const ruta of ["/api/ciclos", "/api/equipos", "/api/prestamos"]) {
    if ((await api("GET", ruta, undefined, token)).data.length)
      throw new Error("La base de regresión no está vacía");
  }
  if (
    (await api("GET", "/api/auth/usuarios?pageSize=200", undefined, token)).data
      .length !== 1
  )
    throw new Error("La base contiene usuarios ajenos a esta ejecución");
  await api(
    "PUT",
    "/api/jornada",
    {
      tramos: [
        "LUNES",
        "MARTES",
        "MIERCOLES",
        "JUEVES",
        "VIERNES",
        "SABADO",
        "DOMINGO",
      ].flatMap((diaSemana) => [
        { diaSemana, horaInicio: "00:00", horaFin: "12:00" },
        { diaSemana, horaInicio: "12:00", horaFin: "00:00" },
      ]),
    },
    token,
  );
  const ciclo = await api(
    "POST",
    "/api/ciclos",
    { anio: Number(formatoFecha.format(new Date()).slice(0, 4)) },
    token,
  );
  const curso = await api(
    "POST",
    `/api/ciclos/${ciclo.id}/cursos`,
    { anio: 1, division: "A" },
    token,
  );
  const materia = await api(
    "POST",
    `/api/cursos/${curso.id}/materias`,
    { nombre: "Programación" },
    token,
  );
  for (const [email, nombre, apellido] of [
    [docenteEmail, "Ada", "Lovelace"],
    [entorno.E2E_OTRO_DOCENTE_EMAIL, "Alan", "Turing"],
  ]) {
    await api("POST", "/api/auth/registro", {
      nombre,
      apellido,
      email,
      password: docentePassword,
      cargoSolicitado: "DOCENTE",
      rolSolicitado: "TITULAR",
    });
    const docente = (
      await api("GET", "/api/auth/usuarios?pageSize=200", undefined, token)
    ).data.find((u) => u.email === email);
    await api(
      "PATCH",
      `/api/auth/usuarios/${docente.id}/estado`,
      { estado: "APROBADA" },
      token,
    );
    await api(
      "POST",
      `/api/materias/${materia.id}/docentes`,
      { usuarioId: docente.id, rol: "TITULAR" },
      token,
    );
  }
  const carro = await api("POST", "/api/carros", { nombre: "Carro 1" }, token);
  const equipos = [];
  for (let i = 1; i <= 8; i++)
    equipos.push(
      await api(
        "POST",
        `/api/carros/${carro.id}/equipos`,
        {
          identificador: i,
          numeroSerie: `REGRESION-${i}`,
          softwareInstalado: "AutoCAD 2026, Office",
        },
        token,
      ),
    );
  if (clase) {
    const sesion = await api("POST", "/api/auth/login", {
      email: docenteEmail,
      password: docentePassword,
    });
    const fecha = formatoFecha.format(
      new Date(Date.now() + 24 * 60 * 60 * 1000),
    );
    await api(
      "POST",
      "/api/reservas",
      {
        materiaId: materia.id,
        equipoIds: [equipos[0].id],
        fecha,
        horaInicio: "09:00",
        horaFin: "10:00",
      },
      sesion.token,
    );
  }
}

try {
  console.log(`Resultados: ${carpeta}`);
  await ejecutar("docker", ["build", "-t", imagenBackend, "."]);
  await ejecutar("docker", ["build", "-t", imagenFrontend, "frontend"]);
  informe.imagenes = {
    backend: await ejecutar(
      "docker",
      ["image", "inspect", "--format", "{{.Id}}", imagenBackend],
      { capturar: true },
    ),
    frontend: await ejecutar(
      "docker",
      ["image", "inspect", "--format", "{{.Id}}", imagenFrontend],
      { capturar: true },
    ),
  };
  for (const bloque of seleccion) {
    const resultado = {
      nombre: bloque.nombre,
      inicio: new Date().toISOString(),
      estado: "EN_CURSO",
    };
    informe.bloques.push(resultado);
    await guardar();
    console.log(
      `\nBloque ${bloque.nombre}: base efímera nueva; mismas imágenes compiladas.`,
    );
    try {
      await compose("up", "-d", "--wait", "--wait-timeout", "120");
      if (bloque.sembrar) await sembrar(bloque.clase);
      const carpetaBloque = join(carpeta, bloque.nombre);
      await mkdir(carpetaBloque);
      const json = join(carpetaBloque, "playwright.json");
      const env = {
        ...entorno,
        PLAYWRIGHT_JSON_OUTPUT_FILE: json,
        ...(bloque.bandera ? { [bloque.bandera]: "1" } : {}),
      };
      let errorPrueba;
      try {
        await ejecutar(
          "npm",
          [
            "run",
            "e2e",
            "--",
            ...bloque.archivos.map((nombre) => `e2e/${nombre}.spec.ts`),
            "--workers=1",
            "--reporter=line,json",
            `--output=${join(carpetaBloque, "artefactos")}`,
          ],
          { cwd: join(raiz, "frontend"), env },
        );
      } catch (error) {
        errorPrueba = error;
      }
      try {
        resultado.pruebas = JSON.parse(await readFile(json, "utf8")).stats;
      } catch {
        /* Se conserva el fallo del proceso si no pudo generar informe. */
      }
      if (errorPrueba) throw errorPrueba;
      if (
        !resultado.pruebas?.expected ||
        resultado.pruebas.skipped ||
        resultado.pruebas.unexpected ||
        resultado.pruebas.flaky
      )
        throw new Error("Hay pruebas omitidas, fallidas o sin resultado");
      resultado.estado = "APROBADO";
    } catch (error) {
      resultado.estado = "FALLO";
      resultado.error = error.message;
      console.error(`${bloque.nombre}: ${error.message}`);
    } finally {
      resultado.fin = new Date().toISOString();
      await guardar();
      await limpiar();
    }
    if (interrumpido) break;
  }
} finally {
  await limpiar();
  informe.fin = new Date().toISOString();
  await guardar();
}
console.log(`Informe: ${join(carpeta, "resultado.json")}`);
console.log(
  informe.bloques
    .map(
      (b) => `${b.nombre}: ${b.estado} (${b.pruebas?.expected ?? 0} aprobadas)`,
    )
    .join("\n"),
);
process.exitCode =
  interrumpido ||
  informe.bloques.length !== seleccion.length ||
  informe.bloques.some((b) => b.estado !== "APROBADO")
    ? 1
    : 0;

async function puertoLibre() {
  const servidor = createServer();
  await new Promise((ok, fallo) => {
    servidor.once("error", fallo);
    servidor.listen(0, "127.0.0.1", ok);
  });
  const puerto = servidor.address().port;
  await new Promise((ok, fallo) =>
    servidor.close((error) => (error ? fallo(error) : ok())),
  );
  return puerto;
}
