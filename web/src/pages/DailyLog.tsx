import dayjs from "dayjs";
import { ChevronDown } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import DailyLogContent from "@/components/DailyLogContent";
import MemoEditor from "@/components/MemoEditor";
import type { EditorState } from "@/components/MemoEditor/state";
import { useMemoFilterContext } from "@/contexts/MemoFilterContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useMemos } from "@/hooks/useMemoQueries";
import { useUser } from "@/hooks/useUserQueries";
import { cn } from "@/lib/utils";
import type { Memo } from "@/types/proto/api/v1/memo_service_pb";
import { MemoType, Visibility } from "@/types/proto/api/v1/memo_service_pb";
import { buildDailyLogDateFilter, extractUserIDFromName, getSelectedDailyLogDate, validateDailyLogContent } from "@/utils/daily-log";
import { combineFilters } from "@/utils/filter";
import { useTranslate } from "@/utils/i18n";

const DailyLog = () => {
  const t = useTranslate();
  const { i18n } = useTranslation();
  const currentUser = useCurrentUser();
  const { filters } = useMemoFilterContext();
  const [showFormatGuide, setShowFormatGuide] = useState(false);

  const selectedDate = getSelectedDailyLogDate(filters);
  const isToday = useMemo(() => dayjs(selectedDate).isSame(dayjs(), "day"), [selectedDate]);
  const selectedDateStart = useMemo(() => dayjs(selectedDate).startOf("day").toDate(), [selectedDate]);
  const selectedDateLabel = useMemo(
    () =>
      dayjs(selectedDate).toDate().toLocaleDateString(i18n.language, {
        year: "numeric",
        month: "long",
        day: "numeric",
        weekday: "short",
      }),
    [i18n.language, selectedDate],
  );

  const dailyLogBaseFilter = useMemo(
    () => combineFilters(`memo_type == "DAILY_LOG"`, buildDailyLogDateFilter(selectedDate)),
    [selectedDate],
  );
  const currentUserID = extractUserIDFromName(currentUser?.name);
  const myDailyLogFilter = useMemo(
    () => combineFilters(dailyLogBaseFilter, currentUserID ? `creator_id == ${currentUserID}` : `creator_id == -1`),
    [currentUserID, dailyLogBaseFilter],
  );

  const { data: myDailyLogResponse, isLoading: isLoadingMyDailyLog } = useMemos({
    filter: myDailyLogFilter,
    pageSize: 10,
    orderBy: "create_time desc",
  });
  const { data: workspaceDailyLogResponse, isLoading: isLoadingWorkspaceDailyLogs } = useMemos({
    filter: dailyLogBaseFilter,
    pageSize: 100,
    orderBy: "create_time desc",
  });

  const myDailyLog = myDailyLogResponse?.memos.at(0);
  const workspaceDailyLogs = useMemo(
    () => (workspaceDailyLogResponse?.memos ?? []).filter((memo) => memo.creator !== currentUser?.name && memo.name !== myDailyLog?.name),
    [currentUser?.name, myDailyLog?.name, workspaceDailyLogResponse?.memos],
  );

  const dailyLogValidator = useMemo(
    () => (state: EditorState) => {
      const result = validateDailyLogContent(state.content);
      if (result.valid) {
        return { valid: true as const };
      }
      if (result.code === "nested") {
        return { valid: false as const, reason: t("daily-log.validation.nested", { line: result.line }) };
      }
      if (result.code === "separator") {
        return { valid: false as const, reason: t("daily-log.validation.separator", { line: result.line }) };
      }
      return { valid: false as const, reason: t("daily-log.validation.prefix", { line: result.line }) };
    },
    [t],
  );
  const dailyLogExampleLines = useMemo(
    () => [
      t("daily-log.example.plain"),
      t("daily-log.example.done"),
      t("daily-log.example.later-done"),
      t("daily-log.example.dropped"),
      t("daily-log.example.learned"),
    ],
    [t],
  );

  return (
    <section className="w-full min-h-full bg-background text-foreground">
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
        {/* Header */}
        <div className="rounded-lg border border-border bg-card px-4 py-4">
          <h1 className="text-lg font-semibold">{t("daily-log.title")}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("daily-log.description")}</p>
          <p className="mt-2 text-sm font-medium text-foreground">{selectedDateLabel}</p>
        </div>

        {/* Collapsible format guide */}
        <div className="rounded-lg border border-border bg-card">
          <button
            type="button"
            className="flex w-full items-center justify-between px-4 py-3 text-sm font-medium text-muted-foreground hover:text-foreground transition-colors"
            onClick={() => setShowFormatGuide((v) => !v)}
          >
            <span>{t("daily-log.format.toggle")}</span>
            <ChevronDown className={cn("h-4 w-4 transition-transform", showFormatGuide && "rotate-180")} />
          </button>
          {showFormatGuide && (
            <div className="border-t border-border px-4 py-3 flex flex-col gap-3">
              <div>
                <p className="text-sm text-muted-foreground">{t("daily-log.format.description")}</p>
                <p className="mt-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">{t("daily-log.format.example")}</p>
              </div>
              <div className="grid gap-2 text-sm text-muted-foreground sm:grid-cols-2">
                <p>{t("daily-log.legend.plain")}</p>
                <p>{t("daily-log.legend.done")}</p>
                <p>{t("daily-log.legend.later-done")}</p>
                <p>{t("daily-log.legend.dropped")}</p>
                <p>{t("daily-log.legend.learned")}</p>
              </div>
              <DailyLogContent content={dailyLogExampleLines.join("\n")} className="rounded-md bg-muted px-3 py-3" />
            </div>
          )}
        </div>

        {/* My daily log: editor (today) or read-only (past) */}
        {isToday ? (
          <MemoEditor
            key={`${selectedDate}:${myDailyLog?.name ?? "new"}`}
            className="w-full"
            cacheKey={`daily-log-editor:${selectedDate}`}
            memo={myDailyLog}
            autoFocus={!myDailyLog}
            placeholder={t("daily-log.editor.placeholder")}
            defaultVisibility={Visibility.PROTECTED}
            defaultType={MemoType.DAILY_LOG}
            defaultCreateTime={selectedDateStart}
            defaultUpdateTime={selectedDateStart}
            hideVisibilitySelector
            hideInsertMenu
            hideMetadata
            hideTimestamps
            customValidator={dailyLogValidator}
          />
        ) : myDailyLog ? (
          <div className="rounded-lg border border-border bg-card px-4 py-4">
            <DailyLogContent content={myDailyLog.content} />
          </div>
        ) : (
          <div className="rounded-lg border border-dashed border-border bg-card px-4 py-6 text-center text-sm text-muted-foreground">
            {t("daily-log.no-log-for-date")}
          </div>
        )}
        {!isToday && <p className="text-center text-xs text-muted-foreground">{t("daily-log.past-readonly")}</p>}

        {/* Workspace logs (always visible) */}
        <section className="flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-foreground">{t("daily-log.workspace.title")}</h2>
            {(isLoadingMyDailyLog || isLoadingWorkspaceDailyLogs) && (
              <span className="text-sm text-muted-foreground">{t("daily-log.loading")}</span>
            )}
          </div>

          {workspaceDailyLogs.length === 0 ? (
            <div className="rounded-lg border border-dashed border-border bg-card px-4 py-6 text-sm text-muted-foreground">
              {t("daily-log.workspace.empty")}
            </div>
          ) : (
            workspaceDailyLogs.map((memo: Memo) => <WorkspaceDailyLogCard key={`${memo.name}-${memo.displayTime}`} memo={memo} />)
          )}
        </section>
      </div>
    </section>
  );
};

/** Renders a single workspace daily log with creator name and styled content. */
const WorkspaceDailyLogCard = ({ memo }: { memo: Memo }) => {
  const creator = useUser(memo.creator).data;
  return (
    <div className="rounded-lg border border-border bg-card px-4 py-3">
      <p className="mb-2 text-xs font-medium text-muted-foreground">{creator?.displayName || creator?.name || memo.creator}</p>
      <DailyLogContent content={memo.content} />
    </div>
  );
};

export default DailyLog;
