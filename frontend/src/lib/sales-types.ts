import { z } from 'zod'

export const salesClassSchema = z.enum(['international', 'domestic', 'miscellaneous'])
export type SalesClass = z.infer<typeof salesClassSchema>
export interface SalesType { id: string; code: string; name: string; salesClass: SalesClass }
export const salesTypeRecordsSchema = z.object({ records: z.array(z.object({ id: z.string(), values: z.record(z.string(), z.string()) })) })
