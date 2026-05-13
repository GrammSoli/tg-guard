import { useState, useCallback } from "react";
import { Trash2, Users } from "lucide-react";
import { hapticImpact } from "@/lib/telegram";
import { useTranslation } from "react-i18next";
import { useSwipeGesture, SPRING_TRANSITION } from "@/hooks/useSwipeGesture";
import { ServiceLogo } from "./ServiceLogo";
import { formatCurrency, localeFor } from "@/lib/format";
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
import type { RoomSummary } from "@/types/room";

interface Props {
  room: RoomSummary;
  /** Formatted display amount in user's currency */
  displayAmount: number;
  /** User's preferred currency code */
  userCurrency: string;
  onClick?: (id: string) => void;
  onDelete?: (id: string) => void;
}

export function SwipeableRoomCard({ room, displayAmount, userCurrency, onClick, onDelete }: Props) {
  const { t, i18n } = useTranslation();
  const lc = localeFor(i18n.language);
  const [deleting, setDeleting] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const swipe = useSwipeGesture(deleting, () => hapticImpact("light"));

  const handleDeleteTap = useCallback(() => {
    if (deleting) return;
    setConfirmOpen(true);
  }, [deleting]);

  const handleConfirmDelete = useCallback(() => {
    if (deleting) return;
    setConfirmOpen(false);
    setDeleting(true);
    hapticImpact("medium");
    setTimeout(() => {
      onDelete?.(room.id);
    }, 300);
  }, [onDelete, room.id, deleting]);

  const handleCancelDelete = useCallback(() => {
    setConfirmOpen(false);
    swipe.close();
  }, [swipe]);

  const handleCardClick = useCallback(() => {
    if (swipe.isOpen) {
      swipe.close();
      return;
    }
    hapticImpact("light");
    onClick?.(room.id);
  }, [swipe, onClick, room.id]);

  return (
    <>
      <div
        className={`relative overflow-hidden rounded-xl transition-all ${
          deleting ? "max-h-0 opacity-0 mb-0" : "max-h-28 opacity-100"
        }`}
        style={{
          transitionDuration: deleting ? "300ms" : "0ms",
          transitionProperty: deleting ? "max-height, opacity, margin" : "none",
        }}
      >
        {/* Delete background */}
        <div className="absolute inset-0 flex items-center justify-end rounded-xl bg-destructive">
          <button
            onClick={handleDeleteTap}
            className="flex h-full w-20 items-center justify-center text-white transition-transform active:scale-90"
            aria-label={`Delete room ${room.name}`}
          >
            <Trash2 className="h-5 w-5" />
          </button>
        </div>

        {/* Card layer */}
        <div
          ref={swipe.cardRef}
          className="relative z-10 touch-pan-y select-none cursor-grab active:cursor-grabbing"
          style={{
            transform: `translateX(${swipe.offset}px)`,
            transition: swipe.isSwiping ? "none" : SPRING_TRANSITION,
          }}
          onTouchStart={swipe.onTouchStart}
          onTouchMove={swipe.onTouchMove}
          onTouchEnd={swipe.onTouchEnd}
          onMouseDown={swipe.onMouseDown}
        >
          <button
            onClick={handleCardClick}
            className="bg-surface hover:bg-surface-elevated flex w-full items-center justify-between rounded-xl border border-white/10 p-4 text-left transition-colors"
          >
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-bold">{room.name}</p>
              <div className="mt-1 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <Users className="h-3 w-3" />
                <span>{t("dashboard.members", { count: room.members })}</span>
                <span className="opacity-50">•</span>
                <span className="font-medium text-foreground">
                  {formatCurrency(displayAmount, userCurrency, lc)} {t("dashboard.perMonth")}
                </span>
              </div>
            </div>
            <div className="ml-3 flex -space-x-1.5">
              {room.services.slice(0, 4).map((s, i) => (
                <ServiceLogo
                  key={i}
                  brand={s.brand}
                  name={s.brand}
                  size={24}
                  rounded="full"
                  className="border border-background"
                />
              ))}
            </div>
          </button>
        </div>
      </div>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent className="rounded-2xl">
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("room.deleteRoomConfirmTitle")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("room.deleteRoomConfirmDesc", { name: room.name })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={handleCancelDelete}>
              {t("card.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirmDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {t("room.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
