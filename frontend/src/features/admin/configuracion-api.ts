import { apiFetch } from "@/lib/api-client"
export type ConfiguracionOperacion = {
  retiroGraciaMinutos: number
  recordatorioAntelacionMinutos: number
  cierreGraciaMinutos: number
  cierreHoraSinJornada: number
  licenciasHoraAviso: number
  version: number
}
export const consultarConfiguracion = () =>
  apiFetch<ConfiguracionOperacion>("/api/configuracion/operacion")
export const guardarConfiguracion = (datos: ConfiguracionOperacion) =>
  apiFetch<ConfiguracionOperacion>("/api/configuracion/operacion", {
    method: "PUT",
    body: datos,
  })
