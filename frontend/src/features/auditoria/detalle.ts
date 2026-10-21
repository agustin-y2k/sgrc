import { formatearFechaLarga, formatearFechaYHora } from "@/lib/fechas"
import { accionLegible, entidadEnFrase, type EntradaDeAuditoria } from "./types"

type Registro = Record<string, unknown>
export type CambioLegible = { campo: string; antes?: string; despues?: string }
export type DetalleLegible = {
  titulo: string
  cambios: CambioLegible[]
  datos: { campo: string; valor: string }[]
  otrosCampos: string[]
  seguridadActualizada: boolean
}

const clave = (campo: string) => campo.replace(/[_\s-]/g, "").toLowerCase()
const CAMPOS: Record<string, string> = {
  estado: "Estado",
  rol: "Rol",
  tipo: "Tipo",
  archivado: "Archivado",
  activo: "Activo",
  anio: "Año",
  division: "División",
  modalidad: "Modalidad",
  reservable: "Se puede reservar",
  dadodebaja: "Retirado del inventario",
  fecha: "Fecha",
  horainicio: "Hora de inicio",
  horafin: "Hora de fin",
  fechaaprobacion: "Fecha de aprobación",
  nombre: "Nombre",
  apellido: "Apellido",
  nombreanterior: "Nombre anterior",
  nombrenuevo: "Nombre nuevo",
  apellidoanterior: "Apellido anterior",
  apellidonuevo: "Apellido nuevo",
  identificador: "Número del equipo",
  numeroserie: "Número de serie",
  cpu: "Procesador",
  ram: "Memoria RAM",
  sistemaoperativo: "Sistema operativo",
  softwareinstalado: "Software instalado",
  freezado: "Protegido contra cambios",
  prioridad: "Prioridad",
  cursoexacto: "Se aplica a un curso concreto",
  materianombre: "Materia",
  motivo: "Motivo",
  soloesta: "Sólo esta fecha",
  usuario: "Cuenta de acceso",
  clase: "Clase de cuenta",
  privilegio: "Privilegio",
  visibilidad: "Quién puede ver la contraseña",
  tienepassword: "La cuenta tiene contraseña",
  debecambiarpassword: "Debe cambiar su contraseña",
  intentos: "Intentos de recuperación",
  fechavencimiento: "Vencimiento",
  ultimarenovacion: "Última renovación",
  vencimientofijadoen: "Fecha de actualización del vencimiento",
  diasduracion: "Duración en días",
  diasaviso: "Anticipación del aviso en días",
  devueltoen: "Fecha de devolución",
  fechaalta: "Fecha de alta",
  fechabaja: "Fecha de baja",
  canceladaen: "Fecha de cancelación",
  entregadoen: "Fecha de entrega",
  cursoscreados: "Cursos creados",
  materiascreadas: "Materias creadas",
  cursosomitidos: "Cursos ya existentes",
  materiasomitidas: "Materias ya existentes",
  reservascanceladas: "Reservas canceladas",
  clasescanceladas: "Clases canceladas",
  reservaseliminadas: "Reservas eliminadas",
  gruposeliminados: "Clases eliminadas",
  docentesnotificados: "Docentes avisados",
  tramos: "Franjas horarias",
  anioanterior: "Año anterior",
  anionuevo: "Año nuevo",
  nuevoestado: "Estado nuevo",
  rolanterior: "Rol anterior",
  rolnuevo: "Rol nuevo",
  cursoorigenid: "Curso de origen",
  cursosdestinoids: "Cursos de destino",
  nuevocicloid: "Nuevo ciclo lectivo",
  equipoid: "Equipo",
  carroid: "Carro",
  carrodestinoid: "Carro de destino",
  materiaid: "Materia",
  cursoid: "Curso",
  espacioid: "Espacio",
  usuarioid: "Persona",
  creadopor: "Creado por",
  canceladopor: "Cancelado por",
  aprobadopor: "Aprobado por",
  entregadopor: "Entregado por",
  recibidopor: "Recibido por",
  entregadoausuarioid: "Responsable del préstamo",
  vencimientofijadopor: "Vencimiento registrado por",
  reglarecurrenciaid: "Serie de reservas",
  reservagrupoid: "Clase reservada",
  reservaid: "Reserva",
  ciclolectivoid: "Ciclo lectivo",
}

