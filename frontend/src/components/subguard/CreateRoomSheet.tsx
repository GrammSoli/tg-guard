import { useState, useMemo, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { domainHintFromName } from "@/lib/brandfetch";
import { Skeleton } from "@/components/ui/skeleton";
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
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { POPULAR_SERVICES } from "@/lib/mockData";
import { SUPPORTED_CURRENCIES, convertCurrency, useFxRates } from "@/lib/currencyRates";
import { formatCurrency, localeFor } from "@/lib/format";
import { useRoomStore } from "@/stores/roomStore";
import { useSettingsStore } from "@/stores/settingsStore";
import { hapticNotification } from "@/lib/telegram";
import { toast } from "sonner";
import { ChevronLeft, Plus, X } from "lucide-react";
import { ServiceLogo } from "./ServiceLogo";
import { BrandIcon } from "./BrandIcon";
import { IconPicker } from "./IconPicker";

interface Props {
  open: boolean;
  onOpenChange: (o: boolean) => void;
}

interface PickedService {
  _tempId: string;
  brand: string;
  name: string;
  amount: number;
  currency: string;
  note?: string;
  icon_name?: string;
  icon_color?: string;
}

/**
 * Wizard steps:
 *   basics   — room name + currency
 *   services — pick from popular services / open custom step / review selection
 *   custom   — full-screen custom-service form, returns to `services` on save
 */
type Step = "basics" | "services" | "custom";

export function CreateRoomSheet({ open, onOpenChange }: Props) {
  const { t, i18n } = useTranslation();
  const lc = localeFor(i18n.language);

  const create = useRoomStore((s) => s.create);
  const defaultCurrency = useSettingsStore((s) => s.settings.defaultCurrency);

  const [step, setStep] = useState<Step>("basics");
  const [name, setName] = useState("");
  const [currency, setCurrency] = useState(defaultCurrency || "USD");
  // Track whether the user has manually picked a currency. If yes, settings
  // arriving late (or changing) must NOT overwrite their explicit choice.
  const [currencyTouched, setCurrencyTouched] = useState(false);
  const [services, setServices] = useState<PickedService[]>([]);
  const [saving, setSaving] = useState(false);
  const [serviceSearch, setServiceSearch] = useState("");

  // Custom service form state — used on step `custom`.
  const [customName, setCustomName] = useState("");
  const [customAmount, setCustomAmount] = useState("");
  const [customCurrency, setCustomCurrency] = useState(defaultCurrency || "USD");
  const [customNote, setCustomNote] = useState("");
  const [customIconName, setCustomIconName] = useState("credit-card");
  const [customIconColor, setCustomIconColor] = useState("blue");

  // Duplicate brand dialog state.
  const [pendingDupService, setPendingDupService] = useState<(typeof POPULAR_SERVICES)[number] | null>(null);
  const [pendingNote, setPendingNote] = useState("");

  // Adopt the settings currency only while the user hasn't edited the field
  // themselves. Once they touch the select we treat their value as canonical.
  useEffect(() => {
    if (!currencyTouched && defaultCurrency) {
      setCurrency(defaultCurrency);
    }
  }, [defaultCurrency, currencyTouched]);

  // Reset wizard + form state when the sheet closes so the next open starts
  // cleanly at step 1.
  useEffect(() => {
    if (!open) {
      setStep("basics");
      setCurrencyTouched(false);
      setName("");
      setServices([]);
      setServiceSearch("");
      setCustomName("");
      setCustomAmount("");
      setCustomNote("");
      setCustomCurrency(defaultCurrency || "USD");
      setCustomIconName("credit-card");
      setCustomIconColor("blue");
      setPendingDupService(null);
      setPendingNote("");
    }
  }, [open, defaultCurrency]);

  const debouncedServiceSearch = useDebouncedValue(serviceSearch, 300);
  const isSearchPending = serviceSearch !== debouncedServiceSearch;

  // Available services — show ALL, allow duplicates (the dupe-check dialog
  // gates them on a per-add basis).
  const availableServices = useMemo(() => POPULAR_SERVICES, []);

  const filteredAvailable = useMemo(() => {
    if (!debouncedServiceSearch.trim()) return availableServices;
    const q = debouncedServiceSearch.trim().toLowerCase();
    return availableServices.filter((p) => p.name.toLowerCase().includes(q));
  }, [debouncedServiceSearch, availableServices]);

  const addService = (p: (typeof POPULAR_SERVICES)[number], note?: string) => {
    setServices((prev) => [
      ...prev,
      {
        _tempId: crypto.randomUUID(),
        brand: p.brand,
        name: p.name,
        amount: p.defaultAmount,
        currency: p.defaultCurrency,
        note,
      },
    ]);
  };

  const removeService = (tempId: string) => {
    setServices((prev) => prev.filter((s) => s._tempId !== tempId));
  };

  const handleServiceClick = (p: (typeof POPULAR_SERVICES)[number]) => {
    const isDuplicate = services.some((s) => s.brand === p.brand);
    if (isDuplicate) {
      setPendingDupService(p);
      setPendingNote("");
    } else {
      addService(p);
    }
  };

  const addCustomService = () => {
    if (!customName.trim() || !customAmount) return;
    setServices((prev) => [
      ...prev,
      {
        _tempId: crypto.randomUUID(),
        brand: "default",
        name: customName.trim(),
        amount: parseFloat(customAmount),
        currency: customCurrency,
        note: customNote || undefined,
        icon_name: customIconName,
        icon_color: customIconColor,
      },
    ]);
    // Reset the custom form so the next visit to step `custom` is blank.
    setCustomName("");
    setCustomAmount("");
    setCustomNote("");
    setCustomIconName("credit-card");
    setCustomIconColor("blue");
    setStep("services");
  };

  const handleSave = async () => {
    if (!name.trim()) {
      // Should be impossible (step 2 unreachable without a name) but guard anyway.
      toast.error(t("toast.enterRoomName"));
      setStep("basics");
      return;
    }
    if (services.length === 0) {
      toast.error(t("toast.addServiceFirst"));
      return;
    }
    setSaving(true);
    try {
      await create({
        name: name.trim(),
        currency,
        services: services.map((s) => ({
          brand: s.brand,
          name: s.name,
          amount: Math.round(convertCurrency(s.amount, s.currency, currency) * 100) / 100,
          currency,
          note: s.note,
          icon_name: s.icon_name,
          icon_color: s.icon_color,
        })),
      });
      hapticNotification("success");
      toast.success(t("toast.roomCreated"));
      onOpenChange(false);
    } catch {
      hapticNotification("error");
      toast.error(t("toast.roomCreationFailed"));
    }
    setSaving(false);
  };

  // Subscribe to FX rates so the inline conversions in the picker list
  // re-render with fresh rates; also drives the `total` recomputation
  // by participating in its useMemo deps.
  const fxRates = useFxRates();

  const total = useMemo(
    () =>
      services.reduce(
        (sum, svc) => sum + convertCurrency(svc.amount, svc.currency, currency),
        0,
      ),
    [services, currency, fxRates],
  );

  // Shared header with an in-drawer back chevron — used by steps 2 and 3.
  const renderBackHeader = (onBack: () => void, title: string) => (
    <DrawerHeader className="px-5">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onBack}
          className="bg-surface hover:bg-surface-elevated flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors"
          aria-label="Back"
        >
          <ChevronLeft className="h-5 w-5" />
        </button>
        <DrawerTitle className="text-xl">{title}</DrawerTitle>
      </div>
    </DrawerHeader>
  );

  return (
    <>
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="bg-background border-border">
          <div className="mx-auto w-full max-w-md">
            {/* ─── Step 1: Basics ─────────────────────────────── */}
            {step === "basics" && (
              <>
                <DrawerHeader className="px-5">
                  <DrawerTitle className="text-xl">{t("createRoom.title")}</DrawerTitle>
                  <DrawerDescription className="text-sm text-muted-foreground">
                    {t("createRoom.description")}
                  </DrawerDescription>
                </DrawerHeader>

                <div className="space-y-4 px-5 pb-2">
                  <div className="bg-surface space-y-2 rounded-2xl p-3">
                    <Label className="px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("createRoom.roomName")}
                    </Label>
                    <Input
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      placeholder={t("createRoom.roomNamePh")}
                      className="border-0 bg-transparent px-1"
                    />
                  </div>

                  <div className="bg-surface space-y-2 rounded-2xl p-3">
                    <Label className="px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("createRoom.currency")}
                    </Label>
                    <Select
                      value={currency}
                      onValueChange={(v) => {
                        setCurrency(v);
                        setCurrencyTouched(true);
                      }}
                    >
                      <SelectTrigger className="border-0 bg-transparent px-1">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {SUPPORTED_CURRENCIES.map((c) => (
                          <SelectItem key={c} value={c}>
                            {c}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <DrawerFooter className="flex flex-col gap-2 px-5 pb-6 pt-4">
                  <Button
                    onClick={() => setStep("services")}
                    disabled={!name.trim()}
                    className="bg-gradient-primary shadow-elevated h-12 w-full rounded-2xl text-base font-semibold text-white transition-transform active:scale-[0.98] disabled:opacity-50"
                  >
                    {t("createRoom.next")}
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

            {/* ─── Step 2: Services ───────────────────────────── */}
            {step === "services" && (
              <>
                {renderBackHeader(() => setStep("basics"), t("createRoom.servicesTitle"))}

                <div className="space-y-4 px-5 pb-2">
                  {/* Selected services */}
                  {services.length > 0 && (
                    <div>
                      <p className="mb-2 px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                        {t("createRoom.selected", { count: services.length })}
                      </p>
                      <div className="space-y-2">
                        {services.map((s) => (
                          <div
                            key={s._tempId}
                            className="bg-surface flex items-center gap-3 rounded-2xl p-3"
                          >
                            {s.brand === "default" && s.icon_name && s.icon_color ? (
                              <BrandIcon
                                brand="default"
                                size="sm"
                                iconName={s.icon_name}
                                iconColor={s.icon_color}
                                domain={domainHintFromName(s.brand, s.name)}
                              />
                            ) : (
                              <ServiceLogo brand={s.brand as any} name={s.name} size={36} rounded="xl" />
                            )}
                            <div className="flex-1">
                              <p className="text-sm font-semibold">
                                {s.name}
                                {s.note && (
                                  <span className="ml-1.5 text-[10px] font-normal text-muted-foreground">— {s.note}</span>
                                )}
                              </p>
                              <p className="text-[11px] text-muted-foreground">
                                {formatCurrency(convertCurrency(s.amount, s.currency, currency), currency, lc)} {t("dashboard.perMonth")}
                              </p>
                            </div>
                            <button
                              onClick={() => removeService(s._tempId)}
                              className="bg-surface-elevated flex h-8 w-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-destructive/20 hover:text-destructive"
                              aria-label="Remove"
                            >
                              <X className="h-3.5 w-3.5" />
                            </button>
                          </div>
                        ))}
                      </div>
                      <div className="mt-2 flex items-center justify-between rounded-xl bg-primary/10 px-3 py-2">
                        <span className="text-xs font-semibold text-muted-foreground">{t("createRoom.total")}</span>
                        <span className="text-sm font-bold text-primary">
                          {formatCurrency(total, currency, lc)} {t("dashboard.perMonth")}
                        </span>
                      </div>
                    </div>
                  )}

                  {/* Search input + custom-service shortcut stay pinned at the top
                      of the card; the results list scrolls inside its own bounded
                      box so the user never has to swipe past 200+ services to
                      reach the «Создать комнату» button in the footer. */}
                  <div className="bg-surface rounded-2xl p-2">
                    <input
                      type="text"
                      placeholder={t("room.searchService")}
                      value={serviceSearch}
                      onChange={(e) => setServiceSearch(e.target.value)}
                      className="bg-surface-elevated mb-1 w-full rounded-xl border-0 px-3 py-2 text-sm outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-primary"
                    />

                    <button
                      type="button"
                      onClick={() => setStep("custom")}
                      className="hover:bg-surface-elevated flex w-full items-center gap-3 rounded-xl p-2 text-left transition-colors"
                    >
                      <div className="bg-primary/15 flex h-8 w-8 items-center justify-center rounded-xl">
                        <Plus className="h-4 w-4 text-primary" />
                      </div>
                      <div className="flex-1">
                        <span className="text-sm font-medium">{t("room.customService")}</span>
                        <p className="text-[10px] text-muted-foreground">{t("room.customServiceHint")}</p>
                      </div>
                    </button>

                    <div
                      className="overflow-y-auto"
                      style={{ maxHeight: "calc(var(--app-vh, 100dvh) * 0.45)" }}
                    >
                      {isSearchPending ? (
                        Array.from({ length: 4 }).map((_, i) => (
                          <div key={i} className="flex items-center gap-3 rounded-xl p-2">
                            <Skeleton className="h-8 w-8 rounded-lg" />
                            <Skeleton className="h-4 flex-1 rounded" />
                            <Skeleton className="h-3 w-12 rounded" />
                          </div>
                        ))
                      ) : filteredAvailable.length === 0 ? (
                        <div className="animate-smooth-fade flex flex-col items-center gap-2 px-3 py-6">
                          <p className="text-xs text-muted-foreground">
                            {availableServices.length === 0 ? t("room.allServicesAdded") : t("room.notFound")}
                          </p>
                          {availableServices.length > 0 && (
                            <>
                              <p className="text-[11px] text-muted-foreground/60">{t("room.tryAnotherQuery")}</p>
                              <button
                                onClick={() => setServiceSearch("")}
                                className="mt-1 rounded-lg bg-primary/10 px-3 py-1.5 text-xs font-semibold text-primary transition-colors hover:bg-primary/20"
                              >
                                {t("room.clearSearch")}
                              </button>
                            </>
                          )}
                        </div>
                      ) : (
                        filteredAvailable.map((p, i) => (
                          <button
                            key={p.id}
                            onClick={() => handleServiceClick(p)}
                            className="animate-smooth-fade hover:bg-surface-elevated flex w-full items-center gap-3 rounded-xl p-2 text-left transition-colors"
                            style={{ animationDelay: `${i * 30}ms`, animationFillMode: "backwards" }}
                          >
                            <ServiceLogo brand={p.brand as any} name={p.name} size={32} rounded="xl" />
                            <span className="flex-1 text-sm font-medium">{p.name}</span>
                          <span className="text-xs text-muted-foreground">
                            {formatCurrency(convertCurrency(p.defaultAmount, p.defaultCurrency, currency), currency, lc)}
                          </span>
                          <Plus className="h-4 w-4 text-muted-foreground" />
                        </button>
                      ))
                    )}
                    </div>
                  </div>
                </div>

                <DrawerFooter className="flex flex-col gap-2 px-5 pb-6 pt-4">
                  <Button
                    onClick={handleSave}
                    disabled={saving || services.length === 0}
                    className="bg-gradient-primary shadow-elevated h-12 w-full rounded-2xl text-base font-semibold text-white transition-transform active:scale-[0.98] disabled:opacity-50"
                  >
                    {saving ? t("createRoom.creating") : t("createRoom.create")}
                  </Button>
                </DrawerFooter>
              </>
            )}

            {/* ─── Step 3: Custom service ─────────────────────── */}
            {step === "custom" && (
              <>
                {renderBackHeader(() => setStep("services"), t("room.customService"))}

                <div className="space-y-3 px-5 pb-2">
                  <div className="bg-surface space-y-2 rounded-2xl p-3">
                    <Label className="px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("modal.customName")}
                    </Label>
                    <Input
                      value={customName}
                      onChange={(e) => setCustomName(e.target.value)}
                      placeholder={t("modal.serviceNamePh")}
                      className="border-0 bg-transparent px-1"
                    />
                  </div>

                  <div className="flex gap-2">
                    <div className="bg-surface flex-1 space-y-2 rounded-2xl p-3">
                      <Label className="px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                        {t("modal.amount")}
                      </Label>
                      <Input
                        type="number"
                        inputMode="decimal"
                        value={customAmount}
                        onChange={(e) => setCustomAmount(e.target.value)}
                        placeholder="0"
                        className="border-0 bg-transparent px-1"
                      />
                    </div>
                    <div className="bg-surface w-28 space-y-2 rounded-2xl p-3">
                      <Label className="px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                        {t("createRoom.currency")}
                      </Label>
                      <Select
                        value={customCurrency}
                        onValueChange={setCustomCurrency}
                      >
                        <SelectTrigger className="border-0 bg-transparent px-1">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {SUPPORTED_CURRENCIES.map((c) => (
                            <SelectItem key={c} value={c}>
                              {c}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>

                  <div className="bg-surface space-y-2 rounded-2xl p-3">
                    <Label className="px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("modal.note")}
                    </Label>
                    <Input
                      value={customNote}
                      onChange={(e) => setCustomNote(e.target.value)}
                      placeholder={t("modal.notePh")}
                      className="border-0 bg-transparent px-1"
                    />
                  </div>

                  <div className="bg-surface rounded-2xl p-3">
                    <IconPicker
                      iconName={customIconName}
                      iconColor={customIconColor}
                      onChange={({ iconName, iconColor }) => {
                        setCustomIconName(iconName);
                        setCustomIconColor(iconColor);
                      }}
                    />
                  </div>
                </div>

                <DrawerFooter className="flex flex-col gap-2 px-5 pb-6 pt-4">
                  <Button
                    onClick={addCustomService}
                    disabled={!customName.trim() || !customAmount}
                    className="bg-gradient-primary shadow-elevated h-12 w-full rounded-2xl text-base font-semibold text-white transition-transform active:scale-[0.98] disabled:opacity-50"
                  >
                    {t("room.addService")}
                  </Button>
                  <Button
                    variant="ghost"
                    onClick={() => setStep("services")}
                    className="h-11 w-full rounded-2xl text-muted-foreground transition-colors hover:bg-muted/50 active:scale-[0.98]"
                  >
                    {t("room.cancel")}
                  </Button>
                </DrawerFooter>
              </>
            )}
          </div>
        </DrawerContent>
      </Drawer>

      {/* Duplicate brand — require Note */}
      <AlertDialog
        open={!!pendingDupService}
        onOpenChange={(o) => !o && setPendingDupService(null)}
      >
        <AlertDialogContent className="rounded-2xl">
          <AlertDialogHeader>
            <AlertDialogTitle>{pendingDupService?.name}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("room.noteRequired")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <input
            type="text"
            placeholder={t("room.noteRequiredPh")}
            value={pendingNote}
            onChange={(e) => setPendingNote(e.target.value)}
            className="w-full rounded-xl border border-white/10 bg-surface-elevated px-3 py-2.5 text-sm outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-primary"
            autoFocus
          />
          <AlertDialogFooter>
            <AlertDialogCancel>{t("room.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              disabled={!pendingNote.trim()}
              className="bg-gradient-primary text-white disabled:opacity-50"
              onClick={() => {
                if (pendingDupService && pendingNote.trim()) {
                  addService(pendingDupService, pendingNote.trim());
                  setPendingDupService(null);
                  setPendingNote("");
                }
              }}
            >
              {t("room.addService")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
