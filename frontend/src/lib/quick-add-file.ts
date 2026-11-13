import { quickAddMaxFileBytes } from "@/types/quick-add"

// Codes match the backend's quick-add error codes (quickAdd.errors.*).
export class QuickAddFileError extends Error {
  code: "file_too_large" | "unsupported_file"

  constructor(code: QuickAddFileError["code"]) {
    super(code)
    this.name = "QuickAddFileError"
    this.code = code
  }
}

const maxImageSide = 2000
const jpegQuality = 0.85
// A JPEG below this that needs no scaling is sent as it is.
const keepJpegBytes = 1024 * 1024

function isPdf(file: File): boolean {
  return file.type === "application/pdf" || file.name.toLowerCase().endsWith(".pdf")
}

// prepareQuickAddFile checks a picked file and shrinks images to a JPEG of at
// most 2000 px on the longest side, which keeps the upload and the vision
// model's input small. An image the browser cannot decode (e.g. HEIC outside
// Safari) is rejected.
export async function prepareQuickAddFile(file: File, allowImages: boolean): Promise<File> {
  if (isPdf(file)) {
    if (file.size > quickAddMaxFileBytes) throw new QuickAddFileError("file_too_large")
    return file
  }
  if (!allowImages) throw new QuickAddFileError("unsupported_file")

  let bitmap: ImageBitmap
  try {
    bitmap = await createImageBitmap(file, { imageOrientation: "from-image" })
  } catch {
    throw new QuickAddFileError("unsupported_file")
  }
  try {
    const scale = Math.min(1, maxImageSide / Math.max(bitmap.width, bitmap.height))
    if (scale === 1 && file.type === "image/jpeg" && file.size <= keepJpegBytes) return file

    const canvas = document.createElement("canvas")
    canvas.width = Math.max(1, Math.round(bitmap.width * scale))
    canvas.height = Math.max(1, Math.round(bitmap.height * scale))
    const ctx = canvas.getContext("2d")
    if (!ctx) throw new QuickAddFileError("unsupported_file")
    // JPEG has no alpha channel; transparent areas would turn black.
    ctx.fillStyle = "#fff"
    ctx.fillRect(0, 0, canvas.width, canvas.height)
    ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height)

    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/jpeg", jpegQuality))
    if (!blob) throw new QuickAddFileError("unsupported_file")
    if (blob.size > quickAddMaxFileBytes) throw new QuickAddFileError("file_too_large")
    return new File([blob], file.name.replace(/\.[^.]*$/, "") + ".jpg", { type: "image/jpeg" })
  } finally {
    bitmap.close()
  }
}
