// Guarda el JWT en localStorage — el backend no expone cookie httpOnly ni
// refresh token, así que hoy es la
// única opción viable para persistir la sesión entre reloads.
export const TOKEN_KEY = "sgrc_token"

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}
