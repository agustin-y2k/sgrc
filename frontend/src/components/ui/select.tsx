import * as React from "react"

import { cn } from "@/lib/utils"

/**
 * El desplegable nativo, con la misma piel que `<Input>`.
 *
 * `bg-control` no es decorativo: el navegador pinta la lista desplegada
 * copiando el `background-color` del campo, y con un color translúcido sale
 * blanca sobre blanco en tema oscuro. La explicación está en index.css,
 * junto a la regla de `option`.
 */
function Select({ className, ...props }: React.ComponentProps<"select">) {
  return (
    <select
      data-slot="select"
      className={cn(
        "border-input bg-control h-8 w-full min-w-0 rounded-lg border px-2.5 text-base transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40",
        className
      )}
      {...props}
    />
  )
}

export { Select }
