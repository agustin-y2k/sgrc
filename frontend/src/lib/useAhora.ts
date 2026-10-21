import { useEffect, useState } from "react"

/** Actualiza horarios y fecha civil aun cuando los datos de la API no cambien. */
export function useAhora(): Date {
  const [ahora, setAhora] = useState(() => new Date())
  useEffect(() => {
    const intervalo = window.setInterval(() => setAhora(new Date()), 60_000)
    return () => window.clearInterval(intervalo)
  }, [])
  return ahora
}
