import { useEffect, useState } from "react"

import * as authApi from "@/features/auth/api"

/** La línea de "si no te llega, fijate en spam" (RF-05.8). */
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
