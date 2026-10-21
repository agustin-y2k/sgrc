import { queryClient } from "@/lib/query-client"

it("no repite una escritura cuando se pierde su respuesta después del commit", async () => {
  let confirmadas = 0
  const mutacion = queryClient.getMutationCache().build(queryClient, {
    retryDelay: 0,
    mutationFn: async () => {
      confirmadas++
      if (confirmadas === 1)
        throw new TypeError("se perdió la respuesta después del commit")
      return "segunda escritura"
    },
  })
  await expect(mutacion.execute(undefined)).rejects.toThrow("se perdió la respuesta")
  expect(confirmadas).toBe(1)
  queryClient.clear()
})
