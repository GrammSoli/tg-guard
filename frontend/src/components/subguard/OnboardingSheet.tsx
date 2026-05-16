import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { CalendarCheck2, Users, Sparkles, type LucideIcon } from "lucide-react";

const STORAGE_KEY = "subguard.onboarded.v1";

interface Slide {
  Icon: LucideIcon;
  titleKey: string;
  bodyKey: string;
}

const SLIDES: Slide[] = [
  {
    Icon: CalendarCheck2,
    titleKey: "onboarding.trackTitle",
    bodyKey: "onboarding.trackBody",
  },
  {
    Icon: Users,
    titleKey: "onboarding.shareTitle",
    bodyKey: "onboarding.shareBody",
  },
  {
    Icon: Sparkles,
    titleKey: "onboarding.saveTitle",
    bodyKey: "onboarding.saveBody",
  },
];

export function OnboardingSheet() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [step, setStep] = useState(0);

  useEffect(() => {
    if (typeof window === "undefined") return;
    // localStorage throws in private/incognito contexts and on Safari
    // with intelligent tracking prevention. Without try/catch every
    // onboarding-affected user in those contexts would see the
    // walkthrough on EVERY app open. Audit Low.
    try {
      if (!localStorage.getItem(STORAGE_KEY)) setOpen(true);
    } catch {
      // Storage unavailable — show the onboarding this session only.
      setOpen(true);
    }
  }, []);

  const finish = () => {
    try {
      localStorage.setItem(STORAGE_KEY, "1");
    } catch {
      // QuotaExceeded / private mode — onboarding will re-appear next
      // session; acceptable degradation, no user-visible error.
    }
    setOpen(false);
  };

  const next = () => {
    if (step < SLIDES.length - 1) setStep(step + 1);
    else finish();
  };

  const slide = SLIDES[step];
  const Icon = slide.Icon;
  const isLast = step === SLIDES.length - 1;

  return (
    <Sheet open={open} onOpenChange={(o) => (o ? setOpen(true) : finish())}>
      <SheetContent
        side="bottom"
        className="bg-background rounded-t-3xl border-t border-white/10 p-0"
      >
        <div className="px-6 pb-8 pt-3">
          <div className="mx-auto mb-6 h-1 w-10 rounded-full bg-white/20" />
          <SheetHeader className="sr-only">
            <SheetTitle>Welcome to SubGuard</SheetTitle>
            <SheetDescription>Onboarding walkthrough</SheetDescription>
          </SheetHeader>

          <div className="flex flex-col items-center text-center">
            <div className="bg-gradient-primary shadow-elevated mb-6 flex h-20 w-20 items-center justify-center rounded-3xl">
              <Icon className="h-10 w-10 text-white" />
            </div>
            <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
              {t(slide.titleKey)}
            </p>
            <h2 className="mt-2 max-w-xs text-2xl font-bold leading-tight">
              {t(slide.bodyKey)}
            </h2>
          </div>

          <div className="mt-6 flex items-center justify-center gap-1.5">
            {SLIDES.map((_, i) => (
              <span
                key={i}
                className={`h-1.5 rounded-full transition-all ${
                  i === step ? "bg-primary w-6" : "bg-white/20 w-1.5"
                }`}
              />
            ))}
          </div>

          <div className="mt-8 flex items-center gap-3">
            {!isLast && (
              <button
                onClick={finish}
                className="bg-surface flex-1 rounded-2xl py-3.5 text-sm font-semibold text-muted-foreground transition-transform active:scale-[0.98]"
              >
                {t("onboarding.skip")}
              </button>
            )}
            <button
              onClick={next}
              className="bg-gradient-primary flex-[2] rounded-2xl py-3.5 text-sm font-semibold text-white shadow-elevated transition-transform active:scale-[0.98]"
            >
              {isLast ? t("onboarding.getStarted") : t("onboarding.next")}
            </button>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}
