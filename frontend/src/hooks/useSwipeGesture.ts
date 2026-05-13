import { useRef, useState, useCallback, useEffect, type TouchEvent, type MouseEvent as ReactMouseEvent } from "react";

const DISTANCE_THRESHOLD = 80;
const VELOCITY_THRESHOLD = 0.4; // px/ms — fast flick opens with less distance
const VELOCITY_REDUCED_DISTANCE = 30; // min distance needed when flicking fast
const SNAP_OPEN = 80;
const MAX_SWIPE = 120;
const AXIS_LOCK_DISTANCE = 6;

export const SPRING_TRANSITION = "transform 300ms cubic-bezier(.25,.46,.45,.94)";

export interface SwipeGestureResult {
  /** Current horizontal offset in px */
  offset: number;
  /** Whether the swipe tray is open */
  isOpen: boolean;
  /** Ref to attach to the sliding card div */
  cardRef: React.RefObject<HTMLDivElement | null>;
  /** Whether the card is mid-swipe (disable CSS transition when true) */
  isSwiping: boolean;
  /** Touch handlers */
  onTouchStart: (e: TouchEvent) => void;
  onTouchMove: (e: TouchEvent) => void;
  onTouchEnd: () => void;
  /** Mouse handler (mouseMove/Up are on window) */
  onMouseDown: (e: ReactMouseEvent) => void;
  /** Snap the card to closed state */
  close: () => void;
}

/**
 * Reusable horizontal swipe-to-reveal gesture supporting both touch and mouse.
 * Returns everything needed to render a swipeable card wrapper.
 *
 * @param disabled - set true to inhibit gestures (e.g. during delete animation)
 * @param onOpen  - optional callback fired when the tray first opens
 */
export function useSwipeGesture(disabled = false, onOpen?: () => void): SwipeGestureResult {
  const startX = useRef(0);
  const startY = useRef(0);
  const currentX = useRef(0);
  const isSwipingRef = useRef(false);
  const lockedAxis = useRef<"x" | "y" | null>(null);
  const animating = useRef(false);
  const cardRef = useRef<HTMLDivElement>(null);
  const lastMoveTime = useRef(0);
  const lastMoveX = useRef(0);
  const velocity = useRef(0);
  const mouseActive = useRef(false);

  const [offset, setOffset] = useState(0);
  const [open, setOpen] = useState(false);
  // Expose a stable boolean for template (refs aren't reactive)
  const [isSwiping, setIsSwiping] = useState(false);

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
    setIsSwiping(false);
    const onEnd = () => {
      animating.current = false;
      cb?.();
      cardRef.current?.removeEventListener("transitionend", onEnd);
    };
    cardRef.current?.addEventListener("transitionend", onEnd, { once: true });
    setTimeout(() => {
      animating.current = false;
      cardRef.current?.removeEventListener("transitionend", onEnd);
    }, 350);
  }, []);

  // ── Snap decision helper ──────────────────────────────────────────
  const decideSnap = useCallback(() => {
    const dist = -currentX.current;
    const v = velocity.current;
    const threshold = v > VELOCITY_THRESHOLD
      ? Math.max(VELOCITY_REDUCED_DISTANCE, DISTANCE_THRESHOLD - v * 80)
      : DISTANCE_THRESHOLD;

    if (dist > threshold) {
      snapTo(-SNAP_OPEN, () => setOpen(true));
      if (!open) onOpen?.();
    } else {
      snapTo(0, () => setOpen(false));
    }
  }, [open, snapTo, onOpen]);

  // ── Touch ─────────────────────────────────────────────────────────
  const handleTouchStart = useCallback((e: TouchEvent) => {
    if (animating.current || disabled) return;
    startX.current = e.touches[0].clientX;
    startY.current = e.touches[0].clientY;
    currentX.current = open ? -SNAP_OPEN : 0;
    isSwipingRef.current = true;
    setIsSwiping(true);
    lockedAxis.current = null;
    lastMoveTime.current = e.timeStamp;
    lastMoveX.current = e.touches[0].clientX;
    velocity.current = 0;
  }, [open, disabled]);

  const handleTouchMove = useCallback((e: TouchEvent) => {
    if (!isSwipingRef.current) return;
    const dx = e.touches[0].clientX - startX.current;
    const dy = e.touches[0].clientY - startY.current;

    if (!lockedAxis.current && (Math.abs(dx) > AXIS_LOCK_DISTANCE || Math.abs(dy) > AXIS_LOCK_DISTANCE)) {
      lockedAxis.current = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
    }

    if (lockedAxis.current === "y") {
      isSwipingRef.current = false;
      setIsSwiping(false);
      return;
    }

    if (lockedAxis.current === "x") {
      e.preventDefault();
      const base = open ? -SNAP_OPEN : 0;
      const raw = base + dx;
      currentX.current = raw;
      setOffset(clampOffset(raw));

      const now = e.timeStamp;
      const dt = now - lastMoveTime.current;
      if (dt > 0) {
        velocity.current = (lastMoveX.current - e.touches[0].clientX) / dt;
      }
      lastMoveTime.current = now;
      lastMoveX.current = e.touches[0].clientX;
    }
  }, [open]);

  const handleTouchEnd = useCallback(() => {
    if (!isSwipingRef.current) return;
    isSwipingRef.current = false;
    lockedAxis.current = null;
    decideSnap();
  }, [decideSnap]);

  // ── Mouse ─────────────────────────────────────────────────────────
  const handleMouseDown = useCallback((e: ReactMouseEvent) => {
    if (animating.current || disabled) return;
    if (e.button !== 0) return;
    e.preventDefault();
    mouseActive.current = true;
    startX.current = e.clientX;
    startY.current = e.clientY;
    currentX.current = open ? -SNAP_OPEN : 0;
    isSwipingRef.current = true;
    setIsSwiping(true);
    lockedAxis.current = null;
    lastMoveTime.current = e.timeStamp;
    lastMoveX.current = e.clientX;
    velocity.current = 0;
  }, [open, disabled]);

  useEffect(() => {
    const handleGlobalMouseMove = (e: globalThis.MouseEvent) => {
      if (!mouseActive.current || !isSwipingRef.current) return;
      const dx = e.clientX - startX.current;
      const dy = e.clientY - startY.current;

      if (!lockedAxis.current && (Math.abs(dx) > AXIS_LOCK_DISTANCE || Math.abs(dy) > AXIS_LOCK_DISTANCE)) {
        lockedAxis.current = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
      }

      if (lockedAxis.current === "y") {
        isSwipingRef.current = false;
        setIsSwiping(false);
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
      if (!isSwipingRef.current) return;
      isSwipingRef.current = false;
      lockedAxis.current = null;
      decideSnap();
    };

    window.addEventListener("mousemove", handleGlobalMouseMove);
    window.addEventListener("mouseup", handleGlobalMouseUp);
    return () => {
      window.removeEventListener("mousemove", handleGlobalMouseMove);
      window.removeEventListener("mouseup", handleGlobalMouseUp);
    };
  }, [open, decideSnap]);

  // ── Public close action ───────────────────────────────────────────
  const close = useCallback(() => {
    snapTo(0, () => setOpen(false));
  }, [snapTo]);

  return {
    offset,
    isOpen: open,
    cardRef,
    isSwiping,
    onTouchStart: handleTouchStart,
    onTouchMove: handleTouchMove,
    onTouchEnd: handleTouchEnd,
    onMouseDown: handleMouseDown,
    close,
  };
}
