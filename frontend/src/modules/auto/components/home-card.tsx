import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useVehicles } from "@/modules/auto/hooks/use-vehicles"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { Car, ArrowRight } from "lucide-react"

export function AutoHomeCard() {
  const { t } = useTranslation()

  const { data: vehicles = [] } = useVehicles()

  return (
    <Link to="/auto" className="group">
      <Card className="transition-colors group-hover:bg-accent/50">
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                <Car className="h-5 w-5 text-primary" />
              </div>
              <CardTitle className="text-xl">{t("nav.auto")}</CardTitle>
            </div>
            <ArrowRight className="h-5 w-5 text-muted-foreground transition-transform group-hover:translate-x-1" />
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-2xl font-bold">{vehicles.length}</p>
              <p className="text-sm text-muted-foreground">{t("home.vehicles")}</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
