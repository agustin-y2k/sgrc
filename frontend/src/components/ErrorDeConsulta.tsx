import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

export function ErrorDeConsulta({
  mensaje,
  reintentando,
  onReintentar,
}: {
  mensaje: string
  reintentando: boolean
  onReintentar: () => void
}) {
  return (
    <Alert variant="destructive">
      <AlertDescription className="grid gap-2">
        <span>{mensaje} Reintentá para consultar los datos.</span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-11 justify-self-start px-4 sm:h-9"
          disabled={reintentando}
          onClick={onReintentar}
        >
          {reintentando ? "Reintentando…" : "Reintentar"}
        </Button>
      </AlertDescription>
    </Alert>
  )
}
