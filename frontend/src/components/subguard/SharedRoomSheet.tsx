import { useMemo, useState } from "react";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { Skeleton } from "@/components/ui/skeleton";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
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
import { POPULAR_SERVICES } from "@/lib/mockData";
import type { BrandKey } from "@/types/subscription";
import { formatCurrency, formatDate, localeFor } from "@/lib/format";
import { convertCurrency } from "@/lib/currencyRates";
import { useTranslation } from "react-i18next";
import {
  Bell,
  CalendarDays,
  Check,
  Clock,
  Copy,
  Pencil,
  Plus,
  Trash2,
  Users,
  UserCircle2,
  UserMinus,
  Wallet,
  X,
} from "lucide-react";
import { useRoomStore } from "@/stores/roomStore";
import { useShallow } from "zustand/react/shallow";
import { hapticImpact, hapticNotification } from "@/lib/telegram";
import { toast } from "sonner";
import { DateTz } from "./DateTz";
import { ServiceLogo } from "./ServiceLogo";
import { useSettingsStore } from "@/stores/settingsStore";

interface Props {
  roomId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function SharedRoomSheet({ roomId, open, onOpenChange }: Props) {
  // Granular state subscription so this large sheet doesn't re-render every
  // time an unrelated piece of roomStore changes (e.g. rooms list refresh).
  const activeDetail = useRoomStore((s) => s.activeDetail);
  const { fetchDetail, remind, markPaid, markUnpaid, removeService, addService, deleteRoom, removeMember, updateRoom } =
    useRoomStore(
      useShallow((s) => ({
        fetchDetail: s.fetchDetail,
        remind: s.remind,
        markPaid: s.markPaid,
        markUnpaid: s.markUnpaid,
        removeService: s.removeService,
        addService: s.addService,
        deleteRoom: s.deleteRoom,
        removeMember: s.removeMember,
        updateRoom: s.updateRoom,
      })),
    );
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const lc = localeFor(locale);
  const [picking, setPicking] = useState(false);
  const [pendingRemoveBrand, setPendingRemoveBrand] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [reminding, setReminding] = useState(false);
  const [serviceSearch, setServiceSearch] = useState("");
  const [pendingKickMember, setPendingKickMember] = useState<{ user_id: number; name: string } | null>(null);
  const [pendingPayAction, setPendingPayAction] = useState<{ user_id: number; name: string; action: "pay" | "unpay" } | null>(null);
  const [editingBillingDay, setEditingBillingDay] = useState(false);
  // Per-user pay/unpay in-flight, blocks double-submits.
  const [pendingPayUid, setPendingPayUid] = useState<number | null>(null);
  const settings = useSettingsStore((s) => s.settings);
  const userCurrency = settings.defaultCurrency;

  const room = activeDetail;
  const total = useMemo(
    () => (room ? room.services.reduce((acc, s) => acc + s.amount, 0) : 0),
    [room?.services],
  );
  const botUsername = import.meta.env.VITE_BOT_USERNAME || "SubGuardBot";
  const appShortname = import.meta.env.VITE_APP_SHORTNAME || "app";

  // Anti-spam: check if remind is on cooldown (24h)
  const isRemindCooldown = useMemo(() => {
    if (!room?.last_reminded_at) return false;
    const next = new Date(room.last_reminded_at).getTime() + 24 * 60 * 60 * 1000;
    return Date.now() < next;
  }, [room?.last_reminded_at]);

  const available = useMemo(
    () =>
      room
        ? POPULAR_SERVICES.filter(
            (p) => p.logoUrl && !room.services.some((s) => s.brand === p.brand),
          )
        : [],
    [room?.services],
  );

  const debouncedServiceSearch = useDebouncedValue(serviceSearch, 300);
  const isSearchPending = serviceSearch !== debouncedServiceSearch;
  const filteredAvailable = useMemo(() => {
    const q = debouncedServiceSearch.trim().toLowerCase();
    return q ? available.filter((p) => p.name.toLowerCase().includes(q)) : available;
  }, [debouncedServiceSearch, available]);

  const handleRemind = async () => {
    if (!room || isRemindCooldown) return;
    setReminding(true);
    try {
      const result = await remind(room.id);
      await fetchDetail(room.id);
      hapticNotification("success");
      toast.success(
        t("toast.reminderSent", { count: result.reminded }),
        { description: result.members.join(", ") },
      );
    } catch (err: any) {
      if (err?.status === 429) {
        hapticNotification("warning");
        toast.error(t("toast.remindCooldown"));
      } else {
        hapticNotification("error");
        toast.error(t("toast.remindersFailed"));
      }
    }
    setReminding(false);
  };

  const handleMarkPaid = async (uid: number) => {
    if (!room || pendingPayUid !== null) return;
    setPendingPayUid(uid);
    try {
      await markPaid(room.id, uid);
      hapticNotification("success");
      toast.success(t("toast.paymentConfirmed"));
    } catch {
      hapticNotification("error");
      toast.error(t("toast.paymentFailed"));
    } finally {
      setPendingPayUid(null);
    }
  };

  const handleMarkUnpaid = async (uid: number) => {
    if (!room || pendingPayUid !== null) return;
    setPendingPayUid(uid);
    try {
      await markUnpaid(room.id, uid);
      hapticNotification("success");
      toast.success(t("toast.paymentReverted"));
    } catch {
      hapticNotification("error");
      toast.error(t("toast.paymentFailed"));
    } finally {
      setPendingPayUid(null);
    }
  };

  const handleCopyInvite = () => {
    if (!room) return;
    const link = `https://t.me/${botUsername}/${appShortname}?startapp=room_${room.invite_code}`;
    navigator.clipboard?.writeText(link);
    hapticNotification("success");
    toast.success(t("room.copyInvite"));
  };

  const handleAddService = async (p: (typeof POPULAR_SERVICES)[number]) => {
    if (!room) return;
    try {
      await addService(room.id, {
        brand: p.brand,
        logo_url: p.logoUrl!,
        name: p.name,
        amount: p.defaultAmount,
        currency: p.defaultCurrency,
      });
      hapticNotification("success");
      setPicking(false);
    } catch {
      hapticNotification("error");
      toast.error(t("toast.addServiceFailed"));
    }
  };

  const confirmRemove = async () => {
    if (!room || !pendingRemoveBrand) return;
    try {
      await removeService(room.id, pendingRemoveBrand as BrandKey);
      hapticNotification("warning");
      setPendingRemoveBrand(null);
    } catch {
      hapticNotification("error");
      toast.error(t("toast.removeServiceFailed"));
    }
  };

  const handleDelete = async () => {
    if (!room) return;
    try {
      await deleteRoom(room.id);
      hapticNotification("warning");
      setConfirmDelete(false);
      onOpenChange(false);
      toast.success(t("toast.roomDeleted"));
    } catch {
      hapticNotification("error");
      toast.error(t("toast.roomDeleteFailed"));
    }
  };

  const handleKickMember = async () => {
    if (!room || !pendingKickMember) return;
    try {
      await removeMember(room.id, pendingKickMember.user_id);
      hapticNotification("success");
      toast.success(t("toast.memberRemoved", { name: pendingKickMember.name }));
      setPendingKickMember(null);
    } catch {
      hapticNotification("error");
      toast.error(t("toast.memberRemoveFailed"));
    }
  };

  const handleBillingDayChange = async (day: number) => {
    if (!room) return;
    try {
      await updateRoom(room.id, { billing_day: day });
      hapticNotification("success");
      toast.success(t("toast.billingDayUpdated", { day }));
      setEditingBillingDay(false);
    } catch {
      hapticNotification("error");
      toast.error(t("toast.billingDayFailed"));
    }
  };

  const openTgProfile = (username?: string) => {
    if (!username || username.trim() === "") {
      toast.error(t("toast.noUsername"));
      return;
    }
    const cleanUsername = username.replace(/^@/, "");
    hapticImpact("light");
    try {
      const tgWebApp = (window as any).Telegram?.WebApp;
      if (tgWebApp?.openTelegramLink) {
        tgWebApp.openTelegramLink(`https://t.me/${cleanUsername}`);
      } else {
        window.open(`https://t.me/${cleanUsername}`, "_blank");
      }
    } catch (e) {
      console.error("[openTgProfile]", e);
      window.open(`https://t.me/${cleanUsername}`, "_blank");
    }
  };

  return (
    <Sheet
      open={open}
      onOpenChange={(o) => {
        if (!o) setPicking(false);
        onOpenChange(o);
      }}
    >
      <SheetContent
        side="bottom"
        className="bg-background max-h-[90vh] overflow-y-auto rounded-t-3xl border-t border-white/10 p-0"
      >
        {room && (
          <div className="px-5 pb-8 pt-2">
            <div className="mx-auto mb-4 h-1 w-10 rounded-full bg-white/20" />
            <SheetHeader className="text-left">
              <SheetTitle className="text-2xl font-bold">{room.name}</SheetTitle>
              <SheetDescription className="sr-only">{t("room.details", "Room details and payment management")}</SheetDescription>
            </SheetHeader>

            {/* Stats */}
            <div className="mt-4 grid grid-cols-2 gap-3">
              <div className="bg-surface rounded-2xl p-4">
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Users className="h-3.5 w-3.5" />
                  {t("room.members")}
                </div>
                <p className="mt-1 text-2xl font-bold">{room.members.length}</p>
              </div>
              <div className="bg-surface rounded-2xl p-4">
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Wallet className="h-3.5 w-3.5" />
                  {t("room.yourShare")}
                </div>
                <p className="mt-1 text-2xl font-bold">
                  {formatCurrency(convertCurrency(room.total_per_member, room.currency, userCurrency), userCurrency, lc)}
                </p>
                <p className="text-[10px] text-muted-foreground">
                  {t("room.totalPerMonth", { total: formatCurrency(convertCurrency(total, room.currency, userCurrency), userCurrency, lc) })}
                </p>
                {room.is_owner ? (
                  <button
                    onClick={() => setEditingBillingDay(!editingBillingDay)}
                    className="mt-0.5 flex items-center gap-1 text-[10px] text-muted-foreground transition-colors hover:text-foreground active:scale-95"
                  >
                    <CalendarDays className="h-3 w-3" />
                    {t("room.billingDayShort", { day: room.billing_day })}
                    <Pencil className="h-2.5 w-2.5 opacity-50" />
                  </button>
                ) : (
                  <p className="mt-0.5 flex items-center gap-1 text-[10px] text-muted-foreground">
                    <CalendarDays className="h-3 w-3" />
                    {t("room.billingDayShort", { day: room.billing_day })}
                  </p>
                )}
                {editingBillingDay && room.is_owner && (
                  <select
                    defaultValue={room.billing_day}
                    onChange={(e) => handleBillingDayChange(Number(e.target.value))}
                    className="mt-1.5 w-full rounded-lg border border-white/10 bg-surface-elevated px-2 py-1.5 text-xs text-foreground outline-none"
                  >
                    {Array.from({ length: 31 }, (_, i) => i + 1).map((d) => (
                      <option key={d} value={d}>{d}</option>
                    ))}
                  </select>
                )}
              </div>
            </div>

            {/* Invite link */}
            <button
              onClick={handleCopyInvite}
              className="bg-surface mt-3 flex w-full items-center gap-3 rounded-2xl p-3 text-left transition-transform active:scale-[0.99]"
            >
              <div className="bg-primary/15 flex h-9 w-9 items-center justify-center rounded-xl">
                <Copy className="h-4 w-4 text-primary" />
              </div>
              <div className="flex-1">
                <p className="text-xs font-semibold">{t("room.inviteLink")}</p>
                <p className="text-[11px] text-muted-foreground">
                  t.me/{botUsername}/{appShortname}?startapp=room_{room.id}
                </p>
              </div>
            </button>

            {/* Members with payment status */}
            <div className="mt-5 mb-3 flex items-center justify-between">
              <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                {t("room.paymentStatus")}
              </p>
              <button
                onClick={handleRemind}
                disabled={reminding || isRemindCooldown}
                className="bg-surface-elevated flex items-center gap-1 rounded-full px-3 py-1 text-xs font-semibold transition-transform active:scale-95 disabled:opacity-50"
              >
                <Bell className="h-3 w-3" />
                {reminding ? t("room.sending") : isRemindCooldown ? t("room.remindCooldown") : t("room.remind")}
              </button>
            </div>

            <div className="space-y-2">
              {room.members.map((m) => (
                <div
                  key={m.user_id}
                  className="bg-surface flex items-center gap-3 rounded-2xl p-3"
                >
                  <button
                    type="button"
                    onClick={() => openTgProfile(m.username)}
                    className={`flex items-center gap-3 flex-1 text-left transition-all ${
                      m.username ? 'cursor-pointer hover:opacity-80 active:scale-[0.98]' : 'cursor-default'
                    }`}
                  >
                    <div className="bg-surface-elevated flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-full">
                      {m.avatar ? (
                        <img src={m.avatar} alt={m.name} className="h-full w-full object-cover" />
                      ) : (
                        <UserCircle2 className="h-5 w-5 text-muted-foreground" />
                      )}
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-semibold">
                        <span>
                          {m.name}
                        </span>
                        {m.user_id === room.owner_id && (
                          <span className="ml-1.5 text-[9px] font-bold uppercase tracking-wide text-primary">
                            {t("room.owner")}
                          </span>
                        )}
                      </p>
                      <p className="text-[11px] text-muted-foreground">
                        {formatCurrency(convertCurrency(room.total_per_member, room.currency, userCurrency), userCurrency, lc)} {t("dashboard.perMonth")}
                      </p>
                    </div>
                  </button>
                  {m.has_paid ? (
                    room.is_owner ? (
                      <button
                        disabled={pendingPayUid !== null}
                        onClick={() => {
                          hapticImpact("medium");
                          setPendingPayAction({ user_id: m.user_id, name: m.name, action: "unpay" });
                        }}
                        className="flex items-center gap-1 rounded-full bg-emerald-500/15 px-2.5 py-1 text-[10px] font-semibold text-emerald-400 transition-transform active:scale-95 disabled:cursor-not-allowed disabled:opacity-50"
                      >
                        <Check className="h-3 w-3" /> {t("room.paid")}
                      </button>
                    ) : (
                      <span className="flex items-center gap-1 rounded-full bg-emerald-500/15 px-2.5 py-1 text-[10px] font-semibold text-emerald-400">
                        <Check className="h-3 w-3" /> {t("room.paid")}
                      </span>
                    )
                  ) : room.is_owner ? (
                    <button
                      disabled={pendingPayUid !== null}
                      onClick={() => {
                        hapticImpact("medium");
                        setPendingPayAction({ user_id: m.user_id, name: m.name, action: "pay" });
                      }}
                      className="flex items-center gap-1 rounded-full bg-amber-500/15 px-2.5 py-1 text-[10px] font-semibold text-amber-400 transition-transform active:scale-95 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      <Clock className="h-3 w-3" /> {t("room.markPaid")}
                    </button>
                  ) : (
                    <span className="flex items-center gap-1 rounded-full bg-amber-500/15 px-2.5 py-1 text-[10px] font-semibold text-amber-400">
                      <Clock className="h-3 w-3" /> {t("room.unpaid")}
                    </span>
                  )}
                  {room.is_owner && m.user_id !== room.owner_id && (
                    <button
                      onClick={() => {
                        hapticImpact("medium");
                        setPendingKickMember({ user_id: m.user_id, name: m.name });
                      }}
                      aria-label={`Remove ${m.name}`}
                      className="bg-surface-elevated hover:bg-destructive/20 hover:text-destructive flex h-8 w-8 items-center justify-center rounded-full transition-colors active:scale-90"
                    >
                      <UserMinus className="h-3.5 w-3.5" />
                    </button>
                  )}
                </div>
              ))}
            </div>

            {/* Shared services */}
            <div className="mt-6 mb-3 flex items-center justify-between">
              <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                {t("room.sharedServices")}
              </p>
              <button
                onClick={() => { setPicking((v) => !v); setServiceSearch(""); }}
                className="bg-surface-elevated flex items-center gap-1 rounded-full px-3 py-1 text-xs font-semibold transition-transform active:scale-95"
              >
                {picking ? (
                  <>
                    <X className="h-3 w-3" /> {t("room.cancel")}
                  </>
                ) : (
                  <>
                    <Plus className="h-3 w-3" /> {t("room.addService")}
                  </>
                )}
              </button>
            </div>

            {picking && (
              <div className="bg-surface mb-3 rounded-2xl p-2">
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
                        {available.length === 0 ? t("room.allServicesAdded") : t("room.notFound")}
                      </p>
                      {available.length > 0 && (
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
                        onClick={() => handleAddService(p)}
                        className="animate-smooth-fade hover:bg-surface-elevated flex w-full items-center gap-3 rounded-xl p-2 text-left transition-colors"
                        style={{ animationDelay: `${i * 30}ms`, animationFillMode: "backwards" }}
                      >
                        <ServiceLogo brand={p.brand as any} name={p.name} size={32} rounded="xl" />
                        <span className="flex-1 text-sm font-medium">{p.name}</span>
                        <span className="text-xs text-muted-foreground">
                          {formatCurrency(convertCurrency(p.defaultAmount, p.defaultCurrency, userCurrency), userCurrency, lc)}
                        </span>
                        <Plus className="h-4 w-4 text-muted-foreground" />
                      </button>
                    ))
                  )}
                </div>
              </div>
            )}

            <div className="space-y-2">
              {room.services.length === 0 && (
                <p className="bg-surface rounded-2xl py-6 text-center text-xs text-muted-foreground">
                  {t("room.noServicesYet")}
                </p>
              )}
              {room.services.map((s) => (
                <div
                  key={s.brand}
                  className="bg-surface flex items-center gap-3 rounded-2xl p-3"
                >
                  <ServiceLogo brand={s.brand as any} name={s.name} size={40} rounded="xl" />
                  <div className="flex-1">
                    <p className="text-sm font-semibold">{s.name}</p>
                    <p className="text-[11px] text-muted-foreground">
                      {formatCurrency(convertCurrency(s.amount, s.currency, userCurrency), userCurrency, lc)} {t("dashboard.perMonth")} • {t("room.splitWays", { count: room.members.length })}
                    </p>
                    {s.next_payment_at && (
                      <p className="mt-0.5 flex items-center gap-1 text-[10px] text-muted-foreground/70">
                        <Clock className="h-3 w-3" />
                        <DateTz>{formatDate(s.next_payment_at, lc)}</DateTz>
                      </p>
                    )}
                  </div>
                  {room.is_owner && (
                    <button
                      onClick={() => setPendingRemoveBrand(s.brand)}
                      aria-label={`Remove ${s.brand}`}
                      className="bg-surface-elevated hover:bg-destructive/20 hover:text-destructive flex h-9 w-9 items-center justify-center rounded-full transition-colors active:scale-90"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  )}
                </div>
              ))}
            </div>

            {/* Actions */}
            <div className="mt-6 space-y-2">
              <button
                onClick={() => onOpenChange(false)}
                className="bg-gradient-primary w-full rounded-2xl py-3.5 text-sm font-semibold text-white shadow-elevated transition-transform active:scale-[0.98]"
              >
                {t("room.done")}
              </button>
              {room.is_owner && (
                <button
                  onClick={() => {
                    hapticImpact("medium");
                    setConfirmDelete(true);
                  }}
                  className="w-full rounded-2xl border border-destructive/30 py-3 text-sm font-semibold text-destructive transition-colors hover:bg-destructive/10 active:scale-[0.98]"
                >
                  {t("room.deleteRoom")}
                </button>
              )}
            </div>

            {/* Created at footer */}
            <p className="mt-4 text-center text-[11px] text-muted-foreground/60">
              {t("room.createdAt", { date: formatDate(room.created_at, lc) })}
            </p>
          </div>
        )}
      </SheetContent>

