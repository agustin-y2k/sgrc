// Guarda el JWT en localStorage — el backend no expone cookie httpOnly ni
// refresh token (ver internal/auth: no hay endpoint de refresh implementado
// pese a que docs/09-seguridad-rbac.md §1 lo describe), así que hoy es la
// única opción viable para persistir la sesión entre reloads.
const TOKEN_KEY = "sgrc_token"

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}
