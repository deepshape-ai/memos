import { cn } from "@/lib/utils";

interface DailyLogContentProps {
  content: string;
  className?: string;
}

interface ParsedLine {
  prefix: string;
  text: string;
  type: "done" | "later-done" | "dropped" | "learned" | "plain";
}

function parseLine(raw: string): ParsedLine {
  if (raw.startsWith("* ")) return { prefix: "* ", text: raw.slice(2), type: "done" };
  if (raw.startsWith("+ ")) return { prefix: "+ ", text: raw.slice(2), type: "later-done" };
  if (raw.startsWith("- ")) return { prefix: "- ", text: raw.slice(2), type: "dropped" };
  if (raw.startsWith("? ")) return { prefix: "? ", text: raw.slice(2), type: "learned" };
  return { prefix: "", text: raw, type: "plain" };
}

const lineStyles: Record<ParsedLine["type"], string> = {
  done: "text-foreground",
  "later-done": "text-muted-foreground line-through",
  dropped: "text-muted-foreground/60 line-through",
  learned: "text-muted-foreground italic",
  plain: "text-foreground",
};

/**
 * Renders daily log content with subtle .plan prefix styling.
 * Uses only muted tones and typography (no bright colors) to match memos' visual language.
 */
const DailyLogContent = ({ content, className }: DailyLogContentProps) => {
  const lines = content.split("\n");

  return (
    <pre className={cn("whitespace-pre-wrap break-words text-sm leading-6 font-sans", className)}>
      {lines.map((raw, i) => {
        if (raw.trim() === "") {
          return <br key={i} />;
        }
        const { prefix, text, type } = parseLine(raw);
        return (
          <div key={i} className={lineStyles[type]}>
            {prefix && <span className="text-muted-foreground select-none">{prefix}</span>}
            {text}
          </div>
        );
      })}
    </pre>
  );
};

export default DailyLogContent;
