// Las marcas se resuelven por el nombre del control medido en cada captura.
// No se reutilizan coordenadas de otra ejecución ni se modifica el sistema.
const m = (texto, tipo, nombre) => ({ texto, tipo, nombre })
const b = (texto, nombre) => m(texto, "button", nombre)
const c = (texto, nombre) => m(texto, "campo", nombre)
const e = (texto, nombre) => m(texto, "enlace", nombre)
const q = (texto, nombre) => m(texto, "checkbox", nombre)
const t = (texto, nombre, tipo = "p") => m(texto, tipo, nombre)
const r = (texto, nombre) => m(texto, "radio", nombre)
const g = (texto, ...miembros) => ({ texto, miembros })
const horario = (texto) =>
  g(
    texto,
    c("", "Hora de inicio: hora"),
    c("", "Hora de inicio: minutos"),
    c("", "Hora de fin: hora"),
    c("", "Hora de fin: minutos")
  )
const ESQUEMA = {
  "docente-01-entrar": [
    c("Escribí tu email", "Email"),
    c("Ingresá tu contraseña", "Contraseña"),
    b("Entrá al sistema", "Iniciar sesión"),
  ],
  "docente-02-crear-cuenta": [
    g(
      "Completá tus datos y contraseña",
      c("", "Nombre"),
      c("", "Apellido"),
      c("", "Email"),
      c("", "Contraseña")
    ),
    g(
      "Indicá tu cargo, rol y materia",
      r("", /^Docente /),
      c("", "¿Sos titular o suplente?"),
      c("", "Curso o lugar donde trabajás"),
      c("", "Materia")
    ),
    b("Enviá tu solicitud", "Crear cuenta"),
  ],
  "docente-02b-donde-vas-a-estar": [
    c("Elegí el curso", "Curso o lugar donde trabajás"),
    c("Elegí la materia", "Materia"),
  ],
  "docente-03-olvide-contrasena": [
    c("Usá tu email registrado", "Email"),
    b("Solicitá el código", "Enviarme el código"),
  ],
  "docente-04-pantalla-de-inicio": [
    e("Hacé una reserva", "Reservar"),
    c("Elegí la fecha que querés ver", "Ir a fecha"),
    c("Consultá también otras materias", "Mostrar reservas"),
  ],
  "docente-05-nueva-reserva": [
    c("Elegí tu materia", "Materia"),
    c("Indicá la fecha", "Fecha"),
    horario("Definí el horario completo"),
  ],
  "docente-23-seleccion-equipos": [
    c("Buscá un equipo", "Buscar una computadora"),
    q("Marcá las computadoras", /^PC 1 /),
    b("Confirmá tu reserva", "Confirmar reserva"),
  ],
  "docente-06-reserva-semanal": [
    b("Elegí una serie semanal", "Se repite todas las semanas"),
    g(
      "Elegí el día y el período",
      c("", "Día de la semana"),
      c("", "Desde"),
      c("", "Hasta")
    ),
    horario("Definí el horario de cada clase"),
  ],
  "docente-22-consentimiento-serie": [
    b("Comprobá todas las fechas", "Comprobar serie"),
    m("Revisá el resultado de cada clase", "desplegable", /^Lunes,/),
    q("Autorizá la prioridad si corresponde", /^Autorizo tomar los equipos/),
  ],
  "docente-07-mis-reservas": [
    e("Consultá el calendario", "Ver calendario"),
    b("Reemplazá una computadora", "Cambiar computadora"),
    b("Cancelá si no vas a usarla", "Cancelar"),
  ],
  "docente-08-cancelar": [
    c("Podés explicar el motivo", "Motivo (opcional)"),
    b("Confirmá sólo si querés cancelar", "Confirmar cancelación"),
    b("Volvé sin cancelar", "Volver"),
  ],
  "docente-09-cambiar-computadora": [
    c("Elegí la PC que dejás", "¿Cuál cambiás?"),
    c("Elegí su reemplazo", "¿Por cuál?"),
    b("Guardá el cambio", "Cambiar"),
  ],
  "docente-10-computadoras": [
    b("Consultá cómo ingresar", "Cómo entrar"),
    b("Avisá si hay una falla", "Reportar problema"),
    e("Consultá cuándo está ocupada", "Ver calendario"),
  ],
  "docente-11-avisar-falla": [
    c("Contá qué ocurrió", "¿Qué le pasa?"),
    c("Indicá la gravedad", "Gravedad"),
    b("Enviá el reporte", "Reportar"),
  ],
  "docente-12-avisos": [
    b("Pedí ayuda", "Pedir ayuda"),
    b("Leé las respuestas", "Ver conversaciones"),
    b("Elegí tus copias por correo", "Elegir cuáles"),
  ],
  "docente-13-conversacion": [
    c("Escribí tu mensaje", "Escribir en esta conversación"),
    b("Enviá la respuesta", "Responder"),
  ],
  "docente-14-copias-por-correo": [
    q("Elegí qué copias querés recibir", "Cambios en los equipos de tus reservas"),
    b("Cerrá para volver a los avisos", "Cerrar"),
  ],
  "docente-15-perfil": [
    b("Corregí tu nombre", "Cambiar mi nombre"),
    b("Solicitá otra materia", "Pedir otra materia"),
    e("Cambiá tu contraseña", /^Cambiar mi contraseña/),
  ],
  "docente-16-cambiar-contrasena": [
    c("Escribí la contraseña actual", "Contraseña actual"),
    c("Elegí la nueva contraseña", "Contraseña nueva"),
    b("Guardá tu nueva contraseña", "Cambiar contraseña"),
  ],
  "docente-17-horario-admins": [
    t("Mirá si hay atención ahora", /^(No disponible|Disponible ahora)$/, "span"),
    t("Consultá el horario habitual", /^Lunes /, "span"),
  ],
  "docente-18-telefono": [
    b("Abrí las funciones del sistema", /^Menú/),
    e("Reservá desde el teléfono", "Reservar"),
  ],
  "docente-19-calendario-equipo": [
    b("Avanzá a otra semana", /Semana siguiente/),
    b("Elegí grilla o lista", "Lista de reservas"),
    b("Abrí una clase para ver el detalle", /^Programación/),
  ],
  "docente-20-como-entrar": [
    t("Identificá la cuenta para ingresar", "taller Administrador"),
    t("Esta clave requiere un Admin", "Solo la ven los administradores", "span"),
    t("Esta cuenta no pide contraseña", "Entra sin contraseña", "span"),
  ],
  "docente-21-como-entrar-telefono": [
    t("Usá el nombre de la cuenta", "taller Administrador"),
    t("Si no está anotada, consultá", "Contraseña no anotada", "span"),
  ],
  "docente-25-calendario-semana": [
    c("Elegí la fecha", "Ir a fecha"),
    c("Mostrá todas las materias", "Mostrar reservas"),
    b("Abrí una clase para ver el detalle", /^Matemática,.*10:30/),
  ],
  "docente-24-calendario-agenda": [
    c("Buscá otra fecha", "Ir a fecha"),
    b("Usá Agenda en el teléfono", "Agenda"),
    e("Volvé a gestionar tus reservas", "Listado y gestión"),
  ],
  "docente-26-agenda-clases": [
    b("Tu clase: abrí su detalle", /^Programación,/),
    b("Otra clase: consultá su ocupación", /^Matemática,.*10:30/),
  ],
  "admin-01-mostrador": [
    b("Entregá sin reserva previa", "Entregar sin reserva"),
    b("Registrá lo que vuelve", /^Recibir /),
    b("Abrí las demás funciones", "Administración"),
  ],
  "admin-02-menu": [
    e("Gestioná cuentas y permisos", "Usuarios"),
    e("Organizá cursos y materias", "Académico"),
    e("Ajustá plazos y avisos", "Configuración del sistema"),
  ],
  "admin-03-aprobacion": [
    b("Aprobá sólo si corresponde", "Aprobar"),
    b("Rechazá una solicitud incorrecta", "Rechazar"),
  ],
  "admin-04-academico": [
    c("Escribí la nueva materia", "Nueva materia"),
    b("Agregala al curso", "Agregar"),
    b("Asigná sus docentes", "Docentes"),
  ],
  "admin-05-usuarios": [
    c("Filtrá por estado de cuenta", "Estado"),
    b("Generá una contraseña temporal", "Restablecer contraseña"),
    b("Creá otro administrador", "Crear otro Admin"),
  ],
  "admin-06-inventario": [
    b("Editá la ficha del equipo", "Editar"),
    b("Consultá las cuentas de ingreso", "Cómo entrar"),
    b("Definí sus prioridades", "Preferencias"),
  ],
  "admin-07-entregas": [
    b("Registrá un retiro espontáneo", "Entregar sin reserva"),
    b("Registrá una devolución", /^Recibir /),
    b("Sacá un equipo a reparación", "Sacar un equipo a reparación"),
  ],
  "admin-08-entregar-sin-reserva": [
    c("Identificá al responsable", "¿A quién?"),
    q("Marcá los equipos que salen", /^PC 1 /),
    b("Registrá la entrega", "Entregar"),
  ],
  "admin-09-licencias": [
    b("Cargá una nueva licencia", "Cargar una licencia"),
    b("Registrá una renovación", "Renovar hoy"),
    b("Corregí sus datos", "Editar"),
  ],
  "admin-10-cargar-licencia": [
    c("Indicá el software", "Software"),
    q("Elegí dónde está instalado", /^PC 1\(/),
    c("Definí su vencimiento", "¿Cuándo vence?"),
  ],
  "admin-11-bloquear": [
    c("Definí fecha y horario", "Fecha"),
    c("Explicá por qué se necesitan", "¿Por qué se bloquean?"),
    b("Revisá el impacto antes de guardar", "Revisar bloqueo"),
  ],
  "admin-12-jornada": [
    b("Elegí los días de apertura", "Lunes a viernes"),
    c("Indicá la hora de apertura", "Abre: hora"),
    b("Editá un tramo existente", /^Editar Todos/),
  ],
  "admin-12b-jornada-impacto": [
    b("Volvé si necesitás revisar", "Volver sin cambiar nada"),
    b("Este botón cancela las clases indicadas", /^Guardar y cancelar/),
  ],
  "admin-12c-primera-jornada": [
    b("Elegí los días", "Lunes a viernes"),
    c("Cargá la hora de apertura", "Abre: hora"),
    b("Guardá la jornada institucional", "Guardar la jornada"),
  ],
  "admin-13-pedidos-de-materia": [
    q("Mostrá sólo los pendientes", "Ver solo los que faltan resolver"),
    b("Revisá y aprobá la asignación", "Aprobar"),
    b("Rechazá si no corresponde", "No aprobar"),
  ],
  "admin-14-soporte": [
    q("Filtrá las consultas pendientes", "Ver solo lo que falta contestar"),
    b("Abrí la conversación", "Ver"),
  ],
  "admin-15-soporte-conversacion": [
    c("Escribí la respuesta", "Escribir en esta conversación"),
    b("Enviá el mensaje", "Responder"),
    b("Cerrá el pedido cuando esté resuelto", "Dar por resuelta"),
  ],
  "admin-16-reportes": [
    c("Elegí el año lectivo", "Ciclo lectivo"),
    c("Acotá el período si hace falta", "Desde"),
    b("Descargá los datos para analizarlos", /^Descargar CSV:/),
  ],
  "admin-17-horarios-admin": [
    b("Corregí tu horario habitual", "Editar"),
    c("Elegí el día que atendés", "Día"),
    b("Agregá el bloque de atención", "Agregar"),
  ],
  "admin-18-avisos": [
    e("Revisá las cuentas pendientes", "Ir a aprobar"),
    b("Atendé las consultas", "Ver conversaciones"),
    b("Marcá lo que ya revisaste", "Marcar todas como leídas"),
  ],
  "admin-19-cerrar-anio": [
    r("Elegí si abrís otro ciclo", "Cerrar y crear el siguiente ciclo"),
    c("Revisá el año de destino", /^Año del ciclo siguiente/),
    b("Cerrar es definitivo: revisá antes", /^Cerrar \d{4} y crear \d{4}$/),
  ],
  "admin-20-calendario-equipo": [
    b("Consultá otra semana", /Semana siguiente/),
    b("Cambiá entre grilla y lista", "Lista de reservas"),
    b("Abrí el detalle de una clase", /^Programación/),
  ],
  "admin-21-perfil": [
    b("Actualizá tu foto", "Cambiar la foto"),
    b("Corregí tu nombre", "Cambiar mi nombre"),
    e("Cambiá tu contraseña", /^Cambiar mi contraseña/),
  ],
  "admin-22-cuentas-de-equipo": [
    b("Anotá una cuenta del equipo", "Agregar cuenta"),
    b("Revelá la clave si la necesitás", "Ver contraseña"),
    b("Corregí los datos de la cuenta", "Editar"),
  ],
  "admin-23-cuenta-nueva": [
    c("Anotá el usuario del equipo", "¿Con qué usuario se entra?"),
    r("Decidí quién puede ver la clave", "Solo los administradores"),
    b("Guardá la ficha de la cuenta", "Guardar"),
  ],
  "admin-24-preferencias": [
    c("Elegí el alcance de la marca", "Alcance"),
    c("Elegí la materia del catálogo", "Materia"),
    c("Un número menor tiene más prioridad", "Prioridad"),
  ],
  "admin-25-auditoria": [
    c("Filtrá la acción que buscás", "Acción"),
    c("Elegí sobre qué se actuó", "Sobre qué"),
    b("Abrí el cambio registrado", "Ver el detalle"),
  ],
  "admin-26-detalle-auditoria": [
    t("Compará el valor anterior", "Antes", "th"),
    t("Con el valor que quedó guardado", "Después", "th"),
  ],
  "admin-27-quitar-docente-compartida": [
    t("Todavía queda otra cuenta: las reservas siguen", /^Queda 1 docente/),
    b("Quitá sólo esta asignación", "Confirmar quitar docente"),
  ],
  "admin-28-quitar-ultimo-docente": [
    t("Sin otra cuenta, se cancelan las futuras", /^Si confirmás y la materia/),
    b("Volvé si necesitás asignar un reemplazo", "Volver"),
  ],
  "admin-29-menu-movil": [
    e("Accedé a entregas", /^Entregas/),
    m(
      "Desplegá las opciones de administración",
      "desplegable",
      "Administración y configuración"
    ),
    b("Cerrá el menú para volver", "Cerrar"),
  ],
  "admin-30-calendario": [
    c("Elegí el período", "Ir a fecha"),
    e("Creá una nueva reserva", "Nueva reserva"),
    b("Consultá la clase antes de actuar", /^Programación,/),
  ],
  "admin-31-editar-curso-prioridades": [
    c("Corregí el curso", "División (opcional)"),
    r(
      "Elegí qué pasa con las prioridades",
      "Conservar las prioridades exactas con el curso"
    ),
    b("Confirmá el alcance revisado", "Confirmar cambio de curso"),
  ],
  "admin-32-cambiar-rotulo": [
    c("Indicá el nuevo rótulo", "Nuevo número de PC"),
    q("Confirmá que es la misma computadora", /^Confirmo que es la misma/),
    b("Guardá el número y conservá su identidad", "Confirmar nuevo rótulo"),
  ],
  "admin-33-calendario-movil": [
    b("Tocá la clase para ver su detalle", /^Programación,/),
    b("Revisá las otras clases del día", /^Matemática,.*10:30/),
  ],
  "admin-34-configuracion": [
    c("Ajustá la espera para retirar", "Espera para retirar una reserva (minutos)"),
    c(
      "Elegí cuándo recordar una clase",
      "Antelación del recordatorio de reserva (minutos)"
    ),
    b("Aplicá los plazos a la institución", "Guardar configuración"),
  ],
  "general-01-recuperar-pantalla": [
    b("Recargá la pantalla", "Recargar la página"),
    e("Si hace falta, volvé al inicio", "Volver al inicio"),
  ],
}

const coincide = (texto, patron) =>
  patron instanceof RegExp ? patron.test(texto) : texto === patron
const referencia = (marca) =>
  JSON.stringify((marca.miembros ?? [marca]).map((m) => [m.tipo, String(m.nombre)]))

export function prepararAnotaciones(capturas) {
  return {
    ...capturas,
    capturas: capturas.capturas.map((captura) => {
      const { controles, ...base } = captura
      const esquema = ESQUEMA[captura.archivo.replace(/\.png$/, "")]
      if (!esquema)
        throw new Error(`Faltan indicaciones visuales para ${captura.archivo}`)
      if (!controles && captura.anotaciones) {
        if (
          captura.anotaciones.length !== esquema.length ||
          esquema.some(
            (marca, i) => captura.anotaciones[i].referencia !== referencia(marca)
          )
        )
          throw new Error(
            `Cambiaron los objetivos de ${captura.archivo}; ejecutar make capturas`
          )
        return {
          ...base,
          anotaciones: captura.anotaciones.map((marca, i) => ({
            ...marca,
            texto: esquema[i].texto,
          })),
        }
      }
      if (!controles || !captura.encuadre)
        throw new Error(
          `Falta medir los controles de ${captura.archivo}; ejecutar make capturas`
        )
      const usados = new Set()
      const anotaciones = esquema.map((marca, indice) => {
        const objetivos = (marca.miembros ?? [marca]).map((miembro) =>
          controles.find(
            (control) =>
              control.tipo === miembro.tipo &&
              coincide(control.texto, miembro.nombre) &&
              !usados.has(control)
          )
        )
        if (objetivos.some((objetivo) => !objetivo))
          throw new Error(
            `${captura.archivo}: no se encuentra dentro del encuadre «${marca.texto}» (${referencia(marca)})`
          )
        for (const objetivo of objetivos) usados.add(objetivo)
        const x = Math.min(...objetivos.map((o) => o.x)),
          y = Math.min(...objetivos.map((o) => o.y))
        const ancho = Math.max(...objetivos.map((o) => o.x + o.ancho)) - x
        const alto = Math.max(...objetivos.map((o) => o.y + o.alto)) - y
        return {
          numero: indice + 1,
          texto: marca.texto,
          objetivo: objetivos.map((o) => o.texto).join(" · "),
          referencia: referencia(marca),
          x,
          y,
          ancho,
          alto,
        }
      })
      const protecciones = controles
        .filter((c) =>
          ["button", "campo", "checkbox", "radio", "enlace", "desplegable"].includes(
            c.tipo
          )
        )
        .map((c) => [c.x, c.y, c.ancho, c.alto].map((v) => Number(v.toFixed(2))))
      return { ...base, anotaciones, protecciones }
    }),
  }
}

const escapar = (s) =>
  String(s).replace(
    /[&<>"']/g,
    (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]
  )
const n = (valor) => Number(valor.toFixed(2))

function atraviesa(a, b, zona) {
  let entrada = 0,
    salida = 1
  for (const eje of [0, 1]) {
    const delta = b[eje] - a[eje]
    if (Math.abs(delta) < 0.001) {
      if (a[eje] < zona[eje] || a[eje] > zona[eje] + zona[eje + 2]) return false
    } else {
      const t1 = (zona[eje] - a[eje]) / delta
      const t2 = (zona[eje] + zona[eje + 2] - a[eje]) / delta
      entrada = Math.max(entrada, Math.min(t1, t2))
      salida = Math.min(salida, Math.max(t1, t2))
      if (entrada > salida) return false
    }
  }
  return true
}

function ubicarFlecha(marca, captura, margen, radio, asignadas) {
  const w = captura.encuadre.ancho,
    h = captura.encuadre.alto
  const zonas = (captura.protecciones ?? [])
    .filter(
      ([x, y, ancho, alto]) =>
        !(
          x >= marca.x - 1 &&
          y >= marca.y - 1 &&
          x + ancho <= marca.x + marca.ancho + 1 &&
          y + alto <= marca.y + marca.alto + 1
        )
    )
    .map(([x, y, ancho, alto]) => [x + margen, y, ancho, alto])
  const candidatos = []
  for (const derecha of [false, true]) {
    const lado = derecha ? "derecha" : "izquierda"
    const minimo = radio + 4,
      maximo = h - radio - 4,
      separacion = radio * 2 + 8
    const deseado = Math.min(maximo, Math.max(minimo, marca.y + marca.alto / 2))
    const libres = [
      deseado,
      minimo,
      maximo,
      ...asignadas[lado].flatMap((y) => [y - separacion, y + separacion]),
    ]
      .filter(
        (y) =>
          y >= minimo &&
          y <= maximo &&
          asignadas[lado].every((otro) => Math.abs(y - otro) >= separacion - 0.01)
      )
      .sort((a, b) => Math.abs(a - deseado) - Math.abs(b - deseado))
    if (!libres.length) continue
    const cy = libres[0]
    const cx = derecha ? w + margen * 1.5 : margen / 2
    const inicio = [cx + (derecha ? -radio : radio), cy]
    const caminos = [
      [
        inicio,
        [margen + marca.x + (derecha ? marca.ancho + 3 : -3), marca.y + marca.alto / 2],
      ],
    ]
    // Rodear la fila si la flecha tacharía el texto de otra acción.
    const gx = derecha ? margen + w + 4 : margen - 4
    const mx = margen + marca.x + marca.ancho / 2
    if (marca.y > 12)
      caminos.push([
        inicio,
        [gx, cy],
        [gx, marca.y - 10],
        [mx, marca.y - 10],
        [mx, marca.y - 3],
      ])
    if (marca.y + marca.alto < h - 12)
      caminos.push([
        inicio,
        [gx, cy],
        [gx, marca.y + marca.alto + 10],
        [mx, marca.y + marca.alto + 10],
        [mx, marca.y + marca.alto + 3],
      ])
    for (const puntos of caminos) {
      let choques = 0,
        longitud = 0
      for (let i = 1; i < puntos.length; i++) {
        choques += zonas.filter((zona) =>
          atraviesa(puntos[i - 1], puntos[i], zona)
        ).length
        longitud += Math.hypot(
          puntos[i][0] - puntos[i - 1][0],
          puntos[i][1] - puntos[i - 1][1]
        )
      }
      candidatos.push({ cx, cy, lado, puntos, costo: choques * 100000 + longitud })
    }
  }
  const mejor = candidatos.sort((a, b) => a.costo - b.costo)[0]
  if (!mejor)
    throw new Error(`No hay espacio para numerar ${captura.archivo}; ampliar el encuadre`)
  asignadas[mejor.lado].push(mejor.cy)
  return {
    cx: mejor.cx,
    cy: mejor.cy,
    linea: mejor.puntos.map((p, i) => `${i ? "L" : "M"} ${n(p[0])} ${n(p[1])}`).join(" "),
  }
}

function ilustracion(imagen, captura, id) {
  const { ancho: w, alto: h } = captura.encuadre
  const margen = Math.max(38, w * 0.034),
    radio = Math.max(16, w * 0.014)
  const ancho = w + margen * 2,
    texto = Math.max(22, w * 0.019)
  const asignadas = { izquierda: [], derecha: [] }
  const formas = captura.anotaciones
    .map((marca) => {
      const { cx, cy, linea } = ubicarFlecha(marca, captura, margen, radio, asignadas)
      const borde = Math.max(2, w / 500)
      return `<rect x="${n(margen + marca.x - 3)}" y="${n(marca.y - 3)}" width="${n(marca.ancho + 6)}" height="${n(marca.alto + 6)}" rx="5" fill="none" stroke="#b91c1c" stroke-width="${n(borde)}"/>
      <path d="${linea}" stroke="white" stroke-width="${n(borde + 3)}" fill="none"/>
      <path d="${linea}" stroke="#b91c1c" stroke-width="${n(borde)}" fill="none" marker-end="url(#${id})"/>
      <circle cx="${n(cx)}" cy="${n(cy)}" r="${n(radio)}" fill="#b91c1c" stroke="white" stroke-width="2"/>
      <text x="${n(cx)}" y="${n(cy)}" text-anchor="middle" dominant-baseline="central" fill="white" font-family="Arial,sans-serif" font-size="${n(texto)}" font-weight="bold">${marca.numero}</text>`
    })
    .join("\n")
  const pasos = captura.anotaciones
    .map(
      (marca) =>
        `<li><span aria-hidden="true">${marca.numero}</span>${escapar(marca.texto)}</li>`
    )
    .join("")
  return `<div class="guia-visual" style="width:min(100%,${n((160 * ancho) / h)}mm)"><div style="margin-inline:${n((margen / ancho) * 100)}%">${imagen}</div><svg xmlns="http://www.w3.org/2000/svg" class="guia-marcas" viewBox="0 0 ${n(ancho)} ${n(h)}" aria-hidden="true" focusable="false"><defs><marker id="${id}" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto" markerUnits="userSpaceOnUse"><path d="M0,0 L8,4 L0,8 Z" fill="#b91c1c"/></marker></defs>${formas}</svg></div><ol class="guia-indicaciones" aria-label="Indicaciones de esta imagen">${pasos}</ol>`
}

const ESTILO = `<style data-anotaciones-guias>
.guia-visual{position:relative;display:inline-block;vertical-align:top;line-height:0;max-width:100%;}
.guia-visual img{display:block;width:100%;max-width:100%;height:auto;max-height:none;}
.guia-marcas{position:absolute;inset:0;width:100%;height:100%;overflow:visible;pointer-events:none;}
.guia-indicaciones{list-style:none;padding:0;margin:3mm 0 2mm;display:flex;flex-wrap:wrap;gap:2mm 4mm;text-align:left;line-height:1.35;font-size:10pt;}
.guia-indicaciones li{display:flex;gap:2mm;align-items:flex-start;flex:1 1 47mm;break-inside:avoid;}
.guia-indicaciones li>span{display:inline-flex;align-items:center;justify-content:center;flex:none;width:5.5mm;height:5.5mm;border-radius:50%;background:#b91c1c;color:white;font-size:9pt;font-weight:700;line-height:1;}
figure[data-guia-anotada]{break-inside:avoid;}
</style>`

export function anotarHTML(html, capturas) {
  const porNombre = new Map(capturas.capturas.map((c) => [c.archivo, c]))
  // Permite regenerar un HTML ya anotado sin duplicar ni acumular marcas.
  html = html.replace(/<style data-anotaciones-guias>[\s\S]*?<\/style>\s*/g, "")
  html = html.replace(/<figure\b[^>]*>[\s\S]*?<\/figure>/g, (figura) => {
    const imagen = figura.match(/<img\b[^>]*src="imagenes\/([^"/]+)"[^>]*>/)
    if (!imagen) return figura
    const captura = porNombre.get(imagen[1])
    if (!captura?.anotaciones?.length) throw new Error(`Faltan marcas para ${imagen[1]}`)
    const pie = figura.match(/<figcaption>[\s\S]*?<\/figcaption>/)?.[0] ?? ""
    const id = "flecha-" + imagen[1].replace(/[^a-zA-Z0-9-]/g, "-")
    return `<figure data-guia-anotada="${escapar(imagen[1])}">\n${ilustracion(imagen[0], captura, id)}\n${pie}\n</figure>`
  })
  return html.replace("</style>", "</style>\n" + ESTILO)
}
