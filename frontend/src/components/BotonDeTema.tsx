import { useEffect, useState } from "react"
import { Moon, Sun } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  aplicarTema,
  guardarTema,
  temaEfectivo,
  temaElegido,
  temaDelSistema,
  type Tema,
} from "@/lib/tema"

/**
 * El interruptor de claro/oscuro de la barra.
 *
 * Muestra el ícono de a dónde va, no el de dónde está: con un sol en modo
 * claro, la mitad de la gente lo lee como "estás en claro" y la otra mitad
 * como "cambiar a claro". Diciendo a dónde lleva, la etiqueta y el ícono
 * dicen lo mismo.
 */
export function BotonDeTema() {
  const [tema, setTema] = useState<Tema>(() => temaEfectivo())

  // Mientras nadie haya elegido a mano, el tema sigue al sistema en vivo:
  // si el usuario tiene el cambio automático al atardecer, la pestaña que
  // dejó abierta acompaña sin recargar.
  useEffect(() => {
    if (temaElegido() !== null) return
    if (typeof matchMedia !== "function") return

    const consulta = matchMedia("(prefers-color-scheme: dark)")
    const alCambiar = () => {
      const nuevo = temaDelSistema()
      setTema(nuevo)
      aplicarTema(nuevo)
    }
    consulta.addEventListener("change", alCambiar)
    return () => consulta.removeEventListener("change", alCambiar)
  }, [])

  function alternar() {
    const nuevo: Tema = tema === "oscuro" ? "claro" : "oscuro"
    setTema(nuevo)
    aplicarTema(nuevo)
    guardarTema(nuevo)
  }

  const vaAOscuro = tema === "claro"

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={alternar}
      // El texto es la etiqueta accesible: el botón es solo un ícono, y sin
      // esto un lector de pantalla anuncia "botón" a secas.
      aria-label={vaAOscuro ? "Cambiar a modo oscuro" : "Cambiar a modo claro"}
      title={vaAOscuro ? "Modo oscuro" : "Modo claro"}
    >
      {vaAOscuro ? (
        <Moon aria-hidden="true" className="size-4" />
      ) : (
        <Sun aria-hidden="true" className="size-4" />
      )}
    </Button>
  )
}
