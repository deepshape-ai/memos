import dayjs from "dayjs";
import { useMemo } from "react";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { useMemos } from "@/hooks/useMemoQueries";
import { Routes } from "@/router";
import { combineFilters } from "@/utils/filter";
import { useTranslate } from "@/utils/i18n";

const DailyLogHomeEntry = () => {
  const t = useTranslate();

  const todayFilter = useMemo(() => {
    const start = dayjs().startOf("day").unix();
    const end = dayjs().add(1, "day").startOf("day").unix();
    return combineFilters(`memo_type == "DAILY_LOG"`, `created_ts >= ${start} && created_ts < ${end}`);
  }, []);

  const { data } = useMemos({ filter: todayFilter, pageSize: 100, orderBy: "create_time desc" });
  const count = data?.memos.length ?? 0;

  return (
    <section className="mb-2 w-full rounded-lg border border-border bg-card px-4 py-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-foreground">{t("daily-log.title")}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {count > 0 ? t("daily-log.home-summary", { count }) : t("daily-log.home-summary-empty")}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button asChild size="sm">
            <Link to={Routes.DAILY_LOG} viewTransition>
              {t("daily-log.open")}
            </Link>
          </Button>
        </div>
      </div>
    </section>
  );
};

export default DailyLogHomeEntry;