const REFERENCIAS: Record<string, string> = {
  equipoid: "equipo",
  carroid: "carro",
  carrodestinoid: "carro",
  materiaid: "materia",
  cursoid: "curso",
  cursoorigenid: "curso",
  cursosdestinoids: "curso",
  espacioid: "espacio",
  usuarioid: "usuario",
  creadopor: "usuario",
  canceladopor: "usuario",
  aprobadopor: "usuario",
  entregadopor: "usuario",
  recibidopor: "usuario",
  entregadoausuarioid: "usuario",
  vencimientofijadopor: "usuario",
  reglarecurrenciaid: "regla_recurrencia",
  reservagrupoid: "reserva_grupo",
  reservaid: "reserva",
  ciclolectivoid: "ciclo_lectivo",
  nuevocicloid: "ciclo_lectivo",
}
const OCULTOS = new Set([
  "id",
  "version",
  "versionsesion",
  "creadaen",
  "actualizadaen",
  "creadoen",
])
const UUID = /^[\da-f]{8}-(?:[\da-f]{4}-){3}[\da-f]{12}$/i
const ESTADOS: Record<string, string> = {
  DISPONIBLE: "Disponible",
  EN_MANTENIMIENTO: "En mantenimiento",
  FUERA_DE_SERVICIO: "Fuera de servicio",
  CONFIRMADA: "Confirmada",
  PARCIALMENTE_CANCELADA: "Parcialmente cancelada",
  CANCELADA: "Cancelada",
  FINALIZADA: "Finalizada",
  NO_RETIRADA: "No retirada",
  NO_RETIRADO: "No retirado",
  APROBADA: "Aprobada",
  PENDIENTE: "Pendiente",
  RECHAZADA: "Rechazada",
  BAJA: "De baja",
  ADMIN: "Administrador",
  DOCENTE: "Docente",
  TITULAR: "Titular",
  SUPLENTE: "Suplente",
  NORMAL: "Reserva de clase",
  BLOQUEO: "Bloqueo institucional",
  PUBLICA: "Todos los usuarios",
  SOLO_ADMIN: "Sólo administradores",
  COMUN: "Usuario común",
  ADMINISTRADOR: "Administrador",
  ABIERTA: "Abierta",
  EN_REPARACION: "En reparación",
  ENVIADA_A_SOPORTE: "Enviada a soporte",
  RESUELTA: "Resuelta",
  LEVE: "Leve",
  MODERADA: "Moderada",
  GRAVE: "Grave",
}

function registro(value: unknown): Registro | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Registro)
    : undefined
}

function privado(campo: string): boolean {
  const k = clave(campo)
  return (
    !["tienepassword", "debecambiarpassword"].includes(k) &&
    /password|contrasena|contraseña|secret|token|hash|salt|^codigo$/.test(k)
  )
}

export function campoLegible(campo: string): string {
  const conocida = CAMPOS[clave(campo)]
  if (conocida) return conocida
  const texto = campo
    .replace(/([a-záéíóúñ])([A-ZÁÉÍÓÚÑ])/g, "$1 $2")
    .replace(/_/g, " ")
    .toLowerCase()
  return texto.charAt(0).toUpperCase() + texto.slice(1)
}

function valorLegible(
  value: unknown,
  campo: string,
  entrada: EntradaDeAuditoria
): string {
  if (value === null || value === undefined) return "Sin asignar"
  if (typeof value === "boolean") return value ? "Sí" : "No"
  if (typeof value === "number") return value.toLocaleString("es-AR")
  if (Array.isArray(value))
    return value.map((v) => valorLegible(v, campo, entrada)).join(", ") || "Ninguno"
  if (typeof value !== "string") return "Información registrada"
  if (UUID.test(value)) {
    const tipo = REFERENCIAS[clave(campo)]
    return (
      entrada.referencias?.[`${tipo}:${value.toLowerCase()}`] ??
      "Referencia no disponible"
    )
  }
  if (ESTADOS[value]) return ESTADOS[value]
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return formatearFechaLarga(value)
  if (/^\d{4}-\d{2}-\d{2}T/.test(value) && !Number.isNaN(Date.parse(value)))
    return formatearFechaYHora(value)
  if (/^\d{2}:\d{2}:\d{2}$/.test(value)) return value.slice(0, 5)
  return value || "Sin asignar"
}

