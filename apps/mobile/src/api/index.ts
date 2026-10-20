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
  getStatement,
  getPaymentInfo,
  statementPdfPath,
  receiptPdfPath,
  getGrades,
  markNotificationRead,
  type FamilyStudent,
  type FamilyEnrollment,
  type FamilyProfile,
  type FamilyMovement,
  type FamilyAppliedPart,
  type FamilyStatement,
  type FamilyStatementTotals,
  type MovementKind,
  type FamilyGradeEntry,
  type FamilyCard,
  type FamilyPaymentInfo,
  getCard,
  cardPhotoPath,
} from "./family";
export {
  getAttendanceMonth,
  type AttendanceMonth,
  type AttendanceDay,
  type AttendanceClass,
  type AttendanceStatus,
  type AttendanceMode,
  type AttendanceSummary,
} from "./attendance";
export { registerDevice, unregisterDevice, type DevicePlatform } from "./devices";
