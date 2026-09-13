/**
 * Un día de escuela a pleno, para ver el mostrador con carga de verdad.
 *
 *   node scripts/sembrar-un-dia-a-full.mjs              # el próximo día hábil
 *   node scripts/sembrar-un-dia-a-full.mjs --fecha=2026-09-14
 *   node scripts/sembrar-un-dia-a-full.mjs --limpiar    # borra lo que sembró
 *
 * **Por qué existe.** Los cuatro paneles del inicio del Admin —«Para entregar
 * ahora», «Lo que sigue hoy», «Afuera del laboratorio» y «En el laboratorio
 * ahora»— con la base vacía dicen todos lo mismo: que no hay nada. Así no se
 * puede saber si el diseño aguanta un martes a las once, que es cuando se usa.
 * Los tests de pantalla tampoco contestan eso: cada uno monta su componente
 * con dos o tres reservas inventadas, que es lo correcto para probar lógica y
 * lo inútil para mirar densidad.
 *
 * Esto siembra once clases repartidas en los dos turnos, con docentes
 * distintos, materias distintas y casi cien reservas de equipo — y deja el
 * laboratorio a mitad de mañana: unas máquinas ya entregadas, otras guardadas
 * sin retirar, y tres que salieron el viernes y no volvieron.
 *
 * **Le pega a la API como cualquier cliente**, igual que
 * `sembrar-datos-de-prueba.sh`: todo lo que crea pasa por las mismas
 * validaciones que usaría una persona —la jornada de la escuela, la
 * disponibilidad de cada equipo, que el docente dicte esa materia—. La única
 * excepción está documentada abajo, en `corregirHoraDeEntrega()`.
 *
 * **Solo para desarrollo local.** Crea docentes con una contraseña conocida,
 * así que se niega a correr contra cualquier cosa que no sea localhost.
 */
import { execFile } from "node:child_process"
import { fileURLToPath } from "node:url"
import { promisify } from "node:util"

const ejecutar = promisify(execFile)

const API = process.env.API ?? "http://localhost:8080"
const ADMIN_EMAIL = process.env.SEED_ADMIN_EMAIL ?? ""
const ADMIN_PASSWORD = process.env.SEED_ADMIN_PASSWORD ?? ""

/**
 * El dominio de los docentes que inventa este script. `.invalid` está
 * reservado por la RFC 2606 justamente para esto: no existe, no se puede
 * entregar correo ahí ni por accidente. Además es el marcador que hace
 * posible `--limpiar`: todo lo que se siembra cuelga de estas cuentas.
 */
const DOMINIO = "dia-a-full.invalid"
const PASSWORD_DOCENTE = "dia_a_full_2026"

/**
 * El día que se arma. Los horarios NO son inventados: salen de la jornada
 * cargada (dos turnos), y las clases se pisan de a dos como pasa de verdad
 * cuando el laboratorio da abasto para dos cursos a la vez.
 *
 * `entrega` dice qué hacer con los equipos de esa clase:
 *   "entregada"  — salieron todos, están afuera
 *   "parcial"    — el docente se llevó una parte y dejó dos sin retirar
 *   undefined    — todavía no pasó nadie a buscarlos
 */
const CLASES = [
  { hi: "07:45", hf: "09:15", equipos: 8 },
  { hi: "07:45", hf: "09:15", equipos: 6 },
  { hi: "09:20", hf: "10:50", equipos: 10 },
  { hi: "09:20", hf: "10:50", equipos: 8 },
  { hi: "11:00", hf: "12:30", equipos: 12, entrega: "parcial" },
  { hi: "11:00", hf: "12:30", equipos: 6, entrega: "entregada" },
  { hi: "13:30", hf: "15:00", equipos: 10 },
  { hi: "13:30", hf: "15:00", equipos: 8 },
  { hi: "15:10", hf: "16:40", equipos: 12 },
  { hi: "16:45", hf: "18:15", equipos: 9 },
  { hi: "16:45", hf: "18:15", equipos: 6 },
]

/** Los docentes que se inventan. Nombres de fantasía: el repo es público. */
const DOCENTES = [
  ["Ana", "Ferraro"],
  ["Bruno", "Salgado"],
  ["Carmen", "Oliveri"],
  ["Damián", "Rivas"],
  ["Elena", "Pacheco"],
  ["Fabián", "Duarte"],
  ["Gabriela", "Monti"],
  ["Hugo", "Lezcano"],
]

