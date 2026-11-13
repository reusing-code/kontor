import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Repeat } from "lucide-react"
import { ApiError } from "@/lib/api"
import { usePageTitle } from "@/hooks/use-page-title"
import { useModules } from "@/hooks/use-modules"
import {
  useLedgerAccounts,
  useLedgerCategories,
  useLedgerEmailAccounts,
  useLedgerEmailOrders,
  useLedgerImports,
  useLedgerReviewQueue,
  useRunLedgerRecurringDetection,
} from "@/modules/ledger/hooks/use-ledger"
import { LedgerAccountList } from "@/modules/ledger/components/ledger-account-list"
import { LedgerImportPanel } from "@/modules/ledger/components/ledger-import-panel"
import { LedgerImportsList } from "@/modules/ledger/components/ledger-imports-list"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

function DetectRecurringButton() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const runDetection = useRunLedgerRecurringDetection()

  function handleRun() {
    runDetection.mutate(undefined, {
      onSuccess: ({ created }) => {
        if (created === 0) {
          toast.info(t("ledger.recurring.none"))
          return
        }
        toast.success(t("ledger.recurring.created", { count: created }), {
          action: { label: t("ledger.recurring.openInbox"), onClick: () => void navigate({ to: "/inbox" }) },
        })
      },
      onError: (error) => {
        toast.error(error instanceof ApiError && error.status === 409 ? t("ledger.recurring.busy") : error.message)
      },
    })
  }

  return (
    <Button type="button" variant="outline" onClick={handleRun} disabled={runDetection.isPending}>
      <Repeat className="mr-2 h-4 w-4" />
      {runDetection.isPending ? t("ledger.recurring.running") : t("ledger.recurring.run")}
    </Button>
  )
}

export function LedgerIndexPage() {
  const { t } = useTranslation()
  usePageTitle(t("nav.ledger"), t("app.title"))
  const { isEnabled } = useModules()

  const { data: accounts = [] } = useLedgerAccounts()
  const { data: emailAccounts = [] } = useLedgerEmailAccounts()
  const { data: emailOrders = [] } = useLedgerEmailOrders()
  const { data: imports = [] } = useLedgerImports()
  const { data: categories = [] } = useLedgerCategories()
  const { data: reviewPage } = useLedgerReviewQueue(10)

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">{t("nav.ledger")}</h1>
          <p className="text-sm text-muted-foreground">{t("ledger.description")}</p>
        </div>
        {isEnabled("contracts") && <DetectRecurringButton />}
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-4">
            <CardTitle>{t("ledger.reviewQueue")}</CardTitle>
            <Button asChild variant="outline">
              <Link to="/ledger/review">{t("ledger.openReview")}</Link>
            </Button>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-semibold">{reviewPage?.items.length ?? 0}</div>
            <p className="text-sm text-muted-foreground">{t("ledger.transactionsNeedReview")}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-4">
            <CardTitle>{t("ledger.categories")}</CardTitle>
            <Button asChild variant="outline">
              <Link to="/ledger/categories">{t("ledger.manageCategories")}</Link>
            </Button>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-semibold">{categories.length}</div>
            <p className="text-sm text-muted-foreground">{t("ledger.categoryTreeDescription")}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-4">
            <CardTitle>{t("ledger.email.accounts")}</CardTitle>
            <Button asChild variant="outline">
              <Link to="/ledger/email-accounts">{t("ledger.email.manage")}</Link>
            </Button>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-semibold">{emailAccounts.length}</div>
            <p className="text-sm text-muted-foreground">{t("ledger.email.ordersCount", { count: emailOrders.length })}</p>
          </CardContent>
        </Card>
      </div>

      <LedgerImportPanel accounts={accounts} />
      <LedgerAccountList accounts={accounts} />
      <LedgerImportsList imports={imports} accounts={accounts} />
    </div>
  )
}
