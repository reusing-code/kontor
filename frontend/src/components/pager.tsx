import { useTranslation } from "react-i18next"
import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"
import { pageItems } from "@/lib/pagination"
import { Button } from "@/components/ui/button"

interface PagerProps {
  // 1-based
  page: number
  pageCount: number
  onChange: (page: number) => void
  // accessible name of the navigation, e.g. "Transaction pages"
  label: string
  disabled?: boolean
}

export function Pager({ page, pageCount, onChange, label, disabled = false }: PagerProps) {
  const { t } = useTranslation()
  if (pageCount <= 1) return null

  return (
    <nav className="flex flex-wrap items-center gap-1 text-sm" aria-label={label}>
      <Button
        type="button"
        variant="outline"
        size="icon-sm"
        aria-label={t("common.pagination.previousPage")}
        disabled={disabled || page <= 1}
        onClick={() => onChange(page - 1)}
      >
        <ChevronLeftIcon />
      </Button>
      {pageItems(page, pageCount).map((item, i) =>
        item === "ellipsis" ? (
          <span key={`ellipsis-${i}`} className="px-1 text-muted-foreground" aria-hidden="true">
            …
          </span>
        ) : (
          <Button
            key={item}
            type="button"
            variant={item === page ? "default" : "outline"}
            size="sm"
            className="min-w-8 px-2 tabular-nums"
            aria-label={t("common.pagination.page", { page: item })}
            aria-current={item === page ? "page" : undefined}
            disabled={disabled}
            onClick={() => item !== page && onChange(item)}
          >
            {item}
          </Button>
        ),
      )}
      <Button
        type="button"
        variant="outline"
        size="icon-sm"
        aria-label={t("common.pagination.nextPage")}
        disabled={disabled || page >= pageCount}
        onClick={() => onChange(page + 1)}
      >
        <ChevronRightIcon />
      </Button>
    </nav>
  )
}
