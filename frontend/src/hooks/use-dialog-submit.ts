// useDialogSubmit returns a form submit handler that closes the dialog only
// once onSubmit resolves. A rejection keeps the dialog open with the input;
// the global mutation error toast reports it.
export function useDialogSubmit<T>(onSubmit: (data: T) => Promise<unknown>, onOpenChange: (open: boolean) => void) {
  return async (data: T) => {
    try {
      await onSubmit(data)
    } catch {
      return
    }
    onOpenChange(false)
  }
}
