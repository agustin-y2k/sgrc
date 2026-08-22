import { useEffect, useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { descargarFoto } from "@/features/perfil/api"

/**
 * La foto de alguien como una URL que un <img> pueda mostrar.
 *
 * Devuelve null mientras no haya foto que mostrar, que incluye el caso más
 * común: esa persona nunca subió una.
 *
 * El Blob se cachea en react-query y la URL se arma en cada componente. Al
 * revés —cachear la URL— un componente que se desmonta la revocaría y le
 * rompería la imagen a los demás que la estén mostrando.
 */
export function useFotoDePerfil(
  usuarioId: string,
  version?: string,
  habilitado = true
): string | null {
  const { data: blob } = useQuery({
    // La versión entra en la clave para que cambiar la foto propia no siga
    // mostrando la vieja desde la caché.
    queryKey: ["foto-de-perfil", usuarioId, version ?? ""],
    queryFn: () => descargarFoto(usuarioId),
    enabled: habilitado && usuarioId !== "",
    // Sin reintentos: el 404 de "no subió foto" es el caso normal, y
    // reintentarlo son tres pedidos fallidos por cada avatar de la pantalla.
    retry: false,
    staleTime: 5 * 60 * 1000,
  })

  const [url, setUrl] = useState<string | null>(null)

  useEffect(() => {
    if (!blob) {
      setUrl(null)
      return
    }
    const nueva = URL.createObjectURL(blob)
    setUrl(nueva)
    return () => URL.revokeObjectURL(nueva)
  }, [blob])

  return url
}