      <AlertDialog
        open={!!pendingRemoveBrand}
        onOpenChange={(o) => !o && setPendingRemoveBrand(null)}
      >
        <AlertDialogContent className="rounded-2xl">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("room.removeServiceTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("room.removeServiceDesc", { brand: pendingRemoveBrand, room: room?.name })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("room.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={confirmRemove}
            >
              {t("room.remove")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent className="rounded-2xl">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("room.deleteRoomConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("room.deleteRoomConfirmDesc", { name: room?.name })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("room.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={handleDelete}
            >
              {t("room.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={!!pendingKickMember}
        onOpenChange={(o) => !o && setPendingKickMember(null)}
      >
        <AlertDialogContent className="rounded-2xl">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("room.kickMemberTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("room.kickMemberDesc", { name: pendingKickMember?.name, room: room?.name })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("room.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={handleKickMember}
            >
              {t("room.kickConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={!!pendingPayAction}
        onOpenChange={(o) => !o && setPendingPayAction(null)}
      >
        <AlertDialogContent className="rounded-2xl">
          <AlertDialogHeader>
            <AlertDialogTitle>
              {pendingPayAction?.action === "pay"
                ? t("room.confirmPayTitle")
                : t("room.confirmUnpayTitle")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {pendingPayAction?.action === "pay"
                ? t("room.confirmPayDesc", { name: pendingPayAction?.name })
                : t("room.confirmUnpayDesc", { name: pendingPayAction?.name })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("room.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              className={
                pendingPayAction?.action === "pay"
                  ? "bg-emerald-600 text-white hover:bg-emerald-700"
                  : "bg-destructive text-destructive-foreground hover:bg-destructive/90"
              }
              onClick={() => {
                if (!pendingPayAction) return;
                if (pendingPayAction.action === "pay") {
                  handleMarkPaid(pendingPayAction.user_id);
                } else {
                  handleMarkUnpaid(pendingPayAction.user_id);
                }
                setPendingPayAction(null);
              }}
            >
              {pendingPayAction?.action === "pay"
                ? t("room.confirmPay")
                : t("room.confirmUnpay")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Sheet>
  );
}
