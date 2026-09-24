/**
 * Shrinks a photo in the browser before it is sent: a phone camera's 4000 px
 * image is 3–6 MB, over the gateway's 2 MB limit, and 800 px is plenty for a
 * file and an ID card.
 */

const MAX_SIDE_PX = 800;
const JPEG_QUALITY = 0.85;

/** The largest size with the same proportions that fits in max×max. Never enlarges. */
export function fitWithin(width: number, height: number, max: number): { width: number; height: number } {
  const scale = Math.min(1, max / Math.max(width, height));
  return { width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)) };
}

/** Draws the image (a file or a camera frame) into a canvas and returns it as JPEG. */
export async function resizeToJpeg(source: Blob | HTMLVideoElement): Promise<Blob> {
  const image = source instanceof Blob ? await createImageBitmap(source) : source;
  const width = image instanceof HTMLVideoElement ? image.videoWidth : image.width;
  const height = image instanceof HTMLVideoElement ? image.videoHeight : image.height;
  const size = fitWithin(width, height, MAX_SIDE_PX);
  const canvas = document.createElement("canvas");
  canvas.width = size.width;
  canvas.height = size.height;
  const context = canvas.getContext("2d");
  if (context === null) {
    throw new Error("canvas unavailable");
  }
  context.drawImage(image, 0, 0, size.width, size.height);
  if (image instanceof ImageBitmap) {
    image.close();
  }
  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((blob) => (blob === null ? reject(new Error("could not encode the photo")) : resolve(blob)), "image/jpeg", JPEG_QUALITY);
  });
}
