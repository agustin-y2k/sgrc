import { Eye, EyeOff } from "lucide-react"
import * as React from "react"

import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

/**
 * Un campo de contraseña con el ojo para verla mientras se escribe.
 *
 * No es una comodidad: escribir a ciegas es la forma más común de no poder
 * entrar, y falla en silencio —la persona ve puntitos, escribe lo que cree
 * que escribió, y el sistema solo le contesta "email o contraseña
 * incorrectos"—. Quien tipea con dos dedos, o desde el teclado de un
 * teléfono, no tiene manera de saber si se equivocó al escribir o si
 * realmente olvidó la contraseña. Son dos problemas con soluciones muy
 * distintas, y sin poder ver el texto se confunden.
 *
 * Arranca oculta: mostrarla es una decisión de quien la escribe, que sabe si
 * tiene a alguien mirando por encima del hombro.
 *
 * Vive en components/ui y no en features/auth porque hay campos de
 * contraseña en seis pantallas de tres módulos distintos (ingreso, registro,
 * recuperación, cambio propio, y el alta de un Admin). Cuando estaban
 * sueltos, arreglar uno no arreglaba los otros.
 */
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
        // botones de la pantalla de inicio del docente. Queda más alto que
        // el campo y se centra sobre él.
        className="absolute top-1/2 right-0 flex size-11 -translate-y-1/2 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
        // aria-pressed: para un lector de pantalla esto es un interruptor con
        // un estado, no dos botones distintos.
        aria-pressed={visible}
        // El nombre NO menciona "contraseña" a propósito. El campo de al lado
        // ya se llama así, y con la palabra repetida quedaban dos elementos
        // con el mismo nombre accesible pegados: quien navega por voz pide
        // "contraseña" y el sistema no sabe cuál de los dos quiere. (Los
        // tests lo detectaron antes que nadie: el selector por etiqueta
        // encontraba dos.)
        aria-label={visible ? "Ocultar lo que escribí" : "Mostrar lo que escribí"}
        // El botón no entra en el recorrido del tabulador: quien navega con
        // teclado va del campo al botón de enviar, que es lo que quiere
        // hacer. El ojo se toca con el mouse o con el dedo.
        tabIndex={-1}
        onClick={() => setVisible((v) => !v)}
      >
        <Icono className="size-4" aria-hidden="true" />
      </button>
    </div>
  )
}

export { InputPassword }
