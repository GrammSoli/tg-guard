import { memo, useState, useCallback } from "react";
import { Trash2 } from "lucide-react";
import type { Subscription } from "@/types/subscription";
import { SubscriptionCard } from "./SubscriptionCard";
import { hapticImpact } from "@/lib/telegram";
import { useTranslation } from "react-i18next";
import { useSwipeGesture, SPRING_TRANSITION } from "@/hooks/useSwipeGesture";
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

interface Props {
  subscription: Subscription;
  onClick?: (s: Subscription) => void;
  onDelete?: (id: string) => void;
}

// React.memo wrap: rendered in a .map() of all the user's subs. Dashboard
// re-renders on every search keystroke and every store update — without
// memo, every keystroke re-renders every swipe wrapper + every card
// underneath. Props are stable references (parent uses useCallback for
// onClick/onDelete), so shallow compare correctly short-circuits.
// See audit F5.
export const SwipeableSubscriptionCard = memo(function SwipeableSubscriptionCard({
  subscription,
  onClick,
  onDelete,
}: Props) {
  const { t } = useTranslation();
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
      onDelete?.(subscription.id);
    }, 300);
  }, [onDelete, subscription.id, deleting]);

  const handleCancelDelete = useCallback(() => {
    setConfirmOpen(false);
    swipe.close();
  }, [swipe]);

  const handleCardClick = useCallback(
    (s: Subscription) => {
      if (swipe.didDrag) return;
      if (swipe.isOpen) {
        swipe.close();
        return;
      }
      onClick?.(s);
    },
    [swipe, onClick],
  );

  return (
    <>
      <div
        className={`relative overflow-hidden rounded-2xl transition-all ${
          deleting ? "max-h-0 opacity-0 mb-0" : "max-h-24 opacity-100"
        }`}
        style={{
          transitionDuration: deleting ? "300ms" : "0ms",
          transitionProperty: deleting ? "max-height, opacity, margin" : "none",
        }}
      >
        {/* Delete background */}
        <div className="absolute inset-0 flex items-center justify-end rounded-2xl bg-destructive">
          <button
            onClick={handleDeleteTap}
            className="flex h-full w-20 items-center justify-center text-white transition-transform active:scale-90"
            aria-label="Delete"
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
          <SubscriptionCard subscription={subscription} onClick={handleCardClick} />
        </div>
      </div>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("card.deleteConfirmTitle")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("card.deleteConfirmDesc", { name: subscription.name })}
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
              {t("card.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
});
