import { Eye, EyeOff } from "lucide-react"
import * as React from "react"

import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

/** Un campo de contraseña con el ojo para verla mientras se escribe. */
function InputPassword({
  className,
  ...props
}: Omit<React.ComponentProps<"input">, "type">) {
  const [visible, setVisible] = React.useState(false)
  const Icono = visible ? EyeOff : Eye

  return (
    <div className="relative">
      <Input
        type={visible ? "text" : "password"}
        // Espacio a la derecha para que el texto no pase por debajo del
        // botón: sin esto, una contraseña larga se lee cortada justo cuando
        // la persona la destapó para leerla.
        className={cn("pr-11", className)}
        {...props}
      />
      <button
        type="button"
        // 44px de lado: es un blanco táctil (WCAG 2.5.5), igual que los
        // botones de la pantalla de inicio del docente.
        className="absolute top-1/2 right-0 flex size-11 -translate-y-1/2 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
        // aria-pressed: para un lector de pantalla esto es un interruptor con
        // un estado, no dos botones distintos.
        aria-pressed={visible}
        // El nombre NO menciona "contraseña" a propósito.
        aria-label={visible ? "Ocultar lo que escribí" : "Mostrar lo que escribí"}
        // El botón no entra en el recorrido del tabulador: quien navega con
        // teclado va del campo al botón de enviar, que es lo que quiere
        // hacer.
        tabIndex={-1}
        onClick={() => setVisible((v) => !v)}
      >
        <Icono className="size-4" aria-hidden="true" />
      </button>
    </div>
  )
}

export { InputPassword }
