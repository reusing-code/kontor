import { useEffect, useState } from "react"
import { formatDate } from "@/lib/format"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Check, Loader2 } from "lucide-react"
import { usePaperlessSearch, useAttachPaperlessDocuments } from "@/hooks/use-paperless"
import type { PaperlessEntityType, PaperlessSearchResult } from "@/types/paperless"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Pager } from "@/components/pager"
import { pageCountOf } from "@/lib/pagination"
import { PaperlessThumbnail } from "@/components/paperless/paperless-thumbnail"
import { cn } from "@/lib/utils"

interface PaperlessSearchDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  entityType: PaperlessEntityType
  entityId: string
  entityUrl: string
  existingDocIds: number[]
}

export function PaperlessSearchDialog({
  open,
  onOpenChange,
  entityType,
  entityId,
  entityUrl,
  existingDocIds,
}: PaperlessSearchDialogProps) {
  const { t } = useTranslation()
  const [searchInput, setSearchInput] = useState("")
  const [query, setQuery] = useState("")
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Map<number, PaperlessSearchResult>>(new Map())
  const attach = useAttachPaperlessDocuments(entityType, entityId)

  // Only a changed search starts again at page 1.
  useEffect(() => {
    if (searchInput === query) return
    const handle = setTimeout(() => {
      setQuery(searchInput)
      setPage(1)
    }, 300)
    return () => clearTimeout(handle)
  }, [searchInput, query])

  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    if (open) {
      setSearchInput("")
      setQuery("")
      setPage(1)
      setSelected(new Map())
    }
  }

  const { data, isFetching, error } = usePaperlessSearch(query, page, open)
  const totalPages = data ? pageCountOf(data.count, data.pageSize) : 1

  function toggle(doc: PaperlessSearchResult) {
    if (existingDocIds.includes(doc.id)) return
    setSelected((prev) => {
      const next = new Map(prev)
      if (next.has(doc.id)) {
        next.delete(doc.id)
      } else {
        next.set(doc.id, doc)
      }
      return next
    })
  }

  function handleAttach() {
    const documents = Array.from(selected.values()).map((doc) => ({ id: doc.id, title: doc.title }))
    attach.mutate(
      { entityUrl, documents },
      {
        onSuccess: (res) => {
          toast.success(t("paperless.attached", { count: res.links.length }))
          for (const warning of res.warnings) {
            toast.warning(warning)
          }
          onOpenChange(false)
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("paperless.attachDocuments")}</DialogTitle>
          <DialogDescription>{t("paperless.searchDescription")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <Input
            placeholder={t("paperless.searchPlaceholder")}
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
          />

          <div className="max-h-96 overflow-y-auto space-y-1">
            {error && <p className="text-sm text-destructive py-2">{error.message}</p>}
            {!error && data && data.results.length === 0 && (
              <p className="text-sm text-muted-foreground py-2">{t("paperless.noResults")}</p>
            )}
            {!error && !data && isFetching && (
              <div className="flex justify-center py-6">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            )}
            {data?.results.map((doc) => {
              const alreadyLinked = existingDocIds.includes(doc.id)
              const isSelected = selected.has(doc.id)
              return (
                <button
                  key={doc.id}
                  type="button"
                  onClick={() => toggle(doc)}
                  disabled={alreadyLinked}
                  className={cn(
                    "w-full flex items-center gap-3 rounded-md border p-2 text-left transition-colors",
                    alreadyLinked && "opacity-50 cursor-not-allowed",
                    !alreadyLinked && "hover:bg-accent",
                    isSelected && "border-primary bg-accent",
                  )}
                >
                  <PaperlessThumbnail documentId={doc.id} className="h-14 w-11 shrink-0" />
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium truncate">{doc.title}</p>
                    <p className="text-xs text-muted-foreground">
                      {doc.created ? formatDate(doc.created) : ""}
                      {alreadyLinked && ` · ${t("paperless.alreadyLinked")}`}
                    </p>
                    {doc.snippet && <p className="text-xs text-muted-foreground line-clamp-2">{doc.snippet}</p>}
                  </div>
                  <div
                    className={cn(
                      "flex h-5 w-5 shrink-0 items-center justify-center rounded border",
                      isSelected ? "bg-primary border-primary text-primary-foreground" : "border-input",
                    )}
                  >
                    {isSelected && <Check className="h-3.5 w-3.5" />}
                  </div>
                </button>
              )
            })}
          </div>

          {data && (
            <div className="flex justify-center">
              <Pager page={page} pageCount={totalPages} label={t("paperless.pagination")} disabled={isFetching} onChange={setPage} />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button onClick={handleAttach} disabled={selected.size === 0 || attach.isPending}>
            {t("paperless.attachSelected", { count: selected.size })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
