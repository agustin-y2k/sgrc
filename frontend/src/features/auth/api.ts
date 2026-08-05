import { apiFetch } from "@/lib/api-client"
import type {
  CambiarEstadoRequest,
  CambiarPasswordRequest,
  Estado,
  ListarUsuariosResponse,
  LoginRequest,
  LoginResponse,
  RegistroRequest,
  ResetPasswordResponse,
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

export function me() {
  return apiFetch<Usuario>("/api/auth/me")
}

/**
 * Devuelve un token nuevo: el actual lleva `debeCambiarPassword: true`
 * congelado en los claims, y el backend responde 403 a todo lo demás
 * mientras eso siga así (RF-01.6). Hay que reemplazarlo, no solo refrescar
 * el usuario.
 */
export function cambiarPassword(req: CambiarPasswordRequest) {
  return apiFetch<{ token: string }>("/api/auth/cambiar-password", {
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
// Estaban duplicados acá sin que nadie los llamara: dos copias del mismo
// endpoint son dos lugares donde mantener el contrato y uno donde olvidarse.
