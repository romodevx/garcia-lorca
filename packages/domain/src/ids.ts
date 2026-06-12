import { z } from "zod";

/**
 * Branded identifier schemas. Branding prevents accidentally passing a raw
 * string (or the wrong id type) where a specific id is expected.
 */
export const studentIdSchema = z.uuid().brand<"StudentId">();
export type StudentId = z.infer<typeof studentIdSchema>;

export const userIdSchema = z.uuid().brand<"UserId">();
export type UserId = z.infer<typeof userIdSchema>;
