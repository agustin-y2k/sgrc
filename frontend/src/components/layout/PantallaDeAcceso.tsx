import type { ReactNode } from "react"

import { PieDeAutoria } from "@/components/PieDeAutoria"

/**
 * El marco de las dos pantallas de afuera: iniciar sesión y registrarse.
 *
 * Son las únicas que ve alguien que todavía no entró —no tienen la barra de
 * navegación— y hasta ahora eran una tarjeta blanca flotando en gris. Acá
 * lleva el nombre del sistema y de la escuela, que es lo que le dice a un
 * docente que llegó a donde quería y no a una página cualquiera.
 *
 * `min-h-svh` y no `min-h-screen`: en un teléfono, `100vh` cuenta la barra
 * del navegador, así que el contenido quedaba empujado y el botón de
 * ingresar caía justo debajo del borde visible.
 */
export function PantallaDeAcceso({
  children,
  ancho = "max-w-sm",
}: {
  children: ReactNode
  /** El registro pide más datos y necesita más lugar que el login. */
  ancho?: string
}) {
  return (
    <div className="from-background to-accent/40 flex min-h-svh flex-col items-center justify-center bg-gradient-to-b px-4 py-8">
      <div className={`w-full ${ancho}`}>
        <div className="mb-6 flex flex-col items-center gap-2 text-center">
          <span
            aria-hidden="true"
            className="bg-primary text-primary-foreground grid size-11 place-items-center rounded-2xl text-base font-bold"
          >
            SG
          </span>
          <div>
            <p className="text-xl font-semibold tracking-tight">SGRC</p>
            <p className="text-muted-foreground text-sm text-balance">
              Sistema de Gestión y Reserva de Computadoras Educativas
            </p>
          </div>
        </div>
        {children}
        <PieDeAutoria className="mt-6" />
      </div>
    </div>
  )
}
