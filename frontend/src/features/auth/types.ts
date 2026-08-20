import type { PaginacionMeta } from "@/components/Paginador"

// Espeja internal/auth/interfaces/http/dto.go — la fuente de verdad es ese
// archivo, no docs/08-api-spec.yaml (ver nota en api-client.ts).

export type Rol = "ADMIN" | "DOCENTE"
export type Estado = "PENDIENTE" | "APROBADA" | "RECHAZADA" | "BAJA"

export type Usuario = {
  id: string
  nombre: string
  apellido: string
  email: string
  rol: Rol
  estado: Estado
  fechaRegistro: string
  fechaAprobacion: string | null
  debeCambiarPassword: boolean
  /** Lo que declaró al registrarse (RF-01.3), para la pantalla de aprobación. */
  cursoSolicitado?: string
  materiaSolicitada?: string
  rolSolicitado?: RolSolicitado
  /**
   * Con qué cargo dijo registrarse. NO es `rol`: es una declaración y no
   * otorga permisos — quien dice ser administrador de sistema queda igual
   * DOCENTE/PENDIENTE, y el Admin lo promueve aparte después de aprobarlo.
   * Ausente en las cuentas anteriores a este campo y en los Admin creados por
   * otro Admin.
   */
  cargoSolicitado?: CargoSolicitado
  /** Cómo puede entrar esta cuenta. */
  tienePassword?: boolean
  vinculadaAGoogle?: boolean
}

/** GET /api/auth/config — lo que la pantalla de login necesita sin sesión. */
export type ConfigPublica = {
  /** Vacío = este despliegue no tiene ingreso con Google configurado. */
  googleClientId: string
  /**
   * false = no hay SMTP configurado, así que el sistema no puede mandar el
   * código y el login no muestra "olvidé mi contraseña".
   */
  recuperacionPorEmail?: boolean
  /** Desde qué dirección salen los avisos. */
  remitenteDeCorreo?: string
}

/** POST /api/auth/password/olvide — paso 1 de la recuperación. */
export type OlvidePasswordRequest = {
  email: string
}

/**
 * POST /api/auth/password/restablecer — paso 2. El email va de nuevo y no
 * queda en una sesión intermedia: así el paso 2 funciona aunque la persona
 * haya pedido el código en la computadora de la escuela y lo lea en el
 * celular.
 */
export type RestablecerPasswordRequest = {
  email: string
  codigo: string
  passwordNueva: string
}

export type GoogleLoginRequest = {
  /** El ID token que Google le entrega al navegador. */
  credential: string
}

export type GoogleRegistroRequest = GoogleLoginRequest & {
  /** Vacíos, el backend usa los del token (given_name / family_name). */
  nombre?: string
  apellido?: string
  /** Obligatorios los dos, para los dos cargos (RF-01.3). */
  cargoSolicitado: CargoSolicitado
  rolSolicitado: RolSolicitado
  /** Solo tienen sentido para el cargo DOCENTE; el backend los descarta si no. */
  cursoSolicitado?: string
  materiaSolicitada?: string
}

/** Con qué rol se ofrece quien se registra. Obligatorio para los dos cargos. */
export type RolSolicitado = "TITULAR" | "SUPLENTE"

/**
 * Con qué cargo se registra. ADMIN_SISTEMA cubre al auxiliar informático, al
 * administrador de red y a los demás cargos docentes que administran el
 * laboratorio sin estar frente a alumnos.
 */
export type CargoSolicitado = "DOCENTE" | "ADMIN_SISTEMA"

export type LoginRequest = {
  email: string
  password: string
}

export type LoginResponse = {
  token?: string
  debeCambiarPassword: boolean
}

export type RegistroRequest = {
  nombre: string
  apellido: string
  email: string
  password: string
  /** Obligatorios los dos, para los dos cargos (RF-01.3). */
  cargoSolicitado: CargoSolicitado
  rolSolicitado: RolSolicitado
  /** Qué va a dictar. Solo para el cargo DOCENTE, y los dos opcionales. */
  cursoSolicitado?: string
  materiaSolicitada?: string
}

/** PATCH /api/auth/mi-perfil — cambiar el propio nombre y apellido. */
export type ActualizarMisDatosRequest = {
  nombre: string
  apellido: string
}

/**
 * El token viene en la respuesta porque el anterior lleva el nombre viejo en
 * los claims: hay que reemplazarlo para que deje de mentir.
 */
export type ActualizarMisDatosResponse = {
  usuario: Usuario
  token: string
}

export type CambiarPasswordRequest = {
  passwordActual: string
  passwordNueva: string
}

export type CambiarEstadoRequest = {
  estado: "APROBADA" | "RECHAZADA" | "BAJA"
}

export type CrearAdminRequest = {
  nombre: string
  apellido: string
  email: string
  password: string
}

export type ResetPasswordResponse = {
  passwordTemporal: string
}

export type ListarUsuariosResponse = {
  data: Usuario[]
  meta: PaginacionMeta
}
