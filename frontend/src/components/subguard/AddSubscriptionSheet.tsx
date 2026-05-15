import { useEffect, useState } from "react";
import { format, parse } from "date-fns";
import { ru, enUS } from "date-fns/locale";
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
import { Pencil } from "lucide-react";
import { categoryKey } from "@/lib/categoryKey";
import { DEFAULT_ICON_COLOR, DEFAULT_ICON_NAME } from "@/lib/customIcons";
import { IconPicker } from "./IconPicker";
import { Switch } from "@/components/ui/switch";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Calendar } from "@/components/ui/calendar";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import type { BillingPeriod, BrandKey, Subscription } from "@/types/subscription";
import { ServiceCatalog } from "./ServiceCatalog";
import { BrandIcon } from "./BrandIcon";
import { type PopularService, SERVICE_CATEGORIES } from "@/lib/mockData";
import { CalendarIcon, ChevronRight } from "lucide-react";

interface Props {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  initial?: Subscription | null;
  onSave: (s: Omit<Subscription, "id"> & { id?: string }) => void;
  onDelete?: (id: string) => void;
}

const todayIso = () => new Date().toISOString().slice(0, 10);

export function AddSubscriptionSheet({ open, onOpenChange, initial, onSave, onDelete }: Props) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;

  const [step, setStep] = useState<"catalog" | "form">("catalog");
  const [name, setName] = useState("");
  const [brand, setBrand] = useState<BrandKey>("default");
  const [tag, setTag] = useState("");
  const [note, setNote] = useState("");
  const [amount, setAmount] = useState("9.99");
  const [currency, setCurrency] = useState("USD");
  const [period, setPeriod] = useState<BillingPeriod>("monthly");
  const [nextDate, setNextDate] = useState(todayIso());
  const [isTrial, setIsTrial] = useState(false);
  const [autoPay, setAutoPay] = useState(true);
  // Custom-subscription appearance. Only sent to the API when brand
  // resolves to "default" (i.e. it's actually a custom sub).
  const [iconName, setIconName] = useState<string>(DEFAULT_ICON_NAME);
  const [iconColor, setIconColor] = useState<string>(DEFAULT_ICON_COLOR);

  useEffect(() => {
    if (!open) return;
    if (initial) {
      setStep("form");
      setName(initial.name);
      setBrand(initial.brand);
      setTag(initial.tag ?? "");
      setNote(initial.note ?? "");
      setIconName(initial.icon_name ?? DEFAULT_ICON_NAME);
      setIconColor(initial.icon_color ?? DEFAULT_ICON_COLOR);
      setAmount(String(initial.amount));
      setCurrency(initial.currency);
      setPeriod(initial.period);
      setNextDate(initial.next_payment_at.slice(0, 10));
      setIsTrial(initial.is_trial);
      setAutoPay(initial.is_auto_pay);
    } else {
      setStep("catalog");
      setName("");
      setBrand("default");
      setTag("");
      setNote("");
      setIconName(DEFAULT_ICON_NAME);
      setIconColor(DEFAULT_ICON_COLOR);
      setAmount("9.99");
      setCurrency("USD");
      setPeriod("monthly");
      setNextDate(todayIso());
      setIsTrial(false);
      setAutoPay(true);
    }
  }, [open, initial]);

  const pickService = (s: PopularService) => {
    setName(s.name);
    setBrand(s.brand);
    setTag(s.category);
    setAmount(String(s.defaultAmount));
    setCurrency(s.defaultCurrency);
    setStep("form");
  };

  const pickCustom = () => {
    setName("");
    setBrand("default");
    setStep("form");
  };

  const handleSave = () => {
    const isCustom = brand === "default";
    onSave({
      id: initial?.id,
      name: name.trim() || t("modal.untitled"),
      brand,
      tag: tag.trim() || undefined,
      note: note.trim() || undefined,
      // Only emit icon fields for custom subscriptions. Real brand logos
      // ignore them, so sending the user's picker selection along would
      // just be noise in the DB.
      icon_name: isCustom ? iconName : undefined,
      icon_color: isCustom ? iconColor : undefined,
      amount: parseFloat(amount) || 0,
      currency,
      period,
      // Send the calendar date as a date-only "yyyy-MM-dd" string so
      // the backend can anchor it at noon in the user's stored
      // timezone (audit #14). Previously we did `new Date(nextDate)
      // .toISOString()` which served the day as UTC-midnight — for
      // every user west of UTC that resolved to the PREVIOUS calendar
      // day in their local time, so the notification worker fired
      // reminders one day early.
      next_payment_at: nextDate,
      is_trial: isTrial,
      trial_ends_at: isTrial ? nextDate : null,
      is_auto_pay: autoPay,
    });
    onOpenChange(false);
  };

  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent className="bg-background border-border">
        <div className="mx-auto w-full max-w-md">
          <DrawerHeader className="px-5">
            <DrawerTitle className="text-xl">
              {initial ? t("modal.editTitle") : t("modal.addTitle")}
            </DrawerTitle>
            <DrawerDescription className="sr-only">
              {t("modal.addTitle")}
            </DrawerDescription>
          </DrawerHeader>

          {step === "catalog" ? (
            <div className="px-5 pb-6">
              <ServiceCatalog onSelect={pickService} onCustom={pickCustom} />
            </div>
          ) : (
          <>
          <div className="space-y-4 px-5 pb-2">
            <div className="bg-surface flex items-center gap-3 rounded-2xl p-3">
              {brand === "default" ? (
                <Popover>
                  <PopoverTrigger asChild>
                    <button
                      type="button"
                      aria-label={t("modal.editIcon")}
                      className="group relative shrink-0 rounded-2xl transition-transform active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                    >
                      <BrandIcon brand={brand} iconName={iconName} iconColor={iconColor} />
                      {/* Pencil badge — small, sits just outside the avatar in the
                          bottom-right. Border-2 in the surface colour creates a
                          clean cut-out look against the avatar tint. */}
                      <span className="bg-foreground text-background border-surface absolute -bottom-1 -right-1 flex h-5 w-5 items-center justify-center rounded-full border-2 shadow transition-transform group-hover:scale-110">
                        <Pencil className="h-2.5 w-2.5" />
                      </span>
                    </button>
                  </PopoverTrigger>
                  <PopoverContent
                    align="start"
                    side="bottom"
                    className="w-72 p-3"
                  >
                    <IconPicker
                      iconName={iconName}
                      iconColor={iconColor}
                      onChange={({ iconName: n, iconColor: c }) => {
                        setIconName(n);
                        setIconColor(c);
                      }}
                    />
                  </PopoverContent>
                </Popover>
              ) : (
                <BrandIcon brand={brand} iconName={iconName} iconColor={iconColor} />
              )}
              <button
                type="button"
                onClick={() => setStep("catalog")}
                className="flex flex-1 items-center text-left transition-transform active:scale-[0.99]"
              >
                <div className="flex-1">
                  <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                    {t("modal.selectedService")}
                  </p>
                  <p className="text-sm font-semibold">
                    {name || t("modal.selectService")}
                  </p>
                </div>
                <span className="text-primary inline-flex items-center gap-1 text-xs font-semibold">
                  {t("modal.changeService")}
                  <ChevronRight className="h-4 w-4" />
                </span>
              </button>
            </div>

            {brand === "default" && (
              <Field label={t("modal.customName")}>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t("modal.serviceNamePh")}
                />
              </Field>
            )}

            <div className="grid grid-cols-3 gap-3">
              <div className="col-span-2">
                <Field label={t("modal.amount")}>
                  <Input
                    inputMode="decimal"
                    value={amount}
                    onChange={(e) => setAmount(e.target.value)}
                  />
                </Field>
              </div>
              <Field label={t("modal.currency")}>
                <Select value={currency} onValueChange={setCurrency}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="USD">USD</SelectItem>
                    <SelectItem value="EUR">EUR</SelectItem>
                    <SelectItem value="RUB">RUB</SelectItem>
                    <SelectItem value="GBP">GBP</SelectItem>
                    <SelectItem value="KZT">KZT</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            </div>

            <Field label={t("modal.period")}>
              <Select
                value={period}
                onValueChange={(v) => setPeriod(v as BillingPeriod)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="monthly">{t("modal.period.monthly")}</SelectItem>
                  <SelectItem value="yearly">{t("modal.period.yearly")}</SelectItem>
                  <SelectItem value="weekly">{t("modal.period.weekly")}</SelectItem>
                </SelectContent>
              </Select>
            </Field>

            <Row
              label={t("modal.trialConnected")}
              control={<Switch checked={isTrial} onCheckedChange={setIsTrial} />}
            />

            <Field label={isTrial ? t("modal.trialEnds") : t("modal.nextPayment")}>
              <DatePickerField
                value={nextDate}
                onChange={setNextDate}
                locale={locale}
              />
            </Field>

            <Field label={t("modal.tag")}>
              <Select value={tag} onValueChange={setTag}>
                <SelectTrigger>
                  <SelectValue placeholder={t("modal.tagPh")} />
                </SelectTrigger>
                <SelectContent>
                  {SERVICE_CATEGORIES.map((c) => (
                    <SelectItem key={c} value={c}>
                      {t(categoryKey(c))}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field label={t("modal.note")}>
              <Input
                value={note}
                onChange={(e) => setNote(e.target.value)}
                placeholder={t("modal.notePh")}
                maxLength={128}
              />
            </Field>

            <Row
              label={t("modal.autoPay")}
              hint={t("modal.autoPayHint")}
              control={<Switch checked={autoPay} onCheckedChange={setAutoPay} />}
            />
          </div>

          <DrawerFooter className="flex flex-col gap-2 px-5 pb-6 pt-4">
            <Button
              onClick={handleSave}
              className="bg-gradient-primary shadow-elevated h-12 w-full rounded-2xl text-base font-semibold text-white transition-transform active:scale-[0.98]"
            >
              {t("modal.save")}
            </Button>
            <Button
              variant="ghost"
              onClick={() => onOpenChange(false)}
              className="h-11 w-full rounded-2xl text-muted-foreground transition-colors hover:bg-muted/50 active:scale-[0.98]"
            >
              {t("modal.cancel")}
            </Button>
          </DrawerFooter>
          </>
          )}
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function DatePickerField({
  value,
  onChange,
  locale,
}: {
  value: string;
  onChange: (v: string) => void;
  locale: string;
}) {
  const { t } = useTranslation();
  const dateLocale = locale === "ru" ? ru : enUS;
  const date = value ? parse(value, "yyyy-MM-dd", new Date()) : undefined;
  const [open, setOpen] = useState(false);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          className={cn(
            "flex h-9 w-full items-center gap-2 rounded-lg px-1 text-left text-sm transition-colors hover:bg-muted/40",
            !date && "text-muted-foreground",
          )}
        >
          <CalendarIcon className="h-4 w-4 text-muted-foreground" />
          {date ? format(date, "d MMMM yyyy", { locale: dateLocale }) : t("modal.selectDate", "Select date")}
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start" side="top">
        <Calendar
          mode="single"
          selected={date}
          onSelect={(d) => {
            if (d) {
              onChange(format(d, "yyyy-MM-dd"));
              setOpen(false);
            }
          }}
          locale={dateLocale}
          initialFocus
          className={cn("p-3 pointer-events-auto")}
        />
      </PopoverContent>
    </Popover>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="bg-surface space-y-2 rounded-2xl p-3">
      <Label className="px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
        {label}
      </Label>
      <div className="[&_input]:bg-transparent [&_input]:border-0 [&_input]:px-1 [&_input]:h-9 [&_button]:bg-transparent [&_button]:border-0 [&_button]:px-1 [&_button]:h-9">
        {children}
      </div>
    </div>
  );
}

function Row({
  label,
  hint,
  control,
}: {
  label: string;
  hint?: string;
  control: React.ReactNode;
}) {
  return (
    <div className="bg-surface flex items-center justify-between rounded-2xl p-4">
      <div>
        <p className="text-sm font-semibold">{label}</p>
        {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
      </div>
      {control}
    </div>
  );
}
