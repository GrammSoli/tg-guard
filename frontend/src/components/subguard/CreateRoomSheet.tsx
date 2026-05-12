import { useState, useMemo, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { POPULAR_SERVICES } from "@/lib/mockData";
import { SUPPORTED_CURRENCIES, convertCurrency } from "@/lib/currencyRates";
import { formatCurrency, localeFor } from "@/lib/format";
import { useRoomStore } from "@/stores/roomStore";
import { useSettingsStore } from "@/stores/settingsStore";
import { hapticNotification } from "@/lib/telegram";
import { toast } from "sonner";
import { Check, Plus, X } from "lucide-react";
import { ServiceLogo } from "./ServiceLogo";

interface Props {
  open: boolean;
  onOpenChange: (o: boolean) => void;
}

interface PickedService {
  brand: string;
  logoUrl: string;
  name: string;
  amount: number;
  currency: string;
}

export function CreateRoomSheet({ open, onOpenChange }: Props) {
  const { t, i18n } = useTranslation();
  const lc = localeFor(i18n.language);
  const create = useRoomStore((s) => s.create);
  const defaultCurrency = useSettingsStore((s) => s.settings.defaultCurrency);
  const [name, setName] = useState("");
  const [currency, setCurrency] = useState(defaultCurrency || "USD");
  // Track whether the user has manually picked a currency. If yes, settings
  // arriving late (or changing) must NOT overwrite their explicit choice.
  const [currencyTouched, setCurrencyTouched] = useState(false);
  const [services, setServices] = useState<PickedService[]>([]);
  const [saving, setSaving] = useState(false);
  const [serviceSearch, setServiceSearch] = useState("");

  // Adopt the settings currency only while the user hasn't edited the field
  // themselves. Once they touch the select we treat their value as canonical.
  useEffect(() => {
    if (!currencyTouched && defaultCurrency) {
      setCurrency(defaultCurrency);
    }
  }, [defaultCurrency, currencyTouched]);

  // Reset edit-tracking + state when the sheet closes so the next open picks
  // up the latest settings cleanly.
  useEffect(() => {
    if (!open) {
      setCurrencyTouched(false);
      setName("");
      setServices([]);
      setServiceSearch("");
    }
  }, [open]);

  const debouncedServiceSearch = useDebouncedValue(serviceSearch, 300);
  const isSearchPending = serviceSearch !== debouncedServiceSearch;

  // Available services (not yet picked)
  const availableServices = useMemo(
    () => POPULAR_SERVICES.filter((p) => !services.some((s) => s.brand === p.brand)),
    [services],
  );

  // Filtered by search query
  const filteredAvailable = useMemo(() => {
    if (!debouncedServiceSearch.trim()) return availableServices;
    const q = debouncedServiceSearch.trim().toLowerCase();
    return availableServices.filter((p) => p.name.toLowerCase().includes(q));
  }, [debouncedServiceSearch, availableServices]);

  const addService = (p: (typeof POPULAR_SERVICES)[number]) => {
    setServices((prev) => [
      ...prev,
      {
        brand: p.brand,
        logoUrl: p.logoUrl!,
        name: p.name,
        amount: p.defaultAmount,
        currency: p.defaultCurrency,
      },
    ]);
  };

  const removeService = (brand: string) => {
    setServices((prev) => prev.filter((s) => s.brand !== brand));
  };

  const handleSave = async () => {
    if (!name.trim()) {
      toast.error(t("toast.enterRoomName"));
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
        services: services.map(s => ({
          brand: s.brand,
          logo_url: s.logoUrl,
          name: s.name,
          amount: Math.round(convertCurrency(s.amount, s.currency, currency) * 100) / 100,
          currency,
        }))
      });
      hapticNotification("success");
      toast.success(t("toast.roomCreated"));
      setName("");
      setServices([]);
      onOpenChange(false);
    } catch {
      hapticNotification("error");
      toast.error(t("toast.roomCreationFailed"));
    }
    setSaving(false);
  };

  const total = useMemo(
    () => services.reduce(
      (sum, svc) => sum + convertCurrency(svc.amount, svc.currency, currency),
      0,
    ),
    [services, currency],
  );

  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent className="bg-background border-border">
        <div className="mx-auto w-full max-w-md">
          <DrawerHeader className="px-5">
            <DrawerTitle className="text-xl">{t("createRoom.title")}</DrawerTitle>
            <DrawerDescription className="text-sm text-muted-foreground">
              {t("createRoom.description")}
            </DrawerDescription>
          </DrawerHeader>

          <div className="max-h-[55vh] space-y-4 overflow-y-auto px-5 pb-2">
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

            {/* Selected services */}
            {services.length > 0 && (
              <div>
                <p className="mb-2 px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                  {t("createRoom.selected", { count: services.length })}
                </p>
                <div className="space-y-2">
                  {services.map((s) => (
                    <div
                      key={s.brand}
                      className="bg-surface flex items-center gap-3 rounded-2xl p-3"
                    >
                      <ServiceLogo brand={s.brand as any} name={s.name} size={36} rounded="xl" />
                      <div className="flex-1">
                        <p className="text-sm font-semibold">{s.name}</p>
                        <p className="text-[11px] text-muted-foreground">
                          {formatCurrency(convertCurrency(s.amount, s.currency, currency), currency, lc)} {t("dashboard.perMonth")}
                        </p>
                      </div>
                      <button
                        onClick={() => removeService(s.brand)}
                        className="bg-surface-elevated flex h-8 w-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-destructive/20 hover:text-destructive"
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

            {/* Add services */}
            <div>
              <p className="mb-2 px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                {t("createRoom.addServices")}
              </p>
              <div className="bg-surface rounded-2xl p-2">
                <input
                  type="text"
                  placeholder={t("room.searchService")}
                  value={serviceSearch}
                  onChange={(e) => setServiceSearch(e.target.value)}
                  className="bg-surface-elevated mb-2 w-full rounded-xl border-0 px-3 py-2 text-sm outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-primary"
                />
                <div className="max-h-48 min-h-[120px] overflow-y-auto">
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
                        onClick={() => addService(p)}
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
          </div>

          <DrawerFooter className="flex flex-col gap-2 px-5 pb-6 pt-4">
            <Button
              onClick={handleSave}
              disabled={saving}
              className="bg-gradient-primary shadow-elevated h-12 w-full rounded-2xl text-base font-semibold text-white transition-transform active:scale-[0.98]"
            >
              {saving ? t("createRoom.creating") : t("createRoom.create")}
            </Button>
            <Button
              variant="ghost"
              onClick={() => onOpenChange(false)}
              className="h-11 w-full rounded-2xl text-muted-foreground transition-colors hover:bg-muted/50 active:scale-[0.98]"
            >
              {t("modal.cancel")}
            </Button>
          </DrawerFooter>
        </div>
      </DrawerContent>
    </Drawer>
  );
}
