import { z } from "zod";

const DEFAULT_PAGE_SIZE = 20;
const MAX_PAGE_SIZE = 100;

/** Shared cursor-pagination request params for list endpoints. */
export const paginationParamsSchema = z.object({
  pageSize: z.coerce
    .number()
    .int()
    .positive()
    .max(MAX_PAGE_SIZE)
    .default(DEFAULT_PAGE_SIZE),
  cursor: z.string().optional()
});

export type PaginationParams = z.infer<typeof paginationParamsSchema>;
