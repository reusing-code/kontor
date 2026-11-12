import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { CopyIcon } from "lucide-react"
import { format } from "date-fns"
import { useCreateToken, useRevokeToken, useTokens } from "@/hooks/use-tokens"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"
import { TokenCreateDialog } from "@/components/tokens/token-create-dialog"
import { TokenRevealDialog } from "@/components/tokens/token-reveal-dialog"
import { tokenScopes, type Token } from "@/types/token"

function isExpired(token: Token): boolean {
  return token.expiresAt !== undefined && new Date(token.expiresAt).getTime() <= Date.now()
}

function day(value: string): string {
  return format(new Date(value), "yyyy-MM-dd")
}

function CopyBlock({ label, text }: { label: string; text: string }) {
  const { t } = useTranslation()

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(t("tokens.copied"))
    } catch {
      toast.error(t("tokens.copyFailed"))
    }
  }

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium">{label}</span>
        <Button type="button" variant="ghost" size="sm" onClick={handleCopy}>
          <CopyIcon className="h-3.5 w-3.5" />
          {t("tokens.copy")}
        </Button>
      </div>
      <pre className="overflow-x-auto rounded-md border bg-muted p-3 text-xs">{text}</pre>
    </div>
  )
}

export function TokensSettingsCard() {
  const { t } = useTranslation()
  const { data: tokens, isPending, error } = useTokens()
  const createToken = useCreateToken()
  const revokeToken = useRevokeToken()

  const [createOpen, setCreateOpen] = useState(false)
  const [revealed, setRevealed] = useState<string | null>(null)
  const [revoking, setRevoking] = useState<Token | null>(null)

  const mcpUrl = `${window.location.origin}/mcp`
  const claudeSnippet = `claude mcp add --transport http kontor ${mcpUrl} --header "Authorization: Bearer <your-token>"`
  const opencodeSnippet = JSON.stringify(
    { mcp: { kontor: { type: "remote", url: mcpUrl, headers: { Authorization: "Bearer <your-token>" } } } },
    null,
    2,
  )

  function handleCreate(input: Parameters<typeof createToken.mutate>[0]) {
    createToken.mutate(input, {
      onSuccess: (created) => {
        setCreateOpen(false)
        setRevealed(created.token)
        createToken.reset()
      },
      onError: (err) => toast.error(err.message || t("tokens.createFailed")),
    })
  }

  function handleRevoke() {
    if (!revoking) return
    revokeToken.mutate(revoking.id, {
      onSuccess: () => toast.success(t("tokens.revoked")),
      onError: (err) => toast.error(err.message || t("tokens.revokeFailed")),
    })
    setRevoking(null)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("tokens.title")}</CardTitle>
        <CardDescription>{t("tokens.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <div>
          <Button onClick={() => setCreateOpen(true)}>{t("tokens.create")}</Button>
        </div>

        {isPending ? (
          <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
        ) : error ? (
          <p className="text-sm text-destructive" role="alert">
            {t("tokens.loadFailed", { message: error.message })}
          </p>
        ) : tokens && tokens.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("tokens.empty")}</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("tokens.name")}</TableHead>
                <TableHead>{t("tokens.scopes")}</TableHead>
                <TableHead>{t("tokens.created")}</TableHead>
                <TableHead>{t("tokens.expires")}</TableHead>
                <TableHead>{t("tokens.lastUsed")}</TableHead>
                <TableHead className="text-right">{t("common.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(tokens ?? []).map((token) => (
                <TableRow key={token.id}>
                  <TableCell className="font-medium">{token.name}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {token.scopes.map((scope) => (
                        <Badge key={scope} variant="secondary">
                          {t(`tokens.scope.${scope}`)}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>{day(token.createdAt)}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      {token.expiresAt ? day(token.expiresAt) : t("tokens.never")}
                      {isExpired(token) && <Badge variant="destructive">{t("tokens.expired")}</Badge>}
                    </div>
                  </TableCell>
                  <TableCell>{token.lastUsedAt ? day(token.lastUsedAt) : t("tokens.never")}</TableCell>
                  <TableCell className="text-right">
                    <Button variant="outline" size="sm" onClick={() => setRevoking(token)}>
                      {t("tokens.revoke")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}

        <div className="space-y-4 border-t pt-4">
          <p className="text-sm font-medium">{t("tokens.connect")}</p>
          <CopyBlock label={t("tokens.mcpUrl")} text={mcpUrl} />
          <CopyBlock label={t("tokens.claudeCode")} text={claudeSnippet} />
          <CopyBlock label={t("tokens.openCode")} text={opencodeSnippet} />
          <p className="text-sm text-muted-foreground">{t("tokens.snippetHint")}</p>
        </div>

        <div className="space-y-2 border-t pt-4">
          <p className="text-sm font-medium">{t("tokens.scopesExplained")}</p>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("tokens.scopes")}</TableHead>
                <TableHead>{t("tokens.allows")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokenScopes.map((scope) => (
                <TableRow key={scope}>
                  <TableCell className="font-medium">{t(`tokens.scope.${scope}`)}</TableCell>
                  <TableCell>{t(`tokens.scopeDetail.${scope}`)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>

        <div className="space-y-1 text-sm text-muted-foreground">
          <p>{t("tokens.noteRest")}</p>
          <p>{t("tokens.noteRead")}</p>
          <p>{t("tokens.noteLeak")}</p>
        </div>
      </CardContent>

      <TokenCreateDialog open={createOpen} onOpenChange={setCreateOpen} pending={createToken.isPending} onSubmit={handleCreate} />
      <TokenRevealDialog token={revealed} onClose={() => setRevealed(null)} />
      <DeleteConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => !open && setRevoking(null)}
        description={t("tokens.revokeConfirm", { name: revoking?.name ?? "" })}
        actionLabel={t("tokens.revoke")}
        onConfirm={handleRevoke}
      />
    </Card>
  )
}
