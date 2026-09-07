/**
 * Public surface of the mobile API client. Screens and state import from here, not
 * from the individual modules, so the internal file layout can change freely.
 */
export { API_BASE_URL, apiUrl } from "./config";
export { ApiError } from "./errors";
export { login, logout, refresh, type LoginResult } from "./auth";
export { signIn } from "./signIn";
export { signOut, endSession } from "./endSession";
export { providerLoginEnabled } from "./providerFlag";
export { apiFetch } from "./client";
export { onSessionLost, emitSessionLost } from "./session";
export { getAccessToken, getRefreshToken, loadTokens, clearTokens, saveTokens, type TokenPair } from "./tokenStore";
export { listFamilyNotifications, type FamilyNotification } from "./notifications";
export {
  listChildren,
  getProfile,
  getAccount,
  getGrades,
  getAttendance,
  markNotificationRead,
  type FamilyStudent,
  type FamilyEnrollment,
  type FamilyProfile,
  type FamilyCharge,
  type FamilyPayment,
  type FamilyAccount,
  type FamilyGradeEntry,
  type FamilyAttendanceRecord,
  type FamilyAttendanceStatus,
} from "./family";
