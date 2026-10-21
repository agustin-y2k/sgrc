import { Button } from "@/components/ui/button"

/** Recargar vuelve a descargar también los módulos rechazados por React.lazy. */
export function ErrorDePantalla() {
  return (
    <div className="mx-auto grid max-w-lg gap-4 p-4 py-12">
      <h1 className="text-xl font-semibold">No pudimos abrir esta pantalla</h1>
      <p className="text-muted-foreground">
        Puede haber fallado la conexión o haberse actualizado el sistema. Recargá la
        página para volver a intentarlo.
      </p>
      <div className="flex flex-wrap gap-3">
        <Button type="button" className="h-11" onClick={() => window.location.reload()}>
          Recargar la página
        </Button>
        <Button asChild variant="outline" className="h-11">
          <a href="/">Volver al inicio</a>
        </Button>
      </div>
    </div>
  )
}
