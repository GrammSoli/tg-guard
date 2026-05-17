import { useCallback, useRef } from "react";

/**
 * useDragScroll — enables horizontal drag-to-scroll on desktop.
 *
 * Returns a callback ref to attach to the scrollable container.
 * A callback ref (rather than a RefObject + useEffect) is required so
 * the listeners attach at the moment the node actually mounts — some
 * consumers render the container conditionally (e.g. SponsoredOffers
 * returns null until its data loads), and a one-shot useEffect would
 * run before that node ever exists.
 *
 * Automatically prevents click events when the user drags more than
 * `CLICK_THRESHOLD` pixels, so links/buttons don't fire on swipe.
 */
const CLICK_THRESHOLD = 5;

export function useDragScroll<T extends HTMLElement = HTMLDivElement>() {
  const state = useRef({
    isDown: false,
    startX: 0,
    scrollLeft: 0,
    dragged: false,
  });
  const cleanupRef = useRef<(() => void) | null>(null);

  return useCallback((el: T | null) => {
    // Detach from a previous node (or on unmount, when el is null).
    cleanupRef.current?.();
    cleanupRef.current = null;
    if (!el) return;

    const s = state.current;

    const onMouseDown = (e: MouseEvent) => {
      s.isDown = true;
      s.dragged = false;
      s.startX = e.pageX - el.offsetLeft;
      s.scrollLeft = el.scrollLeft;
      el.style.cursor = "grabbing";
    };

    const onMouseMove = (e: MouseEvent) => {
      if (!s.isDown) return;
      e.preventDefault();
      const x = e.pageX - el.offsetLeft;
      const walk = x - s.startX;
      if (Math.abs(walk) > CLICK_THRESHOLD) {
        s.dragged = true;
      }
      el.scrollLeft = s.scrollLeft - walk;
    };

    const onMouseUp = () => {
      s.isDown = false;
      el.style.cursor = "";
    };

    const onMouseLeave = () => {
      s.isDown = false;
      el.style.cursor = "";
    };

    // Capture click events and cancel them if we dragged.
    const onClick = (e: MouseEvent) => {
      if (s.dragged) {
        e.preventDefault();
        e.stopPropagation();
        s.dragged = false;
      }
    };

    el.addEventListener("mousedown", onMouseDown);
    el.addEventListener("mousemove", onMouseMove);
    el.addEventListener("mouseup", onMouseUp);
    el.addEventListener("mouseleave", onMouseLeave);
    // Capture phase so we intercept before child onClick fires.
    el.addEventListener("click", onClick, true);

    cleanupRef.current = () => {
      el.removeEventListener("mousedown", onMouseDown);
      el.removeEventListener("mousemove", onMouseMove);
      el.removeEventListener("mouseup", onMouseUp);
      el.removeEventListener("mouseleave", onMouseLeave);
      el.removeEventListener("click", onClick, true);
      el.style.cursor = "";
    };
  }, []);
}
