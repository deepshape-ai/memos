import dayjs from "dayjs";
import type { MemoFilter } from "@/contexts/MemoFilterContext";

export const DAILY_LOG_PREFIXES = ["* ", "+ ", "- ", "? "] as const;

export interface DailyLogValidationResult {
  valid: boolean;
  line?: number;
  code?: "nested" | "separator" | "prefix";
}

export const normalizeDailyLogContent = (content: string) => content.replaceAll("\r\n", "\n").replaceAll("\r", "\n").trim();

export const validateDailyLogContent = (content: string): DailyLogValidationResult => {
  const normalized = normalizeDailyLogContent(content);
  const lines = normalized.split("\n");

  for (const [index, line] of lines.entries()) {
    if (line.trim() === "") {
      continue;
    }
    if (/^[\t ]+/.test(line)) {
      return { valid: false, line: index + 1, code: "nested" };
    }
    if (/^====+/.test(line.trim())) {
      return { valid: false, line: index + 1, code: "separator" };
    }
    if (/^[*+\-?](?! )/.test(line)) {
      return { valid: false, line: index + 1, code: "prefix" };
    }
  }

  return { valid: true };
};

export const getSelectedDailyLogDate = (filters: MemoFilter[]) => {
  return filters.find((filter) => filter.factor === "displayTime")?.value ?? dayjs().format("YYYY-MM-DD");
};

export const buildDailyLogDateFilter = (selectedDate: string) => {
  const start = dayjs(selectedDate).startOf("day").unix();
  const end = dayjs(selectedDate).add(1, "day").startOf("day").unix();
  return `created_ts >= ${start} && created_ts < ${end}`;
};

export const extractUserIDFromName = (name?: string) => {
  if (!name) {
    return undefined;
  }
  const match = name.match(/users\/(\d+)/);
  return match ? Number(match[1]) : undefined;
};
