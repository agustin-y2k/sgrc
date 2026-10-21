import type { DetalleLegible } from "./detalle"

export function DetalleDeAuditoria({ detalle }: { detalle: DetalleLegible }) {
  return (
    <div className="bg-muted/40 mt-2 grid gap-3 rounded-md p-3 text-sm">
      {detalle.cambios.length > 0 && (
        <div
          className="overflow-x-auto"
          tabIndex={0}
          role="region"
          aria-label="Cambios registrados"
        >
          <table className="w-full text-left">
            <caption className="sr-only">Cambios registrados</caption>
            <thead>
              <tr className="border-b">
                <th scope="col" className="p-2">
                  Dato
                </th>
                <th scope="col" className="p-2">
                  Antes
                </th>
                <th scope="col" className="p-2">
                  Después
                </th>
              </tr>
            </thead>
            <tbody>
              {detalle.cambios.map((cambio, index) => (
                <tr key={index} className="border-b last:border-0">
                  <th scope="row" className="p-2 align-top font-medium wrap-anywhere">
                    {cambio.campo}
                  </th>
                  <td className="p-2 align-top wrap-anywhere">
                    {cambio.antes ?? "Sin datos registrados"}
                  </td>
                  <td className="p-2 align-top wrap-anywhere">
                    {cambio.despues ?? "Sin datos registrados"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {detalle.datos.length > 0 && (
        <dl className="grid gap-2">
          {detalle.datos.map((dato, index) => (
            <div key={index} className="grid gap-1 sm:grid-cols-2">
              <dt className="font-medium wrap-anywhere">{dato.campo}</dt>
              <dd className="wrap-anywhere">{dato.valor}</dd>
            </div>
          ))}
        </dl>
      )}
      {detalle.otrosCampos.length > 0 && (
        <p className="wrap-anywhere">
          También se registraron cambios en: {detalle.otrosCampos.join(", ")}. Los valores
          anteriores y nuevos no están disponibles en este registro.
        </p>
      )}
      {detalle.seguridadActualizada && (
        <p>
          Se modificó información de seguridad de la cuenta. Las contraseñas y los códigos
          no se muestran.
        </p>
      )}
      {!detalle.cambios.length &&
        !detalle.datos.length &&
        !detalle.otrosCampos.length &&
        !detalle.seguridadActualizada && (
          <p>No hay información adicional disponible para esta acción.</p>
        )}
    </div>
  )
}
