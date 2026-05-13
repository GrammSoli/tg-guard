import { useRef, useState, useCallback, useEffect, type TouchEvent, type MouseEvent as ReactMouseEvent } from "react";
import { Trash2 } from "lucide-react";
import type { Subscription } from "@/types/subscription";
import { SubscriptionCard } from "./SubscriptionCard";
import { hapticImpact } from "@/lib/telegram";
import { useTranslation } from "react-i18next";
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

const DISTANCE_THRESHOLD = 80;
const VELOCITY_THRESHOLD = 0.4; // px/ms — fast flick opens with less distance
const VELOCITY_REDUCED_DISTANCE = 30; // min distance needed when flicking fast
const SNAP_OPEN = 80;
const MAX_SWIPE = 120;
const AXIS_LOCK_DISTANCE = 6;
const SPRING_TRANSITION = "transform 300ms cubic-bezier(.25,.46,.45,.94)";

export function SwipeableSubscriptionCard({ subscription, onClick, onDelete }: Props) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const startX = useRef(0);
  const startY = useRef(0);
  const currentX = useRef(0);
  const isSwiping = useRef(false);
  const lockedAxis = useRef<"x" | "y" | null>(null);
  const animating = useRef(false);
  const cardRef = useRef<HTMLDivElement>(null);
  const lastMoveTime = useRef(0);
  const lastMoveX = useRef(0);
  const velocity = useRef(0);

  const [offset, setOffset] = useState(0);
  const [open, setOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  // Rubber-band effect past MAX_SWIPE
  const clampOffset = (raw: number) => {
    const clamped = Math.min(0, raw);
    if (clamped < -MAX_SWIPE) {
      const over = -clamped - MAX_SWIPE;
      return -(MAX_SWIPE + over * 0.3);
    }
    return clamped;
  };

  const snapTo = useCallback((target: number, cb?: () => void) => {
    animating.current = true;
    setOffset(target);
    const onEnd = () => {
      animating.current = false;
      cb?.();
      cardRef.current?.removeEventListener("transitionend", onEnd);
    };
    cardRef.current?.addEventListener("transitionend", onEnd, { once: true });
    // Safety fallback in case transitionend doesn't fire
    setTimeout(() => {
      animating.current = false;
      cardRef.current?.removeEventListener("transitionend", onEnd);
    }, 350);
  }, []);

  const handleTouchStart = useCallback((e: TouchEvent) => {
    if (animating.current || deleting) return;
    startX.current = e.touches[0].clientX;
    startY.current = e.touches[0].clientY;
    currentX.current = open ? -SNAP_OPEN : 0;
    isSwiping.current = true;
    lockedAxis.current = null;
    lastMoveTime.current = e.timeStamp;
    lastMoveX.current = e.touches[0].clientX;
    velocity.current = 0;
  }, [open, deleting]);

  // ── Mouse pointer support (desktop) ───────────────────────────────
  const mouseActive = useRef(false);

  const handleMouseDown = useCallback((e: ReactMouseEvent) => {
    if (animating.current || deleting) return;
    // Only left button
    if (e.button !== 0) return;
    e.preventDefault(); // prevent text selection drag
    mouseActive.current = true;
    startX.current = e.clientX;
    startY.current = e.clientY;
    currentX.current = open ? -SNAP_OPEN : 0;
    isSwiping.current = true;
    lockedAxis.current = null;
    lastMoveTime.current = e.timeStamp;
    lastMoveX.current = e.clientX;
    velocity.current = 0;
  }, [open, deleting]);

  const handleTouchMove = useCallback(
    (e: TouchEvent) => {
      if (!isSwiping.current) return;
      const dx = e.touches[0].clientX - startX.current;
      const dy = e.touches[0].clientY - startY.current;

      // Lock axis after small movement
      if (!lockedAxis.current && (Math.abs(dx) > AXIS_LOCK_DISTANCE || Math.abs(dy) > AXIS_LOCK_DISTANCE)) {
        lockedAxis.current = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
      }

      // Vertical scroll — release immediately
      if (lockedAxis.current === "y") {
        isSwiping.current = false;
        return;
      }

      // Horizontal — prevent scroll and move card
      if (lockedAxis.current === "x") {
        e.preventDefault();
        const base = open ? -SNAP_OPEN : 0;
        const raw = base + dx;
        currentX.current = raw;
        setOffset(clampOffset(raw));

        // Track velocity (px/ms)
        const now = e.timeStamp;
        const dt = now - lastMoveTime.current;
        if (dt > 0) {
          velocity.current = (lastMoveX.current - e.touches[0].clientX) / dt;
        }
        lastMoveTime.current = now;
        lastMoveX.current = e.touches[0].clientX;
      }
    },
    [open],
  );

  const handleTouchEnd = useCallback(() => {
    if (!isSwiping.current) return;
    isSwiping.current = false;
    lockedAxis.current = null;

    const dist = -currentX.current;
    const v = velocity.current; // positive = swiping left
    // Fast flick reduces the distance needed to open
    const threshold = v > VELOCITY_THRESHOLD
      ? Math.max(VELOCITY_REDUCED_DISTANCE, DISTANCE_THRESHOLD - v * 80)
      : DISTANCE_THRESHOLD;

    if (dist > threshold) {
      snapTo(-SNAP_OPEN, () => setOpen(true));
      if (!open) hapticImpact("light");
    } else {
      snapTo(0, () => setOpen(false));
    }
  }, [open, snapTo]);

  // ── Global mouse move/up (captured on window so drag works outside card) ──
  useEffect(() => {
    const handleGlobalMouseMove = (e: globalThis.MouseEvent) => {
      if (!mouseActive.current || !isSwiping.current) return;
      const dx = e.clientX - startX.current;
      const dy = e.clientY - startY.current;

      if (!lockedAxis.current && (Math.abs(dx) > AXIS_LOCK_DISTANCE || Math.abs(dy) > AXIS_LOCK_DISTANCE)) {
        lockedAxis.current = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
      }

      if (lockedAxis.current === "y") {
        isSwiping.current = false;
        mouseActive.current = false;
        return;
      }

      if (lockedAxis.current === "x") {
        const base = open ? -SNAP_OPEN : 0;
        const raw = base + dx;
        currentX.current = raw;
        setOffset(clampOffset(raw));

        const now = e.timeStamp;
        const dt = now - lastMoveTime.current;
        if (dt > 0) {
          velocity.current = (lastMoveX.current - e.clientX) / dt;
        }
        lastMoveTime.current = now;
        lastMoveX.current = e.clientX;
      }
    };

    const handleGlobalMouseUp = () => {
      if (!mouseActive.current) return;
      mouseActive.current = false;
      if (!isSwiping.current) return;
      isSwiping.current = false;
      lockedAxis.current = null;

      const dist = -currentX.current;
      const v = velocity.current;
      const threshold = v > VELOCITY_THRESHOLD
        ? Math.max(VELOCITY_REDUCED_DISTANCE, DISTANCE_THRESHOLD - v * 80)
        : DISTANCE_THRESHOLD;

      if (dist > threshold) {
        snapTo(-SNAP_OPEN, () => setOpen(true));
        if (!open) hapticImpact("light");
      } else {
        snapTo(0, () => setOpen(false));
      }
    };

    window.addEventListener("mousemove", handleGlobalMouseMove);
    window.addEventListener("mouseup", handleGlobalMouseUp);
    return () => {
      window.removeEventListener("mousemove", handleGlobalMouseMove);
      window.removeEventListener("mouseup", handleGlobalMouseUp);
    };
  }, [open, snapTo]);

  const handleDeleteTap = useCallback(() => {
    if (animating.current || deleting) return;
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
    snapTo(0, () => setOpen(false));
  }, [snapTo]);

  const handleCardClick = useCallback(
    (s: Subscription) => {
      if (animating.current) return;
      if (open) {
        snapTo(0, () => setOpen(false));
        return;
      }
      onClick?.(s);
    },
    [open, onClick, snapTo],
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
          ref={cardRef}
          className="relative z-10 touch-pan-y select-none cursor-grab active:cursor-grabbing"
          style={{
            transform: `translateX(${offset}px)`,
            transition: isSwiping.current ? "none" : SPRING_TRANSITION,
          }}
          onTouchStart={handleTouchStart}
          onTouchMove={handleTouchMove}
          onTouchEnd={handleTouchEnd}
          onMouseDown={handleMouseDown}
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
}
