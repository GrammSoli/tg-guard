import { useEffect, useState } from "react";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { useRoomStore } from "@/stores/roomStore";
import { hapticImpact, hapticNotification } from "@/lib/telegram";
import { toast } from "sonner";
import { LogIn, Loader2, AlertCircle, CheckCircle2, Info } from "lucide-react";
import { useTranslation } from "react-i18next";

// 4–32 chars, only latin letters, digits, hyphens, underscores
const INVITE_CODE_RE = /^[a-zA-Z0-9_-]{4,32}$/;

type Status =
  | { type: "empty" }
  | { type: "typing" }
  | { type: "invalid"; message: string }
  | { type: "valid" }
  | { type: "used"; message: string }
  | { type: "loading" };

// `t` is the react-i18next translation function. Typed as a minimal
// callable signature rather than `any` so we keep usefulness of the
// type system at the call sites (TS catches a typo'd t() arity here)
// without pulling react-i18next's full generic-heavy TFunction shape
// — which is overkill for this 5-string usage. Audit Low.
function deriveStatus(code: string, touched: boolean, t: (key: string) => string): Status {
  const trimmed = code.trim();
  if (!trimmed) return touched ? { type: "empty" } : { type: "empty" };
  if (trimmed.length < 4) return { type: "typing" };
  if (trimmed.length > 32)
    return { type: "invalid", message: t("join.invalidLength") };
  if (!INVITE_CODE_RE.test(trimmed))
    return { type: "invalid", message: t("join.invalidChars") };
  return { type: "valid" };
}

const STATUS_CONFIG: Record<
  Status["type"],
  { icon: typeof AlertCircle | null; color: string; defaultTextKey?: string }
> = {
  empty: { icon: Info, color: "text-muted-foreground", defaultTextKey: "join.stateEmpty" },
  typing: { icon: Info, color: "text-muted-foreground", defaultTextKey: "join.stateTyping" },
  invalid: { icon: AlertCircle, color: "text-destructive" },
  valid: { icon: CheckCircle2, color: "text-green-500", defaultTextKey: "join.stateValid" },
  used: { icon: AlertCircle, color: "text-destructive" },
  loading: { icon: Loader2, color: "text-muted-foreground", defaultTextKey: "join.stateChecking" },
};

interface Props {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  initialCode?: string;
}

export function JoinRoomSheet({ open, onOpenChange, initialCode = "" }: Props) {
  const [code, setCode] = useState(initialCode);
  const [loading, setLoading] = useState(false);
  const [touched, setTouched] = useState(false);
  const [joinError, setJoinError] = useState<string | null>(null);
  // Granular selector — was destructuring the whole roomStore and
  // re-rendering on every unrelated mutation (mark-paid in a different
  // room, fetchDetail success, etc.). On low-end Android this caused
  // visible input lag while typing the invite code. See audit F3.
  const join = useRoomStore((s) => s.join);

  const { t } = useTranslation();

  const baseStatus = loading
    ? ({ type: "loading" } as Status)
    : joinError
      ? ({ type: "used", message: joinError } as Status)
      : deriveStatus(code, touched, t);

  const isValid = baseStatus.type === "valid";

  useEffect(() => {
    if (initialCode) {
      setCode(initialCode);
      setTouched(true);
    }
  }, [initialCode]);

  useEffect(() => {
    if (!open) {
      setCode(initialCode || "");
      setLoading(false);
      setTouched(false);
      setJoinError(null);
    }
  }, [open, initialCode]);

  // Clear join error when user edits the code
  useEffect(() => {
    setJoinError(null);
  }, [code]);

  const handleJoin = async () => {
    setTouched(true);
    if (!isValid) {
      hapticNotification("error");
      return;
    }
    setLoading(true);
    setJoinError(null);
    try {
      const room = await join(code.trim());
      hapticNotification("success");
      toast.success(t("toast.roomJoined", { name: room.name }));
      onOpenChange(false);
    } catch {
      hapticNotification("error");
      setJoinError(t("toast.roomNotFound"));
    } finally {
      setLoading(false);
    }
  };

  const cfg = STATUS_CONFIG[baseStatus.type];
  const Icon = cfg.icon;
  const statusText =
    "message" in baseStatus ? (baseStatus as { message: string }).message : cfg.defaultTextKey ? t(cfg.defaultTextKey) : undefined;
  const isError = baseStatus.type === "invalid" || baseStatus.type === "used";

  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent className="bg-background px-5 pb-8">
        <DrawerHeader className="px-0 text-left">
          <DrawerTitle className="text-lg font-semibold">{t("join.title")}</DrawerTitle>
          <DrawerDescription className="text-muted-foreground text-sm">
            {t("join.description")}
          </DrawerDescription>
        </DrawerHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="invite-code">{t("join.label")}</Label>
            <Input
              id="invite-code"
              placeholder={t("join.placeholder")}
              value={code}
              onChange={(e) => {
                setCode(e.target.value);
                if (!touched) setTouched(true);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleJoin();
              }}
              autoFocus
              maxLength={32}
              className={`text-base ${isError ? "border-destructive focus-visible:ring-destructive" : ""}`}
            />
            {statusText && Icon && (
              <div className={`flex items-center gap-1.5 text-xs ${cfg.color}`}>
                <Icon className={`h-3.5 w-3.5 shrink-0 ${baseStatus.type === "loading" ? "animate-spin" : ""}`} />
                <span>{statusText}</span>
              </div>
            )}
          </div>
        </div>

        <DrawerFooter className="flex flex-col gap-2 px-0">
          <Button
            onClick={handleJoin}
            disabled={!isValid || loading}
            className="bg-gradient-primary shadow-elevated h-12 w-full gap-2 rounded-2xl text-base font-semibold text-white transition-transform active:scale-[0.98]"
          >
            {loading ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <LogIn className="h-4 w-4" />
            )}
            {t("join.submit")}
          </Button>
          <Button
            variant="ghost"
            onClick={() => {
              hapticImpact("light");
              onOpenChange(false);
            }}
            className="h-11 w-full rounded-2xl text-muted-foreground transition-colors hover:bg-muted/50 active:scale-[0.98]"
          >
            {t("join.cancel")}
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
}
