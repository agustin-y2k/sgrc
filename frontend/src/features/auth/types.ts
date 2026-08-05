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