// ── Guardas ───────────────────────────────────────────────────────────
if (!/^http:\/\/(localhost|127\.0\.0\.1):/.test(API)) {
  console.error(`Este script es solo para desarrollo y API apunta a ${API}.`)
  console.error("Crea docentes con una contraseña que está escrita en el código.")
  process.exit(1)
}
if (!ADMIN_EMAIL || !ADMIN_PASSWORD) {
  console.error("Faltan SEED_ADMIN_EMAIL y SEED_ADMIN_PASSWORD en el entorno.")
  console.error("Probá:  set -a; . ./.env; set +a  (o pasalas a mano)")
  process.exit(1)
}

// ── Hablar con la API ─────────────────────────────────────────────────

async function api(metodo, ruta, cuerpo, token) {
  const res = await fetch(API + ruta, {
    method: metodo,
    headers: {
      ...(cuerpo ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: cuerpo ? JSON.stringify(cuerpo) : undefined,
  })
  const texto = await res.text()
  // No todos los errores vuelven en JSON, y un `JSON.parse` que explota acá
  // esconde el mensaje que explica qué pasó.
  let datos = null
  try {
    datos = texto ? JSON.parse(texto) : null
  } catch {
    datos = null
  }
  if (!res.ok) {
    const mensaje = datos?.error ?? datos?.message ?? texto.trim()
    const error = new Error(mensaje || `HTTP ${res.status}`)
    error.status = res.status
    error.ruta = `${metodo} ${ruta}`
    throw error
  }
  return datos
}

const entrar = (email, password) =>
  api("POST", "/api/auth/login", { email, password }).then((r) => r.token)

/**
 * `psql` dentro del contenedor. Hace falta para dos cosas que la API no
 * expone —y no debería: mover la hora de una entrega ya registrada, y borrar
 * cuentas—. Las dos son operaciones de laboratorio, no del sistema.
 */
async function sql(consulta) {
  // En UNA línea: la consulta viaja dentro del `-c` de psql, y psql lee un
  // salto de línea seguido de barra como una meta-orden suya («invalid command
  // \n»). Al SQL los espacios le dan igual.
  const enUnaLinea = consulta.replace(/\s+/g, " ").trim()
  const { stdout } = await ejecutar(
    "docker",
    [
      "compose",
      "exec",
      "-T",
      "postgres",
      "sh",
      "-c",
      `psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -A -t -c ${JSON.stringify(enUnaLinea)}`,
    ],
    { cwd: fileURLToPath(new URL("..", import.meta.url)) }
  )
  return stdout.trim()
}

// ── Fechas ────────────────────────────────────────────────────────────

const iso = (d) => d.toISOString().slice(0, 10)

/** El próximo día hábil, hoy incluido si hoy lo es. La escuela abre L a V. */
function proximoDiaHabil() {
  const d = new Date()
  d.setHours(12, 0, 0, 0)
  while (d.getDay() === 0 || d.getDay() === 6) d.setDate(d.getDate() + 1)
  return iso(d)
}

/** El viernes anterior a una fecha: de ahí salen las máquinas que no volvieron. */
function viernesAnterior(fecha) {
  const d = new Date(`${fecha}T12:00:00`)
  do {
    d.setDate(d.getDate() - 1)
  } while (d.getDay() !== 5)
  return iso(d)
}

// ── Limpieza ──────────────────────────────────────────────────────────

/**
 * Borra TODO lo que sembró este script y nada más: cuelga de las cuentas
 * `@dia-a-full.invalid`, así que el alcance lo define esa condición y no una
 * fecha —que podría llevarse por delante reservas de verdad—.
 */
async function limpiar() {
  const cuentas = `SELECT id FROM usuario WHERE email LIKE '%@${DOMINIO}'`
  const reservas = `SELECT id FROM reserva WHERE creado_por IN (${cuentas})`

  const antes = await sql(`
    SELECT (SELECT count(*) FROM prestamo
            WHERE reserva_id IN (${reservas}) OR entregado_a_nombre LIKE '%[${DOMINIO}]')
        || ' préstamos, '
        || (SELECT count(*) FROM reserva WHERE creado_por IN (${cuentas}))
        || ' reservas, '
        || (SELECT count(*) FROM usuario WHERE email LIKE '%@${DOMINIO}')
        || ' cuentas'`)

  // En orden y en una sola transacción: psql manda todo el `-c` como una
  // consulta, así que o se borra entero o no se borra nada. El orden lo
  // imponen las claves foráneas —el préstamo apunta a la reserva, la reserva
  // al grupo y al usuario—, no el gusto.
  //
  // El préstamo suelto (el del viernes) no cuelga de ninguna reserva: se lo
  // identifica por el nombre, que este script escribe con el marcador.
  await sql(`
    DELETE FROM prestamo
      WHERE reserva_id IN (${reservas}) OR entregado_a_nombre LIKE '%[${DOMINIO}]';
    DELETE FROM reserva WHERE creado_por IN (${cuentas});
    DELETE FROM reserva_grupo WHERE creado_por IN (${cuentas});
    DELETE FROM notificacion WHERE usuario_id IN (${cuentas});
    DELETE FROM docente_materia WHERE usuario_id IN (${cuentas});
    DELETE FROM preferencia_email WHERE usuario_id IN (${cuentas});
    DELETE FROM audit_log WHERE usuario_id IN (${cuentas});
    DELETE FROM usuario WHERE email LIKE '%@${DOMINIO}';`)

  console.log(antes ? `Borrado: ${antes}` : "No había nada que borrar.")
}

// ── Siembra ───────────────────────────────────────────────────────────

/**
 * El autorregistro es público y sin token, y admite CINCO por minuto y por IP
 * (`RateLimit(5, time.Minute)` en auth/routes.go). Sembrar ocho docentes de un
 * saque choca con ese límite siempre.
 *
 * Esperar no es un parche: es lo que mantiene al script del lado correcto de
 * la puerta. La alternativa —escribir las cuentas directo en la base— se
 * saltearía el hasheo de la contraseña, el estado inicial PENDIENTE y el aviso
 * al Admin, que son justamente las cosas que uno quiere ver funcionando.
 */
async function registrarConEspera(cuerpo) {
  for (let intento = 0; ; intento++) {
    try {
      await api("POST", "/api/auth/registro", cuerpo)
      return
    } catch (e) {
      // Ya existía: es lo que hace que se pueda volver a correr sin romper.
      if (e.status === 409) return
      if (e.status === 429 && intento < 4) {
        console.log("   (cinco registros por minuto: esperando)")
        await new Promise((r) => setTimeout(r, 61_000))
        continue
      }
      throw new Error(`registrando ${cuerpo.email}: ${e.message}`)
    }
  }
}

/** Registra (o reutiliza) un docente aprobado y asignado a una materia. */
async function prepararDocente([nombre, apellido], materiaId, tokenAdmin) {
  // Sin el índice de la clase: quien da dos materias es UNA persona con dos
  // asignaciones, no dos cuentas homónimas.
  const email = `${nombre}.${apellido}@${DOMINIO}`
    .toLowerCase()
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")

  await registrarConEspera({
    nombre,
    apellido,
    email,
    password: PASSWORD_DOCENTE,
    cargoSolicitado: "DOCENTE",
    rolSolicitado: "TITULAR",
  })

  const listado = await api(
    "GET",
    "/api/auth/usuarios?rol=DOCENTE&pageSize=200",
    null,
    tokenAdmin
  )
  const usuario = listado.data.find((u) => u.email === email)
  if (!usuario) throw new Error(`no apareció ${email} después de registrarlo`)

  // Aprobar y asignar son idempotentes en la práctica: si ya estaban, 409.
  await api(
    "PATCH",
    `/api/auth/usuarios/${usuario.id}/estado`,
    { estado: "APROBADA" },
    tokenAdmin
  ).catch(() => {})
  await api(
    "POST",
    `/api/materias/${materiaId}/docentes`,
    { usuarioId: usuario.id, rol: "TITULAR" },
    tokenAdmin
  ).catch(() => {})

  return { ...usuario, email, nombre, apellido }
}

/**
 * Elige once pares (curso, materia) de cursos DISTINTOS, para que el panel no
 * repita el mismo nombre once veces.
 */
async function elegirMaterias(tokenAdmin, cuantas) {
  const ciclos = await api("GET", "/api/ciclos", null, tokenAdmin)
  const ciclo = ciclos.data.find((c) => c.activo) ?? ciclos.data[0]
  if (!ciclo) throw new Error("no hay ningún ciclo lectivo cargado")

  const cursos = await api("GET", `/api/ciclos/${ciclo.id}/cursos`, null, tokenAdmin)
  const elegidas = []

  for (const curso of cursos.data) {
    if (elegidas.length === cuantas) break
    const materias = await api("GET", `/api/cursos/${curso.id}/materias`, null, tokenAdmin)
    const vivas = materias.data.filter((m) => !m.archivado)
    if (vivas.length === 0) continue
    // Corriendo el índice un lugar por curso, y NO la primera de cada uno: las
    // materias vienen alfabéticas y "Ciencias Naturales" es la primera de todos
    // los primeros y segundos años, así que las once clases salían con el mismo
    // nombre — un panel lleno de renglones idénticos no muestra nada.
    elegidas.push({ curso, materia: vivas[elegidas.length % vivas.length] })
  }

  if (elegidas.length < cuantas) {
    throw new Error(
      `hacen falta ${cuantas} cursos con materias y solo hay ${elegidas.length}`
    )
  }
  return elegidas
}

/**
 * La hora en que se anotó la entrega la pone el servidor con SU reloj, y está
 * bien que así sea: es un hecho, no un dato que el cliente elija. Pero este
 * script arma un día que todavía no pasó, así que las entregas quedarían
 * fechadas en el momento de correrlo —«entregada a las 23:40» en una clase de
 * las once—. Se corrige acá, con la fecha del día sembrado.
 *
 * Es la ÚNICA escritura directa a la base de todo el script, y es lo que
 * separa una demostración de un dato real: en el sistema andando esta función
 * no existe.
 */
async function corregirHoraDeEntrega(prestamoIds, fecha, hora) {
  if (prestamoIds.length === 0) return
  const lista = prestamoIds.map((id) => `'${id}'`).join(",")
  await sql(
    `UPDATE prestamo SET entregado_en = TIMESTAMPTZ '${fecha} ${hora}-03'
     WHERE id IN (${lista})`
  )
}

async function sembrar(fecha) {
  console.log(`→ día a sembrar: ${fecha}`)
  const tokenAdmin = await entrar(ADMIN_EMAIL, ADMIN_PASSWORD)

  console.log("→ eligiendo cursos y materias del ciclo activo")
  const materias = await elegirMaterias(tokenAdmin, CLASES.length)

  console.log(`→ ${DOCENTES.length} docentes (autorregistro + aprobación + asignación)`)
  const docentes = []
  for (let i = 0; i < CLASES.length; i++) {
    // Ocho docentes para once clases: los tres primeros dan dos materias, que
    // es exactamente lo que pasa en una escuela.
    const persona = DOCENTES[i % DOCENTES.length]
    docentes.push(await prepararDocente(persona, materias[i].materia.id, tokenAdmin))
  }

  console.log("→ clases")
  const sesiones = []
  for (const [i, clase] of CLASES.entries()) {
    const docente = docentes[i]
    const { curso, materia } = materias[i]
    const token = await entrar(docente.email, PASSWORD_DOCENTE)

    const disponibles = await api(
      "GET",
      `/api/equipos-disponibles?fecha=${fecha}&horaInicio=${clase.hi}&horaFin=${clase.hf}`,
      null,
      token
    )
    const equipos = disponibles.data.slice(0, clase.equipos).map((e) => e.equipoId)
    if (equipos.length < clase.equipos) {
      throw new Error(
        `${clase.hi}: se pidieron ${clase.equipos} equipos y hay ${equipos.length} libres`
      )
    }

    const creada = await api(
      "POST",
      "/api/reservas",
      {
        materiaId: materia.id,
        fecha,
        horaInicio: clase.hi,
        horaFin: clase.hf,
        equipoIds: equipos,
      },
      token
    )
    const reservaIds = (creada.reservas ?? []).map((r) => r.id)
    sesiones.push({ clase, curso, materia, docente, reservaIds, token })
    console.log(
      `   ${clase.hi}–${clase.hf}  ${materia.nombre} · ${curso.nombre} — ${equipos.length} equipos`
    )
  }

  console.log("→ entregas del turno que está en curso")
  const entregados = []
  for (const s of sesiones) {
    if (!s.clase.entrega) continue
    // "parcial": el docente se llevó lo que necesitaba y dejó dos guardadas.
    // Es el estado que más se ve en el mostrador y el que la pantalla tiene
    // que saber contar: "sin retirar" no es lo mismo que "liberada".
    const aEntregar =
      s.clase.entrega === "parcial" ? s.reservaIds.slice(0, -2) : s.reservaIds
    // Con el token del ADMIN: entregar es del mostrador, no del docente
    // (`soloAdmin` en reservation/routes.go). Con el del docente da 403.
    const res = await api(
      "POST",
      "/api/prestamos/por-reserva",
      { reservaIds: aEntregar },
      tokenAdmin
    ).catch((e) => {
      throw new Error(`entregando ${s.materia.nombre}: ${e.message}`)
    })
    const ids = (res.entregadas ?? []).map((p) => p.id)
    entregados.push(...ids)
    console.log(`   ${s.materia.nombre}: ${aEntregar.length} afuera`)
  }
  // Se anotaron "a las 11", no a la hora en que corriste esto.
  await corregirHoraDeEntrega(entregados, fecha, "11:05:00")

  console.log("→ las tres que salieron el viernes y no volvieron")
  const viernes = viernesAnterior(fecha)
  const libres = await api(
    "GET",
    `/api/equipos-disponibles?fecha=${fecha}&horaInicio=07:45&horaFin=18:15`,
    null,
    tokenAdmin
  )
  // Del final de la lista: son los que ninguna clase del día reservó, así que
  // que estén afuera no le rompe la reserva a nadie.
  const sueltos = libres.data.slice(-3).map((e) => e.equipoId)
  const quedaronAfuera = []
  for (const [i, equipoId] of sueltos.entries()) {
    const destino = ["Biblioteca", "Dirección", "Sala de profesores"][i]
    const res = await api(
      "POST",
      "/api/prestamos",
      {
        equipoIds: [equipoId],
        // El marcador va en el nombre: es lo que hace que `--limpiar` los
        // encuentre sin tener que adivinar por fecha.
        nombre: `Pedido del viernes [${DOMINIO}]`,
        destino,
        // En el pasado REAL, no en el del día sembrado: "demorado" lo calcula
        // el servidor contra su propio reloj, así que esta es la única forma
        // de ver encendido el aviso de "sin devolver a horario".
        devolucionEstimada: new Date(`${viernes}T12:45:00-03:00`).toISOString(),
      },
      tokenAdmin
    )
    quedaronAfuera.push(...(res.entregadas ?? []).map((p) => p.id))
    console.log(`   ${destino}: 1 equipo, debía volver el ${viernes}`)
  }
  await corregirHoraDeEntrega(quedaronAfuera, viernes, "08:10:00")

  const afuera = entregados.length + quedaronAfuera.length
  console.log(`
Listo. El ${fecha} tiene ${CLASES.length} clases y ${sesiones.reduce((n, s) => n + s.reservaIds.length, 0)} reservas de equipo.
  Afuera del laboratorio: ${afuera} equipos, ${quedaronAfuera.length} sin devolver a horario.
  Sin retirar:            2 guardadas para su docente.
  Docentes inventados:    ${DOCENTES.length}, todos @${DOMINIO} / ${PASSWORD_DOCENTE}

Para verlo: entrá como Admin el ${fecha}. Para borrarlo:
  node scripts/sembrar-un-dia-a-full.mjs --limpiar
`)
}

// ── Main ──────────────────────────────────────────────────────────────

const argumentos = process.argv.slice(2)
const fechaPedida = argumentos.find((a) => a.startsWith("--fecha="))?.slice(8)

try {
  if (argumentos.includes("--limpiar")) {
    await limpiar()
  } else {
    await sembrar(fechaPedida ?? proximoDiaHabil())
  }
} catch (e) {
  console.error(`\nFalló: ${e.message}`)
  process.exit(1)
}
