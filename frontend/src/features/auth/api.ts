import { apiFetch } from "@/lib/api-client"
import type {
  CambiarEstadoRequest,
  CambiarPasswordRequest,
  ConfigPublica,
  Estado,
  GoogleLoginRequest,
  GoogleRegistroRequest,
  ListarUsuariosResponse,
  LoginRequest,
  LoginResponse,
  OlvidePasswordRequest,
  RegistroRequest,
  ResetPasswordResponse,
  RestablecerPasswordRequest,
  Rol,
  Usuario,
} from "@/features/auth/types"

export function login(req: LoginRequest) {
  return apiFetch<LoginResponse>("/api/auth/login", { method: "POST", body: req })
}

// 201 sin body (c.SendStatus, ver internal/auth/interfaces/http/handlers.go
// Registrar) — la cuenta queda PENDIENTE, no hay nada que devolver todavía.
export function registrar(req: RegistroRequest) {
  return apiFetch<void>("/api/auth/registro", { method: "POST", body: req })
}

/** Ingreso con una cuenta de Google ya registrada. */
export function loginConGoogle(credential: string, recordarme = false) {
  return apiFetch<LoginResponse>("/api/auth/google", {
    method: "POST",
    body: { credential, recordarme } satisfies GoogleLoginRequest,
  })
}

// 201 sin body, igual que el registro con contraseña: la cuenta queda
// PENDIENTE hasta que un Admin la apruebe (RF-01.3).
export function registrarConGoogle(req: GoogleRegistroRequest) {
  return apiFetch<void>("/api/auth/google/registro", { method: "POST", body: req })
}

/** Configuración pública, sin sesión. */
export function configPublica() {
  return apiFetch<ConfigPublica>("/api/auth/config")
}

export function me() {
  return apiFetch<Usuario>("/api/auth/me")
}

/**
 * Devuelve un token nuevo: el actual lleva `debeCambiarPassword: true`
 * congelado en los claims, y el backend responde 403 a todo lo demás mientras
 * eso siga así (RF-01.6).
 */
export function cambiarPassword(req: CambiarPasswordRequest) {
  return apiFetch<{ token: string }>("/api/auth/cambiar-password", {
    method: "POST",
    body: req,
  })
}

/** Paso 1 de la recuperación: pedir que llegue un código al email. */
export function olvidePassword(req: OlvidePasswordRequest) {
  return apiFetch<{ mensaje: string }>("/api/auth/password/olvide", {
    method: "POST",
    body: req,
  })
}

/** Paso 2: cambiar la contraseña con el código. */
export function restablecerPassword(req: RestablecerPasswordRequest) {
  return apiFetch<void>("/api/auth/password/restablecer", {
    method: "POST",
    body: req,
  })
}

// Solo ADMIN — usado por feature/frontend-admin más adelante para el panel
// completo; acá lo usa AprobacionPage filtrando estado=PENDIENTE.
export function listarUsuarios(filtros?: { estado?: Estado; rol?: Rol }) {
  const params = new URLSearchParams()
  if (filtros?.estado) params.set("estado", filtros.estado)
  if (filtros?.rol) params.set("rol", filtros.rol)
  const query = params.toString()
  return apiFetch<ListarUsuariosResponse>(`/api/auth/usuarios${query ? `?${query}` : ""}`)
}

export function cambiarEstado(id: string, req: CambiarEstadoRequest) {
  return apiFetch<void>(`/api/auth/usuarios/${id}/estado`, {
    method: "PATCH",
    body: req,
  })
}

// Solo ADMIN — no tiene pantalla en esta slice (ver plan de
// feature/frontend-auth), pero queda tipado y listo para frontend-admin.
export function resetearPassword(id: string) {
  return apiFetch<ResetPasswordResponse>(`/api/auth/usuarios/${id}/reset-password`, {
    method: "POST",
  })
}

// eliminarDefinitivamente (RF-01.9) y crearAdmin (RF-01.4) viven en
// features/admin/api.ts, que es de donde los usa la pantalla de usuarios.