export function actorLegible(entrada: EntradaDeAuditoria): string {
  if (entrada.actorId === "00000000-0000-0000-0000-000000000000")
    return "Sin usuario identificado"
  return entrada.actorNombre || "Una cuenta que después se eliminó"
}

export function detalleLegible(entrada: EntradaDeAuditoria): DetalleLegible {
  const detalle = registro(entrada.detalle)
  const modelo: DetalleLegible = {
    titulo: accionLegible(entrada.accion),
    cambios: [],
    datos: [],
    otrosCampos: [],
    seguridadActualizada: false,
  }
  const transaccional = entrada.accion === "CAMBIO_TRANSACCIONAL"
  if (transaccional && detalle) {
    const operacion = { INSERT: "Se creó", UPDATE: "Se actualizó", DELETE: "Se eliminó" }[
      String(detalle.operacion)
    ]
    if (operacion)
      modelo.titulo = `${operacion} ${entidadEnFrase(entrada.entidad)
        .replace(/^este /, "el ")
        .replace(/^esta /, "la ")}`
    const antes = registro(detalle.antes) ?? {}
    const despues = registro(detalle.despues) ?? {}
    const campos = new Set([
      ...(Array.isArray(detalle.campos)
        ? detalle.campos.filter((c): c is string => typeof c === "string")
        : []),
      ...Object.keys(antes),
      ...Object.keys(despues),
    ])
    for (const campo of campos) {
      if (privado(campo) || clave(campo) === "versionsesion") {
        modelo.seguridadActualizada = true
        continue
      }
      if (OCULTOS.has(clave(campo)) || /_norm$/.test(campo)) continue
      const tieneAntes = Object.hasOwn(antes, campo)
      const tieneDespues = Object.hasOwn(despues, campo)
      if (tieneAntes || tieneDespues) {
        modelo.cambios.push({
          campo: campoLegible(campo),
          antes: tieneAntes ? valorLegible(antes[campo], campo, entrada) : undefined,
          despues: tieneDespues
            ? valorLegible(despues[campo], campo, entrada)
            : undefined,
        })
      } else modelo.otrosCampos.push(campoLegible(campo))
    }
  } else if (detalle) {
    for (const [campo, value] of Object.entries(detalle)) {
      if (privado(campo)) {
        modelo.seguridadActualizada = true
        continue
      }
      if (OCULTOS.has(clave(campo))) continue
      const cambio = registro(value)
      if (
        cambio &&
        (Object.hasOwn(cambio, "antes") || Object.hasOwn(cambio, "despues"))
      ) {
        modelo.cambios.push({
          campo: campoLegible(campo),
          antes: Object.hasOwn(cambio, "antes")
            ? valorLegible(cambio.antes, campo, entrada)
            : undefined,
          despues: Object.hasOwn(cambio, "despues")
            ? valorLegible(cambio.despues, campo, entrada)
            : undefined,
        })
      } else if (!cambio)
        modelo.datos.push({
          campo: campoLegible(campo),
          valor: valorLegible(value, campo, entrada),
        })
      else modelo.otrosCampos.push(campoLegible(campo))
    }
  } else if (typeof entrada.detalle === "string" && !transaccional) {
    modelo.datos.push({ campo: "Información", valor: entrada.detalle })
  }
  return modelo
}

export function resumenDelDetalle(detalle: DetalleLegible): string {
  const cambios = detalle.cambios
    .slice(0, 2)
    .map(
      (c) =>
        `${c.campo}: ${c.antes !== undefined && c.despues !== undefined ? `${c.antes} → ${c.despues}` : (c.despues ?? c.antes)}`
    )
  if (cambios.length) return cambios.join(". ")
  if (detalle.datos.length)
    return detalle.datos
      .slice(0, 2)
      .map((d) => `${d.campo}: ${d.valor}`)
      .join(". ")
  if (detalle.seguridadActualizada)
    return "Se actualizó información de seguridad de la cuenta."
  if (detalle.otrosCampos.length)
    return `Datos actualizados: ${detalle.otrosCampos.slice(0, 3).join(", ")}.`
  return ""
}
