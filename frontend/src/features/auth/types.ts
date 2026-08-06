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
  /**
   * Cómo puede entrar esta cuenta. Una creada con Google no tiene
   * contraseña (`tienePassword: false`), y ofrecerle "cambiar contraseña"
   * sería mandarla a un 409.
   *
   * Opcionales en el tipo porque las respuestas guardadas de antes de la
   * migración 008 no las traen.
   */
  tienePassword?: boolean
  vinculadaAGoogle?: boolean
}

/** GET /api/auth/config — lo que la pantalla de login necesita sin sesión. */
export type ConfigPublica = {
  /** Vacío = este despliegue no tiene ingreso con Google configurado. */
  googleClientId: string
  /**
   * false = no hay SMTP configurado, así que el sistema no puede mandar el
   * código y el login no muestra "olvidé mi contraseña". Opcional en el
   * tipo por la misma razón que `tienePassword`: una respuesta cacheada de
   * antes de esta versión no lo trae.
   */
  recuperacionPorEmail?: boolean
}

/** POST /api/auth/password/olvide — paso 1 de la recuperación. */
export type OlvidePasswordRequest = {
  email: string
}

/**
 * POST /api/auth/password/restablecer — paso 2.
 *
 * El email va de nuevo y no queda en una sesión intermedia: así el paso 2
 * funciona aunque la persona haya pedido el código en la computadora de la
 * escuela y lo lea en el celular.
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
  cursoSolicitado?: string
  materiaSolicitada?: string
}

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
  /**
   * Qué va a dictar. Opcionales: son una declaración de intención para que
   * el Admin sepa a qué asignarlo al aprobarlo (y si tiene que crear el
   * curso o la materia antes), no una referencia a nada que ya exista.
   */
  cursoSolicitado?: string
  materiaSolicitada?: string
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
