// Datos ficticios creados exclusivamente mediante la API en la base efímera.
import { readFile } from "node:fs/promises"

const { version } = JSON.parse(
  await readFile(new URL("../../../frontend/package.json", import.meta.url), "utf8")
)

export async function prepararDatos(api, admin) {
  const hoy = new Date()
  const fecha = (d) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
  const lunes = new Date(hoy.getFullYear(), hoy.getMonth(), hoy.getDate() + 2, 12)
  while (lunes.getDay() !== 1) lunes.setDate(lunes.getDate() + 1)
  // Toda la serie de demostración debe pertenecer al mismo año lectivo.
  if (
    new Date(
      lunes.getFullYear(),
      lunes.getMonth(),
      lunes.getDate() + 21
    ).getFullYear() !== lunes.getFullYear()
  ) {
    lunes.setFullYear(lunes.getFullYear() + 1, 0, 1)
    while (lunes.getDay() !== 1) lunes.setDate(lunes.getDate() + 1)
  }
  const dia = (offset) =>
    fecha(new Date(lunes.getFullYear(), lunes.getMonth(), lunes.getDate() + offset, 12))
  const dias = ["DOMINGO", "LUNES", "MARTES", "MIERCOLES", "JUEVES", "VIERNES", "SABADO"]
  await api(
    "PUT",
    "/api/jornada",
    {
      tramos: dias.map((diaSemana) => ({
        diaSemana,
        horaInicio: "07:00",
        horaFin: "23:00",
      })),
    },
    admin
  )
  const ciclo = await api("POST", "/api/ciclos", { anio: lunes.getFullYear() }, admin)
  const curso = await api(
    "POST",
    `/api/ciclos/${ciclo.id}/cursos`,
    { anio: 1, division: "1" },
    admin
  )
  const tecnico = await api(
    "POST",
    `/api/ciclos/${ciclo.id}/cursos`,
    { anio: 4, division: "2", modalidad: "Electromecánica" },
    admin
  )
  await api("POST", `/api/ciclos/${ciclo.id}/espacios`, { nombre: "Biblioteca" }, admin)
  const programacion = await api(
    "POST",
    `/api/cursos/${curso.id}/materias`,
    { nombre: "Programación" },
    admin
  )
  const matematica = await api(
    "POST",
    `/api/cursos/${curso.id}/materias`,
    { nombre: "Matemática" },
    admin
  )
  await api("POST", `/api/cursos/${tecnico.id}/materias`, { nombre: "Taller" }, admin)
  async function docente(nombre, apellido, email, aprobada, cargo = "DOCENTE") {
    await api("POST", "/api/auth/registro", {
      nombre,
      apellido,
      email,
      password: process.env.GUIA_DOCENTE_PASSWORD,
      cargoSolicitado: cargo,
      rolSolicitado: "TITULAR",
    })
    const u = (
      await api("GET", "/api/auth/usuarios?pageSize=200", undefined, admin)
    ).data.find((u) => u.email === email)
    if (aprobada)
      await api(
        "PATCH",
        `/api/auth/usuarios/${u.id}/estado`,
        { estado: "APROBADA" },
        admin
      )
    return u
  }
  const ana = await docente("Ana", "Gómez", process.env.GUIA_DOCENTE_EMAIL, true)
  const ada = await docente("Ada", "Lovelace", "ada.lovelace@escuela.test", true)
  await docente("Marcos", "Ruiz", "marcos.ruiz@escuela.test", false, "ADMIN_SISTEMA")
  for (const [materia, usuario] of [
    [programacion, ana],
    [matematica, ana],
    [matematica, ada],
  ])
    await api(
      "POST",
      `/api/materias/${materia.id}/docentes`,
      { usuarioId: usuario.id, rol: "TITULAR" },
      admin
    )
  const tokenAna = (
    await api("POST", "/api/auth/login", {
      email: process.env.GUIA_DOCENTE_EMAIL,
      password: process.env.GUIA_DOCENTE_PASSWORD,
    })
  ).token
  const tokenAda = (
    await api("POST", "/api/auth/login", {
      email: "ada.lovelace@escuela.test",
      password: process.env.GUIA_DOCENTE_PASSWORD,
    })
  ).token
  const carro = await api(
    "POST",
    "/api/carros",
    { nombre: "Carro 1", descripcion: "Notebooks del laboratorio" },
    admin
  )
  const pcs = []
  for (let i = 1; i <= 8; i++)
    pcs.push(
      await api(
        "POST",
        `/api/carros/${carro.id}/equipos`,
        {
          identificador: i,
          numeroSerie: `DEMO-PC-${String(i).padStart(2, "0")}`,
          cpu: "Intel Core i5",
          ram: "8 GB",
          sistemaOperativo: "Windows 11",
          softwareInstalado: "Office 2021, AutoCAD 2026",
          freezado: true,
        },
        admin
      )
    )
  const proyector = await api(
    "POST",
    "/api/equipos",
    {
      nombre: "Proyector 1",
      tipo: "Proyector",
      numeroSerie: "DEMO-PRY-01",
      reservable: true,
    },
    admin
  )
  await api(
    "POST",
    "/api/equipos",
    { nombre: "Cargador 1", tipo: "Cargador", reservable: false },
    admin
  )
  await api(
    "POST",
    "/api/equipos",
    {
      nombre: "Notebook de Dirección",
      tipo: "Notebook",
      esComputadora: true,
      reservable: true,
      cpu: "Intel Core i5",
      ram: "8 GB",
      sistemaOperativo: "Windows 11",
      softwareInstalado: "Office 2021",
    },
    admin
  )
  for (const cuenta of [
    {
      usuario: "alumno",
      clase: "Local",
      privilegio: "COMUN",
      visibilidad: "PUBLICA",
      tienePassword: false,
      notas: "La que usan los estudiantes",
    },
    {
      usuario: "taller",
      clase: "Local",
      privilegio: "ADMINISTRADOR",
      visibilidad: "PUBLICA",
      tienePassword: true,
      password: "Ejemplo.2026",
      notas: "Contraseña ficticia para la guía",
    },
    {
      usuario: "soporte",
      clase: "Microsoft",
      privilegio: "ADMINISTRADOR",
      visibilidad: "SOLO_ADMIN",
      tienePassword: true,
      password: "SoloDemostracion.2026",
    },
    {
      usuario: "profesor",
      clase: "Linux",
      privilegio: "COMUN",
      visibilidad: "PUBLICA",
      tienePassword: true,
      notas: "Contraseña sin anotar",
    },
  ])
    await api("POST", `/api/equipos/${pcs[0].id}/cuentas`, cuenta, admin)
  for (const marca of [
    { materiaNombre: "Programación", prioridad: 1 },
    {
      materiaNombre: "Matemática",
      anio: 1,
      division: "1",
      prioridad: 2,
      cursoExacto: true,
    },
  ])
    await api(
      "POST",
      "/api/preferencias",
      { equipoIds: [pcs[0].id, pcs[1].id], ...marca },
      admin
    )
  await api(
    "POST",
    "/api/reservas",
    {
      materiaId: programacion.id,
      fecha: dia(0),
      horaInicio: "10:00",
      horaFin: "11:30",
      equipoIds: pcs.slice(0, 6).map((p) => p.id),
    },
    tokenAna
  )
  await api(
    "POST",
    "/api/reservas/recurrentes",
    {
      materiaId: matematica.id,
      diaSemana: "MIERCOLES",
      fechaInicio: dia(2),
      fechaFin: dia(16),
      horaInicio: "08:00",
      horaFin: "09:30",
      equipoIds: pcs.slice(0, 3).map((p) => p.id),
    },
    tokenAna
  )
  for (const offset of [0, 7])
    await api(
      "POST",
      "/api/reservas",
      {
        materiaId: matematica.id,
        fecha: dia(offset),
        horaInicio: "14:00",
        horaFin: "15:30",
        equipoIds: [pcs[0].id],
      },
      tokenAda
    )
  // Una clase breve simultánea muestra cómo leer la semana y abrir su detalle.
  await api(
    "POST",
    "/api/reservas",
    {
      materiaId: matematica.id,
      fecha: dia(0),
      horaInicio: "10:30",
      horaFin: "10:45",
      equipoIds: [pcs[6].id],
    },
    tokenAda
  )
  await api(
    "POST",
    "/api/licencias",
    {
      nombre: "AutoCAD 2026",
      equipoIds: pcs.slice(0, 3).map((p) => p.id),
      diasDuracion: 365,
      venceEl: dia(90),
      diasAviso: 15,
    },
    admin
  )
  await api(
    "POST",
    "/api/licencias",
    {
      nombre: "Office 365",
      equipoIds: pcs.slice(3, 5).map((p) => p.id),
      diasDuracion: 365,
      venceEl: fecha(new Date(hoy.getFullYear(), hoy.getMonth(), hoy.getDate() + 2, 12)),
      diasAviso: 7,
    },
    admin
  )
  await api(
    "POST",
    "/api/incidencias",
    {
      equipoId: pcs[7].id,
      descripcion: "La batería no conserva carga; usar conectada hasta el reemplazo.",
      gravedad: "MODERADA",
      categoria: "Batería",
    },
    tokenAna
  )
  await api(
    "PATCH",
    `/api/equipos/${pcs[7].id}/estado`,
    { estado: "EN_MANTENIMIENTO", motivo: "Cambio de batería" },
    admin
  )
  await api(
    "POST",
    "/api/prestamos",
    {
      equipoIds: [proyector.id],
      nombre: "Secretaría",
      destino: "Sala de reuniones",
      devolucionEstimada: new Date(Date.now() + 2 * 60 * 60 * 1000).toISOString(),
    },
    admin
  )
  const hilo = await api(
    "POST",
    "/api/sugerencias",
    {
      tipo: "AYUDA",
      asunto: "Cambiar una computadora de mi clase",
      texto:
        "Necesito cambiar una notebook sin perder las demás reservas. ¿Cómo lo hago?",
      pantalla: "/reservas",
      version,
    },
    tokenAna
  )
  await api(
    "POST",
    `/api/sugerencias/${hilo.id}/mensajes`,
    {
      texto:
        "Hola Ana. En Reservas, abrí tu clase y elegí Cambiar computadora. Elegí la máquina de origen y otra libre; si es una serie, revisá el alcance antes de guardar.",
    },
    admin
  )
  await api(
    "POST",
    "/api/pedidos-de-materia",
    {
      materiaId: programacion.id,
      motivo:
        "Me asignaron el segundo turno y necesito reservar equipos para esa materia.",
    },
    tokenAda
  )
  for (const diaSemana of ["LUNES", "MARTES", "MIERCOLES", "JUEVES", "VIERNES"])
    await api(
      "POST",
      "/api/mi-horario",
      { diaSemana, horaInicio: "08:00", horaFin: "18:00" },
      admin
    )
  // Un cambio de ficha amplía los ejemplos del registro de auditoría.
  await api(
    "PATCH",
    `/api/carros/${carro.id}`,
    {
      nombre: "Carro 1",
      descripcion: "Notebooks del laboratorio · planta baja",
    },
    admin
  )
  return {
    ciclo,
    curso,
    programacion,
    matematica,
    carro,
    pcs,
    hilo,
    lunes: dia(0),
    miercoles: dia(2),
    finSerie: dia(21),
    tokenAna,
  }
}
