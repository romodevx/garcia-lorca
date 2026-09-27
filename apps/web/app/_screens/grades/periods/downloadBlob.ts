/** Long enough for the browser to start the download before the URL goes away. */
const REVOKE_DELAY_MS = 1000;

/** Hands a fetched file to the browser as a download with the given name. */
export function downloadBlob(blob: Blob, fileName: string): void {
  const href = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = href;
  anchor.download = fileName;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(href), REVOKE_DELAY_MS);
}
