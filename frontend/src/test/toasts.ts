// Titles of the error toasts currently shown by the Toaster of renderWithProviders.
export function errorToasts(): string[] {
  return [...document.querySelectorAll('[data-sonner-toast][data-type="error"] [data-title]')].map((el) => el.textContent ?? "")
}
