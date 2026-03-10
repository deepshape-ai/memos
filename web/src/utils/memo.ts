import type { Memo } from "@/types/proto/api/v1/memo_service_pb";
import { MemoType, Visibility } from "@/types/proto/api/v1/memo_service_pb";

export const convertVisibilityFromString = (visibility: string) => {
  switch (visibility) {
    case "PUBLIC":
      return Visibility.PUBLIC;
    case "PROTECTED":
      return Visibility.PROTECTED;
    case "PRIVATE":
      return Visibility.PRIVATE;
    default:
      return Visibility.PUBLIC;
  }
};

export const convertVisibilityToString = (visibility: Visibility) => {
  switch (visibility) {
    case Visibility.PUBLIC:
      return "PUBLIC";
    case Visibility.PROTECTED:
      return "PROTECTED";
    case Visibility.PRIVATE:
      return "PRIVATE";
    default:
      return "PRIVATE";
  }
};

export const isDailyLogMemo = (memo?: Pick<Memo, "type"> | null) => memo?.type === MemoType.DAILY_LOG;
