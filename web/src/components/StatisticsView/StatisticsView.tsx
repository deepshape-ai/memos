import dayjs from "dayjs";
import { useEffect, useMemo, useState } from "react";
import { MonthCalendar } from "@/components/ActivityCalendar";
import { useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { useDateFilterNavigation } from "@/hooks";
import type { StatisticsData } from "@/types/statistics";
import { MonthNavigator } from "./MonthNavigator";

interface Props {
  statisticsData: StatisticsData;
  allowZeroCountClick?: boolean;
}

const StatisticsView = (props: Props) => {
  const { statisticsData, allowZeroCountClick = false } = props;
  const { activityStats } = statisticsData;
  const { getFiltersByFactor } = useMemoFilterContext();
  const navigateToDateFilter = useDateFilterNavigation();
  const [visibleMonthString, setVisibleMonthString] = useState(dayjs().format("YYYY-MM"));
  const selectedDate = getFiltersByFactor("displayTime")[0]?.value ?? dayjs().format("YYYY-MM-DD");

  const maxCount = useMemo(() => {
    const counts = Object.values(activityStats);
    return Math.max(...counts, 1);
  }, [activityStats]);

  useEffect(() => {
    setVisibleMonthString(dayjs(selectedDate).format("YYYY-MM"));
  }, [selectedDate]);

  return (
    <div className="group w-full mt-2 flex flex-col text-muted-foreground animate-fade-in">
      <MonthNavigator
        visibleMonth={visibleMonthString}
        onMonthChange={setVisibleMonthString}
        activityStats={activityStats}
        selectedDate={selectedDate}
        allowZeroCountClick={allowZeroCountClick}
      />

      <div className="w-full animate-scale-in">
        <MonthCalendar
          month={visibleMonthString}
          data={activityStats}
          maxCount={maxCount}
          onClick={navigateToDateFilter}
          selectedDate={selectedDate}
          allowZeroCountClick={allowZeroCountClick}
        />
      </div>
    </div>
  );
};

export default StatisticsView;
