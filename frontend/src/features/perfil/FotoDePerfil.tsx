import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRef, useState } from "react"

import { Avatar } from "@/components/Avatar"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import * as perfilApi from "@/features/perfil/api"
import { getErrorMessage } from "@/lib/api-client"

/**
 * Cuánto mide la foto que se guarda, de lado.
 *
 * El recorte y el achicado los hace el navegador ANTES de subir: la cámara
 * de un teléfono saca fotos de varios megabytes, y mandarlas enteras por la
 * conexión de una escuela para que el servidor las tire es hacer esperar a
 * la persona por nada. 256 píxeles alcanzan de sobra para un redondel que
 * en pantalla mide 28.
 */
const LADO = 256

/**
 * Achica y recorta la imagen a un cuadrado de LADO×LADO, en WEBP.
 *
 * El recorte es al centro y por el lado más corto, que es lo que espera
 * cualquiera que sube una foto de frente: una imagen apaisada pierde los
 * bordes, no la cara.
 *
 * Devuelve null si el archivo no se puede leer como imagen — ahí el mensaje
 * lo da esta pantalla, sin viajar al servidor para que lo rechace.
 */
async function aCuadradoChico(archivo: File): Promise<Blob | null> {
  const url = URL.createObjectURL(archivo)
  try {
    const img = await new Promise<HTMLImageElement | null>((resolve) => {
      const el = new Image()
      el.onload = () => resolve(el)
      el.onerror = () => resolve(null)
      el.src = url
    })
    if (!img) return null

    const lienzo = document.createElement("canvas")
    lienzo.width = LADO
    lienzo.height = LADO
    const ctx = lienzo.getContext("2d")
    if (!ctx) return null

    const lado = Math.min(img.width, img.height)
    ctx.drawImage(img, (img.width - lado) / 2, (img.height - lado) / 2, lado, lado, 0, 0, LADO, LADO)

    return await new Promise<Blob | null>((resolve) =>
      lienzo.toBlob((b) => resolve(b), "image/webp", 0.85)
    )
  } finally {
    URL.revokeObjectURL(url)
  }
}

/**
 * El redondel del perfil, con los botones para cambiar o sacar la foto.
 *
 * El botón dice "Cambiar la foto" y abre el explorador de archivos: el
 * `<input type="file">` real queda escondido porque su aspecto no se puede
 * cambiar y su texto por defecto ("Sin archivos seleccionados") no dice nada
 * de lo que va a pasar.
 */
export function FotoDePerfil({
  usuarioId,
  nombre,
  apellido,
}: {
  usuarioId: string
  nombre: string
  apellido: string
}) {
  const qc = useQueryClient()
  const inputRef = useRef<HTMLInputElement>(null)
  const [error, setError] = useState("")
  // Cambia con cada foto nueva para saltear la caché del navegador.
  const [version, setVersion] = useState<string | undefined>(undefined)
  const [sinFoto, setSinFoto] = useState(false)

  const subir = useMutation({
    mutationFn: async (archivo: File) => {
      const chica = await aCuadradoChico(archivo)
      if (!chica) throw new Error("Ese archivo no es una imagen que se pueda mostrar.")
      return perfilApi.subirMiFoto(chica)
    },
    onSuccess: (r) => {
      setError("")
      setSinFoto(false)
      setVersion(r.actualizadaEn)
      qc.invalidateQueries({ queryKey: ["perfil", "foto"] })
    },
    onError: (e) => setError(getErrorMessage(e)),
  })

  const borrar = useMutation({
    mutationFn: perfilApi.eliminarMiFoto,
    onSuccess: () => {
      setError("")
      setSinFoto(true)
      qc.invalidateQueries({ queryKey: ["perfil", "foto"] })
    },
    onError: (e) => setError(getErrorMessage(e)),
  })

  const trabajando = subir.isPending || borrar.isPending

  return (
    <div className="flex flex-wrap items-center gap-4">
      <Avatar
        usuarioId={usuarioId}
        nombre={nombre}
        apellido={apellido}
        tieneFoto={!sinFoto}
        version={version}
        className="size-20 text-2xl"
      />

      <div className="grid gap-2">
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-11 px-4 sm:h-9"
            disabled={trabajando}
            onClick={() => inputRef.current?.click()}
          >
            {trabajando ? "Guardando…" : "Cambiar la foto"}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-11 px-4 sm:h-9"
            disabled={trabajando}
            onClick={() => borrar.mutate()}
          >
            Sacar la foto
          </Button>
        </div>
        <p className="text-muted-foreground text-sm">
          Es opcional. Si no ponés ninguna, se muestran tus iniciales.
        </p>
      </div>

      <input
        ref={inputRef}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={(e) => {
          const archivo = e.target.files?.[0]
          // El valor se limpia para que elegir DOS VECES el mismo archivo
          // vuelva a disparar el cambio: si no, el segundo intento —después
          // de un error— no hacía nada y parecía que el botón se había roto.
          e.target.value = ""
          if (archivo) subir.mutate(archivo)
        }}
      />

      {error && (
        <Alert variant="destructive" className="w-full">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
    </div>
  )
}
