import { useEffect, useState } from "react"

import * as authApi from "@/features/auth/api"

/**
 * La línea de "si no te llega, fijate en spam" (RF-05.8).
 *
 * Va SOLO en las pantallas donde alguien está esperando un correo concreto:
 * la recuperación de contraseña, la confirmación de registro y la de
 * notificaciones. Repetirla en todas las pantallas la vuelve invisible en dos
 * días y, peor, hace ver al sistema como si no funcionara.
 *
 * Lo que de verdad resuelve el problema no es avisar dónde buscar, sino que
 * la persona marque el remitente como conocido una vez. Por eso el texto
 * nombra la dirección exacta cuando el servidor la publica, en vez de decir
 * "revisá spam" en abstracto.
 *
 * Si este despliegue no manda correos, la dirección viene vacía y no se
 * dibuja nada: no tiene sentido hablar de correos que no existen.
 */
export function AvisoDeSpam({ children }: { children?: React.ReactNode }) {
  const remitente = useRemitenteDeCorreo()
  if (!remitente) return null

  return (
    <p className="text-muted-foreground text-xs">
      {children ?? "Si no te llega, fijate en la carpeta de spam."} Los correos salen de{" "}
      <span className="font-medium">{remitente}</span>: agregalo a tus contactos y no te
      va a volver a pasar.
    </p>
  )
}

/**
 * La dirección desde la que salen los avisos, o vacío si el despliegue no
 * tiene correo configurado.
 *
 * Un fallo de la consulta se traga a propósito: esto es una ayuda al margen
 * de la pantalla, y un error acá no puede tapar el formulario que la persona
 * vino a usar.
 */
function useRemitenteDeCorreo(): string {
  const [remitente, setRemitente] = useState("")

  useEffect(() => {
    let cancelado = false
    authApi
      .configPublica()
      .then(({ remitenteDeCorreo }) => {
        if (!cancelado) setRemitente(remitenteDeCorreo ?? "")
      })
      .catch(() => {})
    return () => {
      cancelado = true
    }
  }, [])

  return remitente
}
